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
```

## 支持的协议

- **Shadowsocks**（`ss://`）
- **VMess**（`vmess://`）
- **ShadowsocksR**（`ssr://`）

订阅中若包含 Trojan 等其它类型，会尝试转换；实际出站依赖 [Merkur](https://github.com/Qingluan/merkur) 的解析与拨号能力。

## 命令一览

| 命令 | 说明 |
|------|------|
| `broom sub add <URL>` | 添加/覆盖订阅地址 |
| `broom sub update` | 拉取订阅并更新本地节点列表 |
| `broom start` | 启动代理（代理模式） |
| `broom start --global` | 启动代理并设为系统代理（全局模式） |
| `broom stop` | 停止已运行的代理并关闭系统代理 |

## License

MIT
