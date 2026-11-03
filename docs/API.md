# Management API v1

Base: `/api/v1`，使用 `sbroute_session` HttpOnly / SameSite=Strict cookie。修改请求使用 `application/json`；浏览器 Origin 必须与 Host 一致。登录失败限频。默认返回脱敏配置，只有显式 `?secrets=1` 才返回凭据。

| Method | Path | 内容 |
| --- | --- | --- |
| POST | `/login` | `{username,password}` |
| GET / POST | `/session` / `/logout` | 检查会话 / 注销 |
| GET / PUT | `/config` | 获取或提交完整配置；PUT 必须携带当前 `version` |
| POST | `/config/validate` | 验证配置和核心兼容性，不占用入口端口 |
| GET | `/config/export` | YAML，默认脱敏；`?secrets=1` 为私密导出 |
| GET | `/presets` | 三种资源预设 |
| GET | `/stats` | 全局与入口状态，包含 RSS / heap / FD / cgroup 信息 |
| GET | `/stats?nodes=1&offset=0` | 附带最多 100 个节点状态 |
| GET | `/stats?nodes=1&ids=a,b` | 按 ID 选择最多 100 个节点 |
| GET | `/events` | 最近 100 条事件 |
| GET | `/events/stream` | SSE，最多 8 个并发订阅 |
| POST | `/imports/preview` | `{content,source_id}`，返回脱敏预览和 `preview_id` |
| POST | `/imports/apply` | `{preview_id,version}`；预览有效期 5 分钟，最多保留 2 份 |
| POST | `/nodes/{id}/probe` | `{network:"tcp"或"udp"}`，共享并发预算 |
| POST | `/nodes/{id}/exit-ip` | `{}`，显式访问出口 IP 查询服务 |
| GET | `/subscriptions/status` | 检查时间、错误和待审核摘要，不解码待审核节点凭据 |
| POST | `/subscriptions/{id}/refresh` | `{}`，手动刷新 |
| POST | `/subscriptions/{id}/accept` | `{version}`，接受已审核的更新，包括空订阅 |
| GET | `/revisions` | 最近 20 个配置版本 |
| POST | `/revisions/{target}/rollback` | `{version}`，将历史版本作为新版本应用 |
| GET | `/backup` | 下载包含凭据的完整 SQLite 备份 |

独立的 `GET /healthz` 不需要认证，只返回服务存活状态，不保证所有节点都健康。

版本冲突为 HTTP 409；验证失败为 400；未登录为 401；跨域修改为 403；请求过多为 429。错误体为 `{ "error": "具体原因" }`。新配置先验证和准备资源，失败保留旧配置；成功后版本号递增。监听协议、认证、目标、UDP 超时的同端口修改需先禁用入口并应用，随后再修改启用。

配置 JSON 对应 `internal/model/config.go`。节点 `outbound` 使用核心独立出站格式。脱敏节点携带 `_redacted:true`，提交时按原 ID 保留现有完整出站；改变连接配置应提交完整节点出站。订阅 URL 和密码的 `<redacted>` 同样按 ID/用户名保留。
