package app

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/tionmon/sbroute/internal/core"
	"github.com/tionmon/sbroute/internal/importer"
	"github.com/tionmon/sbroute/internal/model"
	"github.com/tionmon/sbroute/internal/store"
)

var ErrConflict = errors.New("configuration version conflict; reload before applying")

type Service struct {
	Store           *store.Store
	Engine          *core.Engine
	mu              sync.Mutex
	refreshing      sync.Map
	fetchSlots      chan struct{}
	backgroundSlots chan struct{}
	ctx             context.Context
	cancel          context.CancelFunc
	workers         sync.WaitGroup
}

func New(ctx context.Context, dir string) (*Service, error) {
	s := &Service{fetchSlots: make(chan struct{}, 2), backgroundSlots: make(chan struct{}, 2)}
	s.ctx, s.cancel = context.WithCancel(ctx)
	var err error
	s.Store, err = store.Open(dir)
	if err != nil {
		s.cancel()
		return nil, err
	}
	s.Engine, err = core.New(s.ctx, s.Store.Event)
	if err != nil {
		s.Store.Close()
		s.cancel()
		return nil, err
	}
	cfg, err := s.Store.Load()
	cfg.Normalize()
	if err == nil {
		var commit func(int64)
		var rollback func()
		commit, rollback, err = s.Engine.Prepare(cfg)
		if err == nil {
			if cfg.Version == 0 {
				cfg.Version, err = s.Store.Prepare(cfg)
				if err == nil {
					err = s.Store.Activate(cfg.Version)
				}
			}
			if err == nil {
				commit(cfg.Version)
			} else {
				rollback()
			}
		}
	}
	if err != nil {
		s.Engine.Close()
		s.Store.Close()
		s.cancel()
		return nil, err
	}
	s.workers.Add(1)
	go s.subscriptionLoop()
	return s, nil
}
func (s *Service) Close() error {
	s.cancel()
	s.workers.Wait()
	err := s.Engine.Close()
	dbErr := s.Store.Close()
	return errors.Join(err, dbErr)
}
func (s *Service) Apply(c model.Config) (model.Config, error) {
	c.Normalize()
	s.mu.Lock()
	defer s.mu.Unlock()
	old := s.Engine.Config()
	if c.Version != old.Version {
		return model.Config{}, ErrConflict
	}
	c.RestoreSecrets(old)
	commit, rollback, err := s.Engine.Prepare(c)
	if err != nil {
		return model.Config{}, err
	}
	version, err := s.Store.Prepare(c)
	if err != nil {
		rollback()
		return model.Config{}, err
	}
	if err = s.Store.Activate(version); err != nil {
		s.Store.Abort(version)
		rollback()
		return model.Config{}, err
	}
	commit(version)
	s.Store.Event("config", fmt.Sprintf("configuration v%d applied", version))
	return s.Engine.Config().Redacted(), nil
}
func (s *Service) Validate(c model.Config) error {
	old := s.Engine.Config()
	c.RestoreSecrets(old)
	return s.Engine.Validate(c)
}
func (s *Service) ValidateNode(n model.Node) error {
	if n.Outbound["type"] == "direct" {
		return errors.New("direct is not accepted from subscriptions or batch imports; only explicit administrator configuration may use it")
	}
	c := model.Default()
	c.Nodes = []model.Node{n}
	return s.Engine.Validate(c)
}
func (s *Service) Import(data []byte, source string) importer.Result {
	return importer.Parse(data, source, s.ValidateNode)
}
func (s *Service) Rollback(version int64) (model.Config, error) {
	return s.RollbackAt(version, s.Engine.Config().Version)
}
func (s *Service) RollbackAt(version, expected int64) (model.Config, error) {
	c, err := s.Store.Revision(version)
	if err != nil {
		return c, err
	}
	c.Version = expected
	return s.Apply(c)
}
