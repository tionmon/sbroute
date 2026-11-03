package core

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"testing"
	"time"
)

func echoUDP(t testing.TB) net.PacketConn {
	t.Helper()
	conn, e := net.ListenPacket("udp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { conn.Close() })
	go func() {
		buf := make([]byte, 8192)
		for {
			n, a, e := conn.ReadFrom(buf)
			if e != nil {
				return
			}
			_, _ = conn.WriteTo(buf[:n], a)
		}
	}()
	return conn
}
func TestSocksUDPAssociationStableAcrossTargets(t *testing.T) {
	e := newTestEngine(t)
	port := freePort(t)
	c := directConfig(port, "mixed", "")
	applyTest(t, e, c)
	targetA := echoUDP(t)
	targetB := echoUDP(t)
	control, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		t.Fatal(err)
	}
	defer control.Close()
	_ = control.SetDeadline(time.Now().Add(5 * time.Second))
	_, _ = control.Write([]byte{5, 1, 0})
	greet := make([]byte, 2)
	if _, err = io.ReadFull(control, greet); err != nil {
		t.Fatal(err)
	}
	_, _ = control.Write([]byte{5, 3, 0, 1, 0, 0, 0, 0, 0, 0})
	response := make([]byte, 10)
	if _, err = io.ReadFull(control, response); err != nil {
		t.Fatal(err)
	}
	if response[1] != 0 || response[3] != 1 {
		t.Fatalf("UDP bind failed: %v", response)
	}
	relay := &net.UDPAddr{IP: net.IP(response[4:8]), Port: int(binary.BigEndian.Uint16(response[8:10]))}
	udp, err := net.DialUDP("udp", nil, relay)
	if err != nil {
		t.Fatal(err)
	}
	defer udp.Close()
	_ = udp.SetDeadline(time.Now().Add(5 * time.Second))
	for i, target := range []net.PacketConn{targetA, targetB} {
		if i == 1 {
			time.Sleep(11 * time.Second)
		}
		_ = udp.SetDeadline(time.Now().Add(5 * time.Second))
		dest := target.LocalAddr().(*net.UDPAddr)
		packet := []byte{0, 0, 0, 1, 127, 0, 0, 1, 0, 0}
		binary.BigEndian.PutUint16(packet[8:], uint16(dest.Port))
		payload := []byte(fmt.Sprintf("message%d", i))
		packet = append(packet, payload...)
		if _, err = udp.Write(packet); err != nil {
			t.Fatal(err)
		}
		buf := make([]byte, 1024)
		n, err := udp.Read(buf)
		if err != nil {
			t.Fatal(err)
		}
		if n < 10 || string(buf[10:n]) != string(payload) {
			t.Fatalf("unexpected UDP response %x", buf[:n])
		}
	}
	if e.udp.Load() != 1 {
		t.Fatalf("association split into %d sessions", e.udp.Load())
	}
	control.Close()
	for i := 0; i < 100 && e.udp.Load() != 0; i++ {
		time.Sleep(10 * time.Millisecond)
	}
	if e.udp.Load() != 0 {
		t.Fatal("UDP association not released after control connection closed")
	}
	if e.up.Load() != 16 || e.down.Load() != 16 {
		t.Fatalf("UDP traffic up=%d down=%d", e.up.Load(), e.down.Load())
	}
}
