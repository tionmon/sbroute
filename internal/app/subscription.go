package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/tionmon/sbroute/internal/importer"
	"github.com/tionmon/sbroute/internal/model"
)

// Usage is the provider's optional subscription-userinfo header (bytes; expire is a Unix time).
type Usage struct {
	Upload   int64 `json:"upload"`
	Download int64 `json:"download"`
	Total    int64 `json:"total"`
	Expire   int64 `json:"expire,omitempty"`
}
type SubscriptionState struct {
	ETag          string           `json:"etag,omitempty"`
	LastModified  string           `json:"last_modified,omitempty"`
	URLHash       string           `json:"url_hash,omitempty"`
	LastCheck     time.Time        `json:"last_check"`
	NextCheck     time.Time        `json:"next_check"`
	Error         string           `json:"error,omitempty"`
	Count         int              `json:"count"`
	Pending       *importer.Result `json:"-"`
	HasPending    bool             `json:"has_pending"`
	PendingCount  int              `json:"pending_count"`
	PendingIssues []importer.Issue `json:"pending_issues,omitempty"`
	Removed       int              `json:"removed"`
	Skipped       int              `json:"skipped"`
	Issues        []importer.Issue `json:"issues,omitempty"`
	Usage         *Usage           `json:"usage,omitempty"`
}
type SubscriptionStatus struct {
	ID           string           `json:"id"`
	LastCheck    time.Time        `json:"last_check"`
	NextCheck    time.Time        `json:"next_check"`
	Error        string           `json:"error,omitempty"`
	Count        int              `json:"count"`
	Pending      bool             `json:"pending"`
	PendingCount int              `json:"pending_count"`
	Removed      int              `json:"removed"`
	Skipped      int              `json:"skipped"`
	Issues       []importer.Issue `json:"issues"`
	Usage        *Usage           `json:"usage,omitempty"`
}

const maxStoredIssues = 100

func capIssues(issues []importer.Issue) []importer.Issue {
	if len(issues) <= maxStoredIssues {
		return issues
	}
	return append(append([]importer.Issue{}, issues[:maxStoredIssues]...), importer.Issue{Error: fmt.Sprintf("%d more issues omitted", len(issues)-maxStoredIssues)})
}

// urlHash keeps subscription tokens out of the metadata table.
func urlHash(u string) string {
	h := sha256.Sum256([]byte(u))
	return hex.EncodeToString(h[:])
}

// sameURL also accepts the raw URL that earlier versions stored in url_hash.
func sameURL(state SubscriptionState, u string) bool {
	return state.URLHash == urlHash(u) || state.URLHash == u
}

// parseUsage reads "upload=1; download=2; total=3; expire=4"; unknown or malformed parts are ignored.
func parseUsage(header string) *Usage {
	if header == "" {
		return nil
	}
	var u Usage
	found := false
	for _, part := range strings.Split(header, ";") {
		k, v, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok {
			continue
		}
		n, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
		if err != nil || n < 0 {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(k)) {
		case "upload":
			u.Upload, found = int64(n), true
		case "download":
			u.Download, found = int64(n), true
		case "total":
			u.Total, found = int64(n), true
		case "expire":
			u.Expire, found = int64(n), true
		}
	}
	if !found {
		return nil
	}
	return &u
}

func (s *Service) subState(id string) SubscriptionState {
	var state SubscriptionState
	b, e := s.Store.Get("subscription/" + id)
	if e == nil {
		_ = json.Unmarshal(b, &state)
	}
	return state
}
func (s *Service) saveSub(id string, state SubscriptionState) error {
	if state.Pending != nil {
		state.HasPending = true
		state.PendingCount = len(state.Pending.Nodes)
		state.PendingIssues = capIssues(state.Pending.Issues)
	}
	state.Issues = capIssues(state.Issues)
	b, _ := json.Marshal(state)
	tx, err := s.Store.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.Exec(`INSERT INTO meta(key,value)VALUES(?,?) ON CONFLICT(key)DO UPDATE SET value=excluded.value`, "subscription/"+id, b); err != nil {
		return err
	}
	if state.Pending != nil {
		pending, _ := json.Marshal(state.Pending)
		_, err = tx.Exec(`INSERT INTO meta(key,value)VALUES(?,?) ON CONFLICT(key)DO UPDATE SET value=excluded.value`, "pending/"+id, pending)
	} else if !state.HasPending {
		_, err = tx.Exec(`DELETE FROM meta WHERE key=?`, "pending/"+id)
	}
	if err != nil {
		return err
	}
	return tx.Commit()
}
func (s *Service) SubscriptionStatuses() []SubscriptionStatus {
	v := []SubscriptionStatus{}
	for _, sub := range s.Engine.Config().Subscriptions {
		state := s.subState(sub.ID)
		st := SubscriptionStatus{ID: sub.ID, LastCheck: state.LastCheck, NextCheck: state.NextCheck, Error: state.Error, Count: state.Count, Pending: state.HasPending, Removed: state.Removed, Skipped: state.Skipped, Issues: []importer.Issue{}, Usage: state.Usage}
		if state.HasPending {
			st.PendingCount = state.PendingCount
			st.Issues = state.PendingIssues
		} else if len(state.Issues) > 0 {
			st.Issues = state.Issues
		}
		v = append(v, st)
	}
	return v
}
func (s *Service) Refresh(ctx context.Context, id string) (SubscriptionStatus, error) {
	if _, loaded := s.refreshing.LoadOrStore(id, true); loaded {
		return SubscriptionStatus{}, errors.New("subscription refresh already in progress")
	}
	defer s.refreshing.Delete(id)
	if s.Engine.UnderPressure() {
		return SubscriptionStatus{}, errors.New("memory pressure: refresh paused")
	}
	select {
	case s.fetchSlots <- struct{}{}:
		defer func() { <-s.fetchSlots }()
	default:
		return SubscriptionStatus{}, errors.New("two subscription refreshes already in progress")
	}
	cfg := s.Engine.Config()
	var sub model.Subscription
	found := false
	for _, x := range cfg.Subscriptions {
		if x.ID == id {
			sub = x
			found = true
			break
		}
	}
	if !found {
		return SubscriptionStatus{}, errors.New("subscription not found")
	}
	state := s.subState(id)
	if !sameURL(state, sub.URL) {
		state = SubscriptionState{}
	}
	state.URLHash = urlHash(sub.URL)
	state.LastCheck = time.Now()
	state.NextCheck = time.Now().Add(time.Duration(float64(sub.Interval)*(0.9+rand.Float64()*0.2)) * time.Second)
	fail := func(err error) (SubscriptionStatus, error) {
		state.Error = err.Error()
		if persistErr := s.saveSub(id, state); persistErr != nil {
			err = errors.Join(err, persistErr)
		}
		s.Store.Event("subscription", "refresh failed for "+sub.Name+": "+err.Error())
		return SubscriptionStatus{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", sub.URL, nil)
	if err != nil {
		return fail(errors.New("invalid subscription URL"))
	}
	userAgent := sub.UserAgent
	if userAgent == "" {
		userAgent = "SBRoute/" + model.Version
	}
	req.Header.Set("User-Agent", userAgent)
	if state.ETag != "" {
		req.Header.Set("If-None-Match", state.ETag)
	}
	if state.LastModified != "" {
		req.Header.Set("If-Modified-Since", state.LastModified)
	}
	transport := &http.Transport{Proxy: nil, MaxResponseHeaderBytes: 32 << 10, DisableKeepAlives: true}
	defer transport.CloseIdleConnections()
	if sub.Via != "" {
		dial, release, err := s.Engine.NodeDialer(sub.Via)
		if err != nil {
			return fail(fmt.Errorf("download node unavailable: %w", err))
		}
		defer release()
		transport.DialContext = dial
	}
	client := &http.Client{Transport: transport, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return errors.New("redirect limit")
		}
		if req.URL.Scheme != "http" && req.URL.Scheme != "https" {
			return errors.New("unsupported redirect")
		}
		if via[0].URL.Scheme == "https" && req.URL.Scheme != "https" {
			return errors.New("HTTPS downgrade denied")
		}
		return nil
	}}
	res, err := client.Do(req)
	if err != nil {
		return fail(errors.New("subscription download failed (network/TLS/timeout)"))
	}
	defer res.Body.Close()
	if usage := parseUsage(res.Header.Get("Subscription-Userinfo")); usage != nil {
		state.Usage = usage
	}
	if res.StatusCode == 304 {
		if !state.HasPending {
			state.Error = ""
		}
		if err := s.saveSub(id, state); err != nil {
			return SubscriptionStatus{}, err
		}
		return s.status(id), nil
	}
	if res.StatusCode != 200 {
		return fail(fmt.Errorf("subscription HTTP status %d", res.StatusCode))
	}
	b, err := io.ReadAll(io.LimitReader(res.Body, model.MaxImportBytes+1))
	if err != nil || len(b) > model.MaxImportBytes {
		return fail(errors.New("download failed or exceeds 16MiB"))
	}
	result := s.Import(b, id)
	if len(result.Nodes) == 0 {
		state.Pending = &result
		state.Error = "empty/invalid subscription retained previous nodes"
		if err := s.saveSub(id, state); err != nil {
			return SubscriptionStatus{}, err
		}
		return s.status(id), nil
	}
	// Rebase on the latest configuration, but reject a changed/deleted source.
	latest := s.Engine.Config()
	same := false
	oldCount := 0
	for _, x := range latest.Subscriptions {
		if x.ID == id && x.URL == sub.URL {
			same = true
		}
	}
	if !same {
		return fail(errors.New("subscription changed while downloading; refresh again"))
	}
	for _, n := range latest.Nodes {
		if n.SourceID == id {
			oldCount++
		}
	}
	next, removed := importer.Merge(latest, result.Nodes, id, true)
	state.Removed = removed
	state.Count = len(result.Nodes)
	state.Skipped = len(result.Issues)
	state.Issues = result.Issues
	state.ETag = res.Header.Get("ETag")
	state.LastModified = res.Header.Get("Last-Modified")
	// Unsupported items are skipped and reported; only a large removal needs a human decision.
	if oldCount > 0 && removed*2 > oldCount {
		state.Pending = &result
		state.Error = "update needs review: more than half of nodes removed"
		if err := s.saveSub(id, state); err != nil {
			return SubscriptionStatus{}, err
		}
		return s.status(id), nil
	}
	if _, err = s.Apply(next); err != nil {
		state.ETag, state.LastModified = "", ""
		return fail(err)
	}
	state.Pending = nil
	state.HasPending = false
	state.PendingIssues = nil
	state.Error = ""
	if err := s.saveSub(id, state); err != nil {
		return SubscriptionStatus{}, fmt.Errorf("configuration applied but subscription metadata could not be saved: %w", err)
	}
	message := fmt.Sprintf("%s refreshed: %d nodes", sub.Name, len(result.Nodes))
	if len(result.Issues) > 0 {
		message += fmt.Sprintf(", %d unsupported items skipped", len(result.Issues))
	}
	s.Store.Event("subscription", message)
	return s.status(id), nil
}
func (s *Service) status(id string) SubscriptionStatus {
	for _, v := range s.SubscriptionStatuses() {
		if v.ID == id {
			return v
		}
	}
	return SubscriptionStatus{ID: id}
}
func (s *Service) AcceptSubscription(id string, expected int64) (model.Config, error) {
	if _, busy := s.refreshing.LoadOrStore(id, true); busy {
		return model.Config{}, errors.New("subscription refresh already in progress")
	}
	defer s.refreshing.Delete(id)
	state := s.subState(id)
	if !state.HasPending {
		return model.Config{}, errors.New("no pending update")
	}
	cfg := s.Engine.Config()
	if cfg.Version != expected {
		return model.Config{}, ErrConflict
	}
	found := false
	for _, sub := range cfg.Subscriptions {
		if sub.ID == id && sameURL(state, sub.URL) {
			found = true
		}
	}
	if !found {
		return model.Config{}, errors.New("subscription changed; refresh again")
	}
	b, err := s.Store.Get("pending/" + id)
	if err != nil {
		return model.Config{}, err
	}
	state.Pending = &importer.Result{}
	if err = json.Unmarshal(b, state.Pending); err != nil {
		return model.Config{}, err
	}
	next, _ := importer.Merge(cfg, state.Pending.Nodes, id, true)
	result, err := s.Apply(next)
	if err == nil {
		state.Count = len(state.Pending.Nodes)
		state.Skipped = len(state.Pending.Issues)
		state.Issues = state.Pending.Issues
		state.Pending = nil
		state.HasPending = false
		state.PendingIssues = nil
		state.Error = ""
		if persistErr := s.saveSub(id, state); persistErr != nil {
			return result, fmt.Errorf("configuration applied but subscription metadata could not be saved: %w", persistErr)
		}
	}
	return result, err
}
func (s *Service) subscriptionLoop() {
	defer s.workers.Done()
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			for _, sub := range s.Engine.Config().Subscriptions {
				if !sub.Enabled {
					continue
				}
				// A staged review keeps being re-checked; a later update that no longer needs review replaces it.
				if time.Now().Before(s.subState(sub.ID).NextCheck) {
					continue
				}
				select {
				case s.backgroundSlots <- struct{}{}:
					s.workers.Add(1)
					go func(id string) {
						defer s.workers.Done()
						defer func() { <-s.backgroundSlots }()
						_, _ = s.Refresh(s.ctx, id)
					}(sub.ID)
				default:
				}
			}
		}
	}
}
