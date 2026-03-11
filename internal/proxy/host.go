package proxy

import (
	"encoding/base64"
	"encoding/json"
	"net/url"
	"strconv"
	"strings"
)

// isProxyServerLocalhost 判断代理 URI 的服务器地址是否为本地（127.0.0.1、::1、localhost），
// 此类节点在自动选择时通常不可用，应跳过。
func isProxyServerLocalhost(proxyURI string) bool {
	host, ok := proxyServerHost(proxyURI)
	if !ok {
		return false
	}
	h := strings.TrimSpace(strings.ToLower(host))
	return h == "127.0.0.1" || h == "::1" || h == "localhost" || h == "localhost."
}

func proxyServerHost(proxyURI string) (string, bool) {
	proxyURI = strings.TrimSpace(proxyURI)
	switch {
	case strings.HasPrefix(proxyURI, "trojan://"):
		u, err := url.Parse(proxyURI)
		if err != nil {
			return "", false
		}
		return u.Hostname(), true
	case strings.HasPrefix(proxyURI, "ss://"):
		return ssHost(proxyURI)
	case strings.HasPrefix(proxyURI, "ssr://"):
		return ssrHost(proxyURI)
	case strings.HasPrefix(proxyURI, "vmess://"):
		return vmessHost(proxyURI)
	default:
		return "", false
	}
}

func ssHost(uri string) (string, bool) {
	// ss://[base64]@host:port 或 ss://host:port#name
	uri = strings.TrimPrefix(uri, "ss://")
	if i := strings.Index(uri, "@"); i >= 0 {
		uri = uri[i+1:]
	}
	// host:port 或 [ipv6]:port
	host, _, err := netHostPort(uri)
	if err != nil {
		return "", false
	}
	return host, true
}

func ssrHost(uri string) (string, bool) {
	// ssr://base64(host:port:protocol:method:obfs:base64password/?params)
	uri = strings.TrimPrefix(uri, "ssr://")
	decoded, err := base64.RawURLEncoding.DecodeString(uri)
	if err != nil {
		decoded, err = base64.StdEncoding.DecodeString(uri)
		if err != nil {
			return "", false
		}
	}
	parts := strings.SplitN(string(decoded), "/?", 2)
	mainPart := parts[0]
	// host:port:protocol:method:obfs:base64password
	fields := strings.Split(mainPart, ":")
	if len(fields) < 2 {
		return "", false
	}
	host := fields[0]
	_, err = strconv.Atoi(fields[1])
	if err != nil {
		return "", false
	}
	return host, true
}

func vmessHost(uri string) (string, bool) {
	uri = strings.TrimPrefix(uri, "vmess://")
	decoded, err := base64.StdEncoding.DecodeString(uri)
	if err != nil {
		decoded, err = base64.RawURLEncoding.DecodeString(uri)
		if err != nil {
			return "", false
		}
	}
	var v struct {
		Add string `json:"add"`
	}
	if err := json.Unmarshal(decoded, &v); err != nil {
		return "", false
	}
	if v.Add == "" {
		return "", false
	}
	return v.Add, true
}

// netHostPort 从 "host:port" 或 "[ipv6]:port" 中解析 host
func netHostPort(hostPort string) (host, port string, err error) {
	hostPort = strings.TrimSpace(hostPort)
	if hostPort == "" {
		return "", "", strconv.ErrSyntax
	}
	if hostPort[0] == '[' {
		i := strings.Index(hostPort, "]")
		if i < 0 {
			return "", "", strconv.ErrSyntax
		}
		host = hostPort[1:i]
		if i+1 < len(hostPort) && hostPort[i+1] == ':' {
			port = hostPort[i+2:]
		}
		return host, port, nil
	}
	i := strings.LastIndex(hostPort, ":")
	if i < 0 {
		return hostPort, "", nil
	}
	host = hostPort[:i]
	port = hostPort[i+1:]
	return host, port, nil
}
