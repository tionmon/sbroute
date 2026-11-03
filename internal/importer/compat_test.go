package importer

import (
	"encoding/base64"
	"reflect"
	"testing"
)

func TestCommonProviderLinks(t *testing.T) {
	uuid := "11111111-1111-4111-8111-111111111111"
	ss := base64.RawURLEncoding.EncodeToString([]byte("aes-128-gcm:password"))
	cases := map[string]struct {
		link  string
		check func(map[string]any) bool
	}{
		"reality spx and headerType": {"vless://" + uuid + "@example.com:443?encryption=none&security=reality&sni=www.example.com&fp=chrome&pbk=key&sid=01&spx=%2F&type=tcp&headerType=none#R", func(m map[string]any) bool { return m["tls"] != nil }},
		"ws early data": {"vless://" + uuid + "@example.com:443?security=tls&type=ws&path=%2Fws%3Fed%3D2048&host=a.example.com#W", func(m map[string]any) bool {
			tr := m["transport"].(map[string]any)
			return tr["path"] == "/ws" && tr["max_early_data"] == 2048 && tr["early_data_header_name"] == "Sec-WebSocket-Protocol"
		}},
		"hysteria2 port hopping": {"hysteria2://pass@example.com:443?sni=a.example.com&mport=443,20000-30000#H", func(m map[string]any) bool {
			return reflect.DeepEqual(m["server_ports"], []string{"443:443", "20000:30000"})
		}},
		"ss obfs plugin": {"ss://" + ss + "@example.com:8388/?plugin=obfs-local%3Bobfs%3Dhttp%3Bobfs-host%3Da.example.com#P", func(m map[string]any) bool {
			return m["plugin"] == "obfs-local" && m["plugin_opts"] == "obfs=http;obfs-host=a.example.com"
		}},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			m, err := fromLink(c.link)
			if err != nil {
				t.Fatal(err)
			}
			if !c.check(m) {
				t.Fatalf("unexpected conversion: %+v", m)
			}
		})
	}
	for _, bad := range []string{
		"vless://" + uuid + "@example.com:443?security=tls&type=tcp&headerType=http",
		"vless://" + uuid + "@example.com:443?security=tls&mport=1000-2000",
		"hysteria2://pass@example.com:443?mport=3000-2000",
		"ss://" + ss + "@example.com:8388/?plugin=kcptun",
	} {
		if _, err := fromLink(bad); err == nil {
			t.Fatalf("accepted %s", bad)
		}
	}
}
func TestCommonClashFields(t *testing.T) {
	yaml := `proxies:
  - name: A
    type: ss
    server: example.com
    port: 443
    cipher: aes-128-gcm
    password: p
    udp: true
    tfo: false
    smux:
      enabled: false
    plugin: obfs
    plugin-opts:
      mode: tls
      host: a.example.com
  - name: B
    type: vmess
    server: example.com
    port: 443
    uuid: 11111111-1111-4111-8111-111111111111
    alterId: 0
    cipher: auto
    tls: true
    network: ws
    ws-opts:
      path: /ws?ed=2048
      headers:
        Host: a.example.com
  - name: C
    type: hysteria2
    server: example.com
    port: 443
    password: p
    ports: 20000-30000
  - name: D
    type: vless
    server: example.com
    port: 443
    uuid: 11111111-1111-4111-8111-111111111111
    tls: true
    network: ws
    ws-opts:
      path: /up
      v2ray-http-upgrade: true
  - name: E
    type: ss
    server: example.com
    port: 443
    cipher: aes-128-gcm
    password: p
    smux:
      enabled: true
`
	r := Parse([]byte(yaml), "", nil)
	if len(r.Nodes) != 4 || len(r.Issues) != 1 || r.Issues[0].Name != "E" {
		t.Fatalf("%+v", r)
	}
	byName := map[string]map[string]any{}
	for _, n := range r.Nodes {
		byName[n.Name] = n.Outbound
	}
	if byName["A"]["plugin_opts"] != "obfs-host=a.example.com;obfs=tls" {
		t.Fatal(byName["A"])
	}
	if tr := byName["B"]["transport"].(map[string]any); tr["path"] != "/ws" || tr["max_early_data"] != 2048 {
		t.Fatal(tr)
	}
	if !reflect.DeepEqual(byName["C"]["server_ports"], []string{"20000:30000"}) {
		t.Fatal(byName["C"])
	}
	if byName["D"]["transport"].(map[string]any)["type"] != "httpupgrade" {
		t.Fatal(byName["D"])
	}
}
