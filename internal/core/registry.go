package core

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	box "github.com/sagernet/sing-box"
	"github.com/sagernet/sing-box/adapter"
	"github.com/sagernet/sing-box/adapter/certificate"
	"github.com/sagernet/sing-box/adapter/endpoint"
	"github.com/sagernet/sing-box/adapter/inbound"
	"github.com/sagernet/sing-box/adapter/outbound"
	coreService "github.com/sagernet/sing-box/adapter/service"
	"github.com/sagernet/sing-box/dns"
	"github.com/sagernet/sing-box/dns/transport/local"
	"github.com/sagernet/sing-box/log"
	"github.com/sagernet/sing-box/option"
	"github.com/sagernet/sing-box/protocol/anytls"
	"github.com/sagernet/sing-box/protocol/direct"
	"github.com/sagernet/sing-box/protocol/http"
	"github.com/sagernet/sing-box/protocol/hysteria2"
	"github.com/sagernet/sing-box/protocol/mixed"
	"github.com/sagernet/sing-box/protocol/shadowsocks"
	"github.com/sagernet/sing-box/protocol/socks"
	"github.com/sagernet/sing-box/protocol/trojan"
	"github.com/sagernet/sing-box/protocol/tuic"
	"github.com/sagernet/sing-box/protocol/vless"
	"github.com/sagernet/sing-box/protocol/vmess"
	J "github.com/sagernet/sing/common/json"
	"github.com/tionmon/sbroute/internal/model"
)

func newRegistries(e *Engine) context.Context {
	ir := inbound.NewRegistry()
	mixed.RegisterInbound(ir)
	direct.RegisterInbound(ir)
	or := outbound.NewRegistry()
	socks.RegisterOutbound(or)
	http.RegisterOutbound(or)
	shadowsocks.RegisterOutbound(or)
	vmess.RegisterOutbound(or)
	vless.RegisterOutbound(or)
	trojan.RegisterOutbound(or)
	hysteria2.RegisterOutbound(or)
	tuic.RegisterOutbound(or)
	anytls.RegisterOutbound(or)
	// Direct is useful for local integration tests, but is never a fallback.
	direct.RegisterOutbound(or)
	outbound.Register[struct{}](or, "sbroute", func(ctx context.Context, r adapter.Router, l log.ContextLogger, tag string, _ struct{}) (adapter.Outbound, error) {
		e.coreCtx = ctx
		e.router = r
		e.logger = l
		return &dispatcher{engine: e}, nil
	})
	dr := dns.NewTransportRegistry()
	local.RegisterTransport(dr)
	e.inRegistry = ir
	e.outRegistry = or
	return box.Context(e.ctx, ir, or, endpoint.NewRegistry(), dr, coreService.NewRegistry(), certificate.NewRegistry())
}
func (e *Engine) decodeNode(ctx context.Context, n model.Node) (option.Outbound, error) {
	var o option.Outbound
	if err := independentFields(n.Outbound); err != nil {
		return o, err
	}
	if err := pluginFields(n.Outbound); err != nil {
		return o, err
	}
	if !slices.Contains([]string{"socks", "http", "shadowsocks", "vmess", "vless", "trojan", "hysteria2", "tuic", "anytls", "direct"}, fmt.Sprint(n.Outbound["type"])) {
		return o, fmt.Errorf("node %s: unsupported independent outbound type", n.Name)
	}
	for _, field := range []string{"detour", "domain_resolver", "bind_interface", "netns", "routing_mark", "inet4_bind_address", "inet6_bind_address"} {
		if v, ok := n.Outbound[field]; ok && v != nil && v != "" {
			return o, fmt.Errorf("node %s: field %s is not supported in a managed independent node", n.Name, field)
		}
	}
	b, err := json.Marshal(n.Outbound)
	if err != nil {
		return o, err
	}
	err = J.UnmarshalContext(ctx, b, &o)
	o.Tag = model.Fingerprint(n)[:16]
	return o, err
}
func independentFields(m map[string]any) error {
	for k, v := range m {
		if slices.Contains([]string{"certificate_path", "client_certificate_path", "client_key_path", "key_path", "config_path"}, k) && v != nil && v != "" {
			return fmt.Errorf("managed nodes require inline credentials; %s is not supported", k)
		}
		if nested, ok := v.(map[string]any); ok {
			if err := independentFields(nested); err != nil {
				return err
			}
		}
	}
	return nil
}

// pluginFields admits only the core's in-process SIP003 plugins, without certificate file paths.
func pluginFields(m map[string]any) error {
	plugin, pluginOK := m["plugin"].(string)
	opts, optsOK := m["plugin_opts"].(string)
	if (m["plugin"] != nil && !pluginOK) || (m["plugin_opts"] != nil && !optsOK) {
		return fmt.Errorf("plugin and plugin_opts must be strings")
	}
	if plugin == "" {
		if opts != "" {
			return fmt.Errorf("plugin_opts requires plugin")
		}
		return nil
	}
	if m["type"] != "shadowsocks" || (plugin != "obfs-local" && plugin != "v2ray-plugin") {
		return fmt.Errorf("only built-in obfs-local and v2ray-plugin are supported for Shadowsocks")
	}
	for _, part := range strings.Split(opts, ";") {
		if key, _, _ := strings.Cut(part, "="); strings.TrimSpace(key) == "cert" {
			return fmt.Errorf("managed nodes require inline credentials; plugin cert path is not supported")
		}
	}
	return nil
}
func (e *Engine) Validate(c model.Config) error {
	if err := c.Validate(); err != nil {
		return err
	}
	for _, n := range c.Nodes {
		o, err := e.decodeNode(e.coreCtx, n)
		if err != nil {
			return fmt.Errorf("node %s: %w", n.ID, err)
		}
		if o.Type != "direct" {
			server, ok := n.Outbound["server"].(string)
			port := fmt.Sprint(n.Outbound["server_port"])
			if !ok || server == "" || port == "<nil>" || port == "0" {
				return fmt.Errorf("node %s: server and server_port are required", n.Name)
			}
		}
		scope := adapter.NewScope(e.coreCtx, e.logger)
		_, err = e.outRegistry.CreateOutbound(scope.Context(), e.router, e.logger, o.Tag, o.Type, o.Options)
		_ = scope.Close()
		if err != nil {
			return fmt.Errorf("node %s: %w", n.ID, err)
		}
	}
	return nil
}
