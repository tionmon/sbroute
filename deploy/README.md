# 部署

## systemd（推荐）

在发布包目录执行：

```bash
sudo useradd --system --home-dir /var/lib/sbroute --shell /usr/sbin/nologin sbroute
sudo install -m 755 sbroute /usr/local/bin/sbroute
sudo install -d -m 700 -o sbroute -g sbroute /var/lib/sbroute
sudo -u sbroute /usr/local/bin/sbroute admin --data-dir /var/lib/sbroute
sudo install -m 644 deploy/sbroute.service /etc/systemd/system/sbroute.service
sudo systemctl daemon-reload
sudo systemctl enable --now sbroute
```

已有 `sbroute` 用户时跳过 `useradd`。模板使用普通用户、高端口、768MiB 进程组上限和 65536 FD；需要更多容量时通过 `systemctl edit sbroute` 调整 `MemoryMax`，并同步调整面板资源预算与连接上限。启用高吞吐预设或超过约一万 TCP 连接时，将 `LimitNOFILE` 同步提高到 131072；Linux splice 也使用 pipe FD，不能只按每连接两个 socket 估算。不修改全局网络 sysctl。

```bash
sudo systemctl status sbroute
sudo journalctl -u sbroute -n 100
ssh -N -L 8080:127.0.0.1:8080 YOUR_SERVER
```

面板默认仅本机访问。如使用反向代理，保留原 Host、启用 HTTPS，并关闭事件流的代理缓冲；同时在 `ExecStart` 追加 `--trusted-proxy 127.0.0.1`（代理所在地址），让登录限频识别真实客户端 IP。初始无管理员时请停服务后运行 `sbroute admin`；离线备份/恢复同样先停服务。

## Docker（Linux host network）

在源码 `deploy/` 执行：

```bash
docker compose build
docker compose run --rm -it sbroute admin --data-dir /var/lib/sbroute
docker compose up -d
```

host 网络允许面板创建动态入口端口，管理端仍监听主机回环地址。入口是否对外开放由每个入口的监听地址决定。数据卷保存配置与凭据，不随镜像更换；升级前从面板下载完整备份。Docker 构建文件提供在仓库中，但本次测试服务器未安装 Docker，因此容器启动未作为已验证结果。
