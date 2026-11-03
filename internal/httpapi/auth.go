package httpapi

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"sync"
	"time"

	"github.com/tionmon/sbroute/internal/store"
	"golang.org/x/crypto/bcrypt"
)

type credential struct {
	Username string `json:"username"`
	Hash     []byte `json:"hash"`
}

func SetAdmin(s *store.Store, username, password string) error {
	if len(password) < 12 || len(password) > 72 {
		return errors.New("administrator password must be 12–72 bytes")
	}
	if len(username) < 1 || len(username) > 64 {
		return errors.New("invalid username")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), 12)
	if err != nil {
		return err
	}
	b, _ := json.Marshal(credential{username, hash})
	return s.Set("administrator", b)
}

type attempt struct {
	count int
	until time.Time
}
type auth struct {
	store      *store.Store
	mu         sync.Mutex
	sessions   map[[32]byte]time.Time
	attempts   map[string]attempt
	loginSlots chan struct{}
	trusted    []netip.Prefix
}

// maxTrackedHosts bounds limiter memory; the entry closest to expiry is evicted instead of locking everyone out.
const maxTrackedHosts = 4096

func newAuth(s *store.Store, trusted []netip.Prefix) *auth {
	return &auth{store: s, sessions: map[[32]byte]time.Time{}, attempts: map[string]attempt{}, loginSlots: make(chan struct{}, 2), trusted: trusted}
}
func (a *auth) isTrusted(addr netip.Addr) bool {
	for _, p := range a.trusted {
		if p.Contains(addr) {
			return true
		}
	}
	return false
}

// clientIP walks X-Forwarded-For from the right only while hops are trusted proxies, so clients cannot spoof it.
func (a *auth) clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	addr, err := netip.ParseAddr(host)
	if err != nil || !a.isTrusted(addr.Unmap()) {
		return host
	}
	hops := strings.Split(strings.Join(r.Header.Values("X-Forwarded-For"), ","), ",")
	for i := len(hops) - 1; i >= 0; i-- {
		hop, err := netip.ParseAddr(strings.TrimSpace(hops[i]))
		if err != nil {
			break
		}
		addr = hop.Unmap()
		if !a.isTrusted(addr) {
			break
		}
	}
	return addr.String()
}

// secure reports whether the browser reached us over HTTPS, directly or through a trusted proxy.
func (a *auth) secure(r *http.Request) bool {
	if r.TLS != nil || strings.HasPrefix(r.Header.Get("Origin"), "https://") {
		return true
	}
	host, _, _ := net.SplitHostPort(r.RemoteAddr)
	addr, err := netip.ParseAddr(host)
	return err == nil && a.isTrusted(addr.Unmap()) && strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}
func (a *auth) valid(r *http.Request) bool {
	cookie, err := r.Cookie("sbroute_session")
	if err != nil {
		return false
	}
	key := sha256.Sum256([]byte(cookie.Value))
	a.mu.Lock()
	defer a.mu.Unlock()
	expires, ok := a.sessions[key]
	if ok && time.Now().Before(expires) {
		return true
	}
	delete(a.sessions, key)
	return false
}
func (a *auth) login(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := decode(w, r, &input); err != nil {
		fail(w, 400, err)
		return
	}
	host := a.clientIP(r)
	a.mu.Lock()
	for k, v := range a.attempts {
		if time.Now().After(v.until) {
			delete(a.attempts, k)
		}
	}
	v, tracked := a.attempts[host]
	if !tracked && len(a.attempts) >= maxTrackedHosts {
		var oldest string
		for k, x := range a.attempts {
			if oldest == "" || x.until.Before(a.attempts[oldest].until) {
				oldest = k
			}
		}
		delete(a.attempts, oldest)
	}
	if v.count >= 10 {
		a.mu.Unlock()
		fail(w, 429, errors.New("too many login attempts; retry in 5 minutes"))
		return
	}
	v.count++
	v.until = time.Now().Add(5 * time.Minute)
	a.attempts[host] = v
	a.mu.Unlock()
	select {
	case a.loginSlots <- struct{}{}:
		defer func() { <-a.loginSlots }()
	default:
		fail(w, 429, errors.New("login busy"))
		return
	}
	b, err := a.store.Get("administrator")
	if err != nil {
		fail(w, 503, errors.New("run sbroute admin to initialize administrator"))
		return
	}
	var cred credential
	if json.Unmarshal(b, &cred) != nil {
		fail(w, 503, errors.New("administrator credential unreadable"))
		return
	}
	validPass := bcrypt.CompareHashAndPassword(cred.Hash, []byte(input.Password)) == nil
	if !validPass || cred.Username != input.Username {
		fail(w, 401, errors.New("invalid username or password"))
		return
	}
	token := make([]byte, 32)
	_, _ = rand.Read(token)
	value := hex.EncodeToString(token)
	a.mu.Lock()
	delete(a.attempts, host)
	for k, t := range a.sessions {
		if time.Now().After(t) {
			delete(a.sessions, k)
		}
	}
	if len(a.sessions) >= 128 {
		a.mu.Unlock()
		fail(w, 429, errors.New("session limit"))
		return
	}
	a.sessions[sha256.Sum256([]byte(value))] = time.Now().Add(12 * time.Hour)
	a.mu.Unlock()
	http.SetCookie(w, &http.Cookie{Name: "sbroute_session", Value: value, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode, Secure: a.secure(r), MaxAge: 43200})
	reply(w, 200, map[string]any{"username": cred.Username})
}
func (a *auth) logout(w http.ResponseWriter, r *http.Request) {
	if c, e := r.Cookie("sbroute_session"); e == nil {
		a.mu.Lock()
		delete(a.sessions, sha256.Sum256([]byte(c.Value)))
		a.mu.Unlock()
	}
	http.SetCookie(w, &http.Cookie{Name: "sbroute_session", Value: "", Path: "/", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteStrictMode})
	reply(w, 200, map[string]bool{"ok": true})
}
