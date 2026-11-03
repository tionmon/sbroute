package core

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net"
	"net/http"
	"slices"
	"sync/atomic"
	"time"

	D "github.com/miekg/dns"
	M "github.com/sagernet/sing/common/metadata"
	"github.com/tionmon/sbroute/internal/model"
)

type probeJob struct {
	node    *nodeRuntime
	network string
}

func (n *nodeRuntime) probeFlag(network string) *atomic.Bool {
	if network == "udp" {
		return &n.probeUDP
	}
	return &n.probeTCP
}
func (e *Engine) queueProbe(n *nodeRuntime, network string) {
	if e.pressure.Load() || n.disposed.Load() || time.Since(n.state(network).LastCheck) < 10*time.Second {
		return
	}
	flag := n.probeFlag(network)
	if !flag.CompareAndSwap(false, true) {
		return
	}
	select {
	case e.probeQueue <- probeJob{n, network}:
	default:
		flag.Store(false)
	}
}
func (e *Engine) probeWorker() {
	defer e.workers.Done()
	for {
		select {
		case <-e.ctx.Done():
			return
		case job := <-e.probeQueue:
			_ = e.probe(e.ctx, job.node, job.network, false)
		}
	}
}
func (e *Engine) probe(ctx context.Context, n *nodeRuntime, network string, claim bool) error {
	flag := n.probeFlag(network)
	if claim && !flag.CompareAndSwap(false, true) {
		ticker := time.NewTicker(20 * time.Millisecond)
		defer ticker.Stop()
		for flag.Load() {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-ticker.C:
			}
		}
		if n.state(network).Status == "healthy" {
			return nil
		}
		return errors.New(n.state(network).Error)
	}
	defer flag.Store(false)
	if n.disposed.Load() {
		return errors.New("node version retired")
	}
	for !reserve(&e.probing, int64(e.snap.Load().config.Resources.ProbeConcurrency)) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-e.probeAvailable:
		}
	}
	defer e.releaseProbe()
	if e.closing.Load() {
		return context.Canceled
	}
	settings := e.snap.Load().config.Health
	out, release, err := n.acquire()
	if err != nil {
		e.recordHealth(n, network, 0, err, false)
		return err
	}
	defer release()
	if !slices.Contains(out.Network(), network) {
		err = fmt.Errorf("node does not support %s", network)
		e.recordHealth(n, network, 0, err, true)
		return err
	}
	start := time.Now()
	if network == "tcp" {
		for _, u := range settings.URLs {
			probeCtx, cancel := context.WithTimeout(ctx, time.Duration(settings.Timeout)*time.Second)
			transport := &http.Transport{Proxy: nil, DisableKeepAlives: true, ForceAttemptHTTP2: false, DialContext: func(c context.Context, netw, addr string) (net.Conn, error) {
				return out.DialContext(c, "tcp", M.ParseSocksaddr(addr))
			}, MaxResponseHeaderBytes: 16 << 10}
			client := &http.Client{Transport: transport, CheckRedirect: func(_ *http.Request, via []*http.Request) error {
				if len(via) >= 3 {
					return errors.New("probe redirect limit")
				}
				return nil
			}}
			req, _ := http.NewRequestWithContext(probeCtx, "GET", u, nil)
			var res *http.Response
			res, err = client.Do(req)
			if err == nil {
				_, _ = io.CopyN(io.Discard, res.Body, 512)
				res.Body.Close()
				if res.StatusCode != 204 {
					err = fmt.Errorf("probe HTTP status %d (expected 204)", res.StatusCode)
				}
			}
			transport.CloseIdleConnections()
			cancel()
			if err == nil {
				break
			}
			if ctx.Err() != nil {
				break
			}
		}
	} else {
		probeCtx, cancel := context.WithTimeout(ctx, time.Duration(settings.Timeout)*time.Second)
		defer cancel()
		var conn net.PacketConn
		dest := M.ParseSocksaddr(settings.UDPDNS)
		conn, err = out.ListenPacket(probeCtx, dest)
		if err == nil {
			defer conn.Close()
			_ = conn.SetDeadline(time.Now().Add(time.Duration(settings.Timeout) * time.Second))
			msg := new(D.Msg)
			msg.SetQuestion("example.com.", D.TypeA)
			var b []byte
			b, err = msg.Pack()
			if err == nil {
				_, err = conn.WriteTo(b, dest.UDPAddr())
			}
			if err == nil {
				buf := make([]byte, 4096)
				var count int
				count, _, err = conn.ReadFrom(buf)
				if err == nil {
					var answer D.Msg
					err = answer.Unpack(buf[:count])
					if err == nil && (answer.Id != msg.Id || !answer.Response || answer.Rcode != D.RcodeSuccess) {
						err = errors.New("invalid DNS probe response")
					}
				}
			}
		}
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	e.recordHealth(n, network, time.Since(start).Milliseconds(), err, false)
	return err
}
func (e *Engine) releaseProbe() {
	e.probing.Add(-1)
	select {
	case e.probeAvailable <- struct{}{}:
	default:
	}
}
func (e *Engine) recordHealth(n *nodeRuntime, network string, latency int64, err error, unsupported bool) {
	old := n.state(network)
	v := *old
	v.LastCheck = time.Now()
	v.LatencyMS = latency
	settings := e.snap.Load().config
	v.NextCheck = v.LastCheck.Add(time.Duration(float64(settings.Resources.ProbeInterval)*(0.8+rand.Float64()*0.4)) * time.Second)
	if err == nil {
		v.Error = ""
		v.Failures = 0
		v.Successes++
		if old.Status == "unknown" || old.Status == "healthy" || v.Successes >= settings.Health.RecoveryThreshold {
			v.Status = "healthy"
			if old.Status != "healthy" {
				v.HealthySince = time.Now()
			}
		}
	} else {
		v.Error = err.Error()
		if len(v.Error) > 400 {
			v.Error = v.Error[:400]
		}
		v.Successes = 0
		v.Failures++
		if unsupported {
			v.Status = "unsupported"
		} else if old.Status != "healthy" || v.Failures >= settings.Health.FailureThreshold {
			v.Status = "unhealthy"
			v.HealthySince = time.Time{}
			backoff := time.Duration(1<<min(v.Failures, 6)) * 5 * time.Second
			v.NextCheck = time.Now().Add(backoff)
		}
	}
	if network == "udp" {
		n.udp.Store(&v)
	} else {
		n.tcp.Store(&v)
	}
	if old.Status != v.Status {
		e.event("health", fmt.Sprintf("%s %s: %s → %s", n.definition.Name, network, old.Status, v.Status))
	}
	e.signal()
}
func (e *Engine) maintenance() {
	defer e.workers.Done()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-e.ctx.Done():
			return
		case <-e.probeSignal:
			e.mu.Lock()
			e.rebuildPools(e.snap.Load())
			e.mu.Unlock()
		case <-ticker.C:
			s := e.snap.Load()
			rss := residentMemory()
			e.rss.Store(rss)
			groupUsed, groupLimit := cgroupMemory()
			e.groupUsed.Store(groupUsed)
			e.groupLimit.Store(groupLimit)
			if e.fdLimit.Load() == 0 || time.Now().Unix()%10 == 0 {
				count, limit := fileDescriptors()
				e.fdCount.Store(count)
				e.fdLimit.Store(limit)
			}
			budget := uint64(s.config.Resources.MemoryMiB) << 20
			if limit := memoryLimit(); limit > 0 && budget > uint64(limit)*6/10 {
				budget = uint64(limit) * 6 / 10
			}
			if rss > budget*85/100 || (groupLimit > 0 && groupUsed > groupLimit*85/100) {
				e.pressure.Store(true)
			} else if rss < budget*75/100 && (groupLimit == 0 || groupUsed < groupLimit*75/100) {
				e.pressure.Store(false)
			}
			e.mu.Lock()
			active := map[*nodeRuntime]map[string]bool{}
			for _, r := range s.routes {
				if time.Now().Unix()-r.lastUsed.Load() > 1800 {
					continue
				}
				for _, n := range r.nodes {
					if active[n] == nil {
						active[n] = map[string]bool{}
					}
					if containsNetwork(r.entry.Network, "tcp") {
						active[n]["tcp"] = true
					}
					if containsNetwork(r.entry.Network, "udp") {
						active[n]["udp"] = true
					}
				}
			}
			for n, networks := range active {
				for network := range networks {
					st := n.state(network)
					if st.Status != "unsupported" && time.Now().After(st.NextCheck) {
						e.queueProbe(n, network)
					}
				}
			}
			for key, n := range e.nodes {
				n.mu.Lock()
				retired := n.retired
				refs := n.refs
				n.mu.Unlock()
				if !retired.IsZero() && (refs == 0 || time.Since(retired) > 30*time.Minute) {
					n.disposed.Store(true)
					n.closeIdle(refs != 0)
					delete(e.nodes, key)
					continue
				}
				if refs == 0 && time.Now().Unix()-n.lastBusiness.Load() > 600 {
					n.closeIdle(false)
				}
			}
			e.mu.Unlock()
		}
	}
}
func containsNetwork(value, network string) bool { return value == network || value == "tcp,udp" }
func (e *Engine) trimRetiredLocked() {
	versions := map[string][]*nodeRuntime{}
	for _, n := range e.nodes {
		n.mu.Lock()
		retired := !n.retired.IsZero()
		n.mu.Unlock()
		if retired {
			versions[n.definition.ID] = append(versions[n.definition.ID], n)
		}
	}
	for _, v := range versions {
		if len(v) <= 2 {
			continue
		}
		slices.SortFunc(v, func(a, b *nodeRuntime) int { return a.retired.Compare(b.retired) })
		for _, n := range v[:len(v)-2] {
			n.disposed.Store(true)
			n.closeIdle(true)
			delete(e.nodes, n.key)
			e.event("drain", "closed oldest draining version for "+n.definition.Name)
		}
	}
}
func (e *Engine) Probe(ctx context.Context, id, network string) error {
	if network != "tcp" && network != "udp" {
		return errors.New("network must be tcp or udp")
	}
	e.mu.Lock()
	var n *nodeRuntime
	for _, def := range e.snap.Load().config.Nodes {
		if def.ID == id {
			if def.Disabled {
				e.mu.Unlock()
				return errors.New("node disabled")
			}
			key := model.Fingerprint(def)
			n = e.nodes[key]
			if n == nil {
				n = &nodeRuntime{engine: e, key: key, definition: def}
				n.tcp.Store(&HealthState{Status: "unknown"})
				n.udp.Store(&HealthState{Status: "unknown"})
				e.nodes[key] = n
			}
			break
		}
	}
	e.mu.Unlock()
	if n == nil {
		return errors.New("node not found")
	}
	return e.probe(ctx, n, network, true)
}
