package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/gofrs/flock"
	"github.com/tionmon/sbroute/internal/model"
	_ "modernc.org/sqlite"
)

type Store struct {
	DB   *sql.DB
	Path string
	lock *flock.Flock
	mu   sync.Mutex
	subs map[chan Event]struct{}
}
type Revision struct {
	Version int64  `json:"version"`
	Created string `json:"created"`
	State   string `json:"state"`
}

func Open(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	_ = os.Chmod(dir, 0700)
	p := filepath.Join(dir, "sbroute.db")
	lock := flock.New(filepath.Join(dir, "sbroute.lock"))
	locked, lockErr := lock.TryLock()
	if lockErr != nil {
		return nil, lockErr
	}
	if !locked {
		return nil, errors.New("data directory is in use; stop SBRoute or use its management API")
	}
	success := false
	defer func() {
		if !success {
			_ = lock.Unlock()
		}
	}()
	db, e := sql.Open("sqlite", p)
	if e != nil {
		return nil, e
	}
	db.SetMaxOpenConns(1)
	_, e = db.Exec(`PRAGMA journal_mode=WAL; PRAGMA busy_timeout=5000; PRAGMA synchronous=FULL;
CREATE TABLE IF NOT EXISTS configs(version INTEGER PRIMARY KEY AUTOINCREMENT, created TEXT NOT NULL, state TEXT NOT NULL, body BLOB NOT NULL);
CREATE TABLE IF NOT EXISTS meta(key TEXT PRIMARY KEY, value BLOB NOT NULL);
CREATE TABLE IF NOT EXISTS events(id INTEGER PRIMARY KEY AUTOINCREMENT, created TEXT NOT NULL, kind TEXT NOT NULL, message TEXT NOT NULL);`)
	if e != nil {
		db.Close()
		return nil, e
	}
	_ = os.Chmod(p, 0600)
	s := &Store{DB: db, Path: p, lock: lock}
	_, e = db.Exec(`UPDATE configs SET state='aborted' WHERE state='prepared'`)
	if e != nil {
		db.Close()
		return nil, e
	}
	success = true
	return s, nil
}
func (s *Store) Close() error { return errors.Join(s.DB.Close(), s.lock.Unlock()) }
func (s *Store) Load() (model.Config, error) {
	var b []byte
	var version int64
	e := s.DB.QueryRow(`SELECT version,body FROM configs WHERE state='active' ORDER BY version DESC LIMIT 1`).Scan(&version, &b)
	if errors.Is(e, sql.ErrNoRows) {
		return model.Default(), nil
	}
	if e != nil {
		return model.Config{}, e
	}
	var c model.Config
	e = json.Unmarshal(b, &c)
	c.Version = version
	return c, e
}
func (s *Store) Prepare(c model.Config) (int64, error) {
	b, e := json.Marshal(c)
	if e != nil {
		return 0, e
	}
	r, e := s.DB.Exec(`INSERT INTO configs(created,state,body)VALUES(?,'prepared',?)`, time.Now().UTC().Format(time.RFC3339), b)
	if e != nil {
		return 0, e
	}
	return r.LastInsertId()
}
func (s *Store) Activate(v int64) error {
	tx, e := s.DB.Begin()
	if e != nil {
		return e
	}
	defer tx.Rollback()
	if _, e = tx.Exec(`UPDATE configs SET state='history' WHERE state='active'`); e != nil {
		return e
	}
	r, e := tx.Exec(`UPDATE configs SET state='active' WHERE version=? AND state='prepared'`, v)
	if e != nil {
		return e
	}
	n, _ := r.RowsAffected()
	if n != 1 {
		return fmt.Errorf("prepared revision missing")
	}
	if _, e = tx.Exec(`DELETE FROM configs WHERE version NOT IN (SELECT version FROM configs ORDER BY version DESC LIMIT 20)`); e != nil {
		return e
	}
	return tx.Commit()
}
func (s *Store) Abort(v int64) {
	_, _ = s.DB.Exec(`UPDATE configs SET state='aborted' WHERE version=? AND state='prepared'`, v)
}
func (s *Store) Revision(v int64) (model.Config, error) {
	var b []byte
	e := s.DB.QueryRow(`SELECT body FROM configs WHERE version=?`, v).Scan(&b)
	var c model.Config
	if e == nil {
		e = json.Unmarshal(b, &c)
	}
	return c, e
}
func (s *Store) Revisions() ([]Revision, error) {
	rows, e := s.DB.Query(`SELECT version,created,state FROM configs ORDER BY version DESC LIMIT 20`)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	v := []Revision{}
	for rows.Next() {
		var r Revision
		if e = rows.Scan(&r.Version, &r.Created, &r.State); e != nil {
			return nil, e
		}
		v = append(v, r)
	}
	return v, rows.Err()
}
func (s *Store) Get(k string) ([]byte, error) {
	var b []byte
	e := s.DB.QueryRow(`SELECT value FROM meta WHERE key=?`, k).Scan(&b)
	return b, e
}
func (s *Store) Set(k string, b []byte) error {
	_, e := s.DB.Exec(`INSERT INTO meta(key,value)VALUES(?,?) ON CONFLICT(key)DO UPDATE SET value=excluded.value`, k, b)
	return e
}

type Event struct {
	ID      int64  `json:"id"`
	Created string `json:"created"`
	Kind    string `json:"kind"`
	Message string `json:"message"`
}

func (s *Store) Event(kind, msg string) {
	created := time.Now().UTC().Format(time.RFC3339)
	r, err := s.DB.Exec(`INSERT INTO events(created,kind,message)VALUES(?,?,?)`, created, kind, msg)
	_, _ = s.DB.Exec(`DELETE FROM events WHERE id < (SELECT COALESCE(MAX(id),0)-1000 FROM events)`)
	if err != nil {
		return
	}
	id, err := r.LastInsertId()
	if err != nil {
		return
	}
	event := Event{ID: id, Created: created, Kind: kind, Message: msg}
	s.mu.Lock()
	defer s.mu.Unlock()
	for ch := range s.subs {
		// A slow reader misses live pushes; it still gets every event from the database on reconnect.
		select {
		case ch <- event:
		default:
		}
	}
}

// Subscribe delivers events as they are recorded until the returned cancel function is called.
func (s *Store) Subscribe() (<-chan Event, func()) {
	ch := make(chan Event, 64)
	s.mu.Lock()
	if s.subs == nil {
		s.subs = map[chan Event]struct{}{}
	}
	s.subs[ch] = struct{}{}
	s.mu.Unlock()
	return ch, func() {
		s.mu.Lock()
		delete(s.subs, ch)
		s.mu.Unlock()
	}
}
func (s *Store) Events() ([]Event, error) {
	rows, e := s.DB.Query(`SELECT id,created,kind,message FROM events ORDER BY id DESC LIMIT 100`)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	v := []Event{}
	for rows.Next() {
		var r Event
		if e = rows.Scan(&r.ID, &r.Created, &r.Kind, &r.Message); e != nil {
			return nil, e
		}
		v = append(v, r)
	}
	return v, rows.Err()
}

// VACUUM INTO produces a consistent backup, including committed WAL data.
func (s *Store) Backup(path string) error {
	if _, e := os.Stat(path); !os.IsNotExist(e) {
		return fmt.Errorf("backup destination must not exist")
	}
	_, e := s.DB.Exec(`VACUUM INTO ?`, path)
	if e == nil {
		e = os.Chmod(path, 0600)
	}
	return e
}
