---
name: broom
description: Work on broom—a command-line (CLI-only) proxy tool compatible with Clash nodes/subscriptions. Use when modifying broom, adding proxy features, fixing subscription or TLS, or extending commands.
---

# Broom 项目 Skill

Broom 是纯**命令行**代理工具（无 GUI、不依赖桌面环境），兼容 **Clash 节点/订阅**：使用与 Clash 相同的订阅链接与节点格式，自建本地 HTTP/SOCKS5，多协议出站（Trojan 自实现，ss/vmess/ssr 用 Merkur），支持全局/代理模式与后台运行。

## 项目结构

```
broom/
├── cmd/broom/main.go           # CLI：sub add/update, start, stop；start 含 --global/--auto-select/--insecure/--daemon
├── internal/
│   ├── config/                 # broom.yaml、proxies.txt 读写；ConfigDirPath = ~/.config/broom
│   ├── daemon/                 # PID 文件（broom.pid），供 stop 发 SIGTERM
│   ├── proxy/
│   │   ├── server.go           # 本地代理：HTTP（CONNECT + 普通 GET/POST 转发）+ SOCKS5；出站经 UpstreamDialer
│   │   ├── trojan.go           # Trojan 拨号（TLS + 首包）；支持 skipTLSVerify
│   │   ├── selector.go         # SelectBest：测速、多目标回退、过滤 127.0.0.1
│   │   ├── host.go             # 从 URI 解析 proxy server host，isProxyServerLocalhost
│   │   └── vmess_insecure.go   # vmess URI 重写，写入 allowInsecure/skipCertVerify
│   ├── subscription/           # GetProxyURLs：拉订阅 → Clash YAML / Base64 解析 → []string URI
│   └── sysproxy/               # darwin: networksetup 开/关系统代理；其他(linux/windows): 仅提示用 eval $(broom env)
├── go.mod
├── README.md
├── skills/                     # 项目 SKILL，供 AI/文档参考（不依赖 .cursor 目录）
└── DEVELOPMENT.md              # 开发过程与选型说明
```

## 约定与入口

- **新增协议/出站**：在 `internal/proxy` 增加拨号实现，在 `UpstreamDialer` 中按 URI scheme 分支；Trojan 在 `trojan.go`，其余走 Merkur。
- **订阅格式**：在 `internal/subscription` 扩展；Clash 新 proxy 类型在 `clashProxy` 与 `clashProxyToURI` 中补充。
- **新 start 参数**：在 `cmd/broom/main.go` 的 start 相关 flag 与 `runStart` 中处理；若需 daemon 继承，在 `runAsDaemonParent` 的 env 里增加对应 `BROOM_*` 并在子进程里读取。
- **配置项**：在 `internal/config.BroomConfig` 增加字段，保存/加载由现有 YAML 逻辑处理。

## 构建与配置路径

```bash
go build -o broom ./cmd/broom
```

- 配置：`~/.config/broom/broom.yaml`
- 节点列表：`~/.config/broom/proxies.txt`
- PID：`~/.config/broom/broom.pid`
- 后台日志：`~/.config/broom/daemon.log`

## 关键逻辑速查

| 需求 | 位置 / 做法 |
|------|--------------|
| 改测速目标/超时 | `internal/proxy/selector.go`：DefaultTestURL、DefaultTestLimit、fallbackTestURLs |
| 跳过 TLS 校验 | `skipTLSVerify` 传至 `UpstreamDialer`；Trojan 用 `tls.Config.InsecureSkipVerify`；VMess 用 `vmessURIWithInsecure` 重写后交给 Merkur |
| 过滤本地节点 | `internal/proxy/host.go`：isProxyServerLocalhost；`selector.SelectBest` 先过滤再测速 |
| 后台运行 | `--daemon`：当前进程 re-exec 子进程并设 BROOM_DAEMON_CHILD/GLOBAL/AUTO_SELECT/INSECURE，子进程 stdout/stderr 重定向到 daemon.log |
| 命令行代理（非 macOS） | 不设桌面/系统代理；提示用户在当前 shell 执行 `eval $(broom env)` 设置 http_proxy/https_proxy/all_proxy；`broom env` 输出 export 行，端口来自 config |
| curl 走 HTTP 代理 GET | `internal/proxy/server.go`：支持 `GET http(s)://host/path` 的普通代理请求；使用 `http.Client` + `Transport.DialContext=s.Dialer` 兼容 HTTP/2；不自动跟随 3xx（curl 用 `-L`） |

## 命令行代理用法（写进 README / 文档时）

- Broom 是**命令行工具**，Linux/Windows 下不改系统或桌面代理。
- 用户启动 `broom start`（或 `broom start --global`）后，在当前 shell 执行 **`eval $(broom env)`** 即可让该会话的 curl、git、npm 等走代理。
- 取消代理：`unset http_proxy https_proxy all_proxy`。单独 `broom env` 只打印 export 行。新终端需再次执行或写入 `.bashrc`/`.zshrc`。
- 对会从 http 跳转到 https 的站点（如 `reddit.com`、`youtube.com`），用 `curl -L` 跟随重定向；broom 会把 301/302 原样返回给 curl。

## 修改时注意

- 不要依赖 Merkur 的 `ParserOrder`（未导出）；订阅解析在 `subscription` 包内完成。
- Clash YAML 的 `port` 可能为数字或字符串，使用 `subscription.flexiblePort`。
- 新增依赖请用 `go get` 并保持 `go.mod` 整洁；回复用户时优先用中文。
