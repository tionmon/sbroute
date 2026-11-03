package main

import (
	"github.com/tionmon/sbroute/internal/model"
	"github.com/tionmon/sbroute/internal/store"
	"path/filepath"
	"testing"
)

func TestRestoreRejectsLiveFileAndInvalidConfig(t *testing.T) {
	dir := t.TempDir()
	s, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	c := model.Default()
	v, _ := s.Prepare(c)
	if err = s.Activate(v); err != nil {
		t.Fatal(err)
	}
	path := s.Path
	s.Close()
	if err = restore(dir, path); err == nil {
		t.Fatal("restored database from itself")
	}
	invalidDir := t.TempDir()
	bad, _ := store.Open(invalidDir)
	c.Nodes = []model.Node{{ID: "bad", Name: "Invalid", Outbound: map[string]any{"type": "unsupported"}}}
	bv, _ := bad.Prepare(c)
	bad.Activate(bv)
	backup := filepath.Join(t.TempDir(), "invalid.db")
	if err = bad.Backup(backup); err != nil {
		t.Fatal(err)
	}
	bad.Close()
	if err = restore(dir, backup); err == nil {
		t.Fatal("accepted unsupported core config")
	}
	s, err = store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	loaded, err := s.Load()
	if err != nil || len(loaded.Nodes) != 0 {
		t.Fatal("failed restore changed live database")
	}
}
