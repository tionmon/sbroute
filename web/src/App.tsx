import { useEffect, useMemo, useState } from "react";
import {
  Check,
  ChevronRight,
  GitBranch,
  Globe2,
  Layers,
  LayoutDashboard,
  LogOut,
  Network,
  Plus,
  RefreshCw,
  Server,
  Settings2,
  ShieldCheck,
  Terminal,
  Upload,
  X,
} from "lucide-react";
import type {
  Config,
  Event,
  Resources,
  Revision,
  Stats,
  SubStatus,
} from "./types";
import { api, bytes, clone, useDebounced } from "./lib";
import { Empty } from "./ui";
import type { Console, ModalState } from "./console";
import { Dialogs } from "./pages/Dialogs";
import { Entrances } from "./pages/Entrances";
import { Events } from "./pages/Events";
import { Groups } from "./pages/Groups";
import { Login } from "./pages/Login";
import { Nodes } from "./pages/Nodes";
import { Overview } from "./pages/Overview";
import { Settings } from "./pages/Settings";
import { Subscriptions } from "./pages/Subscriptions";

const nav = [
  { key: "overview", label: "运行概览", icon: LayoutDashboard },
  { key: "entrances", label: "分发入口", icon: Network },
  { key: "nodes", label: "出口节点", icon: Globe2 },
  { key: "groups", label: "节点分组", icon: Layers },
  { key: "subscriptions", label: "订阅来源", icon: RefreshCw },
  { key: "events", label: "事件日志", icon: Terminal },
  { key: "settings", label: "系统设置", icon: Settings2 },
];

export function App() {
  const [authenticated, setAuthenticated] = useState<boolean | null>(null);
  const [page, setPage] = useState("overview");
  const [cfg, setCfg] = useState<Config>();
  const [stats, setStats] = useState<Stats>();
  const [events, setEvents] = useState<Event[]>([]);
  const [subStatus, setSubStatus] = useState<SubStatus[]>([]);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [busy, setBusy] = useState(false);
  const [modal, setModal] = useState<ModalState>();
  const [search, setSearch] = useState("");
  const [nodePage, setNodePage] = useState(0);
  const [points, setPoints] = useState<number[]>([]);
  const [presets, setPresets] = useState<Resources[]>([]);
  const [revisions, setRevisions] = useState<Revision[]>([]);
  // Filtering and the node-status poll follow the search box only after typing pauses.
  const debouncedSearch = useDebounced(search, 250);
  const nodes = useMemo(
    () =>
      (cfg?.nodes || []).filter((n) =>
        `${n.name} ${n.outbound.type} ${n.outbound.server}`
          .toLowerCase()
          .includes(debouncedSearch.toLowerCase()),
      ),
    [cfg, debouncedSearch],
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
      api<Revision[]>("/revisions"),
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
  }, [authenticated, cfg?.version, page, nodePage, debouncedSearch]);
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
    return <Login ctx={{ busy, error, action, setAuthenticated }} />;
  const current = nav.find((n) => n.key === page)!;
  const openImport = () => setModal({ kind: "import" });
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
  const ctx: Console | undefined = cfg && {
    cfg,
    stats,
    busy,
    error,
    action,
    save,
    modal,
    setModal,
    load,
    tell,
    changePage,
    remove,
    openImport,
    refreshStats,
    points,
    events,
    subStatus,
    presets,
    revisions,
    search,
    setSearch,
    nodes,
    visibleNodes,
    nodePage,
    setNodePage,
    setAuthenticated,
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
          {!ctx ? (
            <Empty title="正在加载配置" text="请稍候…" />
          ) : (
            <>
              {page === "overview" && <Overview ctx={ctx} />}
              {page === "entrances" && <Entrances ctx={ctx} />}
              {page === "nodes" && <Nodes ctx={ctx} />}
              {page === "groups" && <Groups ctx={ctx} />}
              {page === "subscriptions" && <Subscriptions ctx={ctx} />}
              {page === "events" && <Events ctx={ctx} />}
              {page === "settings" && <Settings ctx={ctx} />}
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
      {modal && ctx && <Dialogs ctx={ctx} />}
    </div>
  );
}
