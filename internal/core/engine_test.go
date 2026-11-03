package core

import (
	"context"
	"fmt"
	"io"
	"net"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/tionmon/sbroute/internal/model"
	"golang.org/x/net/proxy"
)

func freePort(t testing.TB) uint16 {
	t.Helper()
	l, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	defer l.Close()
	return uint16(l.Addr().(*net.TCPAddr).Port)
}
func echoTCP(t testing.TB) string {
	t.Helper()
	l, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { l.Close() })
	go func() {
		for {
			c, e := l.Accept()
			if e != nil {
				return
			}
			go func() { defer c.Close(); _, _ = io.Copy(c, c) }()
		}
	}()
	return l.Addr().String()
}
func newTestEngine(t testing.TB) *Engine {
	t.Helper()
	e, err := New(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := e.Close(); err != nil {
			t.Error(err)
		}
	})
	return e
}
func applyTest(t testing.TB, e *Engine, c model.Config) {
	t.Helper()
	commit, _, err := e.Prepare(c)
	if err != nil {
		t.Fatal(err)
	}
	commit(c.Version + 1)
	e.mu.Lock()
	for _, n := range e.nodes {
		n.tcp.Store(&HealthState{Status: "healthy", HealthySince: time.Now().Add(-time.Minute), NextCheck: time.Now().Add(time.Hour)})
		n.udp.Store(&HealthState{Status: "healthy", HealthySince: time.Now().Add(-time.Minute), NextCheck: time.Now().Add(time.Hour)})
	}
	for _, r := range e.snap.Load().routes {
		r.rebuild()
	}
	e.mu.Unlock()
}
func directConfig(port uint16, kind, target string) model.Config {
	c := model.Default()
	c.Nodes = []model.Node{{ID: "direct", Name: "Local test direct", Outbound: map[string]any{"type": "direct"}}}
	c.Groups = []model.Group{{ID: "g", Name: "Local", Members: []string{"direct"}}}
	c.Entrances = []model.Entrance{{ID: "in", Name: "Test", Enabled: true, Listen: "127.0.0.1", Port: port, Kind: kind, Network: "tcp,udp", Target: target, GroupID: "g", Strategy: "round_robin"}}
	return c
}
func TestNativeTCPForwardAndSocks(t *testing.T) {
	target := echoTCP(t)
	for _, kind := range []string{"forward", "mixed"} {
		t.Run(kind, func(t *testing.T) {
			e := newTestEngine(t)
			port := freePort(t)
			applyTest(t, e, directConfig(port, kind, target))
			addr := net.JoinHostPort("127.0.0.1", strconv.Itoa(int(port)))
			var c net.Conn
			var err error
			if kind == "mixed" {
				d, _ := proxy.SOCKS5("tcp", addr, nil, proxy.Direct)
				c, err = d.Dial("tcp", target)
			} else {
				c, err = net.Dial("tcp", addr)
			}
			if err != nil {
				t.Fatal(err)
			}
			_ = c.SetDeadline(time.Now().Add(3 * time.Second))
			if _, err = c.Write([]byte("hello")); err != nil {
				t.Fatal(err)
			}
			b := make([]byte, 5)
			if _, err = io.ReadFull(c, b); err != nil {
				t.Fatal(err)
			}
			if string(b) != "hello" {
				t.Fatal(string(b))
			}
			c.Close()
			for i := 0; i < 50 && e.total.Load() != 0; i++ {
				time.Sleep(10 * time.Millisecond)
			}
			if e.total.Load() != 0 {
				t.Fatal("session count leak")
			}
			if e.up.Load() != 5 || e.down.Load() != 5 {
				t.Fatalf("traffic counts up=%d down=%d", e.up.Load(), e.down.Load())
			}
		})
	}
}
func TestNativeUDPForward(t *testing.T) {
	target, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer target.Close()
	go func() {
		buf := make([]byte, 2048)
		for {
			n, a, e := target.ReadFrom(buf)
			if e != nil {
				return
			}
			_, _ = target.WriteTo(buf[:n], a)
		}
	}()
	e := newTestEngine(t)
	port := freePort(t)
	applyTest(t, e, directConfig(port, "forward", target.LocalAddr().String()))
	c, err := net.Dial("udp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(3 * time.Second))
	_, _ = c.Write([]byte("udp"))
	buf := make([]byte, 32)
	n, err := c.Read(buf)
	if err != nil {
		t.Fatal(err)
	}
	if string(buf[:n]) != "udp" {
		t.Fatal(string(buf[:n]))
	}
	if e.udp.Load() != 1 {
		t.Fatalf("UDP sessions: %d", e.udp.Load())
	}
}
func TestSelectorStrategies(t *testing.T) {
	nodes := make([]*nodeRuntime, 3)
	for i := range nodes {
		nodes[i] = &nodeRuntime{key: fmt.Sprint(i)}
		nodes[i].tcp.Store(&HealthState{Status: "healthy", HealthySince: time.Now().Add(-time.Minute)})
	}
	p := &pool{nodes: nodes}
	excluded := map[*nodeRuntime]bool{}
	for i := 0; i < 12; i++ {
		if n := p.choose("round_robin", "", uint64(i), excluded); n != nodes[i%3] {
			t.Fatal("round robin order")
		}
	}
	nodes[0].active.Store(10)
	nodes[1].active.Store(5)
	if p.choose("least_conn", "", 0, excluded) != nodes[2] {
		t.Fatal("least connections")
	}
	if p.choose("failover", "", 0, map[*nodeRuntime]bool{nodes[0]: true}) != nodes[1] {
		t.Fatal("failover priority")
	}
	r := &route{entry: model.Entrance{Strategy: "ip_hash"}, nodes: nodes}
	for _, n := range nodes {
		n.udp.Store(&HealthState{Status: "healthy"})
	}
	r.rebuild()
	p = r.tcp.Load()
	chosen := p.choose("ip_hash", "source", 0, excluded)
	for i := 0; i < 100; i++ {
		if p.choose("ip_hash", "source", uint64(i), excluded) != chosen {
			t.Fatal("unstable hash")
		}
	}
	r.nodes = []*nodeRuntime{nodes[0], nodes[1]}
	r.rebuild()
	for i := 0; i < 1000; i++ {
		key := fmt.Sprint(i)
		old := p.choose("ip_hash", key, 0, excluded)
		next := r.tcp.Load().choose("ip_hash", key, 0, excluded)
		if old != nodes[2] && old != next {
			t.Fatal("hash removal remapped surviving node")
		}
	}
}
func TestAdmissionConcurrentAndNoFallback(t *testing.T) {
	e := newTestEngine(t)
	c := model.Default()
	c.Resources.MaxSessions = 10
	c.Resources.MaxUDP = 2
	applyTest(t, e, c)
	var wg sync.WaitGroup
	var releases sync.Map
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			done, err := e.admit(false)
			if err == nil {
				releases.Store(i, done)
			}
		}(i)
	}
	wg.Wait()
	if e.total.Load() != 10 {
		t.Fatalf("admitted %d", e.total.Load())
	}
	releases.Range(func(_, v any) bool { v.(func())(); v.(func())(); return true })
	if e.total.Load() != 0 {
		t.Fatal("release must be idempotent")
	}
	p := &pool{}
	if p.choose("round_robin", "", 0, nil) != nil {
		t.Fatal("empty pool fallback")
	}
}
