import { ArrowRight, GitBranch } from "lucide-react";
import { api } from "../lib";
import { Field } from "../ui";
import type { Console } from "../console";

export function Login({
  ctx,
}: {
  ctx: Pick<Console, "busy" | "error" | "action" | "setAuthenticated">;
}) {
  const { busy, error, action, setAuthenticated } = ctx;
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
}
