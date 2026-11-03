package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/tionmon/sbroute/internal/app"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"testing"
)

func TestAuthenticatedConfigImportAndCSRF(t *testing.T) {
	s, e := app.New(context.Background(), t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	if e = SetAdmin(s.Store, "admin", "test-only-password-12345"); e != nil {
		t.Fatal(e)
	}
	server := httptest.NewServer(New(s))
	defer server.Close()
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	request := func(method, path string, body any, origin string) (int, []byte) {
		t.Helper()
		var b []byte
		if body != nil {
			b, _ = json.Marshal(body)
		}
		req, _ := http.NewRequest(method, server.URL+path, bytes.NewReader(b))
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		res, e := client.Do(req)
		if e != nil {
			t.Fatal(e)
		}
		defer res.Body.Close()
		data, _ := io.ReadAll(res.Body)
		return res.StatusCode, data
	}
	if status, _ := request("GET", "/api/v1/config", nil, ""); status != 401 {
		t.Fatal("unauthenticated config accessible")
	}
	if status, b := request("POST", "/api/v1/login", map[string]string{"username": "admin", "password": "test-only-password-12345"}, server.URL); status != 200 {
		t.Fatalf("login: %d %s", status, b)
	}
	status, b := request("GET", "/api/v1/config", nil, "")
	if status != 200 {
		t.Fatal(string(b))
	}
	var cfg map[string]any
	_ = json.Unmarshal(b, &cfg)
	if status, _ = request("PUT", "/api/v1/config", cfg, "https://attacker.example"); status != 403 {
		t.Fatal("cross origin accepted")
	}
	status, b = request("POST", "/api/v1/imports/preview", map[string]string{"content": "socks5://user:secret@example.com:1080#Node", "source_id": ""}, server.URL)
	if status != 200 {
		t.Fatal(string(b))
	}
	if bytes.Contains(b, []byte("secret")) {
		t.Fatal("preview leaked password")
	}
	var p struct {
		PreviewID string `json:"preview_id"`
	}
	_ = json.Unmarshal(b, &p)
	status, b = request("POST", "/api/v1/imports/apply", map[string]any{"preview_id": p.PreviewID, "version": cfg["version"]}, server.URL)
	if status != 200 {
		t.Fatalf("import: %s", b)
	}
	if bytes.Contains(b, []byte("secret")) {
		t.Fatal("config response leaked password")
	}
	status, b = request("GET", "/api/v1/config?secrets=1", nil, "")
	if status != 200 || !bytes.Contains(b, []byte("secret")) {
		t.Fatal("explicit secret export unavailable")
	}
	if status, _ = request("PUT", "/api/v1/config", cfg, server.URL); status != 409 {
		t.Fatal("stale config accepted")
	}
	if status, _ = request("POST", "/api/v1/logout", map[string]string{}, server.URL); status != 200 {
		t.Fatal("logout failed")
	}
	if status, _ = request("GET", "/api/v1/config", nil, ""); status != 401 {
		t.Fatal("logout did not revoke session")
	}
}
