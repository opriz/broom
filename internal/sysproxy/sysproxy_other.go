//go:build !darwin

package sysproxy

import (
	"fmt"
)

// On 非 macOS 暂仅打印提示，不修改系统代理
func On(host string, port int) error {
	fmt.Printf("System proxy not auto-set on this OS. Please set HTTP proxy to %s:%d manually.\n", host, port)
	return nil
}

// Off 非 macOS 无操作
func Off() error {
	return nil
}
