import {
  Activity,
  ArrowDownUp,
  ArrowRight,
  Globe2,
  Layers,
  Network,
  Plus,
  Radio,
  Server,
  Zap,
} from "lucide-react";
import { strategies, bytes } from "../lib";
import { Empty, Sparkline, Metric, SystemRow, EventList } from "../ui";
import type { Console } from "../console";

export function Overview({ ctx }: { ctx: Console }) {
  const { cfg, stats, action, changePage, openImport, points, events } = ctx;
  return (
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
          value={bytes((stats?.upload || 0) + (stats?.download || 0))}
          sub={`↑ ${bytes(stats?.upload)}　↓ ${bytes(stats?.download)}`}
          icon={<ArrowDownUp size={18} />}
        />
        <Metric
          label="分发入口"
          value={String(cfg.entrances.filter((e) => e.enabled).length)}
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
                const es = stats?.entrances.find((s) => s.id === e.id);
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
                        {cfg.groups.find((g) => g.id === e.group_id)?.name ||
                          "未绑定"}
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
                <button className="button primary" onClick={openImport}>
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
            <SystemRow name="Go 堆内存" value={bytes(stats?.heap_bytes)} />
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
          <button className="text-button" onClick={() => changePage("events")}>
            查看全部 <ArrowRight size={15} />
          </button>
        </div>
        <EventList events={events.slice(0, 5)} />
      </section>
    </>
  );
}
