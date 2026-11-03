package core

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net"
	"net/netip"
	"os"
	"runtime"
	"runtime/debug"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	box "github.com/sagernet/sing-box"
	"github.com/sagernet/sing-box/adapter"
	"github.com/sagernet/sing-box/log"
	"github.com/sagernet/sing-box/option"
	B "github.com/sagernet/sing/common/bufio"
	J "github.com/sagernet/sing/common/json"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
	"github.com/tionmon/sbroute/internal/model"
)

var ErrUnavailable = errors.New("no healthy node available (probing or group empty)")

type EventFunc func(string, string)
type Engine struct {
	ctx                context.Context
	cancel             context.CancelFunc
	box                *box.Box
	coreCtx            context.Context
	router             adapter.Router
	logger             log.ContextLogger
	inRegistry         adapter.InboundRegistry
	outRegistry        adapter.OutboundRegistry
	mu                 sync.Mutex
	snap               atomic.Pointer[snapshot]
	nodes              map[string]*nodeRuntime
	listeners          map[string]*listener
	event              EventFunc
	workers            sync.WaitGroup
	probeQueue         chan probeJob
	probeSignal        chan struct{}
	probeAvailable     chan struct{}
	total              atomic.Int64
	udp                atomic.Int64
	dials              atomic.Int64
	probing            atomic.Int64
	up                 atomic.Int64
	down               atomic.Int64
	rejected           atomic.Int64
	pressure           atomic.Bool
	rss                atomic.Uint64
	groupUsed          atomic.Uint64
	groupLimit         atomic.Uint64
	fdCount            atomic.Uint64
	fdLimit            atomic.Uint64
	started            time.Time
	closing            atomic.Bool
	inboundTCP         atomic.Int64
	inboundConnections sync.Map
}
type snapshot struct {
	config model.Config
	routes map[string]*route
}
type routeCounters struct {
	active atomic.Int64
	up     atomic.Int64
	down   atomic.Int64
}
type route struct {
	*routeCounters
	entry    model.Entrance
	nodes    []*nodeRuntime
	prefixes []netip.Prefix
	tcp      atomic.Pointer[pool]
	udp      atomic.Pointer[pool]
	counter  atomic.Uint64
	lastUsed atomic.Int64
}
type pool struct {
	nodes   []*nodeRuntime
	ring    []ringPoint
	network string
}
type ringPoint struct {
	hash uint64
	node *nodeRuntime
}
type listener struct {
	signature string
	inbound   adapter.Inbound
	scope     *adapter.Scope
}
type nodeRuntime struct {
	engine       *Engine
	key          string
	definition   model.Node
	mu           sync.Mutex
	outbound     adapter.Outbound
	scope        *adapter.Scope
	refs         int
	retired      time.Time
	lastBusiness atomic.Int64
	active       atomic.Int64
	pending      atomic.Int64
	up           atomic.Int64
	down         atomic.Int64
	tcp          atomic.Pointer[HealthState]
	udp          atomic.Pointer[HealthState]
	probeTCP     atomic.Bool
	probeUDP     atomic.Bool
	connections  sync.Map
	disposed     atomic.Bool
	exitIP       atomic.Pointer[string]
}
type HealthState struct {
	Status       string    `json:"status"`
	LatencyMS    int64     `json:"latency_ms"`
	LastCheck    time.Time `json:"last_check"`
	NextCheck    time.Time `json:"-"`
	Error        string    `json:"error,omitempty"`
	Failures     int       `json:"failures"`
	Successes    int       `json:"successes"`
	HealthySince time.Time `json:"healthy_since"`
}
type NodeStats struct {
	ID          string      `json:"id"`
	TCP         HealthState `json:"tcp"`
	UDP         HealthState `json:"udp"`
	Active      int64       `json:"active"`
	Pending     int64       `json:"pending"`
	Upload      int64       `json:"upload"`
	Download    int64       `json:"download"`
	Initialized bool        `json:"initialized"`
	Draining    bool        `json:"draining"`
	ExitIP      string      `json:"exit_ip,omitempty"`
}
type EntryStats struct {
	ID         string `json:"id"`
	HealthyTCP int    `json:"healthy_tcp"`
	HealthyUDP int    `json:"healthy_udp"`
	Active     int64  `json:"active"`
	Upload     int64  `json:"upload"`
	Download   int64  `json:"download"`
}
type Stats struct {
	Version     string       `json:"version"`
	CoreVersion string       `json:"core_version"`
	Uptime      int64        `json:"uptime_seconds"`
	Active      int64        `json:"active"`
	UDP         int64        `json:"udp"`
	Pending     int64        `json:"pending"`
	Upload      int64        `json:"upload"`
	Download    int64        `json:"download"`
	Rejected    int64        `json:"rejected"`
	RSS         uint64       `json:"rss_bytes"`
	Heap        uint64       `json:"heap_bytes"`
	Goroutines  int          `json:"goroutines"`
	Pressure    bool         `json:"memory_pressure"`
	Nodes       []NodeStats  `json:"nodes"`
	Entrances   []EntryStats `json:"entrances"`
	Draining    int          `json:"draining_versions"`
	CgroupUsed  uint64       `json:"cgroup_used_bytes"`
	CgroupLimit uint64       `json:"cgroup_limit_bytes"`
	FDCount     uint64       `json:"fd_count"`
	FDLimit     uint64       `json:"fd_limit"`
}

func New(parent context.Context, event EventFunc) (*Engine, error) {
	ctx, cancel := context.WithCancel(parent)
	e := &Engine{ctx: ctx, cancel: cancel, nodes: map[string]*nodeRuntime{}, listeners: map[string]*listener{}, event: event, probeQueue: make(chan probeJob, 256), probeSignal: make(chan struct{}, 1), probeAvailable: make(chan struct{}, 64), started: time.Now()}
	if e.event == nil {
		e.event = func(string, string) {}
	}
	ctx = newRegistries(e)
	var opts option.Options
	err := J.UnmarshalContext(ctx, []byte(`{"log":{"level":"error"},"dns":{"servers":[{"type":"local","tag":"local"}],"final":"local","cache_capacity":1024},"route":{"final":"dispatch","default_domain_resolver":"local"},"outbounds":[{"type":"sbroute","tag":"dispatch"}]}`), &opts)
	if err != nil {
		cancel()
		return nil, err
	}
	e.box, err = box.New(box.Options{Options: opts, Context: ctx})
	if err != nil {
		cancel()
		return nil, err
	}
	if err = e.box.Start(); err != nil {
		cancel()
		e.box.Close()
		return nil, err
	}
	e.snap.Store(&snapshot{config: model.Default(), routes: map[string]*route{}})
	for i := 0; i < 64; i++ {
		e.workers.Add(1)
		go e.probeWorker()
	}
	e.workers.Add(1)
	go e.maintenance()
	return e, nil
}
func (e *Engine) Config() model.Config { return e.snap.Load().config.Clone() }
func (e *Engine) Prepare(c model.Config) (func(int64), func(), error) {
	if err := e.Validate(c); err != nil {
		return nil, nil, err
	}
	e.mu.Lock()
	if e.closing.Load() {
		e.mu.Unlock()
		return nil, nil, errors.New("engine closing")
	}
	next := &snapshot{config: c.Clone(), routes: map[string]*route{}}
	newListeners := map[string]*listener{}
	newNodes := map[string]*nodeRuntime{}
	rollback := func() {
		for _, l := range newListeners {
			_ = l.scope.Close()
		}
		e.mu.Unlock()
	}
	for _, entry := range c.Entrances {
		if !entry.Enabled {
			continue
		}
		r := &route{entry: entry, routeCounters: &routeCounters{}}
		r.lastUsed.Store(time.Now().Unix())
		for _, p := range entry.AllowedCIDRs {
			v, _ := netip.ParsePrefix(p)
			r.prefixes = append(r.prefixes, v)
		}
		var group model.Group
		for _, g := range c.Groups {
			if g.ID == entry.GroupID {
				group = g
				break
			}
		}
		seen := map[string]bool{}
		for _, n := range model.Members(c, group) {
			key := model.Fingerprint(n)
			if seen[key] {
				continue
			}
			seen[key] = true
			nr := e.nodes[key]
			if nr == nil {
				nr = newNodes[key]
			}
			if nr == nil {
				nr = &nodeRuntime{engine: e, key: key, definition: n}
				nr.tcp.Store(&HealthState{Status: "unknown"})
				nr.udp.Store(&HealthState{Status: "unknown"})
				newNodes[key] = nr
			}
			r.nodes = append(r.nodes, nr)
		}
		if old := e.snap.Load().routes[entry.ID]; old != nil {
			r.routeCounters = old.routeCounters
			r.counter.Store(old.counter.Load())
			r.lastUsed.Store(old.lastUsed.Load())
			r.tcp.Store(old.tcp.Load())
			r.udp.Store(old.udp.Load())
		}
		next.routes[entry.ID] = r
		sigBytes, _ := json.Marshal(struct {
			Entry   model.Entrance
			Timeout int
		}{Entry: listenerSettings(entry), Timeout: c.Resources.UDPTimeout})
		sig := string(sigBytes)
		if l := e.listeners[entry.ID]; l != nil && l.signature == sig {
			continue
		}
		// Rebinding an occupied socket cannot be staged safely. Reject instead of silently interrupting it.
		for oldID, l := range e.listeners {
			old := e.snap.Load().routes[oldID]
			if old != nil && old.entry.Port == entry.Port && (old.entry.Listen == entry.Listen || old.entry.Listen == "0.0.0.0" || entry.Listen == "0.0.0.0" || old.entry.Listen == "::" || entry.Listen == "::") {
				_ = l
				rollback()
				return nil, nil, fmt.Errorf("entrance %s: changing listener options on the same socket requires disabling/applying it first", entry.ID)
			}
		}
		l, err := e.createListener(entry, c.Resources.UDPTimeout)
		if err != nil {
			rollback()
			return nil, nil, fmt.Errorf("entrance %s: %w", entry.ID, err)
		}
		l.signature = sig
		newListeners[entry.ID] = l
	}
	e.rebuildPools(next)
	commit := func(version int64) {
		next.config.Version = version
		for k, n := range newNodes {
			e.nodes[k] = n
		}
		e.snap.Store(next)
		for id, l := range e.listeners {
			if _, exists := next.routes[id]; !exists || newListeners[id] != nil {
				_ = l.scope.Close()
				delete(e.listeners, id)
			}
		}
		for id, l := range newListeners {
			e.listeners[id] = l
		}
		wanted := map[string]bool{}
		for _, r := range next.routes {
			for _, n := range r.nodes {
				wanted[n.key] = true
			}
		}
		for key, n := range e.nodes {
			n.mu.Lock()
			if wanted[key] {
				n.retired = time.Time{}
			} else if n.retired.IsZero() {
				n.retired = time.Now()
			}
			n.mu.Unlock()
		}
		e.trimRetiredLocked()
		e.setMemoryBudget(c.Resources.MemoryMiB)
		e.mu.Unlock()
		e.signal()
		for i := 0; i < 64; i++ {
			select {
			case e.probeAvailable <- struct{}{}:
			default:
			}
		}
	}
	return commit, rollback, nil
}
func listenerSettings(e model.Entrance) model.Entrance {
	e.Name = ""
	e.GroupID = ""
	e.Strategy = ""
	e.AllowedCIDRs = nil
	return e
}
func (e *Engine) createListener(entry model.Entrance, timeout int) (*listener, error) {
	m := map[string]any{"type": "mixed", "tag": entry.ID, "listen": entry.Listen, "listen_port": entry.Port, "udp_timeout": fmt.Sprintf("%ds", timeout)}
	if entry.Kind == "forward" {
		host, p, _ := net.SplitHostPort(entry.Target)
		port, _ := strconv.Atoi(p)
		m["type"] = "direct"
		m["override_address"] = host
		m["override_port"] = port
		m["network"] = strings.Split(entry.Network, ",")
	} else {
		m["users"] = entry.Users
	}
	b, _ := json.Marshal(m)
	var opt option.Inbound
	if err := J.UnmarshalContext(e.coreCtx, b, &opt); err != nil {
		return nil, err
	}
	scope := adapter.NewScope(e.coreCtx, e.logger)
	in, err := e.inRegistry.Create(scope.Context(), &admissionRouter{Router: e.router}, e.logger, entry.ID, opt.Type, opt.Options)
	if err != nil {
		scope.Close()
		return nil, err
	}
	if entry.Kind == "mixed" {
		if err = e.startGuardedProxy(entry, in, scope); err != nil {
			_ = scope.Close()
			return nil, err
		}
		return &listener{inbound: in, scope: scope}, nil
	}
	for _, stage := range adapter.ListStartStages {
		if err = in.Start(stage, scope); err != nil {
			scope.Close()
			return nil, err
		}
	}
	return &listener{inbound: in, scope: scope}, nil
}
func (n *nodeRuntime) acquire() (adapter.Outbound, func(), error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.engine.closing.Load() || n.disposed.Load() {
		return nil, nil, errors.New("engine closing or node version retired")
	}
	if n.outbound == nil {
		scope := adapter.NewScope(n.engine.coreCtx, n.engine.logger)
		o, err := n.engine.decodeNode(scope.Context(), n.definition)
		if err != nil {
			scope.Close()
			return nil, nil, err
		}
		out, err := n.engine.outRegistry.CreateOutbound(scope.Context(), n.engine.router, n.engine.logger, o.Tag, o.Type, o.Options)
		if err != nil {
			scope.Close()
			return nil, nil, err
		}
		if lifecycle, ok := out.(adapter.Lifecycle); ok {
			for _, stage := range adapter.ListStartStages {
				if err = lifecycle.Start(stage, scope); err != nil {
					scope.Close()
					return nil, nil, err
				}
			}
		}
		n.scope = scope
		n.outbound = out
	}
	n.refs++
	return n.outbound, func() { n.mu.Lock(); n.refs--; n.mu.Unlock() }, nil
}
func (n *nodeRuntime) closeIdle(force bool) {
	n.mu.Lock()
	if !force && n.refs > 0 {
		n.mu.Unlock()
		return
	}
	scope := n.scope
	n.scope = nil
	n.outbound = nil
	n.mu.Unlock()
	if force {
		n.connections.Range(func(k, v any) bool { _ = k.(io.Closer).Close(); return true })
	}
	if scope != nil {
		_ = scope.Close()
	}
}
func (e *Engine) Close() error {
	if !e.closing.CompareAndSwap(false, true) {
		return nil
	}
	e.cancel()
	e.workers.Wait()
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, l := range e.listeners {
		l.scope.Close()
	}
	e.inboundConnections.Range(func(k, v any) bool { _ = k.(io.Closer).Close(); return true })
	for _, n := range e.nodes {
		n.closeIdle(true)
	}
	return e.box.Close()
}
func (e *Engine) signal() {
	select {
	case e.probeSignal <- struct{}{}:
	default:
	}
}
func (r *route) rebuild() {
	for _, network := range []string{"tcp", "udp"} {
		p := &pool{network: network, nodes: make([]*nodeRuntime, 0, len(r.nodes))}
		for _, n := range r.nodes {
			s := n.state(network)
			if s.Status == "healthy" {
				p.nodes = append(p.nodes, n)
			}
		}
		old := r.tcp.Load()
		if network == "udp" {
			old = r.udp.Load()
		}
		if old != nil && slices.Equal(old.nodes, p.nodes) && (len(p.nodes) == 0 || (r.entry.Strategy == "ip_hash") == (len(old.ring) > 0)) {
			continue
		}
		if r.entry.Strategy == "ip_hash" {
			p.ring = make([]ringPoint, 0, len(p.nodes)*64)
			for _, n := range p.nodes {
				for i := 0; i < 64; i++ {
					p.ring = append(p.ring, ringPoint{hash: hash(n.key + ":" + strconv.Itoa(i)), node: n})
				}
			}
			sort.Slice(p.ring, func(i, j int) bool { return p.ring[i].hash < p.ring[j].hash })
		}
		if network == "tcp" {
			r.tcp.Store(p)
		} else {
			r.udp.Store(p)
		}
	}
}
func (e *Engine) rebuildPools(s *snapshot) {
	cache := map[string][2]*pool{}
	for _, r := range s.routes {
		key := r.entry.GroupID + "/" + strconv.FormatBool(r.entry.Strategy == "ip_hash")
		if pools, ok := cache[key]; ok {
			r.tcp.Store(pools[0])
			r.udp.Store(pools[1])
		} else {
			r.rebuild()
			cache[key] = [2]*pool{r.tcp.Load(), r.udp.Load()}
		}
	}
}
func hash(s string) uint64 { h := sha256.Sum256([]byte(s)); return binary.LittleEndian.Uint64(h[:8]) }
func (n *nodeRuntime) state(network string) *HealthState {
	if network == "udp" {
		return n.udp.Load()
	}
	return n.tcp.Load()
}
func (p *pool) choose(strategy, key string, index uint64, excluded map[*nodeRuntime]bool) *nodeRuntime {
	if len(p.nodes) == 0 {
		return nil
	}
	allowed := func(n *nodeRuntime) bool { return !excluded[n] }
	load := func(n *nodeRuntime) int64 { return n.active.Load() + n.pending.Load() }
	switch strategy {
	case "ip_hash":
		if len(p.ring) == 0 {
			return nil
		}
		v := hash(key)
		idx := sort.Search(len(p.ring), func(i int) bool { return p.ring[i].hash >= v })
		for i := 0; i < len(p.ring); i++ {
			n := p.ring[(idx+i)%len(p.ring)].node
			if allowed(n) {
				return n
			}
		}
	case "least_conn":
		var best *nodeRuntime
		for i := 0; i < len(p.nodes); i++ {
			n := p.nodes[(int(index%uint64(len(p.nodes)))+i)%len(p.nodes)]
			if allowed(n) && (best == nil || load(n) < load(best)) {
				best = n
			}
		}
		return best
	case "failover":
		network := p.network
		if network == "" {
			network = "tcp"
		}
		for _, n := range p.nodes {
			if allowed(n) {
				if n.state(network).HealthySince.IsZero() || time.Since(n.state(network).HealthySince) >= 30*time.Second {
					return n
				}
			}
		}
		for _, n := range p.nodes {
			if allowed(n) {
				return n
			}
		}
	case "random", "p2c":
		start := rand.IntN(len(p.nodes))
		var first *nodeRuntime
		for i := 0; i < len(p.nodes); i++ {
			n := p.nodes[(start+i)%len(p.nodes)]
			if allowed(n) {
				first = n
				break
			}
		}
		if first == nil {
			return nil
		}
		if strategy == "random" || len(p.nodes) == 1 {
			return first
		}
		other := p.nodes[(start+1+rand.IntN(len(p.nodes)-1))%len(p.nodes)]
		if allowed(other) && load(other) < load(first) {
			return other
		}
		return first
	default:
		for i := 0; i < len(p.nodes); i++ {
			n := p.nodes[(int(index%uint64(len(p.nodes)))+i)%len(p.nodes)]
			if allowed(n) {
				return n
			}
		}
	}
	return nil
}
func (e *Engine) routeFor(ctx context.Context, network string) (*route, string, error) {
	m := adapter.ContextFrom(ctx)
	if m == nil {
		return nil, "", errors.New("missing inbound metadata")
	}
	s := e.snap.Load()
	r := s.routes[m.Inbound]
	if r == nil {
		return nil, "", errors.New("entrance disabled")
	}
	if !strings.Contains(r.entry.Network, network) {
		return nil, "", errors.New("network disabled on entrance")
	}
	ip := m.Source.Addr.Unmap()
	if len(r.prefixes) > 0 {
		ok := false
		for _, p := range r.prefixes {
			if p.Contains(ip) {
				ok = true
				break
			}
		}
		if !ok {
			return nil, "", errors.New("source address is not allowed")
		}
	}
	r.lastUsed.Store(time.Now().Unix())
	return r, r.entry.ID + "/" + ip.String(), nil
}
func reserve(counter *atomic.Int64, max int64) bool {
	for {
		v := counter.Load()
		if v >= max {
			return false
		}
		if counter.CompareAndSwap(v, v+1) {
			return true
		}
	}
}
func (e *Engine) admit(udp bool) (func(), error) {
	r := e.snap.Load().config.Resources
	if e.pressure.Load() || !reserve(&e.total, int64(r.MaxSessions)) {
		e.rejected.Add(1)
		return nil, errors.New("session or memory budget exhausted")
	}
	if udp && !reserve(&e.udp, int64(r.MaxUDP)) {
		e.total.Add(-1)
		e.rejected.Add(1)
		return nil, errors.New("UDP session budget exhausted")
	}
	var once sync.Once
	return func() {
		once.Do(func() {
			e.total.Add(-1)
			if udp {
				e.udp.Add(-1)
			}
		})
	}, nil
}

type dispatcher struct{ engine *Engine }

func (d *dispatcher) Type() string           { return "sbroute" }
func (d *dispatcher) Tag() string            { return "dispatch" }
func (d *dispatcher) Network() []string      { return []string{"tcp", "udp"} }
func (d *dispatcher) Dependencies() []string { return nil }
func (d *dispatcher) DialContext(ctx context.Context, network string, dest M.Socksaddr) (net.Conn, error) {
	e := d.engine
	r, key, err := e.routeFor(ctx, "tcp")
	if err != nil {
		return nil, err
	}
	release, err := e.admit(false)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	conn, n, done, err := e.dial(ctx, r, key, "tcp", dest)
	if err != nil {
		release()
		return nil, err
	}
	r.active.Add(1)
	n.active.Add(1)
	n.lastBusiness.Store(time.Now().Unix())
	w := &trackedConn{CounterConn: B.NewInt64CounterConn(conn, []*atomic.Int64{&e.down, &r.down, &n.down}, []*atomic.Int64{&e.up, &r.up, &n.up})}
	w.done = func() { n.connections.Delete(w); n.active.Add(-1); r.active.Add(-1); done(); release() }
	n.connections.Store(w, true)
	if n.disposed.Load() || e.closing.Load() {
		_ = w.Close()
		return nil, errors.New("node version retired while dialing")
	}
	return w, nil
}
func (d *dispatcher) ListenPacket(ctx context.Context, dest M.Socksaddr) (net.PacketConn, error) {
	e := d.engine
	r, key, err := e.routeFor(ctx, "udp")
	if err != nil {
		return nil, err
	}
	release, err := e.admit(true)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	raw, n, done, err := e.dialPacket(ctx, r, key, dest)
	if err != nil {
		release()
		return nil, err
	}
	r.active.Add(1)
	n.active.Add(1)
	n.lastBusiness.Store(time.Now().Unix())
	counter := B.NewInt64CounterPacketConn(B.NewPacketConn(raw), []*atomic.Int64{&e.down, &r.down, &n.down}, nil, []*atomic.Int64{&e.up, &r.up, &n.up}, nil)
	w := &trackedPacketConn{NetPacketConn: B.NewNetPacketConn(counter)}
	w.done = func() { n.connections.Delete(w); n.active.Add(-1); r.active.Add(-1); done(); release() }
	n.connections.Store(w, true)
	if n.disposed.Load() || e.closing.Load() {
		_ = w.Close()
		return nil, errors.New("node version retired while dialing")
	}
	return w, nil
}
func (e *Engine) selectReady(ctx context.Context, r *route, network, key string, excluded map[*nodeRuntime]bool) (*nodeRuntime, error) {
	p := r.tcp.Load()
	if network == "udp" {
		p = r.udp.Load()
	}
	if p == nil || len(p.nodes) == 0 {
		for _, n := range r.nodes {
			if excluded[n] {
				continue
			}
			if n.state(network).Status == "unknown" {
				_ = e.probe(ctx, n, network, true)
				if n.state(network).Status == "healthy" {
					break
				}
			}
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
		}
		r.rebuild()
		p = r.tcp.Load()
		if network == "udp" {
			p = r.udp.Load()
		}
	}
	n := p.choose(r.entry.Strategy, key, r.counter.Add(1)-1, excluded)
	if n == nil {
		return nil, ErrUnavailable
	}
	return n, nil
}
func (e *Engine) dial(ctx context.Context, r *route, key, network string, dest M.Socksaddr) (net.Conn, *nodeRuntime, func(), error) {
	if !reserve(&e.dials, int64(e.snap.Load().config.Resources.MaxDials)) {
		return nil, nil, nil, errors.New("dial concurrency exhausted")
	}
	defer e.dials.Add(-1)
	excluded := map[*nodeRuntime]bool{}
	var last error
	for i := 0; i < 3; i++ {
		n, err := e.selectReady(ctx, r, network, key, excluded)
		if err != nil {
			if last != nil {
				return nil, nil, nil, last
			}
			return nil, nil, nil, err
		}
		excluded[n] = true
		n.pending.Add(1)
		out, done, err := n.acquire()
		if err == nil {
			var c net.Conn
			c, err = out.DialContext(ctx, network, dest)
			n.pending.Add(-1)
			if err == nil {
				return c, n, done, nil
			}
			done()
		} else {
			n.pending.Add(-1)
		}
		last = err
		e.queueProbe(n, network)
		if ctx.Err() != nil {
			break
		}
	}
	return nil, nil, nil, fmt.Errorf("all safe connection attempts failed: %w", last)
}
func (e *Engine) dialPacket(ctx context.Context, r *route, key string, dest M.Socksaddr) (net.PacketConn, *nodeRuntime, func(), error) {
	if !reserve(&e.dials, int64(e.snap.Load().config.Resources.MaxDials)) {
		return nil, nil, nil, errors.New("dial concurrency exhausted")
	}
	defer e.dials.Add(-1)
	excluded := map[*nodeRuntime]bool{}
	var last error
	for i := 0; i < 3; i++ {
		n, err := e.selectReady(ctx, r, "udp", key, excluded)
		if err != nil {
			if last != nil {
				return nil, nil, nil, last
			}
			return nil, nil, nil, err
		}
		excluded[n] = true
		n.pending.Add(1)
		out, done, err := n.acquire()
		if err == nil {
			var c net.PacketConn
			c, err = out.ListenPacket(ctx, dest)
			n.pending.Add(-1)
			if err == nil {
				return c, n, done, nil
			}
			done()
		} else {
			n.pending.Add(-1)
		}
		last = err
		e.queueProbe(n, "udp")
		if ctx.Err() != nil {
			break
		}
	}
	return nil, nil, nil, last
}

type trackedConn struct {
	*B.CounterConn
	once sync.Once
	done func()
	err  error
}

func (c *trackedConn) Close() error {
	c.once.Do(func() { c.err = c.CounterConn.Close(); c.done() })
	return c.err
}

type trackedPacketConn struct {
	N.NetPacketConn
	once sync.Once
	done func()
	err  error
}

func (c *trackedPacketConn) Close() error {
	c.once.Do(func() { c.err = c.NetPacketConn.Close(); c.done() })
	return c.err
}
func (c *trackedPacketConn) ReaderReplaceable() bool { return true }
func (c *trackedPacketConn) WriterReplaceable() bool { return true }
func (c *trackedPacketConn) Upstream() any           { return c.NetPacketConn }

func (e *Engine) Stats() Stats {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	v := Stats{Version: model.Version, CoreVersion: model.CoreVersion, Uptime: int64(time.Since(e.started).Seconds()), Active: e.total.Load(), UDP: e.udp.Load(), Pending: e.dials.Load(), Upload: e.up.Load(), Download: e.down.Load(), Rejected: e.rejected.Load(), RSS: e.rss.Load(), Heap: m.HeapAlloc, Goroutines: runtime.NumGoroutine(), Pressure: e.pressure.Load(), Nodes: []NodeStats{}, Entrances: []EntryStats{}}
	v.CgroupUsed = e.groupUsed.Load()
	v.CgroupLimit = e.groupLimit.Load()
	v.FDCount = e.fdCount.Load()
	v.FDLimit = e.fdLimit.Load()
	s := e.snap.Load()
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, n := range s.config.Nodes {
		key := model.Fingerprint(n)
		nr := e.nodes[key]
		ns := NodeStats{ID: n.ID, TCP: HealthState{Status: "inactive"}, UDP: HealthState{Status: "inactive"}}
		if n.Disabled {
			ns.TCP.Status = "disabled"
			ns.UDP.Status = "disabled"
		}
		if nr != nil {
			ns.TCP = *nr.tcp.Load()
			ns.UDP = *nr.udp.Load()
			ns.Active = nr.active.Load()
			ns.Pending = nr.pending.Load()
			ns.Upload = nr.up.Load()
			ns.Download = nr.down.Load()
			if ip := nr.exitIP.Load(); ip != nil {
				ns.ExitIP = *ip
			}
			nr.mu.Lock()
			ns.Initialized = nr.outbound != nil
			ns.Draining = !nr.retired.IsZero()
			nr.mu.Unlock()
		}
		v.Nodes = append(v.Nodes, ns)
	}
	for _, r := range s.routes {
		v.Entrances = append(v.Entrances, EntryStats{ID: r.entry.ID, HealthyTCP: len(r.tcp.Load().nodes), HealthyUDP: len(r.udp.Load().nodes), Active: r.active.Load(), Upload: r.up.Load(), Download: r.down.Load()})
	}
	for _, n := range e.nodes {
		n.mu.Lock()
		if !n.retired.IsZero() {
			v.Draining++
		}
		n.mu.Unlock()
	}
	return v
}
func (e *Engine) setMemoryBudget(mib int) {
	budget := int64(mib) << 20
	if limit := memoryLimit(); limit > 0 && budget > limit*6/10 {
		budget = limit * 6 / 10
	}
	if os.Getenv("GOMEMLIMIT") == "" {
		debug.SetMemoryLimit(budget * 7 / 10)
	}
}
