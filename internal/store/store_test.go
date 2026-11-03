package store

import (
	"github.com/tionmon/sbroute/internal/model"
	"os"
	"path/filepath"
	"testing"
)

func TestVersionsLockAndBackup(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if duplicate, e := Open(dir); e == nil {
		duplicate.Close()
		t.Fatal("second instance acquired the same state")
	}
	c := model.Default()
	v1, e := s.Prepare(c)
	if e != nil {
		t.Fatal(e)
	}
	if e = s.Activate(v1); e != nil {
		t.Fatal(e)
	}
	c.Resources.MemoryMiB = 256
	v2, _ := s.Prepare(c)
	loaded, _ := s.Load()
	if loaded.Resources.MemoryMiB != 512 {
		t.Fatal("prepared config became active")
	}
	s.Abort(v2)
	backup := filepath.Join(t.TempDir(), "backup.db")
	if e = s.Backup(backup); e != nil {
		t.Fatal(e)
	}
	b, e := os.ReadFile(backup)
	if e != nil || len(b) < 16 || string(b[:16]) != "SQLite format 3\x00" {
		t.Fatal("invalid backup")
	}
	s.Close()
	s, e = Open(dir)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	loaded, e = s.Load()
	if e != nil || loaded.Version != v1 {
		t.Fatal("active version was not durable")
	}
}
