package core

import (
	"fmt"
	"io"
	"net"
	"testing"
	"time"
)

func TestEmptyGroupNeverConnectsDirectly(t *testing.T) {
	target, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer target.Close()
	connected := make(chan struct{}, 1)
	go func() {
		c, e := target.Accept()
		if e != nil {
			return
		}
		connected <- struct{}{}
		_, _ = c.Write([]byte("BYPASS"))
		c.Close()
	}()
	e := newTestEngine(t)
	port := freePort(t)
	cfg := directConfig(port, "forward", target.Addr().String())
	cfg.Groups[0].Members = nil
	applyTest(t, e, cfg)
	c, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	c.SetDeadline(time.Now().Add(2 * time.Second))
	_, _ = c.Write([]byte("test"))
	buf := make([]byte, 16)
	n, _ := c.Read(buf)
	if n != 0 {
		t.Fatal("empty group forwarded traffic")
	}
	select {
	case <-connected:
		t.Fatal("empty group connected directly to the target")
	default:
	}
}
func TestFailedListenerPreparationKeepsOldPort(t *testing.T) {
	target := echoTCP(t)
	e := newTestEngine(t)
	port := freePort(t)
	cfg := directConfig(port, "forward", target)
	applyTest(t, e, cfg)
	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer occupied.Close()
	next := e.Config()
	next.Entrances[0].Port = uint16(occupied.Addr().(*net.TCPAddr).Port)
	if commit, rollback, err := e.Prepare(next); err == nil {
		rollback()
		_ = commit
		t.Fatal("occupied port accepted")
	}
	if e.Config().Entrances[0].Port != port {
		t.Fatal("failed apply changed old configuration")
	}
	c, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	c.SetDeadline(time.Now().Add(2 * time.Second))
	_, _ = c.Write([]byte("ok"))
	b := make([]byte, 2)
	if _, err = io.ReadFull(c, b); err != nil || string(b) != "ok" {
		t.Fatalf("old listener stopped working: %v", err)
	}
}
