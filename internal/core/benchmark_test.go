package core

import (
	"context"
	"encoding/json"
	"fmt"
	box "github.com/sagernet/sing-box"
	"github.com/sagernet/sing-box/option"
	J "github.com/sagernet/sing/common/json"
	"github.com/tionmon/sbroute/internal/model"
	"io"
	"net"
	"os"
	"runtime"
	"strconv"
	"testing"
	"time"
)

func nativeRelay(t testing.TB, target string) (string, func()) {
	t.Helper()
	e := &Engine{ctx: context.Background()}
	ctx := newRegistries(e)
	port := freePort(t)
	host, p, _ := net.SplitHostPort(target)
	destPort, _ := strconv.Atoi(p)
	raw := map[string]any{"log": map[string]any{"level": "error"}, "dns": map[string]any{"servers": []map[string]any{{"type": "local", "tag": "local"}}, "final": "local"}, "route": map[string]any{"final": "plain", "default_domain_resolver": "local"}, "inbounds": []map[string]any{{"type": "direct", "tag": "native", "listen": "127.0.0.1", "listen_port": port, "override_address": host, "override_port": destPort}}, "outbounds": []map[string]any{{"type": "direct", "tag": "plain"}}}
	b, _ := json.Marshal(raw)
	var opts option.Options
	if err := J.UnmarshalContext(ctx, b, &opts); err != nil {
		t.Fatal(err)
	}
	core, err := box.New(box.Options{Context: ctx, Options: opts})
	if err != nil {
		t.Fatal(err)
	}
	if err = core.Start(); err != nil {
		t.Fatal(err)
	}
	return fmt.Sprintf("127.0.0.1:%d", port), func() { core.Close() }
}
func BenchmarkRelay(b *testing.B) {
	for _, mode := range []string{"native", "round_robin", "random", "p2c", "ip_hash", "least_conn"} {
		b.Run(mode, func(b *testing.B) {
			target := echoTCP(b)
			var addr string
			if mode == "native" {
				var close func()
				addr, close = nativeRelay(b, target)
				defer close()
			} else {
				e := newTestEngine(b)
				port := freePort(b)
				c := directConfig(port, "forward", target)
				c.Entrances[0].Strategy = mode
				applyTest(b, e, c)
				addr = fmt.Sprintf("127.0.0.1:%d", port)
			}
			conn, err := net.Dial("tcp", addr)
			if err != nil {
				b.Fatal(err)
			}
			defer conn.Close()
			buf := make([]byte, 32<<10)
			b.SetBytes(int64(len(buf)))
			b.ReportAllocs()
			b.ResetTimer()
			cpuStart := processCPUSeconds()
			for i := 0; i < b.N; i++ {
				if _, err = conn.Write(buf); err != nil {
					b.Fatal(err)
				}
				if _, err = io.ReadFull(conn, buf); err != nil {
					b.Fatal(err)
				}
			}
			b.StopTimer()
			if cpuEnd := processCPUSeconds(); cpuEnd > cpuStart {
				b.ReportMetric((cpuEnd-cpuStart)/(float64(b.N*len(buf))/(1<<30)), "cpu-s/GiB")
			}
		})
	}
}
func BenchmarkSelect300Nodes(b *testing.B) {
	nodes := make([]*nodeRuntime, 300)
	for i := range nodes {
		nodes[i] = &nodeRuntime{key: fmt.Sprint(i)}
		nodes[i].tcp.Store(&HealthState{Status: "healthy", HealthySince: time.Now().Add(-time.Hour)})
		nodes[i].udp.Store(&HealthState{Status: "healthy"})
	}
	for _, strategy := range []string{"round_robin", "random", "p2c", "ip_hash", "least_conn"} {
		b.Run(strategy, func(b *testing.B) {
			r := &route{nodes: nodes, entry: model.Entrance{Strategy: strategy}}
			r.rebuild()
			p := r.tcp.Load()
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_ = p.choose(strategy, "stable-source-ip", uint64(i), nil)
			}
		})
	}
}

// Opt-in real sockets. Run this without -race for representative resource measurements.
func TestScale(t *testing.T) {
	if os.Getenv("SBROUTE_STRESS") != "1" {
		t.Skip("set SBROUTE_STRESS=1 for socket load test")
	}
	target := echoTCP(t)
	e := newTestEngine(t)
	port := freePort(t)
	cfg := directConfig(port, "forward", target)
	cfg.Resources.MaxSessions = 12000
	cfg.Resources.MemoryMiB = 1536
	cfg.Resources.MaxDials = 128
	for i := 0; i < 299; i++ {
		cfg.Nodes = append(cfg.Nodes, model.Node{ID: fmt.Sprintf("unused%d", i), Name: fmt.Sprintf("Unused %d", i), Outbound: map[string]any{"type": "socks", "server": "127.0.0.1", "server_port": 20000 + i}})
	}
	for i := 0; i < 19; i++ {
		entry := cfg.Entrances[0]
		entry.ID = fmt.Sprintf("extra%d", i)
		entry.Port = freePort(t)
		cfg.Entrances = append(cfg.Entrances, entry)
	}
	applyTest(t, e, cfg)
	time.Sleep(1100 * time.Millisecond)
	t.Logf("idle_300_nodes_20_ports rss=%d", residentMemory())
	connections := []net.Conn{}
	defer func() {
		for _, c := range connections {
			c.Close()
		}
	}()
	for _, limit := range []int{1000, 5000, 10000} {
		for len(connections) < limit {
			conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 3*time.Second)
			if err != nil {
				t.Fatal(err)
			}
			_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
			_, err = conn.Write([]byte{1})
			if err == nil {
				_, err = io.ReadFull(conn, make([]byte, 1))
			}
			if err != nil {
				conn.Close()
				t.Fatalf("connection %d failed: %v", len(connections), err)
			}
			_ = conn.SetDeadline(time.Time{})
			connections = append(connections, conn)
		}
		time.Sleep(time.Second)
		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		t.Logf("connections=%d rss=%d heap=%d goroutines=%d", len(connections), residentMemory(), m.HeapAlloc, runtime.NumGoroutine())
	}
	hold := os.Getenv("SBROUTE_HOLD")
	duration := 30 * time.Second
	if hold != "" {
		var err error
		duration, err = time.ParseDuration(hold)
		if err != nil {
			t.Fatal(err)
		}
	}
	deadline := time.Now().Add(duration)
	for time.Now().Before(deadline) {
		for i := 0; i < len(connections); i += 1000 {
			c := connections[i]
			_ = c.SetDeadline(time.Now().Add(3 * time.Second))
			if _, err := c.Write([]byte{2}); err != nil {
				t.Fatal(err)
			}
			if _, err := io.ReadFull(c, make([]byte, 1)); err != nil {
				t.Fatal(err)
			}
			_ = c.SetDeadline(time.Time{})
		}
		time.Sleep(time.Second)
	}
	t.Logf("hold_complete duration=%s rss=%d", duration, residentMemory())
	for _, c := range connections {
		c.Close()
	}
	connections = nil
	time.Sleep(2 * time.Second)
	if e.total.Load() != 0 {
		t.Fatalf("sessions leaked: %d", e.total.Load())
	}
}
