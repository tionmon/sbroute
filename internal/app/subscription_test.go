package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/tionmon/sbroute/internal/model"
)

func TestSubscriptionSkipsUnsupportedItemsAndReportsUsage(t *testing.T) {
	s, e := New(context.Background(), t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	var content atomic.Value
	content.Store("socks5://example.com:1080#A\nsocks5://example.com:1081#B\nwireguard://key@example.com:51820#Unsupported")
	var agent, via atomic.Value
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		agent.Store(r.UserAgent())
		via.Store(r.RemoteAddr)
		w.Header().Set("Subscription-Userinfo", "upload=10; download=20; total=1000; expire=1893456000")
		_, _ = w.Write([]byte(content.Load().(string)))
	}))
	defer server.Close()
	cfg := s.Engine.Config()
	cfg.Nodes = []model.Node{{ID: "local", Name: "Local test", Outbound: map[string]any{"type": "direct"}}}
	cfg.Subscriptions = []model.Subscription{{ID: "sub", Name: "Source", URL: server.URL + "/token-secret", Enabled: true, Interval: 3600, UserAgent: "clash.meta", Via: "local"}}
	if _, e = s.Apply(cfg); e != nil {
		t.Fatal(e)
	}
	status, e := s.Refresh(context.Background(), "sub")
	if e != nil {
		t.Fatal(e)
	}
	if status.Pending || status.Skipped != 1 || len(status.Issues) != 1 || status.Count != 2 {
		t.Fatalf("unsupported item blocked the update: %+v", status)
	}
	if n := len(s.Engine.Config().Nodes); n != 3 {
		t.Fatalf("expected 2 imported nodes plus local, got %d", n)
	}
	if agent.Load() != "clash.meta" {
		t.Fatalf("user agent %v", agent.Load())
	}
	if status.Usage == nil || status.Usage.Total != 1000 || status.Usage.Download != 20 || status.Usage.Expire != 1893456000 {
		t.Fatalf("usage %+v", status.Usage)
	}
	raw, _ := s.Store.Get("subscription/sub")
	if strings.Contains(string(raw), "token-secret") {
		t.Fatal("subscription URL stored in metadata")
	}

	// A mass removal is staged, then replaced automatically once the source recovers.
	content.Store("socks5://example.com:1080#A")
	cfg = s.Engine.Config()
	cfg.Subscriptions[0].Via = ""
	if _, e = s.Apply(cfg); e != nil {
		t.Fatal(e)
	}
	content.Store("socks5://example.com:1090#Other")
	if status, e = s.Refresh(context.Background(), "sub"); e != nil || !status.Pending {
		t.Fatalf("mass removal not staged: %+v %v", status, e)
	}
	content.Store("socks5://example.com:1080#A\nsocks5://example.com:1081#B\nsocks5://example.com:1082#C")
	if status, e = s.Refresh(context.Background(), "sub"); e != nil || status.Pending {
		t.Fatalf("recovered source still pending: %+v %v", status, e)
	}
	if n := len(s.Engine.Config().Nodes); n != 4 {
		t.Fatalf("expected 3 imported nodes plus local, got %d", n)
	}
}
func TestSubscriptionLegacyURLStateAndValidation(t *testing.T) {
	if !sameURL(SubscriptionState{URLHash: "https://a/b"}, "https://a/b") || !sameURL(SubscriptionState{URLHash: urlHash("https://a/b")}, "https://a/b") || sameURL(SubscriptionState{URLHash: urlHash("https://a/c")}, "https://a/b") {
		t.Fatal("legacy URL comparison")
	}
	if u := parseUsage("upload=1;download=2.0; total=3"); u == nil || u.Upload != 1 || u.Download != 2 || u.Total != 3 {
		t.Fatalf("%+v", u)
	}
	if parseUsage("garbage") != nil {
		t.Fatal("garbage usage parsed")
	}
	c := model.Default()
	c.Subscriptions = []model.Subscription{{ID: "s", Name: "S", URL: "https://example.com", Interval: 3600, Via: "missing"}}
	if c.Validate() == nil {
		t.Fatal("missing download node accepted")
	}
	c.Subscriptions[0].Via = ""
	c.Subscriptions[0].UserAgent = "bad\r\nheader"
	if c.Validate() == nil {
		t.Fatal("header injection accepted")
	}
}
func TestConvertedNodesAcceptedByCore(t *testing.T) {
	s, e := New(context.Background(), t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	for _, outbound := range []map[string]any{
		{"type": "shadowsocks", "server": "example.com", "server_port": 443, "method": "aes-128-gcm", "password": "p", "plugin": "obfs-local", "plugin_opts": "obfs=http;obfs-host=a.example.com"},
		{"type": "hysteria2", "server": "example.com", "server_port": 443, "password": "p", "server_ports": []string{"20000:30000"}, "tls": map[string]any{"enabled": true}},
		{"type": "vless", "server": "example.com", "server_port": 443, "uuid": "11111111-1111-4111-8111-111111111111", "transport": map[string]any{"type": "ws", "path": "/ws", "max_early_data": 2048, "early_data_header_name": "Sec-WebSocket-Protocol"}},
	} {
		if err := s.ValidateNode(model.Node{ID: "n", Name: "N", Outbound: outbound}); err != nil {
			t.Fatalf("%v: %v", outbound["type"], err)
		}
	}
	for _, outbound := range []map[string]any{
		{"type": "shadowsocks", "server": "example.com", "server_port": 443, "method": "aes-128-gcm", "password": "p", "plugin": "/usr/bin/evil"},
		{"type": "shadowsocks", "server": "example.com", "server_port": 443, "method": "aes-128-gcm", "password": "p", "plugin": "v2ray-plugin", "plugin_opts": "tls;cert=/etc/ssl/key.pem"},
	} {
		if err := s.ValidateNode(model.Node{ID: "n", Name: "N", Outbound: outbound}); err == nil {
			t.Fatalf("accepted %v", outbound)
		}
	}
}
