package core

import (
	"bufio"
	"encoding/base64"
	"fmt"
	"github.com/tionmon/sbroute/internal/model"
	"io"
	"net"
	"net/http"
	"testing"
	"time"
)

func TestLiveConnectionsSurvivePolicyUpdatesAndRuntimeIsBounded(t *testing.T) {
	e := newTestEngine(t)
	target := echoTCP(t)
	port := freePort(t)
	cfg := directConfig(port, "forward", target)
	applyTest(t, e, cfg)
	conn, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	roundtrip := func() {
		t.Helper()
		_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
		if _, err := conn.Write([]byte("ok")); err != nil {
			t.Fatal(err)
		}
		b := make([]byte, 2)
		if _, err := io.ReadFull(conn, b); err != nil {
			t.Fatal(err)
		}
	}
	roundtrip()
	for i := 0; i < 100; i++ {
		cfg.Entrances[0].Strategy = []string{"round_robin", "ip_hash", "p2c"}[i%3]
		applyTest(t, e, cfg)
	}
	roundtrip()
	if e.Stats().Entrances[0].Active != 1 {
		t.Fatal("entrance lost active statistics on update")
	}
	conn.Close()
	for i := 0; i < 100; i++ {
		cfg.Nodes[0].Outbound = map[string]any{"type": "socks", "server": "127.0.0.1", "server_port": 30000 + i}
		applyTest(t, e, cfg)
	}
	e.mu.Lock()
	count := len(e.nodes)
	e.mu.Unlock()
	if count > 3 {
		t.Fatalf("retained %d node versions after 100 updates", count)
	}
}
func TestHTTPConnectAuthenticationAndAdmission(t *testing.T) {
	e := newTestEngine(t)
	target := echoTCP(t)
	port := freePort(t)
	cfg := directConfig(port, "mixed", target)
	cfg.Entrances[0].Users = []model.User{{Username: "user", Password: "test-password"}}
	applyTest(t, e, cfg)
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(3 * time.Second))
	auth := base64.StdEncoding.EncodeToString([]byte("user:test-password"))
	fmt.Fprintf(conn, "CONNECT %s HTTP/1.1\r\nHost: %s\r\nProxy-Authorization: Basic %s\r\n\r\n", target, target, auth)
	reader := bufio.NewReader(conn)
	response, err := http.ReadResponse(reader, &http.Request{Method: "CONNECT"})
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != 200 {
		t.Fatalf("CONNECT status %d", response.StatusCode)
	}
	conn.Write([]byte("hello"))
	data := make([]byte, 5)
	if _, err = io.ReadFull(reader, data); err != nil || string(data) != "hello" {
		t.Fatalf("CONNECT failed: %s %v", data, err)
	}
	bad, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer bad.Close()
	bad.SetDeadline(time.Now().Add(3 * time.Second))
	fmt.Fprintf(bad, "CONNECT %s HTTP/1.1\r\nHost: %s\r\n\r\n", target, target)
	res, err := http.ReadResponse(bufio.NewReader(bad), &http.Request{Method: "CONNECT"})
	if err != nil || res.StatusCode != 407 {
		t.Fatalf("unauthenticated proxy accepted: %v", err)
	}
}
func TestHashDistributionAndSharedPools(t *testing.T) {
	e := newTestEngine(t)
	cfg := directConfig(freePort(t), "forward", "127.0.0.1:9")
	other := cfg.Entrances[0]
	other.ID = "second"
	other.Port = freePort(t)
	cfg.Entrances = append(cfg.Entrances, other)
	applyTest(t, e, cfg)
	e.mu.Lock()
	e.rebuildPools(e.snap.Load())
	e.mu.Unlock()
	first := e.snap.Load().routes["in"].tcp.Load()
	second := e.snap.Load().routes["second"].tcp.Load()
	if first != second {
		t.Fatal("same group did not share pool")
	}
	nodes := []*nodeRuntime{}
	for i := 0; i < 3; i++ {
		n := &nodeRuntime{key: fmt.Sprintf("node-%d", i)}
		n.tcp.Store(&HealthState{Status: "healthy"})
		n.udp.Store(&HealthState{Status: "healthy"})
		nodes = append(nodes, n)
	}
	r := &route{entry: model.Entrance{Strategy: "ip_hash"}, nodes: nodes}
	r.rebuild()
	counts := map[*nodeRuntime]int{}
	for i := 0; i < 10000; i++ {
		counts[r.tcp.Load().choose("ip_hash", fmt.Sprint(i), 0, nil)]++
	}
	for _, n := range nodes {
		if counts[n] < 1800 || counts[n] > 5000 {
			t.Fatalf("poor hash distribution: %+v", counts)
		}
	}
}
