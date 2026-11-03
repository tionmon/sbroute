import { useState } from "react";
import { ArrowDown, ArrowUp } from "lucide-react";
import type {
  NodeDef,
  Group,
  Entry,
  Subscription,
  Resources,
  Config,
} from "./types";
import { strategies, id, clone } from "./lib";
import { Field, SaveButton } from "./ui";

export function EntryForm({
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
export function NodeForm({
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
export function GroupForm({
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
export function SubForm({
  value,
  cfg,
  busy,
  save,
}: {
  value?: Subscription;
  cfg: Config;
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
          user_agent: String(f.get("user_agent") || "").trim() || undefined,
          via: String(f.get("via") || "") || undefined,
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
      <div className="form-columns">
        <Field
          label="User-Agent（可选）"
          hint="部分订阅按 UA 返回不同格式，例如 clash.meta、sing-box。"
        >
          <input
            name="user_agent"
            maxLength={256}
            defaultValue={value?.user_agent}
            placeholder="默认 SBRoute"
          />
        </Field>
        <Field
          label="下载方式"
          hint="服务器无法直连订阅地址时，可经由一个节点下载。"
        >
          <select name="via" defaultValue={value?.via || ""}>
            <option value="">直接连接</option>
            {cfg.nodes
              .filter((n) => !n.disabled)
              .map((n) => (
                <option key={n.id} value={n.id}>
                  经由 {n.name}
                </option>
              ))}
          </select>
        </Field>
      </div>
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
export function ResourceForm({
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
        hint="每行一个，预期返回 HTTP 2xx（推荐 204 地址）；主地址失败后使用备用地址复核。"
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
