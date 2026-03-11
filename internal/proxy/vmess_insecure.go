package proxy

import (
	"encoding/base64"
	"encoding/json"
	"strings"
)

// vmessURIWithInsecure 在 vmess:// 的 base64 JSON 中加入 allowInsecure/skipCertVerify，
// 供部分客户端（如 Merkur）在 TLS 校验证书不一致时跳过校验。若解析失败则返回原 URI。
func vmessURIWithInsecure(vmessURI string) string {
	if !strings.HasPrefix(vmessURI, "vmess://") {
		return vmessURI
	}
	payload := strings.TrimPrefix(vmessURI, "vmess://")
	decoded, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
		decoded, err = base64.RawURLEncoding.DecodeString(payload)
		if err != nil {
			return vmessURI
		}
	}
	var m map[string]interface{}
	if err := json.Unmarshal(decoded, &m); err != nil {
		return vmessURI
	}
	m["allowInsecure"] = true
	m["skipCertVerify"] = true
	data, err := json.Marshal(m)
	if err != nil {
		return vmessURI
	}
	return "vmess://" + base64.StdEncoding.EncodeToString(data)
}
