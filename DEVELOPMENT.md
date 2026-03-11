# Broom 开发过程总结

本文档记录 broom 从需求到当前版本的开发过程、技术选型与遇到的问题及解决方案。

---

## 一、需求与目标

- **产品形态**：命令行代理软件，使用 Clash 风格的订阅链接实现代理，项目名为 broom。
- **核心功能**（首版）：
  1. **配置订阅**：支持添加、更新订阅地址，拉取节点列表。
  2. **全局模式**：设置系统代理，使本机流量经 broom 转发。
  3. **代理模式**：仅开启本地代理端口，由应用自行配置是否走代理。
- **明确不做**：首版不实现规则分流、节点分组等，只做「订阅 + 出站 + 模式切换」。

---

## 二、技术路线变更

### 2.1 第一版：基于 Clash 可执行文件

- **思路**：broom 作为「订阅管理 + 配置生成 + 进程控制」的前端，实际代理由本机已安装的 Clash 完成。
- **实现**：
  - 拉取订阅 → 解析/转换为 Clash YAML 配置 → 写入 `~/.config/broom/clash.yaml`。
  - 启动/停止 Clash 进程，通过 `networksetup`（macOS）设置/取消系统代理。
- **问题**：用户希望「实现和 Clash 类似的功能，而不是使用 Clash」，即不依赖 Clash 二进制。

### 2.2 第二版：自建代理核心（当前架构）

- **思路**：broom 自己实现「本地 HTTP/SOCKS5 代理 + 多协议出站」，不依赖 Clash。
- **选型**：
  - **协议与出站**：使用 [Merkur](https://github.com/Qingluan/merkur) 作为拨号层，支持 `ss://`、`vmess://`、`ssr://`；Trojan 在项目内自实现（TLS + 首包协议）。
  - **本地代理**：自实现最小可用版 HTTP CONNECT 与 SOCKS5（无认证、仅 CONNECT），监听端口后统一用「上游 Dialer」出站。
- **结果**：运行时不再需要安装 Clash，仅需 Go 编译出的 broom 二进制。

---

## 三、订阅与配置

### 3.1 订阅解析

- **支持格式**：
  - **Clash YAML**：含 `proxies:` 的配置，解析每个 `type`（ss / vmess / trojan）并转成对应 URI。
  - **Base64 节点列表**：解码后按行拆成 `ss://`、`vmess://`、`ssr://`、`trojan://`。
- **实现细节**：
  - 曾尝试用 Merkur 的 `ParserOrder(subscriptionURL)` 解析订阅，该 API 未导出，改为在项目内实现：HTTP 拉取 → 判断 YAML / Base64 → 解析并输出 URI 列表。
  - Clash 中 `port` 可能为数字或字符串，增加 `flexiblePort` 类型做 YAML 反序列化兼容。

### 3.2 配置与持久化

- **`~/.config/broom/broom.yaml`**：订阅 URL、HTTP/SOCKS 端口、自动选择/测速相关、是否跳过 TLS 校验等。
- **`~/.config/broom/proxies.txt`**：当前节点 URI 列表（每行一个），由 `sub update` 或首次 `start` 拉取后写入，供后续启动直接使用。

---

## 四、代理核心实现

### 4.1 本地代理服务（`internal/proxy/server.go`）

- **HTTP 代理**：监听 `http_port`（默认 7890），对 `CONNECT` 请求用上游 Dialer 连目标地址并做双向转发；非 CONNECT 返回提示。
- **SOCKS5**：监听 `socks_port`（默认 7891），实现无认证、仅 CONNECT 的最小 SOCKS5，同样用上游 Dialer 出站。
- **上游 Dialer**：由 `UpstreamDialer(proxyURI, skipTLSVerify)` 根据 URI 类型返回：
  - `trojan://` → 自实现 Trojan 拨号（TLS + 首包）；
  - 其余 → Merkur `NewProxyDialer(uri)`。

### 4.2 Trojan 支持（`internal/proxy/trojan.go`）

- **原因**：订阅中大量为 Trojan 节点，Merkur 不支持，需自行实现。
- **协议**：TLS 连接至 `server:port`，首包为 `hex(SHA224(password))\r\n` + Trojan 请求（CMD+ATYP+目标地址+端口）`\r\n`，之后为透明转发。
- **证书**：支持通过 `skipTLSVerify` 控制是否校验服务端证书（`InsecureSkipVerify`），以兼容证书与连接域名不一致的机场。

### 4.3 系统代理（`internal/sysproxy`）

- **macOS**：通过 `networksetup` 设置/取消 HTTP(S) 系统代理；全局模式开启时写入本机地址与端口，退出或 `stop` 时恢复。
- **非 macOS**：仅打印提示，不修改系统代理，由用户自行在系统或应用中配置。

---

## 五、功能迭代与问题修复

### 5.1 自动选择节点

- **需求**：订阅多节点时，自动选用延迟较低的节点。
- **实现**：
  - 对节点列表（已排除 127.0.0.1/localhost）并发向测速目标建 TCP 连接，测延迟；
  - 支持多个测速目标回退（如 `1.1.1.1:443`、`www.gstatic.com:443`、`connect.qq.com:443` 等），避免单一目标在某些网络下不可达；
  - 超时与默认测速目标在 `internal/proxy/selector.go` 中可配置。
- **配置**：`auto_select_node` 或命令行 `--auto-select`；可选 `test_url` 指定测速目标。

### 5.2 测速失败：「所有节点测速均失败」

- **现象**：自动选择时报「所有节点测速均失败」或超时，而 Clash 中节点可用。
- **原因与处理**：
  1. **订阅返回格式**：部分订阅仅含 Trojan 或 Clash YAML，需正确解析并转 URI；同时兼容 `port` 为字符串的 Clash 配置。
  2. **测速目标**：默认曾用 `www.gstatic.com:443`，在部分环境存在 DNS 或可达性问题；改为优先使用 IP（如 `1.1.1.1:443`），并增加多目标回退。
  3. **本地节点**：订阅中混有 `127.0.0.1`/localhost 节点，测速会连本机导致 connection refused。在测速前按 URI 解析出 proxy server host，过滤掉本机地址再测速。
  4. **TLS 证书**：节点证书的 CN/SNI 与连接域名不一致（如证书为 `openssl.nodesni.com`，连接为 `cave-hk.xxx.com`），导致 `x509: certificate is valid for ... not ...`。见下节。

### 5.3 跳过 TLS 证书校验（`--insecure` / `skip_tls_verify`）

- **需求**：在证书与域名不一致的机场环境下仍能连上节点（用户自行承担安全风险）。
- **实现**：
  - **Trojan**：在自实现 TLS Client 中根据 `skipTLSVerify` 设置 `tls.Config.InsecureSkipVerify`。
  - **VMess**：Merkur 内部做 TLS，无法直接改配置；在传入 Merkur 前对 `vmess://` 链接做重写：Base64 解码 JSON → 写入 `allowInsecure` / `skipCertVerify` → 再编码，若 Merkur 识别这些字段则可跳过校验。
- **入口**：配置项 `skip_tls_verify: true` 或命令行 `--insecure`，与 `--global`、`--auto-select` 可组合使用。

---

## 六、项目结构概览

```
broom/
├── cmd/broom/main.go          # CLI 入口，sub / start / stop
├── internal/
│   ├── config/                # 配置与 proxies 文件读写
│   ├── daemon/                 # PID 文件，供 stop 杀进程
│   ├── proxy/                  # 代理核心
│   │   ├── server.go           # HTTP + SOCKS5 服务与 UpstreamDialer
│   │   ├── trojan.go           # Trojan 拨号
│   │   ├── selector.go         # 自动选择（测速与回退）
│   │   ├── host.go             # 从 URI 解析 host，过滤 localhost
│   │   └── vmess_insecure.go   # VMess 链接写入 allowInsecure
│   ├── subscription/           # 订阅拉取与解析（YAML/Base64 → URI 列表）
│   └── sysproxy/               # 系统代理（darwin / 其他）
├── go.mod
├── README.md
└── DEVELOPMENT.md              # 本文档
```

---

## 七、依赖与版本

- **Go**：1.21+
- **主要依赖**：
  - `github.com/Qingluan/merkur`：ss / vmess / ssr 出站拨号；
  - `github.com/spf13/cobra`：命令行；
  - `gopkg.in/yaml.v3`：配置与 Clash YAML 解析。
- **标准库**：`net`、`crypto/tls`、`encoding/base64`、`encoding/json` 等。

---

## 八、后续可扩展方向

- 规则分流（按域名/IP 走代理或直连）；
- 定时/后台刷新订阅与重新测速；
- 节点分组与手动切换；
- 其他协议（如 VLESS、Hysteria）的拨号支持。

---

*文档随项目迭代更新，最后整理于当前代码状态。*
