package core

import (
	"context"
	"errors"
	M "github.com/sagernet/sing/common/metadata"
	"github.com/tionmon/sbroute/internal/model"
	"io"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"time"
)

// Exit address checks are explicit user actions, never part of periodic health probes.
func (e *Engine) ExitIP(ctx context.Context, id string) (string, error) {
	e.mu.Lock()
	var node *nodeRuntime
	for _, def := range e.snap.Load().config.Nodes {
		if def.ID == id {
			if def.Disabled {
				e.mu.Unlock()
				return "", errors.New("node disabled")
			}
			key := model.Fingerprint(def)
			node = e.nodes[key]
			if node == nil {
				node = &nodeRuntime{engine: e, key: key, definition: def}
				node.tcp.Store(&HealthState{Status: "unknown"})
				node.udp.Store(&HealthState{Status: "unknown"})
				e.nodes[key] = node
			}
			break
		}
	}
	e.mu.Unlock()
	if node == nil {
		return "", errors.New("node not found")
	}
	if !reserve(&e.probing, int64(e.snap.Load().config.Resources.ProbeConcurrency)) {
		return "", errors.New("probe concurrency exhausted")
	}
	defer e.releaseProbe()
	out, release, err := node.acquire()
	if err != nil {
		return "", err
	}
	defer release()
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	transport := &http.Transport{Proxy: nil, DisableKeepAlives: true, DialContext: func(c context.Context, _ string, addr string) (net.Conn, error) {
		return out.DialContext(c, "tcp", M.ParseSocksaddr(addr))
	}}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
	req, _ := http.NewRequestWithContext(ctx, "GET", "https://api.ipify.org", nil)
	res, err := client.Do(req)
	if err != nil {
		return "", errors.New("exit address check failed")
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return "", errors.New("exit address service rejected request")
	}
	b, err := io.ReadAll(io.LimitReader(res.Body, 256))
	if err != nil {
		return "", err
	}
	ip, err := netip.ParseAddr(strings.TrimSpace(string(b)))
	if err != nil {
		return "", errors.New("exit address service returned invalid IP")
	}
	value := ip.String()
	node.exitIP.Store(&value)
	return value, nil
}
