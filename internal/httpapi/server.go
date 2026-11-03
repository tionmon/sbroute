package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/tionmon/sbroute/internal/app"
	"github.com/tionmon/sbroute/internal/importer"
	"github.com/tionmon/sbroute/internal/model"
	assets "github.com/tionmon/sbroute/internal/web"
	"gopkg.in/yaml.v3"
)

type preview struct {
	result  importer.Result
	source  string
	created time.Time
}
type Server struct {
	service  *app.Service
	auth     *auth
	mux      *http.ServeMux
	mu       sync.Mutex
	previews map[string]preview
	streams  chan struct{}
}

func New(s *app.Service) *Server {
	v := &Server{service: s, auth: newAuth(s.Store), mux: http.NewServeMux(), previews: map[string]preview{}, streams: make(chan struct{}, 8)}
	v.routes()
	return v
}
func (v *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'; frame-ancestors 'none'; base-uri 'self'")
	if strings.HasPrefix(r.URL.Path, "/api/") {
		w.Header().Set("Cache-Control", "no-store")
		if r.Method != "GET" && r.Method != "HEAD" {
			if origin := r.Header.Get("Origin"); origin != "" {
				u, e := url.Parse(origin)
				if e != nil || u.Host != r.Host || (u.Scheme != "http" && u.Scheme != "https") {
					fail(w, 403, errors.New("cross-origin mutation denied"))
					return
				}
			}
			if r.Body != nil && r.ContentLength != 0 && !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
				fail(w, 415, errors.New("application/json required"))
				return
			}
		}
		if r.URL.Path != "/api/v1/login" && !v.auth.valid(r) {
			fail(w, 401, errors.New("authentication required"))
			return
		}
	}
	v.mux.ServeHTTP(w, r)
}
func reply(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func fail(w http.ResponseWriter, status int, err error) {
	reply(w, status, map[string]any{"error": err.Error()})
}
func decode(w http.ResponseWriter, r *http.Request, target any) error {
	r.Body = http.MaxBytesReader(w, r.Body, model.MaxImportBytes+1<<20)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if e := d.Decode(target); e != nil {
		return e
	}
	if e := d.Decode(new(any)); e != io.EOF {
		return errors.New("exactly one JSON document required")
	}
	return nil
}
func statusFor(err error) int {
	if errors.Is(err, app.ErrConflict) {
		return 409
	}
	return 400
}
func (v *Server) routes() {
	m := v.mux
	m.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { reply(w, 200, map[string]string{"status": "ok"}) })
	m.HandleFunc("POST /api/v1/login", v.auth.login)
	m.HandleFunc("POST /api/v1/logout", v.auth.logout)
	m.HandleFunc("GET /api/v1/session", func(w http.ResponseWriter, r *http.Request) { reply(w, 200, map[string]bool{"authenticated": true}) })
	m.HandleFunc("GET /api/v1/config", func(w http.ResponseWriter, r *http.Request) {
		c := v.service.Engine.Config()
		if r.URL.Query().Get("secrets") != "1" {
			c = c.Redacted()
		}
		reply(w, 200, c)
	})
	m.HandleFunc("PUT /api/v1/config", func(w http.ResponseWriter, r *http.Request) {
		var c model.Config
		if e := decode(w, r, &c); e != nil {
			fail(w, 400, e)
			return
		}
		next, e := v.service.Apply(c)
		if e != nil {
			fail(w, statusFor(e), e)
			return
		}
		reply(w, 200, next)
	})
	m.HandleFunc("POST /api/v1/config/validate", func(w http.ResponseWriter, r *http.Request) {
		var c model.Config
		if e := decode(w, r, &c); e != nil {
			fail(w, 400, e)
			return
		}
		if e := v.service.Validate(c); e != nil {
			fail(w, 400, e)
			return
		}
		reply(w, 200, map[string]bool{"valid": true})
	})
	m.HandleFunc("GET /api/v1/config/export", func(w http.ResponseWriter, r *http.Request) {
		c := v.service.Engine.Config()
		if r.URL.Query().Get("secrets") != "1" {
			c = c.Redacted()
		}
		b, e := yaml.Marshal(c)
		if e != nil {
			fail(w, 500, e)
			return
		}
		w.Header().Set("Content-Type", "application/yaml")
		w.Header().Set("Content-Disposition", `attachment; filename="sbroute.yaml"`)
		_, _ = w.Write(b)
	})
	m.HandleFunc("GET /api/v1/revisions", func(w http.ResponseWriter, r *http.Request) {
		values, e := v.service.Store.Revisions()
		if e != nil {
			fail(w, 500, e)
			return
		}
		reply(w, 200, values)
	})
	m.HandleFunc("POST /api/v1/revisions/{version}/rollback", func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Version int64 `json:"version"`
		}
		if err := decode(w, r, &input); err != nil {
			fail(w, 400, err)
			return
		}
		version, e := strconv.ParseInt(r.PathValue("version"), 10, 64)
		if e != nil {
			fail(w, 400, e)
			return
		}
		cfg, e := v.service.RollbackAt(version, input.Version)
		if e != nil {
			fail(w, statusFor(e), e)
			return
		}
		reply(w, 200, cfg)
	})
	m.HandleFunc("GET /api/v1/presets", func(w http.ResponseWriter, r *http.Request) {
		reply(w, 200, []model.Resources{model.Preset("low-memory"), model.Preset("balanced"), model.Preset("throughput")})
	})
	m.HandleFunc("GET /api/v1/stats", func(w http.ResponseWriter, r *http.Request) {
		stats := v.service.Engine.Stats()
		if r.URL.Query().Get("nodes") != "1" {
			stats.Nodes = nil
		} else if ids := r.URL.Query().Get("ids"); ids != "" {
			wanted := map[string]bool{}
			for i, id := range strings.Split(ids, ",") {
				if i >= 100 {
					break
				}
				wanted[id] = true
			}
			filtered := stats.Nodes[:0]
			for _, n := range stats.Nodes {
				if wanted[n.ID] {
					filtered = append(filtered, n)
				}
			}
			stats.Nodes = filtered
		} else {
			offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
			offset = max(0, min(offset, len(stats.Nodes)))
			stats.Nodes = stats.Nodes[offset:min(offset+100, len(stats.Nodes))]
		}
		reply(w, 200, stats)
	})
	m.HandleFunc("GET /api/v1/events", func(w http.ResponseWriter, r *http.Request) {
		events, e := v.service.Store.Events()
		if e != nil {
			fail(w, 500, e)
			return
		}
		reply(w, 200, events)
	})
	m.HandleFunc("GET /api/v1/events/stream", v.streamEvents)
	m.HandleFunc("POST /api/v1/imports/preview", v.importPreview)
	m.HandleFunc("POST /api/v1/imports/apply", v.importApply)
	m.HandleFunc("POST /api/v1/nodes/{id}/probe", func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Network string `json:"network"`
		}
		if e := decode(w, r, &input); e != nil {
			fail(w, 400, e)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
		defer cancel()
		if e := v.service.Engine.Probe(ctx, r.PathValue("id"), input.Network); e != nil {
			fail(w, 400, e)
			return
		}
		reply(w, 200, map[string]bool{"healthy": true})
	})
	m.HandleFunc("GET /api/v1/subscriptions/status", func(w http.ResponseWriter, r *http.Request) { reply(w, 200, v.service.SubscriptionStatuses()) })
	m.HandleFunc("POST /api/v1/nodes/{id}/exit-ip", func(w http.ResponseWriter, r *http.Request) {
		ip, err := v.service.Engine.ExitIP(r.Context(), r.PathValue("id"))
		if err != nil {
			fail(w, 400, err)
			return
		}
		reply(w, 200, map[string]string{"ip": ip})
	})
	m.HandleFunc("POST /api/v1/subscriptions/{id}/refresh", func(w http.ResponseWriter, r *http.Request) {
		st, e := v.service.Refresh(r.Context(), r.PathValue("id"))
		if e != nil {
			fail(w, 400, e)
			return
		}
		reply(w, 200, st)
	})
	m.HandleFunc("POST /api/v1/subscriptions/{id}/accept", func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Version int64 `json:"version"`
		}
		if e := decode(w, r, &input); e != nil {
			fail(w, 400, e)
			return
		}
		c, e := v.service.AcceptSubscription(r.PathValue("id"), input.Version)
		if e != nil {
			fail(w, statusFor(e), e)
			return
		}
		reply(w, 200, c)
	})
	m.HandleFunc("GET /api/v1/backup", v.backup)
	m.Handle("/", assets.Handler())
}
func (v *Server) importPreview(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Content  string `json:"content"`
		SourceID string `json:"source_id"`
	}
	if e := decode(w, r, &input); e != nil {
		fail(w, 400, e)
		return
	}
	if v.service.Engine.Stats().Pressure {
		fail(w, 429, errors.New("memory pressure: import paused"))
		return
	}
	if input.SourceID != "" {
		found := false
		for _, s := range v.service.Engine.Config().Subscriptions {
			if s.ID == input.SourceID {
				found = true
			}
		}
		if !found {
			fail(w, 400, errors.New("source subscription not found"))
			return
		}
	}
	result := v.service.Import([]byte(input.Content), input.SourceID)
	v.mu.Lock()
	defer v.mu.Unlock()
	for k, p := range v.previews {
		if time.Since(p.created) > 5*time.Minute {
			delete(v.previews, k)
		}
	}
	if len(v.previews) >= 2 {
		var oldest string
		var at time.Time
		for k, p := range v.previews {
			if oldest == "" || p.created.Before(at) {
				oldest = k
				at = p.created
			}
		}
		delete(v.previews, oldest)
	}
	id := model.ID()
	v.previews[id] = preview{result: result, source: input.SourceID, created: time.Now()}
	copy := model.Default()
	copy.Nodes = result.Nodes
	reply(w, 200, map[string]any{"preview_id": id, "nodes": copy.Redacted().Nodes, "issues": result.Issues, "duplicates": result.Duplicates})
}
func (v *Server) importApply(w http.ResponseWriter, r *http.Request) {
	var input struct {
		PreviewID string `json:"preview_id"`
		Version   int64  `json:"version"`
	}
	if e := decode(w, r, &input); e != nil {
		fail(w, 400, e)
		return
	}
	v.mu.Lock()
	p, ok := v.previews[input.PreviewID]
	v.mu.Unlock()
	if !ok || time.Since(p.created) > 5*time.Minute {
		fail(w, 400, errors.New("preview expired; preview again"))
		return
	}
	if len(p.result.Nodes) == 0 {
		fail(w, 400, errors.New("no valid nodes to import"))
		return
	}
	c := v.service.Engine.Config()
	if c.Version != input.Version {
		fail(w, 409, app.ErrConflict)
		return
	}
	next, _ := importer.Merge(c, p.result.Nodes, p.source, false)
	result, e := v.service.Apply(next)
	if e != nil {
		fail(w, statusFor(e), e)
		return
	}
	v.mu.Lock()
	delete(v.previews, input.PreviewID)
	v.mu.Unlock()
	reply(w, 200, result)
}
func (v *Server) streamEvents(w http.ResponseWriter, r *http.Request) {
	select {
	case v.streams <- struct{}{}:
		defer func() { <-v.streams }()
	default:
		fail(w, 429, errors.New("event stream limit"))
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		fail(w, 500, errors.New("stream unsupported"))
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("X-Accel-Buffering", "no")
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()
	last := int64(0)
	for {
		events, err := v.service.Store.Events()
		if err != nil {
			return
		}
		for i := len(events) - 1; i >= 0; i-- {
			if events[i].ID <= last {
				continue
			}
			b, _ := json.Marshal(events[i])
			if _, err = fmt.Fprintf(w, "id: %d\ndata: %s\n\n", events[i].ID, b); err != nil {
				return
			}
			last = events[i].ID
		}
		_, _ = io.WriteString(w, ": heartbeat\n\n")
		flusher.Flush()
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			if !v.auth.valid(r) {
				return
			}
		}
	}
}
func (v *Server) backup(w http.ResponseWriter, r *http.Request) {
	f, e := os.CreateTemp(filepath.Dir(v.service.Store.Path), "backup-*.db")
	if e != nil {
		fail(w, 500, e)
		return
	}
	p := f.Name()
	f.Close()
	os.Remove(p)
	defer os.Remove(p)
	if e = v.service.Store.Backup(p); e != nil {
		fail(w, 500, e)
		return
	}
	w.Header().Set("Content-Disposition", `attachment; filename="sbroute-backup.db"`)
	http.ServeFile(w, r, p)
}
