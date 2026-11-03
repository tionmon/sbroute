package model

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// Version and Commit are overridden at release build time with -ldflags "-X".
var (
	Version = "0.1.0-dev"
	Commit  = "unknown"
)

const CoreVersion = "1.14.4"
const MaxImportBytes = 16 << 20

type Config struct {
	Version       int64          `json:"version" yaml:"version"`
	Resources     Resources      `json:"resources" yaml:"resources"`
	Health        Health         `json:"health" yaml:"health"`
	Nodes         []Node         `json:"nodes" yaml:"nodes"`
	Subscriptions []Subscription `json:"subscriptions" yaml:"subscriptions"`
	Groups        []Group        `json:"groups" yaml:"groups"`
	Entrances     []Entrance     `json:"entrances" yaml:"entrances"`
}
type Resources struct {
	Preset           string `json:"preset" yaml:"preset"`
	MemoryMiB        int    `json:"memory_mib" yaml:"memory_mib"`
	MaxSessions      int    `json:"max_sessions" yaml:"max_sessions"`
	MaxUDP           int    `json:"max_udp" yaml:"max_udp"`
	MaxDials         int    `json:"max_dials" yaml:"max_dials"`
	ProbeConcurrency int    `json:"probe_concurrency" yaml:"probe_concurrency"`
	ProbeInterval    int    `json:"probe_interval_seconds" yaml:"probe_interval_seconds"`
	StatsInterval    int    `json:"stats_interval_seconds" yaml:"stats_interval_seconds"`
	UDPTimeout       int    `json:"udp_timeout_seconds" yaml:"udp_timeout_seconds"`
}
type Health struct {
	URLs              []string `json:"urls" yaml:"urls"`
	UDPDNS            string   `json:"udp_dns" yaml:"udp_dns"`
	Timeout           int      `json:"timeout_seconds" yaml:"timeout_seconds"`
	FailureThreshold  int      `json:"failure_threshold" yaml:"failure_threshold"`
	RecoveryThreshold int      `json:"recovery_threshold" yaml:"recovery_threshold"`
}
type Node struct {
	ID       string         `json:"id" yaml:"id"`
	Name     string         `json:"name" yaml:"name"`
	SourceID string         `json:"source_id,omitempty" yaml:"source_id,omitempty"`
	Disabled bool           `json:"disabled" yaml:"disabled"`
	Outbound map[string]any `json:"outbound" yaml:"outbound"`
}
type Subscription struct {
	ID       string `json:"id" yaml:"id"`
	Name     string `json:"name" yaml:"name"`
	URL      string `json:"url" yaml:"url"`
	Enabled  bool   `json:"enabled" yaml:"enabled"`
	Interval int    `json:"interval_seconds" yaml:"interval_seconds"`
	// UserAgent overrides the default download User-Agent; many providers pick the format by it.
	UserAgent string `json:"user_agent,omitempty" yaml:"user_agent,omitempty"`
	// Via downloads through this node instead of connecting directly.
	Via string `json:"via,omitempty" yaml:"via,omitempty"`
}
type Group struct {
	ID      string   `json:"id" yaml:"id"`
	Name    string   `json:"name" yaml:"name"`
	Members []string `json:"members" yaml:"members"`
	Sources []string `json:"sources" yaml:"sources"`
	Keyword string   `json:"keyword,omitempty" yaml:"keyword,omitempty"`
	Regex   string   `json:"regex,omitempty" yaml:"regex,omitempty"`
	Exclude string   `json:"exclude,omitempty" yaml:"exclude,omitempty"`
}
type User struct {
	Username string `json:"username" yaml:"username"`
	Password string `json:"password" yaml:"password"`
}
type Entrance struct {
	ID           string   `json:"id" yaml:"id"`
	Name         string   `json:"name" yaml:"name"`
	Enabled      bool     `json:"enabled" yaml:"enabled"`
	Listen       string   `json:"listen" yaml:"listen"`
	Port         uint16   `json:"port" yaml:"port"`
	Kind         string   `json:"kind" yaml:"kind"`
	Network      string   `json:"network" yaml:"network"`
	Target       string   `json:"target,omitempty" yaml:"target,omitempty"`
	GroupID      string   `json:"group_id" yaml:"group_id"`
	Strategy     string   `json:"strategy" yaml:"strategy"`
	Users        []User   `json:"users,omitempty" yaml:"users,omitempty"`
	AllowedCIDRs []string `json:"allowed_cidrs,omitempty" yaml:"allowed_cidrs,omitempty"`
}

func ID() string { b := make([]byte, 12); _, _ = rand.Read(b); return hex.EncodeToString(b) }
func Preset(name string) Resources {
	r := Resources{Preset: name, MemoryMiB: 512, MaxSessions: 8192, MaxUDP: 2048, MaxDials: 64, ProbeConcurrency: 4, ProbeInterval: 120, StatsInterval: 2, UDPTimeout: 60}
	switch name {
	case "low-memory":
		r.MaxSessions = 4096
		r.MaxUDP = 1024
		r.MaxDials = 16
		r.ProbeConcurrency = 2
		r.ProbeInterval = 180
		r.StatsInterval = 5
		r.UDPTimeout = 30
	case "throughput":
		r.MaxSessions = 16384
		r.MaxUDP = 4096
		r.MaxDials = 128
		r.ProbeConcurrency = 8
		r.ProbeInterval = 60
	}
	return r
}
func Default() Config {
	return Config{Resources: Preset("balanced"), Health: Health{URLs: []string{"https://cp.cloudflare.com/", "https://www.gstatic.com/generate_204"}, UDPDNS: "1.1.1.1:53", Timeout: 5, FailureThreshold: 3, RecoveryThreshold: 2}, Nodes: []Node{}, Groups: []Group{}, Entrances: []Entrance{}, Subscriptions: []Subscription{}}
}
func (c Config) Clone() Config {
	b, _ := json.Marshal(c)
	var v Config
	_ = json.Unmarshal(b, &v)
	return v
}
func (c *Config) Normalize() {
	if c.Nodes == nil {
		c.Nodes = []Node{}
	}
	if c.Groups == nil {
		c.Groups = []Group{}
	}
	if c.Subscriptions == nil {
		c.Subscriptions = []Subscription{}
	}
	if c.Entrances == nil {
		c.Entrances = []Entrance{}
	}
	for i := range c.Groups {
		if c.Groups[i].Members == nil {
			c.Groups[i].Members = []string{}
		}
		if c.Groups[i].Sources == nil {
			c.Groups[i].Sources = []string{}
		}
	}
}
func Fingerprint(n Node) string {
	m := make(map[string]any, len(n.Outbound))
	for k, v := range n.Outbound {
		if k != "tag" {
			m[k] = v
		}
	}
	b, _ := json.Marshal(m)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
func Members(c Config, g Group) []Node {
	var result []Node
	byID := map[string]Node{}
	for _, n := range c.Nodes {
		byID[n.ID] = n
	}
	seen := map[string]bool{}
	re, _ := regexp.Compile(g.Regex)
	exclude, _ := regexp.Compile(g.Exclude)
	add := func(n Node) {
		if n.Disabled || seen[n.ID] || (g.Exclude != "" && exclude != nil && exclude.MatchString(n.Name)) {
			return
		}
		seen[n.ID] = true
		result = append(result, n)
	}
	for _, id := range g.Members {
		if n, ok := byID[id]; ok {
			add(n)
		}
	}
	if len(g.Sources) > 0 || g.Keyword != "" || g.Regex != "" {
		for _, n := range c.Nodes {
			if len(g.Sources) > 0 && !slices.Contains(g.Sources, n.SourceID) {
				continue
			}
			if g.Keyword != "" && !strings.Contains(strings.ToLower(n.Name), strings.ToLower(g.Keyword)) {
				continue
			}
			if g.Regex != "" && !re.MatchString(n.Name) {
				continue
			}
			add(n)
		}
	}
	return result
}
func (c Config) Validate() error {
	r := c.Resources
	if !slices.Contains([]string{"balanced", "low-memory", "throughput"}, r.Preset) {
		return fmt.Errorf("resources.preset: unknown preset")
	}
	if r.MemoryMiB < 64 || r.MemoryMiB > 1048576 || r.MaxSessions < 1 || r.MaxSessions > 1000000 || r.MaxUDP < 0 || r.MaxUDP > r.MaxSessions || r.MaxDials < 1 || r.MaxDials > 4096 || r.ProbeConcurrency < 1 || r.ProbeConcurrency > 64 || r.ProbeInterval < 10 || r.StatsInterval < 1 || r.StatsInterval > 60 || r.UDPTimeout < 5 || r.UDPTimeout > 3600 {
		return fmt.Errorf("resources: invalid budget or limits")
	}
	if c.Health.Timeout < 1 || c.Health.Timeout > 30 || c.Health.FailureThreshold < 1 || c.Health.RecoveryThreshold < 1 || len(c.Health.URLs) == 0 || len(c.Health.URLs) > 3 {
		return fmt.Errorf("health: invalid thresholds, timeout, or URLs")
	}
	for _, v := range c.Health.URLs {
		u, e := url.Parse(v)
		if e != nil || u.Scheme != "https" || u.Host == "" || u.User != nil {
			return fmt.Errorf("health.urls: HTTPS URL required")
		}
	}
	if _, _, e := net.SplitHostPort(c.Health.UDPDNS); e != nil {
		return fmt.Errorf("health.udp_dns: host:port required")
	}
	ids := func(kind string, vs []string) error {
		seen := map[string]bool{}
		for _, id := range vs {
			if id == "" || len(id) > 128 || seen[id] {
				return fmt.Errorf("%s: empty or duplicate id %q", kind, id)
			}
			seen[id] = true
		}
		return nil
	}
	nodeIDs := []string{}
	subIDs := []string{}
	groupIDs := []string{}
	entryIDs := []string{}
	// Sets keep reference checks linear for large node lists.
	nodeSet := map[string]bool{}
	subSet := map[string]bool{}
	groupSet := map[string]bool{}
	for _, n := range c.Nodes {
		nodeIDs = append(nodeIDs, n.ID)
		nodeSet[n.ID] = true
		if n.Name == "" || len(n.Name) > 256 || n.Outbound == nil {
			return fmt.Errorf("node %s: name and outbound required", n.ID)
		}
	}
	for _, s := range c.Subscriptions {
		if s.Name == "" || len(s.Name) > 256 {
			return fmt.Errorf("subscription %s: name must be 1–256 bytes", s.ID)
		}
		subIDs = append(subIDs, s.ID)
		subSet[s.ID] = true
		u, e := url.Parse(s.URL)
		if e != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") || s.Interval < 60 {
			return fmt.Errorf("subscription %s: valid HTTP(S) URL and interval >=60 required", s.ID)
		}
		if len(s.UserAgent) > 256 || strings.ContainsFunc(s.UserAgent, func(r rune) bool { return r < 0x20 || r == 0x7f }) {
			return fmt.Errorf("subscription %s: user agent must be at most 256 printable bytes", s.ID)
		}
		if s.Via != "" && !nodeSet[s.Via] {
			return fmt.Errorf("subscription %s: download node %s missing", s.ID, s.Via)
		}
	}
	for _, g := range c.Groups {
		if g.Name == "" || len(g.Name) > 256 || len(g.Regex) > 4096 || len(g.Exclude) > 4096 {
			return fmt.Errorf("group %s: invalid name or regex too long", g.ID)
		}
		groupIDs = append(groupIDs, g.ID)
		groupSet[g.ID] = true
		for _, p := range []string{g.Regex, g.Exclude} {
			if _, e := regexp.Compile(p); e != nil {
				return fmt.Errorf("group %s: %w", g.ID, e)
			}
		}
		for _, id := range g.Members {
			if !nodeSet[id] {
				return fmt.Errorf("group %s: missing node %s", g.ID, id)
			}
		}
		for _, id := range g.Sources {
			if !subSet[id] {
				return fmt.Errorf("group %s: missing subscription %s", g.ID, id)
			}
		}
	}
	for _, e := range c.Entrances {
		if e.Name == "" || len(e.Name) > 256 {
			return fmt.Errorf("entrance %s: name must be 1–256 bytes", e.ID)
		}
		entryIDs = append(entryIDs, e.ID)
		if !groupSet[e.GroupID] {
			return fmt.Errorf("entrance %s: group missing", e.ID)
		}
		if _, err := netip.ParseAddr(e.Listen); err != nil || e.Port == 0 {
			return fmt.Errorf("entrance %s: listen must be an IP address and port must be nonzero", e.ID)
		}
		if e.Kind != "mixed" && e.Kind != "forward" {
			return fmt.Errorf("entrance %s: kind must be mixed or forward", e.ID)
		}
		if !slices.Contains([]string{"tcp", "udp", "tcp,udp"}, e.Network) {
			return fmt.Errorf("entrance %s: invalid network", e.ID)
		}
		if e.Kind == "forward" {
			host, p, err := net.SplitHostPort(e.Target)
			if err != nil || host == "" || p == "0" || p == "" {
				return fmt.Errorf("entrance %s: target must be host:port", e.ID)
			}
			if port, err := strconv.ParseUint(p, 10, 16); err != nil || port == 0 {
				return fmt.Errorf("entrance %s: invalid target port", e.ID)
			}
		}
		if !slices.Contains([]string{"random", "round_robin", "ip_hash", "least_conn", "failover", "p2c"}, e.Strategy) {
			return fmt.Errorf("entrance %s: invalid strategy", e.ID)
		}
		names := map[string]bool{}
		for _, u := range e.Users {
			if u.Username == "" || u.Password == "" || names[u.Username] {
				return fmt.Errorf("entrance %s: invalid or duplicate user", e.ID)
			}
			names[u.Username] = true
		}
		for _, p := range e.AllowedCIDRs {
			if _, err := netip.ParsePrefix(p); err != nil {
				return fmt.Errorf("entrance %s: invalid CIDR %s", e.ID, p)
			}
		}
	}
	for kind, vs := range map[string][]string{"nodes": nodeIDs, "subscriptions": subIDs, "groups": groupIDs, "entrances": entryIDs} {
		if e := ids(kind, vs); e != nil {
			return e
		}
	}
	if len(c.Nodes) > 10000 || len(c.Entrances) > 1000 || len(c.Groups) > 1000 || len(c.Subscriptions) > 128 {
		return fmt.Errorf("maximum 10000 nodes / 1000 entrances exceeded")
	}
	return nil
}

// Redacted configurations can be round-tripped: missing secrets are restored by ID.
func (c Config) Redacted() Config {
	v := c.Clone()
	for i := range v.Nodes {
		o := v.Nodes[i].Outbound
		v.Nodes[i].Outbound = map[string]any{"type": o["type"], "server": o["server"], "server_port": o["server_port"], "_redacted": true}
	}
	for i := range v.Subscriptions {
		v.Subscriptions[i].URL = "<redacted>"
	}
	for i := range v.Entrances {
		for j := range v.Entrances[i].Users {
			v.Entrances[i].Users[j].Password = "<redacted>"
		}
	}
	return v
}
func (c *Config) RestoreSecrets(old Config) {
	for i, n := range c.Nodes {
		if n.Outbound["_redacted"] == true {
			for _, o := range old.Nodes {
				if n.ID == o.ID {
					c.Nodes[i].Outbound = o.Outbound
				}
			}
		}
	}
	for i, s := range c.Subscriptions {
		if s.URL == "<redacted>" {
			for _, o := range old.Subscriptions {
				if s.ID == o.ID {
					c.Subscriptions[i].URL = o.URL
				}
			}
		}
	}
	for i := range c.Entrances {
		for j, u := range c.Entrances[i].Users {
			if u.Password != "<redacted>" {
				continue
			}
			for _, e := range old.Entrances {
				if e.ID == c.Entrances[i].ID {
					for _, ou := range e.Users {
						if u.Username == ou.Username {
							c.Entrances[i].Users[j].Password = ou.Password
						}
					}
				}
			}
		}
	}
}
