import React, { useEffect, useMemo, useState } from "react";
import { createRoot } from "react-dom/client";
import {
  Activity,
  ArrowDown,
  ArrowDownUp,
  ArrowRight,
  ArrowUp,
  Check,
  ChevronLeft,
  ChevronRight,
  CircleHelp,
  Database,
  Download,
  GitBranch,
  Globe2,
  Layers,
  LayoutDashboard,
  LogOut,
  MoreHorizontal,
  Network,
  Plus,
  Radio,
  RefreshCw,
  Search,
  Server,
  Settings2,
  ShieldCheck,
  SlidersHorizontal,
  Terminal,
  Trash2,
  Upload,
  X,
  Zap,
} from "lucide-react";
import "./style.css";

type NodeDef = {
  id: string;
  name: string;
  source_id?: string;
  disabled: boolean;
  outbound: Record<string, unknown>;
};
type Group = {
  id: string;
  name: string;
  members: string[];
  sources: string[];
  keyword?: string;
  regex?: string;
  exclude?: string;
};
type Entry = {
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
type Subscription = {
  id: string;
  name: string;
  url: string;
  enabled: boolean;
  interval_seconds: number;
};
type Resources = {
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
type Config = {
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
type Health = {
  status: string;
  latency_ms: number;
  error?: string;
  last_check?: string;
};
type NodeStat = {
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
type EntryStat = {
  id: string;
  healthy_tcp: number;
  healthy_udp: number;
  active: number;
  upload: number;
  download: number;
};
type Stats = {
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
type Event = { id: number; created: string; kind: string; message: string };
type SubStatus = {
  id: string;
  error?: string;
  pending: boolean;
  pending_count: number;
  removed: number;
  count: number;
  last_check: string;
  next_check: string;
  issues: { item: number; name?: string; error: string }[];
};
type ImportPreview = {
  preview_id: string;
  nodes: NodeDef[];
  duplicates: number;
  issues: { item: number; name?: string; error: string }[];
};
const strategies: Record<string, string> = {
  round_robin: "轮询",
  random: "随机",
  ip_hash: "IP 哈希",
  least_conn: "最小连接数",
  failover: "故障转移",
  p2c: "双随机择少 · P2C",
};
const nav = [
  { key: "overview", label: "运行概览", icon: LayoutDashboard },
  { key: "entrances", label: "分发入口", icon: Network },
  { key: "nodes", label: "出口节点", icon: Globe2 },
  { key: "groups", label: "节点分组", icon: Layers },
  { key: "subscriptions", label: "订阅来源", icon: RefreshCw },
  { key: "events", label: "事件日志", icon: Terminal },
  { key: "settings", label: "系统设置", icon: Settings2 },
];
const id = () => crypto.randomUUID().replaceAll("-", "");
const clone = <T,>(value: T): T => structuredClone(value);
const bytes = (n = 0) => {
  if (n < 1024) return `${n.toFixed(0)} B`;
  const u = Math.min(3, Math.floor(Math.log(n) / Math.log(1024)));
  return `${(n / 1024 ** u).toFixed(u > 1 ? 1 : 0)} ${["B", "KiB", "MiB", "GiB"][u]}`;
};
const date = (s?: string) =>
  s && !s.startsWith("0001")
    ? new Date(s).toLocaleString("zh-CN", { hour12: false })
    : "尚未检查";
async function api<T>(
  path: string,
  method = "GET",
  body?: unknown,
): Promise<T> {
  const res = await fetch(`/api/v1${path}`, {
    method,
    credentials: "same-origin",
    headers: body === undefined ? {} : { "Content-Type": "application/json" },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  const value = await res.json();
  if (!res.ok) throw new Error(value.error || `HTTP ${res.status}`);
  return value as T;
}
function Badge({ health, label }: { health?: Health; label?: string }) {
  const s = health?.status || "inactive";
  return (
    <span className={`badge ${s}`} title={health?.error}>
      <i />
      {label ||
        {
          healthy: `${health?.latency_ms ?? 0} ms`,
          unhealthy: "不可用",
          unknown: "待探测",
          inactive: "未启用",
          disabled: "已禁用",
          unsupported: "不支持",
        }[s] ||
        s}
    </span>
  );
}
function Empty({
  title,
  text,
  action,
}: {
  title: string;
  text: string;
  action?: React.ReactNode;
}) {
  return (
    <div className="empty">
      <div className="empty-icon">
        <GitBranch size={30} />
      </div>
      <h3>{title}</h3>
      <p>{text}</p>
      {action}
    </div>
  );
}
function Modal({
  title,
  children,
  close,
  wide = false,
}: {
  title: string;
  children: React.ReactNode;
  close: () => void;
  wide?: boolean;
}) {
  useEffect(() => {
    const f = (e: KeyboardEvent) => {
      if (e.key === "Escape") close();
    };
    document.addEventListener("keydown", f);
    return () => document.removeEventListener("keydown", f);
  }, [close]);
  return (
    <div
      className="modal-shade"
      onMouseDown={(e) => {
        if (e.target === e.currentTarget) close();
      }}
    >
      <section
        className={`modal ${wide ? "wide" : ""}`}
        role="dialog"
        aria-modal="true"
        aria-label={title}
      >
        <header>
          <h2>{title}</h2>
          <button className="icon-button" onClick={close} aria-label="关闭">
            <X size={20} />
          </button>
        </header>
        {children}
      </section>
    </div>
  );
}
function Field({
  label,
  hint,
  children,
}: {
  label: string;
  hint?: string;
  children: React.ReactNode;
}) {
  return (
    <label className="field">
      <span>{label}</span>
      {children}
      {hint && <small>{hint}</small>}
    </label>
  );
}
function Sparkline({ points }: { points: number[] }) {
  const max = Math.max(1, ...points);
  const line = points
    .map(
      (n, i) =>
        `${(i * 100) / Math.max(1, points.length - 1)},${31 - (n / max) * 26}`,
    )
    .join(" ");
  return (
    <svg viewBox="0 0 100 34" preserveAspectRatio="none" className="sparkline">
      <polyline
        points={`0,34 ${line} 100,34`}
        fill="rgba(88,215,173,.08)"
        stroke="none"
      />
      <polyline
        points={line}
        fill="none"
        stroke="currentColor"
        strokeWidth="1.3"
        vectorEffect="non-scaling-stroke"
      />
    </svg>
  );
}

function App() {
  const [authenticated, setAuthenticated] = useState<boolean | null>(null);
  const [page, setPage] = useState("overview");
  const [cfg, setCfg] = useState<Config>();
  const [stats, setStats] = useState<Stats>();
  const [events, setEvents] = useState<Event[]>([]);
  const [subStatus, setSubStatus] = useState<SubStatus[]>([]);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [busy, setBusy] = useState(false);
  const [modal, setModal] = useState<{ kind: string; value?: unknown }>();
  const [search, setSearch] = useState("");
  const [nodePage, setNodePage] = useState(0);
  const [points, setPoints] = useState<number[]>([]);
  const [importContent, setImportContent] = useState("");
  const [preview, setPreview] = useState<ImportPreview>();
  const [presets, setPresets] = useState<Resources[]>([]);
  const [revisions, setRevisions] = useState<
    { version: number; created: string; state: string }[]
  >([]);
  const nodes = useMemo(
    () =>
      (cfg?.nodes || []).filter((n) =>
        `${n.name} ${n.outbound.type} ${n.outbound.server}`
          .toLowerCase()
          .includes(search.toLowerCase()),
      ),
    [cfg, search],
  );
  const visibleNodes = nodes.slice(nodePage * 100, (nodePage + 1) * 100);
  const tell = (message: string) => {
    setNotice(message);
    window.setTimeout(() => setNotice(""), 4000);
  };
  async function load() {
    const c = await api<Config>("/config");
    setCfg(c);
    const [ss, ev, pr, rv] = await Promise.all([
      api<SubStatus[]>("/subscriptions/status"),
      api<Event[]>("/events"),
      api<Resources[]>("/presets"),
      api<{ version: number; created: string; state: string }[]>("/revisions"),
    ]);
    setSubStatus(ss);
    setEvents(ev);
    setPresets(pr);
    setRevisions(rv);
  }
  async function refreshStats() {
    const s = await api<Stats>(
      `/stats${page === "nodes" ? `?nodes=1&ids=${visibleNodes.map((n) => n.id).join(",")}` : ""}`,
    );
    setStats(s);
    setPoints((p) => [...p.slice(-59), s.active]);
  }
  async function action(fn: () => Promise<void>) {
    if (busy) return;
    setBusy(true);
    setError("");
    try {
      await fn();
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  async function save(next: Config) {
    let keptUDP = false;
    if (
      cfg &&
      next.resources.preset !== cfg.resources.preset &&
      cfg.entrances.some((e) => e.enabled)
    ) {
      next.resources.udp_timeout_seconds = cfg.resources.udp_timeout_seconds;
      keptUDP = true;
    }
    const c = await api<Config>("/config", "PUT", next);
    setCfg(c);
    setModal(undefined);
    await load();
    tell(
      `配置 v${c.version} 已生效${keptUDP ? "，已保留运行入口的 UDP 超时" : ""}`,
    );
  }
  useEffect(() => {
    api("/session")
      .then(() => setAuthenticated(true))
      .catch(() => setAuthenticated(false));
  }, []);
  useEffect(() => {
    if (authenticated) load().catch((e) => setError(e.message));
  }, [authenticated]);
  useEffect(() => {
    if (!authenticated || !cfg) return;
    let stopped = false;
    const tick = () => {
      if (!stopped && document.visibilityState === "visible")
        refreshStats().catch((e) => setError(e.message));
    };
    tick();
    const timer = window.setInterval(
      tick,
      cfg.resources.stats_interval_seconds * 1000,
    );
    return () => {
      stopped = true;
      clearInterval(timer);
    };
  }, [authenticated, cfg?.version, page, nodePage, search]);
  useEffect(() => {
    if (page !== "events" || !authenticated) return;
    const stream = new EventSource("/api/v1/events/stream");
    stream.onmessage = (e) => {
      const item: Event = JSON.parse(e.data);
      setEvents((old) =>
        [item, ...old.filter((v) => v.id !== item.id)].slice(0, 100),
      );
    };
    return () => stream.close();
  }, [page, authenticated]);
  function changePage(key: string) {
    setPage(key);
    setSearch("");
    setNodePage(0);
    setError("");
  }
  if (authenticated === null)
    return (
      <div className="boot">
        <GitBranch size={32} />
        <span>正在连接调度台…</span>
      </div>
    );
  if (!authenticated)
    return (
      <div className="login-screen">
        <div className="login-decor">
          <div className="brand large">
            <span>
              <GitBranch />
            </span>
            SBRoute
          </div>
          <h1>
            每一次连接，
            <br />
            都有合适的出口。
          </h1>
          <p>轻量运行 · 灵活分组 · 清晰可控</p>
          <div className="login-line">
            <i />
            <i />
            <i />
            <i />
            <i />
          </div>
        </div>
        <form
          className="login-card"
          onSubmit={(e) => {
            e.preventDefault();
            const f = new FormData(e.currentTarget);
            action(async () => {
              await api("/login", "POST", {
                username: f.get("username"),
                password: f.get("password"),
              });
              setAuthenticated(true);
            });
          }}
        >
          <span className="eyebrow">WELCOME BACK</span>
          <h2>登录控制台</h2>
          <p>管理你的节点、入口和分发策略。</p>
          <Field label="管理员账号">
            <input
              name="username"
              autoComplete="username"
              required
              defaultValue="admin"
            />
          </Field>
          <Field label="密码">
            <input
              name="password"
              type="password"
              autoComplete="current-password"
              required
            />
          </Field>
          {error && <div className="alert error">{error}</div>}
          <button className="button primary full" disabled={busy}>
            {busy ? "正在验证…" : "进入控制台"}
            <ArrowRight size={17} />
          </button>
          <small className="login-help">
            首次使用请在服务器运行 <code>sbroute admin</code> 创建管理员。
          </small>
        </form>
        <span className="login-footer">SBRoute / powered by sing-box</span>
      </div>
    );
  const current = nav.find((n) => n.key === page)!;
  const openImport = () => {
    setImportContent("");
    setPreview(undefined);
    setModal({ kind: "import" });
  };
  const remove = async (
    type: "nodes" | "groups" | "entrances" | "subscriptions",
    itemID: string,
  ) => {
    if (
      !confirm("确认删除？已有连接会继续排空；引用此对象的配置也需要同步处理。")
    )
      return;
    const next = clone(cfg!);
    (next[type] as { id: string }[]) = next[type].filter(
      (v) => v.id !== itemID,
    );
    if (type === "nodes")
      next.groups.forEach(
        (g) => (g.members = (g.members || []).filter((x) => x !== itemID)),
      );
    await save(next);
  };
  return (
    <div className="app">
      <aside className="sidebar">
        <a
          className="brand"
          href="#"
          onClick={(e) => {
            e.preventDefault();
            changePage("overview");
          }}
        >
          <span>
            <GitBranch size={23} />
          </span>
          SBRoute<small>CONSOLE</small>
        </a>
        <div className="workspace">
          <i className="live-dot" />
          <div>
            本机调度实例<small>Linux · 单机运行</small>
          </div>
          <Server size={16} />
        </div>
        <span className="nav-label">工作空间</span>
        <nav>
          {nav.map((item) => (
            <button
              key={item.key}
              className={page === item.key ? "selected" : ""}
              onClick={() => changePage(item.key)}
            >
              <item.icon size={19} />
              {item.label}
              {item.key === "nodes" && <em>{cfg?.nodes.length || 0}</em>}
            </button>
          ))}
        </nav>
        <div className="sidebar-bottom">
          <div className="core-info">
            <ShieldCheck size={17} />
            <div>
              sing-box 核心
              <small>v{stats?.core_version || "1.14.4"} · 原生转发</small>
            </div>
            <i className="live-dot" />
          </div>
          <button
            onClick={() =>
              action(async () => {
                await api("/logout", "POST", {});
                setAuthenticated(false);
                setCfg(undefined);
              })
            }
          >
            <LogOut size={17} />
            退出登录
          </button>
        </div>
      </aside>
      <main>
        <div className="topbar">
          <div>
            工作空间 <ChevronRight size={14} />
            <strong>{current.label}</strong>
          </div>
          <div>
            <span
              className={`system-status ${stats?.memory_pressure ? "warning" : ""}`}
            >
              <i />
              {stats?.memory_pressure ? "资源保护中" : "服务运行中"}
            </span>
            <span className="avatar">A</span>
          </div>
        </div>
        <div className="content">
          <header className="page-heading">
            <div>
              <span className="eyebrow">
                {page === "overview"
                  ? "TRAFFIC ORCHESTRATION"
                  : "WORKSPACE / " + page.toUpperCase()}
              </span>
              <h1>{current.label}</h1>
              <p>
                {
                  {
                    overview: "所有连接与出口状态，一目了然。",
                    entrances: "将一个端口，连接到一组可靠的出口。",
                    nodes: "集中管理节点，了解每条线路的可用性。",
                    groups: "让不同来源的节点，按照你的方式组合。",
                    subscriptions: "自动获取节点变化，保留可用的上一版本。",
                    events: "追踪配置变化、健康检查与运行事件。",
                    settings: "为这台机器选择合适的资源与运行参数。",
                  }[page]
                }
              </p>
            </div>
            <div className="heading-actions">
              {page === "overview" ? (
                <>
                  <button
                    className="button"
                    onClick={() => action(load)}
                    disabled={busy}
                  >
                    <RefreshCw size={16} />
                    刷新
                  </button>
                  <button
                    className="button primary"
                    onClick={() => changePage("entrances")}
                  >
                    <Plus size={16} />
                    配置入口
                  </button>
                </>
              ) : page === "nodes" ? (
                <>
                  <button
                    className="button"
                    onClick={() => setModal({ kind: "node" })}
                  >
                    <Plus size={16} />
                    手动添加
                  </button>
                  <button className="button primary" onClick={openImport}>
                    <Upload size={16} />
                    导入节点
                  </button>
                </>
              ) : ["entrances", "groups", "subscriptions"].includes(page) ? (
                <button
                  className="button primary"
                  onClick={() => setModal({ kind: page.slice(0, -1) })}
                >
                  <Plus size={16} />
                  新建
                  {page === "entrances"
                    ? "入口"
                    : page === "groups"
                      ? "分组"
                      : "订阅"}
                </button>
              ) : null}
            </div>
          </header>
          {error && (
            <div className="alert error">
              <span>{error}</span>
              <button onClick={() => setError("")} aria-label="关闭错误">
                <X size={16} />
              </button>
            </div>
          )}
          {notice && (
            <div className="toast">
              <Check size={16} />
              {notice}
            </div>
          )}
          {stats && (
            <div className="runtime-bar">
              <span>
                <Server size={12} /> FD {stats.fd_count || 0} /{" "}
                {stats.fd_limit || "—"}
              </span>
              <span>{stats.goroutines} goroutines</span>
              {!!stats.cgroup_limit_bytes && (
                <span>
                  cgroup {bytes(stats.cgroup_used_bytes)} /{" "}
                  {bytes(stats.cgroup_limit_bytes)}
                </span>
              )}
            </div>
          )}
          {!cfg ? (
            <Empty title="正在加载配置" text="请稍候…" />
          ) : (
            <>
              {page === "overview" && (
                <>
                  <section className="metrics">
                    <Metric
                      label="活动连接"
                      value={String(stats?.active || 0)}
                      sub={`${stats?.udp || 0} 个 UDP 会话`}
                      icon={<Activity size={18} />}
                    >
                      <Sparkline points={points} />
                    </Metric>
                    <Metric
                      label="累计传输"
                      value={bytes(
                        (stats?.upload || 0) + (stats?.download || 0),
                      )}
                      sub={`↑ ${bytes(stats?.upload)}　↓ ${bytes(stats?.download)}`}
                      icon={<ArrowDownUp size={18} />}
                    />
                    <Metric
                      label="分发入口"
                      value={String(
                        cfg.entrances.filter((e) => e.enabled).length,
                      )}
                      sub={`${cfg.entrances.length} 个入口 · ${cfg.groups.length} 个分组`}
                      icon={<Network size={18} />}
                    />
                    <Metric
                      label="内存占用"
                      value={bytes(stats?.rss_bytes)}
                      sub={`${cfg.resources.memory_mib} MiB 预算 · ${cfg.resources.preset === "balanced" ? "均衡模式" : cfg.resources.preset === "low-memory" ? "省内存模式" : "高吞吐模式"}`}
                      icon={<Server size={18} />}
                    />
                  </section>
                  <div className="overview-grid">
                    <section className="panel">
                      <div className="panel-heading">
                        <div>
                          <h2>分发路径</h2>
                          <p>每条新连接选择一次出口，连接期间保持稳定。</p>
                        </div>
                        <button
                          className="text-button"
                          onClick={() => changePage("entrances")}
                        >
                          管理入口 <ArrowRight size={15} />
                        </button>
                      </div>
                      {cfg.entrances.length ? (
                        <div className="route-list">
                          {cfg.entrances.slice(0, 5).map((e) => {
                            const es = stats?.entrances.find(
                              (s) => s.id === e.id,
                            );
                            return (
                              <div className="route-row" key={e.id}>
                                <div className="route-source">
                                  <span
                                    className={`route-icon ${e.enabled ? "" : "muted"}`}
                                  >
                                    <Network size={19} />
                                  </span>
                                  <div>
                                    <strong>{e.name}</strong>
                                    <code>
                                      {e.listen}:{e.port}
                                    </code>
                                  </div>
                                </div>
                                <span className="route-connector">
                                  <i />
                                  <ArrowRight size={15} />
                                </span>
                                <div className="route-group">
                                  <Layers size={15} />
                                  <strong>
                                    {cfg.groups.find((g) => g.id === e.group_id)
                                      ?.name || "未绑定"}
                                  </strong>
                                  <small>{strategies[e.strategy]}</small>
                                </div>
                                <span className="route-connector">
                                  <i />
                                  <ArrowRight size={15} />
                                </span>
                                <div className="route-destination">
                                  <Globe2 size={17} />
                                  <strong>{es?.healthy_tcp || 0}</strong>
                                  <small>可用 TCP 出口</small>
                                </div>
                              </div>
                            );
                          })}
                        </div>
                      ) : (
                        <Empty
                          title="建立第一条分发路径"
                          text="先导入节点并建立分组，再为入口端口选择一组出口。"
                          action={
                            <button
                              className="button primary"
                              onClick={openImport}
                            >
                              <Plus size={16} />
                              导入节点
                            </button>
                          }
                        />
                      )}
                    </section>
                    <section className="panel side-panel">
                      <div className="panel-heading">
                        <div>
                          <h2>运行状态</h2>
                          <p>资源有界，连接可控</p>
                        </div>
                        <Radio size={17} className="green" />
                      </div>
                      <div className="system-list">
                        <SystemRow
                          name="运行时间"
                          value={`${Math.floor((stats?.uptime_seconds || 0) / 3600)}h ${Math.floor((stats?.uptime_seconds || 0) / 60) % 60}m`}
                        />
                        <SystemRow
                          name="待建立连接"
                          value={`${stats?.pending || 0} / ${cfg.resources.max_dials}`}
                        />
                        <SystemRow
                          name="连接容量"
                          value={`${stats?.active || 0} / ${cfg.resources.max_sessions}`}
                        />
                        <SystemRow
                          name="Go 堆内存"
                          value={bytes(stats?.heap_bytes)}
                        />
                        <SystemRow
                          name="排空中的节点版本"
                          value={String(stats?.draining_versions || 0)}
                        />
                        <SystemRow
                          name="资源保护拒绝"
                          value={String(stats?.rejected || 0)}
                        />
                      </div>
                      <div className="info-note">
                        <Zap size={17} />
                        <p>
                          节点越多不等于占用越高。未使用的节点按需初始化，重复节点共享运行资源。
                        </p>
                      </div>
                    </section>
                  </div>
                  <section className="panel">
                    <div className="panel-heading">
                      <div>
                        <h2>最近事件</h2>
                        <p>保留最近的状态变化与操作记录。</p>
                      </div>
                      <button
                        className="text-button"
                        onClick={() => changePage("events")}
                      >
                        查看全部 <ArrowRight size={15} />
                      </button>
                    </div>
                    <EventList events={events.slice(0, 5)} />
                  </section>
                </>
              )}
              {page === "entrances" && (
                <section className="panel">
                  <div className="table-toolbar">
                    <span>{cfg.entrances.length} 个入口</span>
                    <span className="subtle">
                      <CircleHelp size={14} />{" "}
                      所有节点不可用时拒绝连接，不自动直连
                    </span>
                  </div>
                  {cfg.entrances.length ? (
                    <div className="table-scroll">
                      <table>
                        <thead>
                          <tr>
                            <th>入口 / 协议</th>
                            <th>监听地址</th>
                            <th>节点组</th>
                            <th>分发策略</th>
                            <th>活动连接</th>
                            <th>状态</th>
                            <th />
                          </tr>
                        </thead>
                        <tbody>
                          {cfg.entrances.map((e) => (
                            <tr key={e.id}>
                              <td>
                                <strong>{e.name}</strong>
                                <small>
                                  {e.kind === "mixed"
                                    ? "HTTP · SOCKS5"
                                    : `固定目标 → ${e.target}`}
                                </small>
                              </td>
                              <td>
                                <code>
                                  {e.listen}:{e.port}
                                </code>
                                <small>{e.network.toUpperCase()}</small>
                              </td>
                              <td>
                                <span className="group-tag">
                                  <Layers size={13} />
                                  {
                                    cfg.groups.find((g) => g.id === e.group_id)
                                      ?.name
                                  }
                                </span>
                              </td>
                              <td>{strategies[e.strategy]}</td>
                              <td className="numeric">
                                {stats?.entrances.find((v) => v.id === e.id)
                                  ?.active || 0}
                              </td>
                              <td>
                                <button
                                  className={`toggle ${e.enabled ? "on" : ""}`}
                                  aria-label={
                                    e.enabled ? "禁用入口" : "启用入口"
                                  }
                                  disabled={busy}
                                  onClick={() =>
                                    action(async () => {
                                      const next = clone(cfg);
                                      next.entrances.find(
                                        (v) => v.id === e.id,
                                      )!.enabled = !e.enabled;
                                      await save(next);
                                    })
                                  }
                                >
                                  <span />
                                </button>
                              </td>
                              <td>
                                <div className="row-actions">
                                  <button
                                    onClick={() =>
                                      setModal({ kind: "entrance", value: e })
                                    }
                                  >
                                    编辑
                                  </button>
                                  <button
                                    className="icon-button danger"
                                    aria-label="删除入口"
                                    onClick={() =>
                                      action(() => remove("entrances", e.id))
                                    }
                                  >
                                    <Trash2 size={15} />
                                  </button>
                                </div>
                              </td>
                            </tr>
                          ))}
                        </tbody>
                      </table>
                    </div>
                  ) : (
                    <Empty
                      title="还没有分发入口"
                      text="选择端口、节点组和调度策略，即可开始分发连接。"
                      action={
                        <button
                          className="button primary"
                          onClick={() => setModal({ kind: "entrance" })}
                        >
                          <Plus size={16} />
                          新建入口
                        </button>
                      }
                    />
                  )}
                </section>
              )}
              {page === "nodes" && (
                <section className="panel">
                  <div className="table-toolbar">
                    <div className="search">
                      <Search size={16} />
                      <input
                        aria-label="搜索节点"
                        placeholder="搜索节点名称、协议或服务器…"
                        value={search}
                        onChange={(e) => {
                          setSearch(e.target.value);
                          setNodePage(0);
                        }}
                      />
                    </div>
                    <span>{nodes.length} 个节点</span>
                  </div>
                  {nodes.length ? (
                    <>
                      <div className="table-scroll">
                        <table>
                          <thead>
                            <tr>
                              <th>节点</th>
                              <th>服务器</th>
                              <th>TCP 状态</th>
                              <th>UDP 状态</th>
                              <th>连接 / 流量</th>
                              <th />
                            </tr>
                          </thead>
                          <tbody>
                            {visibleNodes.map((n) => {
                              const s = stats?.nodes?.find(
                                (x) => x.id === n.id,
                              );
                              return (
                                <tr key={n.id}>
                                  <td>
                                    <div className="node-name">
                                      <span className="node-icon">
                                        <Globe2 size={17} />
                                      </span>
                                      <div>
                                        <strong>{n.name}</strong>
                                        <small>
                                          {String(
                                            n.outbound.type,
                                          ).toUpperCase()}{" "}
                                          ·{" "}
                                          {cfg.subscriptions.find(
                                            (x) => x.id === n.source_id,
                                          )?.name || "手动导入"}
                                        </small>
                                      </div>
                                    </div>
                                  </td>
                                  <td>
                                    <code>
                                      {String(n.outbound.server || "本机")}:
                                      {String(n.outbound.server_port || "—")}
                                    </code>
                                  </td>
                                  <td>
                                    <Badge health={s?.tcp} />
                                  </td>
                                  <td>
                                    <Badge health={s?.udp} />
                                  </td>
                                  <td>
                                    <strong className="numeric">
                                      {s?.active || 0}
                                    </strong>
                                    <small>
                                      ↑ {bytes(s?.upload)}　↓{" "}
                                      {bytes(s?.download)}
                                    </small>
                                  </td>
                                  <td>
                                    <div className="row-actions">
                                      <button
                                        disabled={busy}
                                        onClick={() =>
                                          action(async () => {
                                            await api(
                                              `/nodes/${n.id}/probe`,
                                              "POST",
                                              { network: "tcp" },
                                            );
                                            await refreshStats();
                                            tell("TCP 探测完成");
                                          })
                                        }
                                      >
                                        探测
                                      </button>
                                      <button
                                        onClick={() =>
                                          setModal({ kind: "node", value: n })
                                        }
                                      >
                                        编辑
                                      </button>
                                      <button
                                        className="icon-button danger"
                                        aria-label="删除节点"
                                        onClick={() =>
                                          action(() => remove("nodes", n.id))
                                        }
                                      >
                                        <Trash2 size={15} />
                                      </button>
                                    </div>
                                  </td>
                                </tr>
                              );
                            })}
                          </tbody>
                        </table>
                      </div>
                      <div className="pagination">
                        <span>
                          第 {nodePage + 1} /{" "}
                          {Math.max(1, Math.ceil(nodes.length / 100))}{" "}
                          页，每页最多 100 个
                        </span>
                        <div>
                          <button
                            className="icon-button"
                            disabled={nodePage === 0}
                            onClick={() => setNodePage((p) => p - 1)}
                            aria-label="上一页"
                          >
                            <ChevronLeft size={18} />
                          </button>
                          <button
                            className="icon-button"
                            disabled={(nodePage + 1) * 100 >= nodes.length}
                            onClick={() => setNodePage((p) => p + 1)}
                            aria-label="下一页"
                          >
                            <ChevronRight size={18} />
                          </button>
                        </div>
                      </div>
                    </>
                  ) : (
                    <Empty
                      title={search ? "没有匹配的节点" : "添加你的出口节点"}
                      text="粘贴分享链接、订阅内容，或导入 Clash / sing-box 节点配置。"
                      action={
                        <button className="button primary" onClick={openImport}>
                          <Upload size={16} />
                          导入节点
                        </button>
                      }
                    />
                  )}
                </section>
              )}
              {page === "groups" &&
                (cfg.groups.length ? (
                  <div className="group-grid">
                    {cfg.groups.map((g) => {
                      const manual = (g.members || []).length;
                      return (
                        <section className="panel group-card" key={g.id}>
                          <div className="group-card-top">
                            <span className="route-icon">
                              <Layers size={22} />
                            </span>
                            <button
                              className="icon-button danger"
                              aria-label="删除分组"
                              onClick={() =>
                                action(() => remove("groups", g.id))
                              }
                            >
                              <Trash2 size={16} />
                            </button>
                          </div>
                          <h2>{g.name}</h2>
                          <p>
                            {manual} 个手动成员 · {(g.sources || []).length}{" "}
                            个订阅来源
                          </p>
                          <div className="group-rules">
                            {g.keyword && <code>包含：{g.keyword}</code>}
                            {g.regex && <code>正则：{g.regex}</code>}
                            {g.exclude && <code>排除：{g.exclude}</code>}
                            {!g.keyword &&
                              !g.regex &&
                              !(g.sources || []).length && (
                                <span>手动选择与优先级排序</span>
                              )}
                          </div>
                          <footer>
                            <span>
                              {
                                cfg.entrances.filter((e) => e.group_id === g.id)
                                  .length
                              }{" "}
                              个入口引用
                            </span>
                            <button
                              className="text-button"
                              onClick={() =>
                                setModal({ kind: "group", value: g })
                              }
                            >
                              编辑分组 <ArrowRight size={15} />
                            </button>
                          </footer>
                        </section>
                      );
                    })}
                  </div>
                ) : (
                  <section className="panel">
                    <Empty
                      title="将节点组织成组"
                      text="按落地地区、订阅来源或用途建立分组，多个入口可共享同一组。"
                      action={
                        <button
                          className="button primary"
                          onClick={() => setModal({ kind: "group" })}
                        >
                          <Plus size={16} />
                          新建分组
                        </button>
                      }
                    />
                  </section>
                ))}
              {page === "subscriptions" && (
                <section className="panel">
                  {cfg.subscriptions.length ? (
                    <div className="subscription-list">
                      {cfg.subscriptions.map((sub) => {
                        const st = subStatus.find((s) => s.id === sub.id);
                        return (
                          <article className="subscription" key={sub.id}>
                            <span className="node-icon">
                              <RefreshCw size={20} />
                            </span>
                            <div className="subscription-main">
                              <h3>
                                {sub.name}
                                <span className="group-tag">
                                  {sub.enabled ? "自动更新" : "暂停更新"}
                                </span>
                              </h3>
                              <p>
                                每 {Math.round(sub.interval_seconds / 60)}{" "}
                                分钟检查 ·{" "}
                                {
                                  cfg.nodes.filter(
                                    (n) => n.source_id === sub.id,
                                  ).length
                                }{" "}
                                个节点
                              </p>
                              <small>最近检查 {date(st?.last_check)}</small>
                              {st?.error && (
                                <div className="inline-warning">{st.error}</div>
                              )}
                              {st?.pending && (
                                <div className="pending-update">
                                  <strong>
                                    待审核：{st.pending_count} 个有效节点，移除{" "}
                                    {st.removed} 个旧节点
                                  </strong>
                                  {st.issues.map((i, k) => (
                                    <small key={k}>
                                      {i.name || `第 ${i.item} 项`}：{i.error}
                                    </small>
                                  ))}
                                  <button
                                    className="button"
                                    disabled={busy}
                                    onClick={() =>
                                      action(async () => {
                                        if (
                                          !confirm(
                                            "确认接受此更新？不支持的节点将不包含在新版本中。",
                                          )
                                        )
                                          return;
                                        await api(
                                          `/subscriptions/${sub.id}/accept`,
                                          "POST",
                                          { version: cfg.version },
                                        );
                                        await load();
                                        tell("订阅更新已接受");
                                      })
                                    }
                                  >
                                    接受更新
                                  </button>
                                </div>
                              )}
                            </div>
                            <div className="row-actions">
                              <button
                                disabled={busy}
                                onClick={() =>
                                  action(async () => {
                                    await api(
                                      `/subscriptions/${sub.id}/refresh`,
                                      "POST",
                                      {},
                                    );
                                    await load();
                                    tell("订阅检查完成");
                                  })
                                }
                              >
                                <RefreshCw size={14} /> 刷新
                              </button>
                              <button
                                onClick={() =>
                                  setModal({ kind: "subscription", value: sub })
                                }
                              >
                                编辑
                              </button>
                              <button
                                className="icon-button danger"
                                aria-label="删除订阅"
                                onClick={() =>
                                  action(() => remove("subscriptions", sub.id))
                                }
                              >
                                <Trash2 size={15} />
                              </button>
                            </div>
                          </article>
                        );
                      })}
                    </div>
                  ) : (
                    <Empty
                      title="接入订阅来源"
                      text="订阅更新失败时保留旧节点，大规模删除前要求审核。"
                      action={
                        <button
                          className="button primary"
                          onClick={() => setModal({ kind: "subscription" })}
                        >
                          <Plus size={16} />
                          添加订阅
                        </button>
                      }
                    />
                  )}
                </section>
              )}
              {page === "events" && (
                <section className="panel">
                  <div className="table-toolbar">
                    <span>
                      <i className="live-dot" /> 实时事件
                    </span>
                    <span className="subtle">最多展示最近 100 条</span>
                  </div>
                  <EventList events={events} />
                </section>
              )}
              {page === "settings" && (
                <div className="settings-grid">
                  <section className="panel">
                    <div className="panel-heading">
                      <div>
                        <h2>资源预设</h2>
                        <p>适合 1–2 核、1–2GB 的均衡配置为默认值。</p>
                      </div>
                      <SlidersHorizontal size={19} />
                    </div>
                    <div className="preset-grid">
                      {presets.map((p) => (
                        <button
                          key={p.preset}
                          className={`preset ${cfg.resources.preset === p.preset ? "chosen" : ""}`}
                          disabled={busy}
                          onClick={() =>
                            action(async () => {
                              const next = clone(cfg);
                              next.resources = {
                                ...p,
                                memory_mib: cfg.resources.memory_mib,
                              };
                              await save(next);
                            })
                          }
                        >
                          <span>
                            {p.preset === "balanced" ? (
                              <Zap size={22} />
                            ) : p.preset === "low-memory" ? (
                              <Server size={22} />
                            ) : (
                              <Activity size={22} />
                            )}
                          </span>
                          <strong>
                            {p.preset === "balanced"
                              ? "均衡"
                              : p.preset === "low-memory"
                                ? "省内存"
                                : "高吞吐"}
                          </strong>
                          <small>
                            {p.max_sessions.toLocaleString()} 连接上限
                            <br />
                            {p.probe_concurrency} 路探测并发
                          </small>
                          {cfg.resources.preset === p.preset && (
                            <Check size={15} className="preset-check" />
                          )}
                        </button>
                      ))}
                    </div>
                    <div className="settings-details">
                      {Object.entries(cfg.resources).map(([key, value]) => (
                        <SystemRow key={key} name={key} value={String(value)} />
                      ))}
                      <button
                        className="button"
                        onClick={() => setModal({ kind: "resources" })}
                      >
                        <Settings2 size={16} />
                        调整资源与探测参数
                      </button>
                    </div>
                  </section>
                  <section className="panel">
                    <div className="panel-heading">
                      <div>
                        <h2>配置与恢复</h2>
                        <p>版本化保存，失败时保留上一配置。</p>
                      </div>
                      <Database size={19} />
                    </div>
                    <div className="settings-details">
                      <SystemRow
                        name="当前配置版本"
                        value={`v${cfg.version}`}
                      />
                      <SystemRow
                        name="程序版本"
                        value={`v${stats?.version || "0.1.0"}`}
                      />
                      <a className="button full" href="/api/v1/config/export">
                        <Download size={16} />
                        导出配置（已脱敏）
                      </a>
                      <a className="button full" href="/api/v1/backup">
                        <Database size={16} />
                        下载完整备份（含凭据）
                      </a>
                      <button
                        className="button full"
                        onClick={() =>
                          setModal({
                            kind: "config",
                            value: JSON.stringify(cfg, null, 2),
                          })
                        }
                      >
                        <Terminal size={16} />
                        高级配置编辑
                      </button>
                      <p className="subtle">
                        完整备份含节点及订阅凭据，请妥善保存。跨机器迁移使用完整备份或明确导出凭据。
                      </p>
                      <h3>历史版本</h3>
                      {revisions
                        .filter((r) => r.state === "history")
                        .slice(0, 5)
                        .map((r) => (
                          <div className="revision" key={r.version}>
                            <span>
                              v{r.version}
                              <small>{date(r.created)}</small>
                            </span>
                            <button
                              className="text-button"
                              disabled={busy}
                              onClick={() =>
                                action(async () => {
                                  if (!confirm(`回滚到 v${r.version}？`))
                                    return;
                                  await api(
                                    `/revisions/${r.version}/rollback`,
                                    "POST",
                                    { version: cfg.version },
                                  );
                                  await load();
                                  tell("配置已回滚");
                                })
                              }
                            >
                              回滚
                            </button>
                          </div>
                        ))}
                    </div>
                  </section>
                </div>
              )}
            </>
          )}
          <footer className="page-footer">
            <span>
              SBRoute <b>·</b> 轻量节点分发
            </span>
            <span>
              <ShieldCheck size={13} /> 基于 sing-box 的原生协议转发
            </span>
          </footer>
        </div>
      </main>
      {modal && cfg && (
        <Modal
          title={
            {
              entrance: modal.value ? "编辑分发入口" : "新建分发入口",
              node: modal.value ? "编辑节点" : "手动添加节点",
              group: modal.value ? "编辑节点分组" : "新建节点分组",
              subscription: modal.value ? "编辑订阅" : "添加订阅",
              import: "导入出口节点",
              resources: "资源与健康检查",
              config: "高级配置编辑",
            }[modal.kind] || "编辑"
          }
          close={() => setModal(undefined)}
          wide={["group", "config", "import", "resources"].includes(modal.kind)}
        >
          {modal.kind === "entrance" && (
            <EntryForm
              value={modal.value as Entry | undefined}
              cfg={cfg}
              busy={busy}
              save={(e) =>
                action(async () => {
                  const next = clone(cfg);
                  next.entrances = [
                    ...next.entrances.filter((v) => v.id !== e.id),
                    e,
                  ];
                  await save(next);
                })
              }
            />
          )}
          {modal.kind === "node" && !!modal.value && (
            <div className="settings-details">
              <button
                className="button"
                disabled={busy}
                onClick={() =>
                  action(async () => {
                    const result = await api<{ ip: string }>(
                      `/nodes/${(modal.value as NodeDef).id}/exit-ip`,
                      "POST",
                      {},
                    );
                    await refreshStats();
                    tell(`出口 IP：${result.ip}`);
                  })
                }
              >
                <Globe2 size={16} />
                检测真实出口 IP
              </button>
              <p className="subtle">
                {stats?.nodes?.find((n) => n.id === (modal.value as NodeDef).id)
                  ?.exit_ip || "按需通过节点访问 IP 查询服务，不进行后台扫描。"}
              </p>
            </div>
          )}
          {modal.kind === "group" && (
            <GroupForm
              value={modal.value as Group | undefined}
              cfg={cfg}
              busy={busy}
              save={(g) =>
                action(async () => {
                  const next = clone(cfg);
                  next.groups = [
                    ...next.groups.filter((v) => v.id !== g.id),
                    g,
                  ];
                  await save(next);
                })
              }
            />
          )}
          {modal.kind === "node" && (
            <NodeForm
              value={modal.value as NodeDef | undefined}
              busy={busy}
              save={(n) =>
                action(async () => {
                  const next = clone(cfg);
                  next.nodes = [...next.nodes.filter((v) => v.id !== n.id), n];
                  await save(next);
                })
              }
            />
          )}
          {modal.kind === "subscription" && (
            <SubForm
              value={modal.value as Subscription | undefined}
              busy={busy}
              save={(s) =>
                action(async () => {
                  const next = clone(cfg);
                  next.subscriptions = [
                    ...next.subscriptions.filter((v) => v.id !== s.id),
                    s,
                  ];
                  await save(next);
                })
              }
            />
          )}
          {modal.kind === "import" && (
            <div className="form-body">
              <p className="subtle">
                支持分享链接、Base64 订阅内容、Clash YAML、sing-box
                JSON。订阅网址请在「订阅来源」添加。
              </p>
              <textarea
                className="code-input"
                rows={10}
                value={importContent}
                onChange={(e) => {
                  setImportContent(e.target.value);
                  setPreview(undefined);
                }}
                placeholder="vless://…&#10;ss://…&#10;或粘贴配置内容"
              />
              {preview && (
                <div className="import-result">
                  <h3>
                    <Check size={17} /> {preview.nodes.length} 个有效节点 ·{" "}
                    {preview.duplicates} 个重复项
                  </h3>
                  {preview.nodes.slice(0, 8).map((n) => (
                    <span key={n.id}>{n.name}</span>
                  ))}
                  {preview.nodes.length > 8 && (
                    <small>另有 {preview.nodes.length - 8} 个…</small>
                  )}
                  {preview.issues.map((v, i) => (
                    <div className="inline-warning" key={i}>
                      {v.name || `第 ${v.item} 项`}：{v.error}
                    </div>
                  ))}
                </div>
              )}
              <div className="form-actions">
                <button
                  className="button"
                  disabled={!importContent || busy}
                  onClick={() =>
                    action(async () =>
                      setPreview(
                        await api<ImportPreview>("/imports/preview", "POST", {
                          content: importContent,
                          source_id: "",
                        }),
                      ),
                    )
                  }
                >
                  解析预览
                </button>
                <button
                  className="button primary"
                  disabled={!preview?.nodes.length || busy}
                  onClick={() =>
                    action(async () => {
                      await api("/imports/apply", "POST", {
                        preview_id: preview!.preview_id,
                        version: cfg.version,
                      });
                      setModal(undefined);
                      await load();
                      tell("节点已导入，可以创建分组");
                    })
                  }
                >
                  <Upload size={16} />
                  确认导入
                </button>
              </div>
            </div>
          )}
          {modal.kind === "resources" && (
            <ResourceForm
              cfg={cfg}
              busy={busy}
              save={(resources, health) =>
                action(async () => {
                  const next = clone(cfg);
                  next.resources = resources;
                  next.health = health;
                  await save(next);
                })
              }
            />
          )}
          {modal.kind === "config" && (
            <form
              className="form-body"
              onSubmit={(e) => {
                e.preventDefault();
                const value = new FormData(e.currentTarget).get(
                  "config",
                ) as string;
                action(async () => save(JSON.parse(value)));
              }}
            >
              <p className="subtle">
                已脱敏的节点凭据会按 ID
                保留。监听地址不变时修改入口协议或认证，请先禁用并保存，再编辑和启用。
              </p>
              <textarea
                className="code-input"
                name="config"
                rows={20}
                defaultValue={modal.value as string}
                spellCheck={false}
              />
              <div className="form-actions">
                <button className="button primary" disabled={busy}>
                  <Check size={16} />
                  校验并应用
                </button>
              </div>
            </form>
          )}
          {error && <div className="modal-error">{error}</div>}
        </Modal>
      )}
    </div>
  );
}
function Metric({
  label,
  value,
  sub,
  icon,
  children,
}: {
  label: string;
  value: string;
  sub: string;
  icon: React.ReactNode;
  children?: React.ReactNode;
}) {
  return (
    <article className="metric">
      <div>
        <span>{label}</span>
        {icon}
      </div>
      <strong>{value}</strong>
      <small>{sub}</small>
      {children}
    </article>
  );
}
function SystemRow({ name, value }: { name: string; value: string }) {
  return (
    <div className="system-row">
      <span>{name}</span>
      <strong>{value}</strong>
    </div>
  );
}
function EventList({ events }: { events: Event[] }) {
  return events.length ? (
    <div className="event-list">
      {events.map((e) => (
        <div className="event" key={e.id}>
          <span className={`event-kind ${e.kind}`}>{e.kind.toUpperCase()}</span>
          <p>{e.message}</p>
          <time>{date(e.created)}</time>
        </div>
      ))}
    </div>
  ) : (
    <div className="event-empty">暂时没有事件，配置变化会显示在这里。</div>
  );
}
function SaveButton({ busy }: { busy: boolean }) {
  return (
    <div className="form-actions">
      <button className="button primary" disabled={busy}>
        <Check size={16} />
        {busy ? "正在应用…" : "保存并应用"}
      </button>
    </div>
  );
}
function EntryForm({
  value,
  cfg,
  busy,
  save,
}: {
  value?: Entry;
  cfg: Config;
  busy: boolean;
  save: (v: Entry) => void;
}) {
  const [kind, setKind] = useState(value?.kind || "mixed");
  return (
    <form
      className="form-body"
      onSubmit={(e) => {
        e.preventDefault();
        const f = new FormData(e.currentTarget);
        const users = String(f.get("users") || "")
          .split("\n")
          .filter(Boolean)
          .map((s) => {
            const at = s.indexOf(":");
            return { username: s.slice(0, at), password: s.slice(at + 1) };
          });
        save({
          id: value?.id || id(),
          name: String(f.get("name")),
          enabled: f.has("enabled"),
          listen: String(f.get("listen")),
          port: Number(f.get("port")),
          kind,
          network: String(f.get("network")),
          target: String(f.get("target") || ""),
          group_id: String(f.get("group")),
          strategy: String(f.get("strategy")),
          users,
          allowed_cidrs: String(f.get("cidrs") || "")
            .split(/[\s,]+/)
            .filter(Boolean),
        });
      }}
    >
      <Field label="入口名称">
        <input
          name="name"
          required
          defaultValue={value?.name}
          placeholder="例如：香港出口分发"
        />
      </Field>
      <div className="form-columns">
        <Field label="入口类型">
          <select value={kind} onChange={(e) => setKind(e.target.value)}>
            <option value="mixed">HTTP + SOCKS5 代理</option>
            <option value="forward">固定目标 TCP / UDP 转发</option>
          </select>
        </Field>
        <Field label="网络">
          <select name="network" defaultValue={value?.network || "tcp,udp"}>
            <option value="tcp,udp">TCP + UDP</option>
            <option value="tcp">仅 TCP</option>
            <option value="udp">仅 UDP</option>
          </select>
        </Field>
      </div>
      <div className="form-columns">
        <Field label="监听地址">
          <input
            name="listen"
            required
            defaultValue={value?.listen || "127.0.0.1"}
          />
        </Field>
        <Field label="监听端口">
          <input
            name="port"
            type="number"
            required
            min={1}
            max={65535}
            defaultValue={value?.port || 10001}
          />
        </Field>
      </div>
      {kind === "forward" && (
        <Field
          label="固定目标"
          hint="域名:端口 或 IP:端口；IPv6 地址使用方括号。"
        >
          <input
            name="target"
            required
            defaultValue={value?.target}
            placeholder="example.com:443"
          />
        </Field>
      )}
      <div className="form-columns">
        <Field label="节点组">
          <select name="group" required defaultValue={value?.group_id || ""}>
            <option value="" disabled>
              选择节点组
            </option>
            {cfg.groups.map((g) => (
              <option key={g.id} value={g.id}>
                {g.name}
              </option>
            ))}
          </select>
        </Field>
        <Field label="调度策略">
          <select
            name="strategy"
            defaultValue={value?.strategy || "round_robin"}
          >
            {Object.entries(strategies).map(([key, name]) => (
              <option key={key} value={key}>
                {name}
              </option>
            ))}
          </select>
        </Field>
      </div>
      {kind === "mixed" && (
        <Field
          label="代理账号（可选）"
          hint="每行一个 用户名:密码；公网代理建议设置认证。"
        >
          <textarea
            name="users"
            rows={2}
            defaultValue={value?.users
              ?.map((u) => `${u.username}:${u.password}`)
              .join("\n")}
            placeholder="user:password"
          />
        </Field>
      )}
      <Field
        label="来源白名单（可选）"
        hint="用逗号或换行分隔 CIDR，留空允许所有来源。"
      >
        <input
          name="cidrs"
          defaultValue={value?.allowed_cidrs?.join(", ")}
          placeholder="192.168.1.0/24, 127.0.0.1/32"
        />
      </Field>
      <label className="check">
        <input
          type="checkbox"
          name="enabled"
          defaultChecked={value?.enabled ?? true}
        />
        启用此入口
      </label>
      <SaveButton busy={busy} />
    </form>
  );
}
function NodeForm({
  value,
  busy,
  save,
}: {
  value?: NodeDef;
  busy: boolean;
  save: (n: NodeDef) => void;
}) {
  return (
    <form
      className="form-body"
      onSubmit={(e) => {
        e.preventDefault();
        const f = new FormData(e.currentTarget);
        try {
          save({
            id: value?.id || id(),
            name: String(f.get("name")),
            disabled: f.has("disabled"),
            source_id: value?.source_id,
            outbound: String(f.get("outbound")).trim()
              ? JSON.parse(String(f.get("outbound")))
              : value!.outbound,
          });
        } catch (err) {
          alert((err as Error).message);
        }
      }}
    >
      <Field label="节点名称">
        <input required name="name" defaultValue={value?.name} />
      </Field>
      <Field
        label="sing-box 出站 JSON"
        hint={
          value
            ? "留空保留原连接配置；替换时填写完整的出站 JSON。"
            : "填写独立节点出站，不包含 outbounds 外层。"
        }
      >
        <textarea
          className="code-input"
          rows={9}
          name="outbound"
          required={!value}
          defaultValue={
            value
              ? ""
              : '{\n  "type": "socks",\n  "server": "127.0.0.1",\n  "server_port": 1080,\n  "version": "5"\n}'
          }
        />
      </Field>
      <label className="check">
        <input
          name="disabled"
          type="checkbox"
          defaultChecked={value?.disabled}
        />
        禁用此节点
      </label>
      <SaveButton busy={busy} />
    </form>
  );
}
function GroupForm({
  value,
  cfg,
  busy,
  save,
}: {
  value?: Group;
  cfg: Config;
  busy: boolean;
  save: (g: Group) => void;
}) {
  const [members, setMembers] = useState(value?.members || []);
  const move = (at: number, delta: number) => {
    const n = [...members];
    [n[at], n[at + delta]] = [n[at + delta], n[at]];
    setMembers(n);
  };
  return (
    <form
      className="form-body"
      onSubmit={(e) => {
        e.preventDefault();
        const f = new FormData(e.currentTarget);
        save({
          id: value?.id || id(),
          name: String(f.get("name")),
          members,
          sources: f.getAll("sources").map(String),
          keyword: String(f.get("keyword")),
          regex: String(f.get("regex")),
          exclude: String(f.get("exclude")),
        });
      }}
    >
      <Field label="分组名称">
        <input
          required
          name="name"
          defaultValue={value?.name}
          placeholder="例如：香港 · 流媒体"
        />
      </Field>
      <div className="form-columns">
        <div>
          <h3>手动选择节点</h3>
          <div className="member-picker">
            {cfg.nodes.map((n) => (
              <label className="check" key={n.id}>
                <input
                  type="checkbox"
                  checked={members.includes(n.id)}
                  onChange={(e) =>
                    setMembers((old) =>
                      e.target.checked
                        ? [...old, n.id]
                        : old.filter((v) => v !== n.id),
                    )
                  }
                />
                {n.name}
              </label>
            ))}
            {!cfg.nodes.length && <small>请先导入节点</small>}
          </div>
        </div>
        <div>
          <h3>优先级顺序</h3>
          <div className="member-picker">
            {members.map((m, i) => (
              <div className="member-order" key={m}>
                <span>
                  {i + 1}. {cfg.nodes.find((n) => n.id === m)?.name}
                </span>
                <button
                  type="button"
                  className="icon-button"
                  disabled={i === 0}
                  onClick={() => move(i, -1)}
                  aria-label="上移"
                >
                  <ArrowUp size={14} />
                </button>
                <button
                  type="button"
                  className="icon-button"
                  disabled={i === members.length - 1}
                  onClick={() => move(i, 1)}
                  aria-label="下移"
                >
                  <ArrowDown size={14} />
                </button>
              </div>
            ))}
          </div>
        </div>
      </div>
      <h3>动态规则（可选）</h3>
      <div className="subscription-picker">
        {cfg.subscriptions.map((s) => (
          <label className="check" key={s.id}>
            <input
              name="sources"
              type="checkbox"
              value={s.id}
              defaultChecked={value?.sources?.includes(s.id)}
            />
            {s.name}
          </label>
        ))}
      </div>
      <div className="form-columns">
        <Field label="名称包含">
          <input
            name="keyword"
            defaultValue={value?.keyword}
            placeholder="香港"
          />
        </Field>
        <Field label="名称正则">
          <input
            name="regex"
            defaultValue={value?.regex}
            placeholder="HK|香港"
          />
        </Field>
      </div>
      <Field
        label="排除正则"
        hint="动态条件取交集，与手动节点取并集；排除规则最终生效。手动顺序用于故障转移。"
      >
        <input
          name="exclude"
          defaultValue={value?.exclude}
          placeholder="剩余流量|过期"
        />
      </Field>
      <SaveButton busy={busy} />
    </form>
  );
}
function SubForm({
  value,
  busy,
  save,
}: {
  value?: Subscription;
  busy: boolean;
  save: (s: Subscription) => void;
}) {
  return (
    <form
      className="form-body"
      onSubmit={(e) => {
        e.preventDefault();
        const f = new FormData(e.currentTarget);
        save({
          id: value?.id || id(),
          name: String(f.get("name")),
          url: String(f.get("url")) || value!.url,
          enabled: f.has("enabled"),
          interval_seconds: Number(f.get("interval")) * 60,
        });
      }}
    >
      <Field label="订阅名称">
        <input name="name" required defaultValue={value?.name} />
      </Field>
      <Field
        label="订阅链接"
        hint={
          value
            ? "留空保留原地址；链接中的凭据不会在列表显示。"
            : "支持 HTTP / HTTPS，建议使用 HTTPS。"
        }
      >
        <input
          name="url"
          type="url"
          required={!value}
          placeholder={value ? "已保存，留空保留" : "https://…"}
          autoComplete="off"
        />
      </Field>
      <Field label="更新间隔（分钟）">
        <input
          name="interval"
          type="number"
          required
          min={1}
          defaultValue={(value?.interval_seconds || 21600) / 60}
        />
      </Field>
      <label className="check">
        <input
          name="enabled"
          type="checkbox"
          defaultChecked={value?.enabled ?? true}
        />
        自动更新
      </label>
      <SaveButton busy={busy} />
    </form>
  );
}
function ResourceForm({
  cfg,
  busy,
  save,
}: {
  cfg: Config;
  busy: boolean;
  save: (r: Resources, h: Config["health"]) => void;
}) {
  const labels: Record<string, string> = {
    memory_mib: "内存预算 MiB",
    max_sessions: "业务连接/会话上限",
    max_udp: "UDP 会话上限",
    max_dials: "并行建连上限",
    probe_concurrency: "探测并发",
    probe_interval_seconds: "探测周期（秒）",
    stats_interval_seconds: "面板刷新（秒）",
    udp_timeout_seconds: "UDP 空闲回收（秒）",
  };
  return (
    <form
      className="form-body"
      onSubmit={(e) => {
        e.preventDefault();
        const f = new FormData(e.currentTarget);
        const r = clone(cfg.resources);
        Object.keys(labels).forEach((k) => {
          (r as unknown as Record<string, unknown>)[k] = Number(f.get(k));
        });
        save(r, {
          urls: String(f.get("urls")).split("\n").filter(Boolean),
          udp_dns: String(f.get("udp_dns")),
          timeout_seconds: Number(f.get("timeout")),
          failure_threshold: Number(f.get("failures")),
          recovery_threshold: Number(f.get("recoveries")),
        });
      }}
    >
      <div className="form-columns">
        {Object.keys(labels).map((k) => (
          <Field key={k} label={labels[k]}>
            <input
              name={k}
              type="number"
              required
              min={k === "max_udp" ? 0 : 1}
              defaultValue={
                (cfg.resources as unknown as Record<string, number>)[k]
              }
            />
          </Field>
        ))}
      </div>
      <Field
        label="HTTPS 探测地址"
        hint="每行一个，预期返回 HTTP 204；主地址失败后使用备用地址复核。"
      >
        <textarea
          name="urls"
          rows={2}
          defaultValue={cfg.health.urls.join("\n")}
        />
      </Field>
      <Field label="UDP DNS 探测服务器">
        <input name="udp_dns" defaultValue={cfg.health.udp_dns} />
      </Field>
      <div className="form-columns">
        <Field label="探测超时（秒）">
          <input
            name="timeout"
            type="number"
            min={1}
            max={30}
            defaultValue={cfg.health.timeout_seconds}
          />
        </Field>
        <Field label="连续失败阈值">
          <input
            name="failures"
            type="number"
            min={1}
            defaultValue={cfg.health.failure_threshold}
          />
        </Field>
        <Field label="恢复成功阈值">
          <input
            name="recoveries"
            type="number"
            min={1}
            defaultValue={cfg.health.recovery_threshold}
          />
        </Field>
      </div>
      <SaveButton busy={busy} />
    </form>
  );
}

export { App };
const rootElement = document.getElementById("root");
if (rootElement)
  createRoot(rootElement).render(
    <React.StrictMode>
      <App />
    </React.StrictMode>,
  );
