import { useState } from "react";
import { Check, Globe2, Upload } from "lucide-react";
import type {
  NodeDef,
  Group,
  Entry,
  Subscription,
  ImportPreview,
} from "../types";
import { clone, api } from "../lib";
import { Modal } from "../ui";
import {
  EntryForm,
  NodeForm,
  GroupForm,
  SubForm,
  ResourceForm,
} from "../forms";
import type { Console } from "../console";

export function Dialogs({ ctx }: { ctx: Console }) {
  const {
    cfg,
    stats,
    busy,
    error,
    action,
    save,
    setModal,
    modal,
    load,
    tell,
    refreshStats,
    nodes,
  } = ctx;
  const [importContent, setImportContent] = useState("");
  const [preview, setPreview] = useState<ImportPreview>();
  if (!modal) return null;
  return (
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
              next.groups = [...next.groups.filter((v) => v.id !== g.id), g];
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
          cfg={cfg}
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
            const value = new FormData(e.currentTarget).get("config") as string;
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
  );
}
