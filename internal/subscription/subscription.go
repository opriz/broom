package subscription

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// GetProxyURLs 从订阅 URL 拉取并解析，返回代理 URI 列表（ss://、vmess://、ssr:// 等）
func GetProxyURLs(subscriptionURL string) ([]string, error) {
	body, err := fetch(subscriptionURL)
	if err != nil {
		return nil, err
	}
	return parseProxyList(body)
}

func fetch(url string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Clash")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("subscription returned status %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

// parseProxyList 解析订阅内容：base64 节点列表或 Clash YAML
func parseProxyList(body []byte) ([]string, error) {
	body = bytes.TrimSpace(body)
	if isClashYAML(body) {
		return parseClashYAML(body)
	}
	decoded, err := base64.StdEncoding.DecodeString(string(body))
	if err != nil {
		return nil, fmt.Errorf("subscription content is neither Clash YAML nor base64: %w", err)
	}
	// 解码后按行拆成 ss://、vmess:// 等
	var list []string
	for _, line := range strings.Split(string(decoded), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if isProxyURI(line) {
			list = append(list, line)
		}
	}
	if len(list) == 0 {
		return nil, fmt.Errorf("no proxy URIs found in subscription")
	}
	return list, nil
}

func isProxyURI(s string) bool {
	return strings.HasPrefix(s, "ss://") ||
		strings.HasPrefix(s, "ssr://") ||
		strings.HasPrefix(s, "vmess://") ||
		strings.HasPrefix(s, "trojan://")
}

func isClashYAML(b []byte) bool {
	s := string(b)
	return strings.Contains(s, "proxies:") || strings.Contains(s, "proxy-groups:")
}

// flexiblePort 兼容 Clash 里 port 为数字或字符串
type flexiblePort int

func (p *flexiblePort) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var v interface{}
	if err := unmarshal(&v); err != nil {
		return err
	}
	switch x := v.(type) {
	case int:
		*p = flexiblePort(x)
		return nil
	case string:
		n, err := strconv.Atoi(x)
		if err != nil {
			return err
		}
		*p = flexiblePort(n)
		return nil
	default:
		return fmt.Errorf("port must be int or string")
	}
}

func (p flexiblePort) Int() int { return int(p) }

// Clash 的 proxy 项（只取常用字段）
type clashProxy struct {
	Name   string       `yaml:"name"`
	Type   string       `yaml:"type"`
	Server string       `yaml:"server"`
	Port   flexiblePort `yaml:"port"`
	// SS
	Password string `yaml:"password"`
	Cipher   string `yaml:"cipher"`
	// VMess
	UUID    string `yaml:"uuid"`
	AlterID int    `yaml:"alterId"`
	// Trojan
	// Password 复用
}

func parseClashYAML(body []byte) ([]string, error) {
	var root struct {
		Proxies []clashProxy `yaml:"proxies"`
	}
	if err := yaml.Unmarshal(body, &root); err != nil {
		return nil, fmt.Errorf("parse Clash YAML: %w", err)
	}
	if len(root.Proxies) == 0 {
		return nil, fmt.Errorf("no proxies in Clash YAML")
	}
	var urls []string
	for _, p := range root.Proxies {
		u, err := clashProxyToURI(p)
		if err != nil {
			continue
		}
		urls = append(urls, u)
	}
	if len(urls) == 0 {
		return nil, fmt.Errorf("could not convert any Clash proxy to URI")
	}
	return urls, nil
}

func clashProxyToURI(p clashProxy) (string, error) {
	switch strings.ToLower(p.Type) {
	case "ss", "shadowsocks":
		return clashSSToURI(p)
	case "vmess":
		return clashVMessToURI(p)
	case "trojan":
		return clashTrojanToURI(p)
	default:
		return "", fmt.Errorf("unsupported type %s", p.Type)
	}
}

func clashSSToURI(p clashProxy) (string, error) {
	if p.Server == "" || p.Port.Int() == 0 || p.Password == "" || p.Cipher == "" {
		return "", fmt.Errorf("incomplete ss proxy")
	}
	// ss://base64(method:password)@server:port
	userInfo := p.Cipher + ":" + p.Password
	encoded := base64.StdEncoding.EncodeToString([]byte(userInfo))
	return "ss://" + encoded + "@" + p.Server + ":" + fmt.Sprintf("%d", p.Port.Int()), nil
}

func clashVMessToURI(p clashProxy) (string, error) {
	if p.Server == "" || p.Port.Int() == 0 || p.UUID == "" {
		return "", fmt.Errorf("incomplete vmess proxy")
	}
	v := map[string]interface{}{
		"v":    "2",
		"ps":   p.Name,
		"add":  p.Server,
		"port": p.Port.Int(),
		"id":   p.UUID,
		"aid":  p.AlterID,
		"net":  "tcp",
		"type": "none",
	}
	data, _ := json.Marshal(v)
	encoded := base64.StdEncoding.EncodeToString(data)
	return "vmess://" + encoded, nil
}

func clashTrojanToURI(p clashProxy) (string, error) {
	if p.Server == "" || p.Port.Int() == 0 || p.Password == "" {
		return "", fmt.Errorf("incomplete trojan proxy")
	}
	// trojan://password@server:port
	return "trojan://" + url.QueryEscape(p.Password) + "@" + p.Server + ":" + fmt.Sprintf("%d", p.Port.Int()), nil
}
