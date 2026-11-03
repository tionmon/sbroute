package core

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"time"

	M "github.com/sagernet/sing/common/metadata"
)

// exitIPServices are tried in order; api64 answers over IPv6 when the node's exit has it.
var exitIPServices = []string{"https://api64.ipify.org", "https://icanhazip.com", "https://ifconfig.me/ip"}

// lookupNode returns the runtime for a configured node, creating a cold one when no entrance uses it yet.
func (e *Engine) lookupNode(id string) (*nodeRuntime, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	snap := e.snap.Load()
	for _, def := range snap.config.Nodes {
		if def.ID != id {
			continue
		}
		if def.Disabled {
			return nil, errors.New("node disabled")
		}
		key := snap.key(def)
		n := e.nodes[key]
		if n == nil {
			n = &nodeRuntime{engine: e, key: key, definition: def}
			n.tcp.Store(&HealthState{Status: "unknown"})
			n.udp.Store(&HealthState{Status: "unknown"})
			e.nodes[key] = n
			e.publishNodesLocked()
		}
		return n, nil
	}
	return nil, errors.New("node not found")
}

// NodeDialer returns a TCP dialer through one node, for management traffic such as subscription downloads.
func (e *Engine) NodeDialer(id string) (func(context.Context, string, string) (net.Conn, error), func(), error) {
	n, err := e.lookupNode(id)
	if err != nil {
		return nil, nil, err
	}
	out, release, err := n.acquire()
	if err != nil {
		return nil, nil, err
	}
	return func(ctx context.Context, _ string, addr string) (net.Conn, error) {
		return out.DialContext(ctx, "tcp", M.ParseSocksaddr(addr))
	}, release, nil
}

// Exit address checks are explicit user actions, never part of periodic health probes.
func (e *Engine) ExitIP(ctx context.Context, id string) (string, error) {
	node, err := e.lookupNode(id)
	if err != nil {
		return "", err
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
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	transport := &http.Transport{Proxy: nil, DisableKeepAlives: true, DialContext: func(c context.Context, _ string, addr string) (net.Conn, error) {
		return out.DialContext(c, "tcp", M.ParseSocksaddr(addr))
	}}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
	var errs []error
	for _, service := range exitIPServices {
		ip, err := queryExitIP(ctx, client, service)
		if err == nil {
			node.exitIP.Store(&ip)
			return ip, nil
		}
		errs = append(errs, fmt.Errorf("%s: %w", strings.TrimPrefix(service, "https://"), err))
		if ctx.Err() != nil {
			break
		}
	}
	return "", fmt.Errorf("exit address check failed: %w", errors.Join(errs...))
}
func queryExitIP(ctx context.Context, client *http.Client, service string) (string, error) {
	req, _ := http.NewRequestWithContext(ctx, "GET", service, nil)
	req.Header.Set("User-Agent", "curl/8")
	res, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return "", fmt.Errorf("HTTP status %d", res.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(res.Body, 256))
	if err != nil {
		return "", err
	}
	ip, err := netip.ParseAddr(strings.TrimSpace(string(b)))
	if err != nil {
		return "", errors.New("invalid IP in response")
	}
	return ip.Unmap().String(), nil
}
