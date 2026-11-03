import {
  Activity,
  Check,
  Database,
  Download,
  Server,
  Settings2,
  SlidersHorizontal,
  Terminal,
  Zap,
} from "lucide-react";
import { clone, date, api } from "../lib";
import { SystemRow } from "../ui";
import type { Console } from "../console";

export function Settings({ ctx }: { ctx: Console }) {
  const {
    cfg,
    stats,
    busy,
    action,
    save,
    setModal,
    load,
    tell,
    presets,
    revisions,
  } = ctx;
  return (
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
          <SystemRow name="当前配置版本" value={`v${cfg.version}`} />
          <SystemRow name="程序版本" value={`v${stats?.version || "0.1.0"}`} />
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
                      if (!confirm(`回滚到 v${r.version}？`)) return;
                      await api(`/revisions/${r.version}/rollback`, "POST", {
                        version: cfg.version,
                      });
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
  );
}
