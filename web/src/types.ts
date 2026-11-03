export type NodeDef = {
  id: string;
  name: string;
  source_id?: string;
  disabled: boolean;
  outbound: Record<string, unknown>;
};
export type Group = {
  id: string;
  name: string;
  members: string[];
  sources: string[];
  keyword?: string;
  regex?: string;
  exclude?: string;
};
export type Entry = {
  id: string;
  name: string;
  enabled: boolean;
  listen: string;
  port: number;
  kind: string;
  network: string;
  target?: string;
  group_id: string;
  strategy: string;
  users?: { username: string; password: string }[];
  allowed_cidrs?: string[];
};
export type Subscription = {
  id: string;
  name: string;
  url: string;
  enabled: boolean;
  interval_seconds: number;
  user_agent?: string;
  via?: string;
};
export type Resources = {
  preset: string;
  memory_mib: number;
  max_sessions: number;
  max_udp: number;
  max_dials: number;
  probe_concurrency: number;
  probe_interval_seconds: number;
  stats_interval_seconds: number;
  udp_timeout_seconds: number;
};
export type Config = {
  version: number;
  nodes: NodeDef[];
  groups: Group[];
  entrances: Entry[];
  subscriptions: Subscription[];
  resources: Resources;
  health: {
    urls: string[];
    udp_dns: string;
    timeout_seconds: number;
    failure_threshold: number;
    recovery_threshold: number;
  };
};
export type Health = {
  status: string;
  latency_ms: number;
  error?: string;
  last_check?: string;
};
export type NodeStat = {
  id: string;
  tcp: Health;
  udp: Health;
  active: number;
  upload: number;
  download: number;
  initialized: boolean;
  pending: number;
  exit_ip?: string;
};
export type EntryStat = {
  id: string;
  healthy_tcp: number;
  healthy_udp: number;
  active: number;
  upload: number;
  download: number;
};
export type Stats = {
  version: string;
  core_version: string;
  active: number;
  udp: number;
  pending: number;
  upload: number;
  download: number;
  rejected: number;
  rss_bytes: number;
  heap_bytes: number;
  goroutines: number;
  memory_pressure: boolean;
  uptime_seconds: number;
  nodes: NodeStat[];
  entrances: EntryStat[];
  draining_versions: number;
  fd_count?: number;
  fd_limit?: number;
  cgroup_used_bytes?: number;
  cgroup_limit_bytes?: number;
};
export type Event = {
  id: number;
  created: string;
  kind: string;
  message: string;
};
export type SubStatus = {
  id: string;
  error?: string;
  pending: boolean;
  pending_count: number;
  removed: number;
  count: number;
  last_check: string;
  next_check: string;
  skipped?: number;
  usage?: Usage;
  issues: { item: number; name?: string; error: string }[];
};
export type Usage = {
  upload: number;
  download: number;
  total: number;
  expire?: number;
};
export type Revision = { version: number; created: string; state: string };
export type ImportPreview = {
  preview_id: string;
  nodes: NodeDef[];
  duplicates: number;
  issues: { item: number; name?: string; error: string }[];
};
