import {
  ChevronLeft,
  ChevronRight,
  Globe2,
  Search,
  Trash2,
  Upload,
} from "lucide-react";
import { bytes, api } from "../lib";
import { Badge, Empty } from "../ui";
import type { Console } from "../console";

export function Nodes({ ctx }: { ctx: Console }) {
  const {
    cfg,
    stats,
    busy,
    action,
    setModal,
    tell,
    remove,
    openImport,
    refreshStats,
    search,
    setSearch,
    nodes,
    visibleNodes,
    nodePage,
    setNodePage,
  } = ctx;
  return (
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
                  const s = stats?.nodes?.find((x) => x.id === n.id);
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
                              {String(n.outbound.type).toUpperCase()} ·{" "}
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
                        <strong className="numeric">{s?.active || 0}</strong>
                        <small>
                          ↑ {bytes(s?.upload)}　↓ {bytes(s?.download)}
                        </small>
                      </td>
                      <td>
                        <div className="row-actions">
                          <button
                            disabled={busy}
                            onClick={() =>
                              action(async () => {
                                await api(`/nodes/${n.id}/probe`, "POST", {
                                  network: "tcp",
                                });
                                await refreshStats();
                                tell("TCP 探测完成");
                              })
                            }
                          >
                            探测
                          </button>
                          <button
                            onClick={() => setModal({ kind: "node", value: n })}
                          >
                            编辑
                          </button>
                          <button
                            className="icon-button danger"
                            aria-label="删除节点"
                            onClick={() => action(() => remove("nodes", n.id))}
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
              第 {nodePage + 1} / {Math.max(1, Math.ceil(nodes.length / 100))}{" "}
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
  );
}
