package httpapi

import (
	"bufio"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/tionmon/sbroute/internal/app"
	"github.com/tionmon/sbroute/internal/core"
	"github.com/tionmon/sbroute/internal/model"
)

func TestClientIPHonoursOnlyTrustedProxies(t *testing.T) {
	a := newAuth(nil, []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")})
	cases := []struct{ remote, xff, want string }{
		{"203.0.113.9:1234", "198.51.100.1", "203.0.113.9"},
		{"10.0.0.2:1234", "198.51.100.1", "198.51.100.1"},
		{"10.0.0.2:1234", "1.1.1.1, 198.51.100.1, 10.0.0.3", "198.51.100.1"},
		{"10.0.0.2:1234", "", "10.0.0.2"},
	}
	for _, c := range cases {
		r := httptest.NewRequest("POST", "/api/v1/login", nil)
		r.RemoteAddr = c.remote
		if c.xff != "" {
			r.Header.Set("X-Forwarded-For", c.xff)
		}
		if got := a.clientIP(r); got != c.want {
			t.Fatalf("%s %q: got %s want %s", c.remote, c.xff, got, c.want)
		}
	}
	r := httptest.NewRequest("POST", "/api/v1/login", nil)
	r.RemoteAddr = "10.0.0.2:1"
	r.Header.Set("X-Forwarded-Proto", "https")
	if !a.secure(r) {
		t.Fatal("trusted proxy HTTPS not detected")
	}
	r.RemoteAddr = "203.0.113.9:1"
	if a.secure(r) {
		t.Fatal("untrusted X-Forwarded-Proto believed")
	}
}
func TestLoginLimiterHasNoGlobalLockout(t *testing.T) {
	s, e := app.New(context.Background(), t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	if e = SetAdmin(s.Store, "admin", "test-only-password-12345"); e != nil {
		t.Fatal(e)
	}
	v := New(s)
	for i := 0; i < maxTrackedHosts+10; i++ {
		v.auth.attempts[fmt.Sprintf("192.0.2.%d/%d", i%250, i)] = attempt{count: 1, until: time.Now().Add(time.Minute)}
	}
	req := httptest.NewRequest("POST", "/api/v1/login", strings.NewReader(`{"username":"admin","password":"test-only-password-12345"}`))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "198.51.100.7:4000"
	w := httptest.NewRecorder()
	v.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("login blocked by other hosts: %d %s", w.Code, w.Body)
	}
	if len(v.auth.attempts) > maxTrackedHosts+10 {
		t.Fatal("limiter grew without bound")
	}
}
func TestEventsArePushedAndAssetsNotFound(t *testing.T) {
	s, e := app.New(context.Background(), t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	v := New(s)
	server := httptest.NewServer(v)
	defer server.Close()
	res, err := http.Get(server.URL + "/assets/index-missing.js")
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != 404 {
		t.Fatalf("missing asset status %d", res.StatusCode)
	}

	// Log in through the handler to obtain a session cookie for the stream.
	if e = SetAdmin(s.Store, "admin", "test-only-password-12345"); e != nil {
		t.Fatal(e)
	}
	login, _ := http.NewRequest("POST", server.URL+"/api/v1/login", strings.NewReader(`{"username":"admin","password":"test-only-password-12345"}`))
	login.Header.Set("Content-Type", "application/json")
	res, err = http.DefaultClient.Do(login)
	if err != nil || res.StatusCode != 200 {
		t.Fatalf("login %v %v", err, res)
	}
	res.Body.Close()
	stream, _ := http.NewRequest("GET", server.URL+"/api/v1/events/stream", nil)
	for _, c := range res.Cookies() {
		stream.AddCookie(c)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err = http.DefaultClient.Do(stream.WithContext(ctx))
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	time.Sleep(100 * time.Millisecond)
	start := time.Now()
	s.Store.Event("test", "pushed-marker")
	scanner := bufio.NewScanner(res.Body)
	for scanner.Scan() {
		if strings.Contains(scanner.Text(), "pushed-marker") {
			if time.Since(start) > 2*time.Second {
				t.Fatal("event was polled, not pushed")
			}
			return
		}
	}
	t.Fatal("event never arrived", scanner.Err())
}
func TestStatsNodePaging(t *testing.T) {
	s, e := app.New(context.Background(), t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	cfg := s.Engine.Config()
	for i := 0; i < 150; i++ {
		cfg.Nodes = append(cfg.Nodes, model.Node{ID: fmt.Sprintf("n%03d", i), Name: fmt.Sprintf("N%d", i), Outbound: map[string]any{"type": "socks", "server": "127.0.0.1", "server_port": 1000 + i}})
	}
	if _, e = s.Apply(cfg); e != nil {
		t.Fatal(e)
	}
	if got := s.Engine.StatsFiltered(nil).Nodes; got != nil {
		t.Fatal("nodes returned without request")
	}
	page := s.Engine.StatsFiltered(&core.NodeFilter{Offset: 120, Limit: 100}).Nodes
	if len(page) != 30 || page[0].ID != "n120" {
		t.Fatalf("page %d", len(page))
	}
	picked := s.Engine.StatsFiltered(&core.NodeFilter{IDs: map[string]bool{"n007": true, "n149": true}}).Nodes
	if len(picked) != 2 {
		t.Fatalf("picked %d", len(picked))
	}
}
