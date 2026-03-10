//go:build darwin

package sysproxy

import (
	"fmt"
	"os/exec"
	"strings"
)

const (
	DefaultHTTPPort  = 7890
	DefaultSOCKSPort = 7891
)

// GetNetworkService 获取当前主要网络服务名（如 Wi-Fi、Ethernet）
func GetNetworkService() (string, error) {
	out, err := exec.Command("networksetup", "-listallnetworkservices").Output()
	if err != nil {
		return "", err
	}
	lines := strings.Split(string(out), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "*") || line == "An asterisk (*) denotes that a network service is disabled." {
			continue
		}
		// 通常选第一个启用的服务，如 Wi-Fi
		return line, nil
	}
	return "", fmt.Errorf("no network service found")
}

// On 开启系统代理，指向本地 clash 的 HTTP 代理端口
func On(host string, port int) error {
	svc, err := GetNetworkService()
	if err != nil {
		return err
	}
	for _, state := range []string{"on", "on"} {
		_ = state
	}
	// Web (HTTP) 代理
	if err := exec.Command("networksetup", "-setwebproxy", svc, host, fmt.Sprintf("%d", port)).Run(); err != nil {
		return err
	}
	if err := exec.Command("networksetup", "-setsecurewebproxy", svc, host, fmt.Sprintf("%d", port)).Run(); err != nil {
		return err
	}
	if err := exec.Command("networksetup", "-setwebproxystate", svc, "on").Run(); err != nil {
		return err
	}
	if err := exec.Command("networksetup", "-setsecurewebproxystate", svc, "on").Run(); err != nil {
		return err
	}
	return nil
}

// Off 关闭系统代理
func Off() error {
	svc, err := GetNetworkService()
	if err != nil {
		return err
	}
	_ = exec.Command("networksetup", "-setwebproxystate", svc, "off").Run()
	_ = exec.Command("networksetup", "-setsecurewebproxystate", svc, "off").Run()
	return nil
}
