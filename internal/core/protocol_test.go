package core

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"github.com/sagernet/sing-box/adapter"
	"github.com/sagernet/sing-box/adapter/inbound"
	"github.com/sagernet/sing-box/option"
	"github.com/sagernet/sing-box/protocol/hysteria2"
	"github.com/sagernet/sing-box/protocol/shadowsocks"
	"github.com/sagernet/sing-box/protocol/tuic"
	"github.com/sagernet/sing-box/protocol/vless"
	J "github.com/sagernet/sing/common/json"
	"github.com/tionmon/sbroute/internal/model"
	"io"
	"math/big"
	"net"
	"testing"
	"time"
)

func testCertificate(t testing.TB) (string, string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "localhost"}, DNSNames: []string{"localhost"}, IPAddresses: []net.IP{net.ParseIP("127.0.0.1")}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	priv, _ := x509.MarshalECPrivateKey(key)
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})), string(pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: priv}))
}
func TestNativeProtocolOutbounds(t *testing.T) {
	cert, key := testCertificate(t)
	uuid := "11111111-1111-4111-8111-111111111111"
	for _, kind := range []string{"shadowsocks", "vless", "hysteria2", "tuic"} {
		t.Run(kind, func(t *testing.T) {
			target := echoTCP(t)
			udpTarget, err := net.ListenPacket("udp", target)
			if err != nil {
				t.Fatal(err)
			}
			defer udpTarget.Close()
			go func() {
				buffer := make([]byte, 2048)
				for {
					n, addr, err := udpTarget.ReadFrom(buffer)
					if err != nil {
						return
					}
					_, _ = udpTarget.WriteTo(buffer[:n], addr)
				}
			}()
			upstream := newTestEngine(t)
			applyTest(t, upstream, directConfig(freePort(t), "forward", target))
			ir := upstream.inRegistry.(*inbound.Registry)
			shadowsocks.RegisterInbound(ir)
			vless.RegisterInbound(ir)
			hysteria2.RegisterInbound(ir)
			tuic.RegisterInbound(ir)
			port := freePort(t)
			server := map[string]any{"type": kind, "tag": "in", "listen": "127.0.0.1", "listen_port": port}
			client := map[string]any{"type": kind, "server": "127.0.0.1", "server_port": port}
			if kind != "shadowsocks" {
				server["tls"] = map[string]any{"enabled": true, "certificate": []string{cert}, "key": []string{key}}
				client["tls"] = map[string]any{"enabled": true, "server_name": "localhost", "certificate": []string{cert}}
			}
			switch kind {
			case "shadowsocks":
				server["method"] = "aes-128-gcm"
				server["password"] = "test-password"
				client["method"] = "aes-128-gcm"
				client["password"] = "test-password"
			case "vless":
				server["users"] = []map[string]any{{"uuid": uuid}}
				client["uuid"] = uuid
			case "hysteria2":
				server["users"] = []map[string]any{{"password": "test-password"}}
				client["password"] = "test-password"
			case "tuic":
				server["users"] = []map[string]any{{"uuid": uuid, "password": "test-password"}}
				client["uuid"] = uuid
				client["password"] = "test-password"
				client["congestion_control"] = "cubic"
			}
			b, _ := json.Marshal(server)
			var opts option.Inbound
			if err := J.UnmarshalContext(upstream.coreCtx, b, &opts); err != nil {
				t.Fatal(err)
			}
			scope := adapter.NewScope(upstream.coreCtx, upstream.logger)
			defer scope.Close()
			in, err := ir.Create(scope.Context(), upstream.router, upstream.logger, "in", opts.Type, opts.Options)
			if err != nil {
				t.Fatal(err)
			}
			for _, stage := range adapter.ListStartStages {
				if err = in.Start(stage, scope); err != nil {
					t.Fatal(err)
				}
			}
			engine := newTestEngine(t)
			localPort := freePort(t)
			cfg := directConfig(localPort, "forward", target)
			cfg.Nodes = []model.Node{{ID: "proxy", Name: kind, Outbound: client}}
			cfg.Groups[0].Members = []string{"proxy"}
			applyTest(t, engine, cfg)
			conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", localPort), 5*time.Second)
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close()
			_ = conn.SetDeadline(time.Now().Add(8 * time.Second))
			payload := []byte("protocol-native-roundtrip")
			if _, err = conn.Write(payload); err != nil {
				t.Fatal(err)
			}
			out := make([]byte, len(payload))
			if _, err = io.ReadFull(conn, out); err != nil {
				t.Fatal(err)
			}
			if string(out) != string(payload) {
				t.Fatal("protocol payload changed")
			}
			udpConn, err := net.Dial("udp", fmt.Sprintf("127.0.0.1:%d", localPort))
			if err != nil {
				t.Fatal(err)
			}
			defer udpConn.Close()
			_ = udpConn.SetDeadline(time.Now().Add(8 * time.Second))
			_, err = udpConn.Write([]byte("udp-native"))
			if err != nil {
				t.Fatal(err)
			}
			buffer := make([]byte, 64)
			count, err := udpConn.Read(buffer)
			if err != nil || string(buffer[:count]) != "udp-native" {
				t.Fatalf("native UDP roundtrip: %q %v", buffer[:count], err)
			}
		})
	}
}
