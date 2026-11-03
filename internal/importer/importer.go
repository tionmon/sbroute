package importer

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/tionmon/sbroute/internal/model"
	"gopkg.in/yaml.v3"
)

type Issue struct {
	Item  int    `json:"item"`
	Name  string `json:"name,omitempty"`
	Error string `json:"error"`
}
type Result struct {
	Nodes      []model.Node `json:"nodes"`
	Issues     []Issue      `json:"issues"`
	Duplicates int          `json:"duplicates"`
}

func Decode64(s string) ([]byte, error) {
	s = strings.TrimSpace(s)
	for _, enc := range []*base64.Encoding{base64.RawURLEncoding, base64.URLEncoding, base64.RawStdEncoding, base64.StdEncoding} {
		if b, e := enc.DecodeString(s); e == nil {
			return b, nil
		}
	}
	return nil, errors.New("invalid base64")
}
func Parse(data []byte, source string, validate func(model.Node) error) Result {
	r := Result{Nodes: []model.Node{}, Issues: []Issue{}}
	if len(data) > model.MaxImportBytes {
		r.Issues = append(r.Issues, Issue{Error: "subscription exceeds 16MiB"})
		return r
	}
	text := strings.TrimSpace(strings.TrimPrefix(string(data), "\ufeff"))
	if text == "" {
		return r
	}
	var objects []map[string]any
	var doc map[string]any
	if json.Unmarshal([]byte(text), &objects) == nil { // standalone JSON node list
	} else if json.Unmarshal([]byte(text), &doc) == nil && doc != nil {
		if raw, ok := doc["outbounds"].([]any); ok {
			for _, item := range raw {
				if m, yes := item.(map[string]any); yes {
					objects = append(objects, m)
				} else {
					r.Issues = append(r.Issues, Issue{Error: "outbounds must contain objects"})
				}
			}
		} else if _, ok := doc["type"]; ok {
			objects = []map[string]any{doc}
		} else {
			r.Issues = append(r.Issues, Issue{Error: "JSON requires outbounds or an outbound object"})
			return r
		}
	} else if strings.Contains(text, "proxies:") {
		var clash struct {
			Proxies []map[string]any `yaml:"proxies"`
		}
		if err := yaml.Unmarshal([]byte(text), &clash); err != nil {
			r.Issues = append(r.Issues, Issue{Error: "invalid Clash YAML"})
			return r
		}
		for i, m := range clash.Proxies {
			out, err := fromClash(m)
			if err != nil {
				r.Issues = append(r.Issues, Issue{Item: i + 1, Name: str(m["name"]), Error: err.Error()})
				continue
			}
			objects = append(objects, out)
		}
	} else {
		if !strings.Contains(text, "://") {
			b, err := Decode64(text)
			if err != nil {
				r.Issues = append(r.Issues, Issue{Error: "not a supported subscription"})
				return r
			}
			text = string(b)
		}
		for i, line := range strings.Split(text, "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			m, err := fromLink(line)
			if err != nil {
				r.Issues = append(r.Issues, Issue{Item: i + 1, Error: err.Error()})
				continue
			}
			objects = append(objects, m)
		}
	}
	seen := map[string]bool{}
	for i, m := range objects {
		name := str(m["tag"])
		if name == "" {
			name = fmt.Sprintf("%s · %s", str(m["type"]), str(m["server"]))
		}
		delete(m, "tag")
		n := model.Node{ID: model.ID(), Name: name, SourceID: source, Outbound: m}
		if validate != nil {
			if err := validate(n); err != nil {
				r.Issues = append(r.Issues, Issue{Item: i + 1, Name: name, Error: err.Error()})
				continue
			}
		}
		fp := model.Fingerprint(n)
		if seen[fp] {
			r.Duplicates++
			continue
		}
		seen[fp] = true
		r.Nodes = append(r.Nodes, n)
	}
	return r
}
func str(v any) string {
	if v == nil {
		return ""
	}
	return fmt.Sprint(v)
}
func intVal(v any) int { n, _ := strconv.Atoi(str(v)); return n }
func yes(v any) bool   { s := str(v); return s == "true" || s == "1" }
func fromLink(link string) (map[string]any, error) {
	if strings.HasPrefix(link, "vmess://") {
		return vmessLink(strings.TrimPrefix(link, "vmess://"))
	}
	if strings.HasPrefix(link, "ss://") {
		return ssLink(link)
	}
	u, err := url.Parse(link)
	if err != nil || u.Hostname() == "" {
		return nil, errors.New("invalid node URL")
	}
	kind := strings.ToLower(u.Scheme)
	if kind == "hy2" {
		kind = "hysteria2"
	}
	if kind == "socks5" {
		kind = "socks"
	}
	if kind == "https" {
		kind = "http"
	}
	if !contains([]string{"vless", "trojan", "hysteria2", "tuic", "anytls", "http", "socks"}, kind) {
		return nil, fmt.Errorf("unsupported scheme %s", u.Scheme)
	}
	port := intVal(u.Port())
	if port == 0 {
		if kind == "http" && u.Scheme != "https" {
			port = 80
		} else if kind == "socks" {
			port = 1080
		} else {
			port = 443
		}
	}
	if port < 1 || port > 65535 {
		return nil, errors.New("invalid port")
	}
	m := map[string]any{"type": kind, "tag": u.Fragment, "server": u.Hostname(), "server_port": port}
	q := u.Query()
	allowed := []string{"security", "sni", "peer", "allowInsecure", "insecure", "alpn", "fp", "pbk", "sid", "type", "path", "host", "serviceName", "mode", "flow", "encryption", "packetEncoding", "obfs", "obfs-password", "upmbps", "downmbps", "congestion_control", "udp_relay_mode", "udp_over_stream", "heartbeat", "disable_sni"}
	for k := range q {
		if !contains(allowed, k) {
			return nil, fmt.Errorf("unsupported URL parameter %s", k)
		}
	}
	user := ""
	password := ""
	if u.User != nil {
		user = u.User.Username()
		password, _ = u.User.Password()
	}
	switch kind {
	case "vless":
		m["uuid"] = user
		if q.Get("encryption") != "" && q.Get("encryption") != "none" {
			return nil, errors.New("unsupported VLESS encryption")
		}
		if q.Get("flow") != "" {
			m["flow"] = q.Get("flow")
		}
		if q.Get("packetEncoding") != "" {
			m["packet_encoding"] = q.Get("packetEncoding")
		}
	case "trojan", "anytls":
		m["password"] = user
		if password != "" {
			return nil, errors.New("password must be URL-encoded as username")
		}
	case "hysteria2":
		m["password"] = user
		if password != "" {
			m["password"] = user + ":" + password
		}
		if q.Get("obfs") != "" {
			if q.Get("obfs") != "salamander" {
				return nil, errors.New("unsupported Hysteria2 obfs")
			}
			m["obfs"] = map[string]any{"type": "salamander", "password": q.Get("obfs-password")}
		}
		if q.Get("upmbps") != "" {
			m["up_mbps"] = intVal(q.Get("upmbps"))
		}
		if q.Get("downmbps") != "" {
			m["down_mbps"] = intVal(q.Get("downmbps"))
		}
	case "tuic":
		m["uuid"] = user
		m["password"] = password
		for _, key := range []string{"congestion_control", "udp_relay_mode", "heartbeat"} {
			if q.Get(key) != "" {
				m[key] = q.Get(key)
			}
		}
		if q.Get("udp_over_stream") != "" {
			m["udp_over_stream"] = yes(q.Get("udp_over_stream"))
		}
	default:
		if user != "" {
			m["username"] = user
			m["password"] = password
		}
		if kind == "socks" {
			m["version"] = "5"
		}
	}
	security := q.Get("security")
	if mode := q.Get("mode"); mode != "" && mode != "gun" {
		return nil, errors.New("non-default gRPC mode requires explicit conversion")
	}
	tlsEnabled := kind == "trojan" || kind == "anytls" || kind == "hysteria2" || kind == "tuic" || u.Scheme == "https" || security == "tls" || security == "reality"
	if security != "" && security != "none" && security != "tls" && security != "reality" {
		return nil, errors.New("unsupported TLS security")
	}
	if tlsEnabled {
		tls := map[string]any{"enabled": true}
		sni := q.Get("sni")
		if sni == "" {
			sni = q.Get("peer")
		}
		if sni != "" {
			tls["server_name"] = sni
		}
		if yes(q.Get("insecure")) || yes(q.Get("allowInsecure")) {
			tls["insecure"] = true
		}
		if a := q.Get("alpn"); a != "" {
			tls["alpn"] = strings.Split(a, ",")
		}
		if fp := q.Get("fp"); fp != "" {
			tls["utls"] = map[string]any{"enabled": true, "fingerprint": fp}
		}
		if security == "reality" {
			tls["reality"] = map[string]any{"enabled": true, "public_key": q.Get("pbk"), "short_id": q.Get("sid")}
			if tls["utls"] == nil {
				tls["utls"] = map[string]any{"enabled": true, "fingerprint": "chrome"}
			}
		}
		if yes(q.Get("disable_sni")) {
			tls["disable_sni"] = true
		}
		m["tls"] = tls
	}
	if err = applyTransport(m, q.Get("type"), q.Get("path"), q.Get("host"), q.Get("serviceName")); err != nil {
		return nil, err
	}
	if kind != "http" && kind != "socks" && user == "" {
		return nil, errors.New("node credentials required")
	}
	return m, nil
}
func contains(v []string, s string) bool {
	for _, x := range v {
		if s == x {
			return true
		}
	}
	return false
}
func applyTransport(m map[string]any, kind, path, host, service string) error {
	if kind == "" || kind == "tcp" {
		return nil
	}
	t := map[string]any{"type": kind}
	switch kind {
	case "ws", "httpupgrade":
		t["path"] = path
		if host != "" {
			t["headers"] = map[string]any{"Host": host}
		}
	case "grpc":
		t["service_name"] = service
	case "h2", "http":
		t["type"] = "http"
		t["path"] = path
		if host != "" {
			t["host"] = strings.Split(host, ",")
		}
	default:
		return fmt.Errorf("unsupported transport %s", kind)
	}
	m["transport"] = t
	return nil
}
func ssLink(link string) (map[string]any, error) {
	rest := strings.TrimPrefix(link, "ss://")
	if !strings.Contains(strings.Split(rest, "#")[0], "@") {
		parts := strings.SplitN(rest, "#", 2)
		b, err := Decode64(parts[0])
		if err != nil {
			return nil, errors.New("invalid SS base64")
		}
		rest = string(b)
		if len(parts) > 1 {
			rest += "#" + parts[1]
		}
	}
	u, err := url.Parse("ss://" + rest)
	if err != nil || u.User == nil {
		return nil, errors.New("invalid SS link")
	}
	if len(u.Query()) > 0 {
		return nil, errors.New("SS URL plugins/parameters require explicit sing-box JSON")
	}
	method := u.User.Username()
	password, has := u.User.Password()
	if !has {
		b, e := Decode64(method)
		if e != nil {
			return nil, errors.New("invalid SS credentials")
		}
		p := strings.SplitN(string(b), ":", 2)
		if len(p) != 2 {
			return nil, errors.New("invalid SS credentials")
		}
		method, password = p[0], p[1]
	}
	port := intVal(u.Port())
	if u.Hostname() == "" || port < 1 || port > 65535 {
		return nil, errors.New("invalid SS server")
	}
	return map[string]any{"type": "shadowsocks", "tag": u.Fragment, "server": u.Hostname(), "server_port": port, "method": method, "password": password}, nil
}
func vmessLink(encoded string) (map[string]any, error) {
	b, err := Decode64(encoded)
	if err != nil {
		return nil, err
	}
	var v map[string]any
	if json.Unmarshal(b, &v) != nil {
		return nil, errors.New("invalid VMess JSON")
	}
	allowed := []string{"v", "ps", "add", "port", "id", "aid", "scy", "net", "type", "host", "path", "tls", "sni", "alpn", "fp", "allowInsecure"}
	for k := range v {
		if !contains(allowed, k) {
			return nil, fmt.Errorf("unsupported VMess field %s", k)
		}
	}
	if t := str(v["type"]); t != "" && t != "none" {
		return nil, errors.New("VMess TCP/header obfuscation is not supported")
	}
	m := map[string]any{"type": "vmess", "tag": str(v["ps"]), "server": str(v["add"]), "server_port": intVal(v["port"]), "uuid": str(v["id"]), "alter_id": intVal(v["aid"]), "security": "auto"}
	if str(v["scy"]) != "" {
		m["security"] = v["scy"]
	}
	if str(v["tls"]) == "tls" {
		t := map[string]any{"enabled": true}
		if str(v["sni"]) != "" {
			t["server_name"] = v["sni"]
		} else if str(v["host"]) != "" {
			t["server_name"] = v["host"]
		}
		if yes(v["allowInsecure"]) {
			t["insecure"] = true
		}
		if str(v["alpn"]) != "" {
			t["alpn"] = strings.Split(str(v["alpn"]), ",")
		}
		if str(v["fp"]) != "" {
			t["utls"] = map[string]any{"enabled": true, "fingerprint": v["fp"]}
		}
		m["tls"] = t
	}
	err = applyTransport(m, str(v["net"]), str(v["path"]), str(v["host"]), str(v["path"]))
	return m, err
}
func fromClash(c map[string]any) (map[string]any, error) {
	kind := str(c["type"])
	if kind == "ss" {
		kind = "shadowsocks"
	}
	if kind == "socks5" {
		kind = "socks"
	}
	allowed := []string{"name", "type", "server", "port", "udp", "tls", "servername", "sni", "skip-cert-verify", "alpn", "client-fingerprint", "reality-opts", "network", "ws-opts", "grpc-opts", "http-opts", "h2-opts", "http-upgrade-opts", "uuid", "password", "username", "cipher", "alterId", "flow", "packet-encoding", "obfs", "obfs-password", "up", "down", "congestion-controller", "udp-relay-mode", "udp-over-stream", "heartbeat-interval", "ip-version"}
	for k := range c {
		if !contains(allowed, k) {
			return nil, fmt.Errorf("unsupported Clash field %s; use sing-box JSON for explicit conversion", k)
		}
	}
	if str(c["ip-version"]) != "" {
		return nil, errors.New("Clash ip-version override requires explicit conversion")
	}
	m := map[string]any{"type": kind, "tag": c["name"], "server": c["server"], "server_port": c["port"]}
	for _, key := range []string{"uuid", "password", "username", "flow"} {
		if v, ok := c[key]; ok {
			m[key] = v
		}
	}
	switch kind {
	case "shadowsocks":
		m["method"] = c["cipher"]
	case "vmess":
		m["security"] = c["cipher"]
		if str(m["security"]) == "" {
			m["security"] = "auto"
		}
		m["alter_id"] = intVal(c["alterId"])
	case "socks":
		m["version"] = "5"
	case "hysteria2":
		if c["obfs"] != nil {
			m["obfs"] = map[string]any{"type": c["obfs"], "password": c["obfs-password"]}
		}
		for _, v := range []string{"up", "down"} {
			if c[v] != nil {
				n, err := strconv.Atoi(str(c[v]))
				if err != nil {
					return nil, fmt.Errorf("Clash %s must be integer Mbps", v)
				}
				m[v+"_mbps"] = n
			}
		}
	case "tuic":
		for from, to := range map[string]string{"congestion-controller": "congestion_control", "udp-relay-mode": "udp_relay_mode", "udp-over-stream": "udp_over_stream", "heartbeat-interval": "heartbeat"} {
			if c[from] != nil {
				m[to] = c[from]
			}
		}
	}
	if c["packet-encoding"] != nil {
		m["packet_encoding"] = c["packet-encoding"]
	}
	if v, ok := c["udp"]; ok && !yes(v) && kind != "http" && kind != "anytls" {
		m["network"] = "tcp"
	}
	if yes(c["tls"]) || c["reality-opts"] != nil || contains([]string{"trojan", "hysteria2", "tuic", "anytls"}, kind) {
		t := map[string]any{"enabled": true}
		for _, k := range []string{"servername", "sni"} {
			if c[k] != nil {
				t["server_name"] = c[k]
			}
		}
		if c["skip-cert-verify"] != nil {
			t["insecure"] = yes(c["skip-cert-verify"])
		}
		if c["alpn"] != nil {
			t["alpn"] = c["alpn"]
		}
		if c["client-fingerprint"] != nil {
			t["utls"] = map[string]any{"enabled": true, "fingerprint": c["client-fingerprint"]}
		}
		if reality, ok := c["reality-opts"].(map[string]any); ok {
			for k := range reality {
				if k != "public-key" && k != "short-id" {
					return nil, fmt.Errorf("unsupported reality field %s", k)
				}
			}
			t["reality"] = map[string]any{"enabled": true, "public_key": reality["public-key"], "short_id": reality["short-id"]}
			if t["utls"] == nil {
				t["utls"] = map[string]any{"enabled": true, "fingerprint": "chrome"}
			}
		}
		m["tls"] = t
	}
	network := str(c["network"])
	if network != "" && network != "tcp" {
		t := map[string]any{"type": network}
		key := network + "-opts"
		if network == "httpupgrade" {
			key = "http-upgrade-opts"
		}
		if network == "h2" {
			t["type"] = "http"
		}
		opts, _ := c[key].(map[string]any)
		for k, v := range opts {
			switch k {
			case "path":
				t["path"] = v
			case "headers":
				t["headers"] = v
			case "host":
				t["host"] = v
			case "grpc-service-name":
				t["service_name"] = v
			default:
				return nil, fmt.Errorf("unsupported Clash transport field %s", k)
			}
		}
		m["transport"] = t
	}
	return m, nil
}

// Merge keeps IDs, names from the refreshed source, disabled flags, and group bindings stable.
func Merge(c model.Config, nodes []model.Node, source string, replace bool) (model.Config, int) {
	oldByFP := map[string]model.Node{}
	for _, n := range c.Nodes {
		if n.SourceID == source {
			oldByFP[model.Fingerprint(n)] = n
		}
	}
	keep := map[string]bool{}
	for i, n := range nodes {
		if old, ok := oldByFP[model.Fingerprint(n)]; ok {
			nodes[i].ID = old.ID
			nodes[i].Disabled = old.Disabled
		}
		keep[nodes[i].ID] = true
	}
	result := c.Clone()
	result.Nodes = nil
	removed := 0
	for _, old := range c.Nodes {
		if old.SourceID == source && replace {
			if !keep[old.ID] {
				removed++
			}
			continue
		}
		duplicate := false
		for _, n := range nodes {
			if old.ID == n.ID {
				duplicate = true
				break
			}
		}
		if !duplicate {
			result.Nodes = append(result.Nodes, old)
		}
	}
	result.Nodes = append(result.Nodes, nodes...)
	existing := map[string]bool{}
	for _, n := range result.Nodes {
		existing[n.ID] = true
	}
	for i := range result.Groups {
		result.Groups[i].Members = nil
		for _, id := range c.Groups[i].Members {
			if existing[id] {
				result.Groups[i].Members = append(result.Groups[i].Members, id)
			}
		}
	}
	return result, removed
}
