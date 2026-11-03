import { CircleHelp, Layers, Plus, Trash2 } from "lucide-react";
import { strategies, clone } from "../lib";
import { Empty } from "../ui";
import type { Console } from "../console";

export function Entrances({ ctx }: { ctx: Console }) {
  const { cfg, stats, busy, action, save, setModal, remove } = ctx;
  return (
    <section className="panel">
      <div className="table-toolbar">
        <span>{cfg.entrances.length} 个入口</span>
        <span className="subtle">
          <CircleHelp size={14} /> 所有节点不可用时拒绝连接，不自动直连
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
                      {cfg.groups.find((g) => g.id === e.group_id)?.name}
                    </span>
                  </td>
                  <td>{strategies[e.strategy]}</td>
                  <td className="numeric">
                    {stats?.entrances.find((v) => v.id === e.id)?.active || 0}
                  </td>
                  <td>
                    <button
                      className={`toggle ${e.enabled ? "on" : ""}`}
                      aria-label={e.enabled ? "禁用入口" : "启用入口"}
                      disabled={busy}
                      onClick={() =>
                        action(async () => {
                          const next = clone(cfg);
                          next.entrances.find((v) => v.id === e.id)!.enabled =
                            !e.enabled;
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
                        onClick={() => setModal({ kind: "entrance", value: e })}
                      >
                        编辑
                      </button>
                      <button
                        className="icon-button danger"
                        aria-label="删除入口"
                        onClick={() => action(() => remove("entrances", e.id))}
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
  );
}
