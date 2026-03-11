# Broom

A **command-line** proxy tool compatible with Clash nodes/subscriptions: uses the same subscription URLs and node formats as Clash, **does not depend on Clash**, runs a local HTTP/SOCKS5 proxy, and supports **global** and **proxy** modes. No GUI—CLI only.

## How it works

- **Subscription URL**: Fetches a node list. Common formats:
  - **Base64**: Decoded to `ss://`, `vmess://`, `ssr://`, etc. (one per line).
  - **Clash YAML**: YAML with `proxies:`; broom parses and converts to the above URIs.
- **Local proxy**: Broom listens on HTTP (default 7890) and SOCKS5 (7891), and forwards traffic via the chosen upstream node.
- **Global mode**: Sets system proxy to `127.0.0.1:7890` so all system traffic goes through broom.
- **Proxy mode**: Does not set system proxy; only apps configured to use the local proxy (e.g. `127.0.0.1:7890`) use it.

## Requirements

- **Go 1.21+** (for building only)
- No need to install Clash or any other proxy core at runtime

**Platforms**: Builds and runs on **macOS**, **Linux**, and Windows (CLI only, no desktop dependency). **Global mode**: on macOS we set system proxy via `networksetup`; on Linux and Windows use **CLI proxy** in your shell: run `eval $(broom env)` after `broom start` so `http_proxy`/`https_proxy`/`all_proxy` apply to that shell (see **CLI proxy** below). Proxy mode works everywhere.

## Install

```bash
git clone https://github.com/zhujian/broom.git
cd broom
go build -o broom ./cmd/broom
# Optional: add to PATH
sudo mv broom /usr/local/bin/
```

## Setup

### 1. Add subscription URL

```bash
broom sub add "https://your-subscription-url"
```

Supports:

- **Base64 node list** (`ss://`, `vmess://`, `ssr://`, etc., one per line).
- **Clash YAML** (with `proxies:`); broom parses and converts to node URIs.

### 2. Update subscription (fetch latest nodes)

```bash
broom sub update
```

Nodes are saved to `~/.config/broom/proxies.txt` and used on next start.

## Usage

### Proxy mode (local proxy only, no system proxy)

```bash
broom start
```

Set your app/browser proxy to:

- **HTTP**: `127.0.0.1:7890`
- **SOCKS5**: `127.0.0.1:7891`

### Global mode (system proxy)

```bash
broom start --global
```

On **macOS**, system HTTP(S) proxy is set automatically; it is cleared on exit or `broom stop`. On **Linux/Windows** (CLI-only), use `eval $(broom env)` in your shell to set `http_proxy`/`https_proxy`/`all_proxy` for that session.

### Run as daemon (background)

```bash
broom start --daemon
broom start --global --auto-select --insecure --daemon
```

Logs go to `~/.config/broom/daemon.log`. Stop with `broom stop`.

### Auto-select fastest node

```bash
broom start --auto-select
broom start --global --auto-select
```

Or in `~/.config/broom/broom.yaml`:

```yaml
auto_select_node: true
# test_url: "www.gstatic.com:443"  # optional
```

Otherwise broom uses the first available node.

### Skip TLS certificate verification

If you see `x509: certificate is valid for ... not ...`, enable skip verify (less secure; use on trusted networks only):

```bash
broom start --insecure
broom start --global --auto-select --insecure
```

Or in config:

```yaml
skip_tls_verify: true
```

### CLI proxy (Linux / Windows / no desktop)

Broom is a **command-line tool**; it does not set desktop or system proxy on Linux/Windows. To make the current shell use the proxy (e.g. for `curl`, `git`, `npm`), run after starting broom:

```bash
eval $(broom env)
```

This sets `http_proxy`, `https_proxy`, and `all_proxy` (SOCKS5) to `127.0.0.1` with ports from your config. To disable: `unset http_proxy https_proxy all_proxy`. Use `broom env` alone to just print the export line. For a new terminal session, run `eval $(broom env)` again or add it to your shell rc (e.g. `.bashrc`).
</think>
正在更新中文版 README 的对应说明：
<｜tool▁calls▁begin｜><｜tool▁call▁begin｜>
Grep

- **Foreground**: press **Ctrl+C** in the terminal running `broom start`.
- **Daemon or other terminal**: run `broom stop`.

## Config

- Config file: `~/.config/broom/broom.yaml`
- Node list: `~/.config/broom/proxies.txt`

Example:

```yaml
subscription_url: "https://..."
http_port: 7890
socks_port: 7891
auto_select_node: false
test_url: "www.gstatic.com:443"
skip_tls_verify: false
```

## Supported protocols

- **Shadowsocks** (`ss://`)
- **VMess** (`vmess://`)
- **ShadowsocksR** (`ssr://`)
- **Trojan** (`trojan://`)

ss/vmess/ssr use [Merkur](https://github.com/Qingluan/merkur); Trojan is built-in.

## Commands

| Command | Description |
|---------|-------------|
| `broom sub add <URL>` | Add or overwrite subscription URL |
| `broom sub update` | Fetch subscription and update local node list |
| `broom start` | Start proxy (proxy mode) |
| `broom start --global` | Start and set system proxy (global mode) |
| `broom start --daemon` | Run in background (log: ~/.config/broom/daemon.log) |
| `broom start --auto-select` | Speed-test nodes and use fastest |
| `broom start --insecure` | Skip TLS certificate verification |
| `broom env` | Print `export` line for proxy (use with `eval $(broom env)`) |
| `broom stop` | Stop proxy and clear system proxy |

## License

MIT

---

# 中文

**Broom** 是纯**命令行**代理工具，兼容 **Clash 节点/订阅**：使用与 Clash 相同的订阅链接与节点格式，**不依赖 Clash**，无图形界面，自建本地 HTTP/SOCKS5 代理，支持**全局模式**和**代理模式**。

## 原理简述

- **订阅链接**：访问后返回节点列表。常见格式有：
  - **Base64**：内容为 base64 编码的 `ss://`、`vmess://`、`ssr://` 等，每行一个；
  - **Clash YAML**：含 `proxies:` 的 YAML，broom 会解析并转成上述 URI。
- **本地代理**：broom 在本地监听 HTTP 代理（默认 7890）和 SOCKS5（默认 7891），收到请求后用选中的上游节点（ss/vmess/ssr）建连并转发流量。
- **全局模式**：把系统代理设为 `127.0.0.1:7890`，所有走系统代理的流量经 broom 转发。
- **代理模式**：不改系统代理，只开本地端口，由各应用自行配置代理（如浏览器填 127.0.0.1:7890）。

## 依赖

- **Go 1.21+**（仅编译时需要）
- 运行时**不需要**安装 Clash 或其它代理核心

**平台**：支持 **macOS**、**Linux**、Windows 编译运行（仅命令行，不依赖桌面）。**全局模式**：macOS 下用 `networksetup` 设置系统代理；Linux/Windows 下在终端内执行 `eval $(broom env)` 为当前 shell 设置 `http_proxy`/`https_proxy`/`all_proxy`（见下文「命令行代理」）。代理模式在各平台均可用。

## 安装

```bash
git clone https://github.com/zhujian/broom.git
cd broom
go build -o broom ./cmd/broom
# 可选：加入 PATH
sudo mv broom /usr/local/bin/
```

## 配置

### 1. 添加订阅地址

```bash
broom sub add "https://你的订阅链接"
```

支持两种常见订阅：

- 返回 **Base64 节点列表**（`ss://`、`vmess://`、`ssr://` 等，每行一个）；
- 返回 **Clash 格式 YAML**（含 `proxies:`），broom 会解析并转换为节点 URI。

### 2. 更新订阅（拉取最新节点并写入本地）

```bash
broom sub update
```

节点会保存到 `~/.config/broom/proxies.txt`，启动时优先从这里读取，避免每次启动都请求订阅。

## 使用

### 代理模式（只开本地代理，不设系统代理）

```bash
broom start
```

把浏览器或应用的代理设为：

- **HTTP 代理**：`127.0.0.1:7890`
- **SOCKS5**：`127.0.0.1:7891`

### 全局模式（系统代理，所有流量走代理）

```bash
broom start --global
```

在 **macOS** 上会自动设置/取消系统 HTTP(S) 代理；退出或执行 `broom stop` 后会关闭系统代理。在 **Linux/Windows**（仅命令行）下，在当前 shell 执行 `eval $(broom env)` 即可为该会话设置 `http_proxy`/`https_proxy`/`all_proxy`。

### 命令行代理（Linux / Windows / 无桌面）

Broom 是**命令行工具**，在 Linux/Windows 下不会自动改系统或桌面代理。若只用终端，可在启动 broom 后于当前 shell 执行：

```bash
eval $(broom env)
```

会按当前配置的端口设置 `http_proxy`、`https_proxy`、`all_proxy`（SOCKS5），该终端里的 curl、git、npm 等会走代理。取消代理：`unset http_proxy https_proxy all_proxy`。单独执行 `broom env` 仅打印 export 行。新开终端需再次执行，或把 `eval $(broom env)` 写入 `.bashrc`/`.zshrc`。

### 后台服务方式运行

不想占着终端时，可加 `--daemon` 以后台方式启动，进程会脱离当前终端继续运行，日志写入 `~/.config/broom/daemon.log`，停止仍用 `broom stop`：

```bash
broom start --daemon
broom start --global --auto-select --insecure --daemon
```

### 自动选择节点

订阅中若有多个节点，可让 broom 在启动时对全部节点测速（TCP 连接延迟），自动选用**延迟最低**的节点：

```bash
broom start --auto-select
broom start --global --auto-select
```

或在配置文件中开启（`~/.config/broom/broom.yaml`）：

```yaml
auto_select_node: true
# test_url: "www.gstatic.com:443"  # 可选，测速目标，默认即为此
```

未开启时，broom 使用节点列表中的**第一个可用**节点。

### 跳过 TLS 证书校验

部分机场的节点证书与连接域名不一致（如证书为 `openssl.nodesni.com`、连接为 `cave-hk.xxx.com`），会报 `x509: certificate is valid for ... not ...`。可开启「跳过证书校验」以兼容（有中间人风险，仅建议在可信网络下使用）。Trojan 会直接跳过 TLS 校验，VMess 会在链接中写入 `allowInsecure` 供底层库使用：

```bash
broom start --insecure
broom start --global --auto-select --insecure
```

或在配置中设置：

```yaml
skip_tls_verify: true
```

### 停止代理

- 在运行 `broom start` 的终端按 **Ctrl+C** 退出；
- 若在后台运行或从另一终端操作：

```bash
broom stop
```

会结束 broom 进程并关闭系统代理（若之前开了全局模式）。

## 配置说明

- 配置文件：`~/.config/broom/broom.yaml`
- 节点列表：`~/.config/broom/proxies.txt`（由 `broom sub update` 或首次 `broom start` 时拉取订阅生成）

可选配置示例：

```yaml
subscription_url: "https://..."
http_port: 7890
socks_port: 7891
auto_select_node: false   # 设为 true 则启动时测速并选用最快节点
test_url: "www.gstatic.com:443"  # 自动选择时的测速目标（可选）
skip_tls_verify: false    # 设为 true 可跳过 TLS 证书校验（部分机场需开启）
```

## 支持的协议

- **Shadowsocks**（`ss://`）
- **VMess**（`vmess://`）
- **ShadowsocksR**（`ssr://`）
- **Trojan**（`trojan://`）

其中 ss/vmess/ssr 出站依赖 [Merkur](https://github.com/Qingluan/merkur)，trojan 为内置实现。

## 命令一览

| 命令 | 说明 |
|------|------|
| `broom sub add <URL>` | 添加/覆盖订阅地址 |
| `broom sub update` | 拉取订阅并更新本地节点列表 |
| `broom start` | 启动代理（代理模式） |
| `broom start --global` | 启动代理并设为系统代理（全局模式） |
| `broom start --daemon` | 后台服务方式运行（脱离终端，日志见 ~/.config/broom/daemon.log） |
| `broom start --auto-select` | 启动时对全部节点测速，选用延迟最低的节点 |
| `broom start --insecure` | 跳过 TLS 证书校验（证书与域名不一致时使用） |
| `broom env` | 输出代理环境变量 export 行（配合 `eval $(broom env)` 使用） |
| `broom stop` | 停止已运行的代理并关闭系统代理 |

## License

MIT
