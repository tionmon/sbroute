package importer

import (
	"encoding/base64"
	"fmt"
	"github.com/tionmon/sbroute/internal/model"
	"testing"
)

func TestImportFormats(t *testing.T) {
	uuid := "11111111-1111-4111-8111-111111111111"
	vmess := base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf(`{"v":"2","ps":"VM","add":"example.com","port":"443","id":%q,"aid":"0","net":"ws","type":"none","host":"example.com","path":"/ws","tls":"tls"}`, uuid)))
	ss := base64.RawURLEncoding.EncodeToString([]byte("aes-128-gcm:password"))
	cases := map[string]string{"links": "vless://" + uuid + "@example.com:443?security=tls&type=ws&path=%2Fws&sni=example.com#JP\nss://" + ss + "@example.com:8388#SS", "base64": base64.StdEncoding.EncodeToString([]byte("socks5://user:password@example.com:1080#SOCKS")), "vmess": "vmess://" + vmess, "sing-box": `{"outbounds":[{"type":"socks","tag":"S","server":"example.com","server_port":1080}]}`, "clash": "proxies:\n  - name: C\n    type: vless\n    server: example.com\n    port: 443\n    uuid: " + uuid + "\n    tls: true\n    network: ws\n    ws-opts:\n      path: /ws\n"}
	for name, input := range cases {
		t.Run(name, func(t *testing.T) {
			r := Parse([]byte(input), "s", nil)
			if len(r.Issues) != 0 || len(r.Nodes) == 0 {
				t.Fatalf("%+v", r)
			}
			for _, n := range r.Nodes {
				if n.SourceID != "s" || n.Name == "" {
					t.Fatal(n)
				}
			}
		})
	}
}
func TestRejectLossyConversion(t *testing.T) {
	for _, input := range []string{"vless://uuid@example.com:443?security=tls&unknown=1", "proxies:\n - name: bad\n   type: ss\n   server: example.com\n   port: 443\n   plugin: obfs\n"} {
		r := Parse([]byte(input), "", nil)
		if len(r.Nodes) != 0 || len(r.Issues) == 0 {
			t.Fatalf("silently accepted unsupported parameters: %+v", r)
		}
	}
}
func TestStableMergeAndDedup(t *testing.T) {
	r := Parse([]byte("socks5://example.com:1080#A\nsocks5://example.com:1080#B"), "source", nil)
	if len(r.Nodes) != 1 || r.Duplicates != 1 {
		t.Fatal(r)
	}
	c := model.Default()
	old := r.Nodes[0]
	old.Disabled = true
	c.Nodes = []model.Node{old}
	c.Groups = []model.Group{{ID: "g", Name: "G", Members: []string{old.ID}}}
	r.Nodes[0].ID = model.ID()
	r.Nodes[0].Name = "Renamed"
	next, removed := Merge(c, r.Nodes, "source", true)
	if removed != 0 || next.Nodes[0].ID != old.ID || !next.Nodes[0].Disabled || next.Nodes[0].Name != "Renamed" || next.Groups[0].Members[0] != old.ID {
		t.Fatalf("unstable refresh: %+v", next)
	}
	next, removed = Merge(next, nil, "source", true)
	if removed != 1 || len(next.Groups[0].Members) != 0 {
		t.Fatal("deleted members were not pruned")
	}
}
func FuzzParse(f *testing.F) {
	for _, s := range []string{"", `{"outbounds":[]}`, "ss://a", "proxies: []", "[null]", "vmess://e30="} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		if len(s) > 65536 {
			t.Skip()
		}
		_ = Parse([]byte(s), "", nil)
	})
}
