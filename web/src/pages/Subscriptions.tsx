import { Plus, RefreshCw, Trash2 } from "lucide-react";
import { bytes, date, api } from "../lib";
import type { Usage } from "../types";
import { Empty } from "../ui";
import type { Console } from "../console";

export function Subscriptions({ ctx }: { ctx: Console }) {
  const { cfg, busy, action, setModal, load, tell, remove, subStatus } = ctx;
  return (
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
                    每 {Math.round(sub.interval_seconds / 60)} 分钟检查 ·{" "}
                    {cfg.nodes.filter((n) => n.source_id === sub.id).length}{" "}
                    个节点
                  </p>
                  <small>
                    最近检查 {date(st?.last_check)}
                    {sub.via &&
                      ` · 经由 ${cfg.nodes.find((n) => n.id === sub.via)?.name || sub.via} 下载`}
                  </small>
                  {st?.usage && <UsageBar usage={st.usage} />}
                  {st?.error && (
                    <div className="inline-warning">{st.error}</div>
                  )}
                  {!st?.pending && !!st?.skipped && (
                    <details className="skipped-items">
                      <summary>
                        已跳过 {st.skipped} 个不支持的条目，其余节点已正常更新
                      </summary>
                      {st.issues.map((i, k) => (
                        <small key={k}>
                          {i.name || `第 ${i.item} 项`}：{i.error}
                        </small>
                      ))}
                    </details>
                  )}
                  {st?.pending && (
                    <div className="pending-update">
                      <strong>
                        待审核：{st.pending_count} 个有效节点，移除 {st.removed}{" "}
                        个旧节点
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
  );
}

// UsageBar shows the provider's subscription-userinfo header: traffic used against quota, and expiry.
function UsageBar({ usage }: { usage: Usage }) {
  const used = usage.upload + usage.download;
  const ratio = usage.total > 0 ? Math.min(1, used / usage.total) : 0;
  return (
    <div className="usage">
      <div className="usage-text">
        <span>
          已用 {bytes(used)}
          {usage.total > 0 && ` / ${bytes(usage.total)}`}
        </span>
        {!!usage.expire && (
          <span>
            到期 {new Date(usage.expire * 1000).toLocaleDateString("zh-CN")}
          </span>
        )}
      </div>
      {usage.total > 0 && (
        <div className={`usage-track ${ratio > 0.9 ? "warning" : ""}`}>
          <i style={{ width: `${ratio * 100}%` }} />
        </div>
      )}
    </div>
  );
}
