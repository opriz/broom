# Broom

命令行代理工具，**不依赖 Clash**，自己实现类似 Clash 的代理能力：通过订阅链接拉取节点，在本机起 HTTP/SOCKS5 代理，支持**全局模式**和**代理模式**。

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

在 macOS 上会自动设置/取消系统 HTTP(S) 代理；退出或执行 `broom stop` 后会关闭系统代理。

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
| `broom start --auto-select` | 启动时对全部节点测速，选用延迟最低的节点 |
| `broom start --insecure` | 跳过 TLS 证书校验（证书与域名不一致时使用） |
| `broom stop` | 停止已运行的代理并关闭系统代理 |

## License

MIT
