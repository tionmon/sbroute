package core

import (
	"fmt"
	"io"
	"net"
	"testing"
	"time"
)

func TestDomainAndIPv6Targets(t *testing.T) {
	for _, network := range []string{"domain", "ipv6"} {
		t.Run(network, func(t *testing.T) {
			bind := "127.0.0.1:0"
			if network == "ipv6" {
				bind = "[::1]:0"
			}
			target, err := net.Listen("tcp", bind)
			if err != nil {
				if network == "ipv6" {
					t.Skip("IPv6 not available")
				}
				t.Fatal(err)
			}
			defer target.Close()
			go func() {
				for {
					c, err := target.Accept()
					if err != nil {
						return
					}
					go func() { defer c.Close(); _, _ = io.Copy(c, c) }()
				}
			}()
			dest := target.Addr().String()
			if network == "domain" {
				dest = fmt.Sprintf("localhost:%d", target.Addr().(*net.TCPAddr).Port)
			}
			e := newTestEngine(t)
			port := freePort(t)
			cfg := directConfig(port, "forward", dest)
			if network == "ipv6" {
				cfg.Entrances[0].Listen = "::1"
				cfg.Entrances[0].Strategy = "ip_hash"
				cfg.Entrances[0].AllowedCIDRs = []string{"::1/128"}
			}
			applyTest(t, e, cfg)
			addr := net.JoinHostPort(cfg.Entrances[0].Listen, fmt.Sprint(port))
			c, err := net.DialTimeout("tcp", addr, 3*time.Second)
			if err != nil {
				t.Fatal(err)
			}
			defer c.Close()
			c.SetDeadline(time.Now().Add(3 * time.Second))
			_, _ = c.Write([]byte("address"))
			b := make([]byte, 7)
			if _, err = io.ReadFull(c, b); err != nil || string(b) != "address" {
				t.Fatalf("address routing failed: %v", err)
			}
		})
	}
}
