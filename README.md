# SBRoute

SBRoute 是一个面向 Debian 12 VPS 的 sing-box 流量分流管理脚本。它把出站节点、路由规则、规则集、DNS 模式和 systemd 服务管理封装成一个轻量命令行工具，适合在服务器上快速维护“哪些流量直连、哪些流量走代理、哪些流量阻断”的配置。

## 功能特性

- 独立管理 `/etc/sbroute` 下的 sing-box 配置，不覆盖系统中已有的 `sing-box` 服务。
- 支持添加和管理 `ss`、`socks`、`http`、`vless`、`vmess`、`direct`、`block` 出站。
- 支持解析 `ss://`、`vless://` 分享链接并写入出站配置。
- 支持按域名、后缀、关键字、正则、IP CIDR、端口、进程名、GeoIP、GeoSite 和远程规则集添加路由规则。
- 内置国内直连和广告拦截预设。
- 支持基础 DNS、国内分流 DNS、配置生成、备份、恢复、导入、导出、日志查看和连通性测试。

## 适用环境

- Debian 12
- root 权限
- systemd
- `bash`
- `jq`
- `curl`、`gpg`，用于首次安装 sing-box

`install` 命令会在缺少 `jq` 或 `sing-box` 时尝试通过 `apt-get` 自动安装。

## 安装

下载脚本后放到可执行路径：

```bash
sudo install -m 0755 sbroute.sh /usr/local/bin/sbroute
sudo sbroute install
```

也可以直接运行脚本：

```bash
chmod +x sbroute.sh
sudo ./sbroute.sh install
```

安装后会创建独立的 systemd 服务：

```text
sbroute.service
```

主要数据目录：

```text
/etc/sbroute/
```

## 快速开始

添加一个出站：

```bash
sudo sbroute out add ss proxy1 example.com 8388 2022-blake3-aes-128-gcm 'password'
```

从分享链接添加出站：

```bash
sudo sbroute out add url 'vless://...'
```

查看出站：

```bash
sudo sbroute out list
```

添加国内直连预设：

```bash
sudo sbroute preset cn-direct
```

添加广告拦截预设：

```bash
sudo sbroute preset ads-block
```

设置默认出站：

```bash
sudo sbroute route default proxy1
```

生成配置并重启服务：

```bash
sudo sbroute apply
```

## 常用命令

```bash
sbroute                              # 交互式菜单
sbroute install                      # 安装并启动独立服务
sbroute uninstall                    # 卸载服务

sbroute out add ss <tag> <server> <port> <method> <password>
sbroute out add socks <tag> <server> <port> [user] [pass]
sbroute out add http <tag> <server> <port> [user] [pass]
sbroute out add vless <tag> <server> <port> <uuid> [flow] [sni]
sbroute out add vmess <tag> <server> <port> <uuid> [security]
sbroute out add direct [tag]
sbroute out add block [tag]
sbroute out add url <share-url>
sbroute out list
sbroute out del <tag>
sbroute out show <tag>

sbroute route add <tag> --domain <values>
sbroute route add <tag> --suffix <values>
sbroute route add <tag> --keyword <values>
sbroute route add <tag> --regex <values>
sbroute route add <tag> --ip-cidr <values>
sbroute route add <tag> --port <values>
sbroute route add <tag> --process <values>
sbroute route add <tag> --geoip <codes>
sbroute route add <tag> --geosite <names>
sbroute route add <tag> --ruleset <url>
sbroute route list
sbroute route del <index>
sbroute route default <tag>

sbroute dns basic
sbroute dns split
sbroute preset cn-direct
sbroute preset ads-block

sbroute apply
sbroute backup [file]
sbroute restore <file>
sbroute export
sbroute import <file>

sbroute status
sbroute start
sbroute stop
sbroute restart
sbroute log [lines]
sbroute test <tag> [url]
```

## 配置文件

SBRoute 会维护以下文件：

```text
/etc/sbroute/config.json     # 生成后的 sing-box 配置
/etc/sbroute/outbounds.json  # 出站列表
/etc/sbroute/routes.json     # 路由规则
/etc/sbroute/rulesets.json   # 规则集
/etc/sbroute/settings.json   # 默认出站、DNS 模式、TUN 设置等
/etc/sbroute/backups/        # 备份目录
```

## 注意事项

- 大多数命令会写入 `/etc/sbroute` 或控制 systemd 服务，因此需要 root 权限。
- `sbroute install` 会添加 SagerNet 的 Debian 软件源以安装 sing-box。
- 该脚本管理的是独立的 `sbroute.service`，不会主动停止、覆盖或删除已有的 `sing-box.service`。
- 导入外部分享链接、规则集或代理配置前，请确认来源可信。

## 许可证

当前仓库尚未声明开源许可证。公开使用或二次分发前，请先补充合适的许可证文件。
