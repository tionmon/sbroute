package app

import (
	"context"
	"errors"
	"github.com/tionmon/sbroute/internal/model"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestApplyConflictRollbackAndRefreshReview(t *testing.T) {
	s, e := New(context.Background(), t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	first := s.Engine.Config()
	next := first.Clone()
	next.Resources.MemoryMiB = 256
	if _, e = s.Apply(next); e != nil {
		t.Fatal(e)
	}
	if _, e = s.Apply(first); !errors.Is(e, ErrConflict) {
		t.Fatalf("stale update accepted: %v", e)
	}
	if _, e = s.Rollback(first.Version); e != nil {
		t.Fatal(e)
	}
	if s.Engine.Config().Resources.MemoryMiB != 512 {
		t.Fatal("rollback failed")
	}
	content := `{"outbounds":[{"type":"socks","tag":"A","server":"example.com","server_port":1080},{"type":"socks","tag":"B","server":"example.com","server_port":1081},{"type":"socks","tag":"C","server":"example.com","server_port":1082}]}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("ETag", "v1")
		_, _ = w.Write([]byte(content))
	}))
	defer server.Close()
	cfg := s.Engine.Config()
	cfg.Subscriptions = []model.Subscription{{ID: "sub", Name: "Test source", URL: server.URL, Enabled: true, Interval: 21600}}
	if _, e = s.Apply(cfg); e != nil {
		t.Fatal(e)
	}
	if _, e = s.Refresh(context.Background(), "sub"); e != nil {
		t.Fatal(e)
	}
	if len(s.Engine.Config().Nodes) != 3 {
		t.Fatal("initial refresh failed")
	}
	oldID := s.Engine.Config().Nodes[0].ID
	content = `{"outbounds":[{"type":"socks","tag":"Renamed","server":"example.com","server_port":1080}]}`
	status, e := s.Refresh(context.Background(), "sub")
	if e != nil {
		t.Fatal(e)
	}
	if !status.Pending || len(s.Engine.Config().Nodes) != 3 {
		t.Fatal("mass deletion was not staged")
	}
	if _, e = s.AcceptSubscription("sub", s.Engine.Config().Version); e != nil {
		t.Fatal(e)
	}
	cfg = s.Engine.Config()
	if len(cfg.Nodes) != 1 || cfg.Nodes[0].ID != oldID || cfg.Nodes[0].Name != "Renamed" {
		t.Fatal("subscription identity changed")
	}
	content = ""
	if _, e = s.Refresh(context.Background(), "sub"); e != nil {
		t.Fatal(e)
	}
	if len(s.Engine.Config().Nodes) != 1 {
		t.Fatal("empty source erased active nodes")
	}
}
