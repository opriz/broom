//go:build !darwin

package sysproxy

import (
	"fmt"
)

// On 非 macOS 时仅打印命令行代理环境变量说明，不修改系统/桌面设置
func On(host string, port int) error {
	fmt.Printf("CLI proxy: run  eval $(broom env)  in this shell to set http_proxy/https_proxy/all_proxy.\n")
	return nil
}

// Off 无操作（env 需用户 unset）
func Off() error {
	fmt.Println("To disable CLI proxy: unset http_proxy https_proxy all_proxy")
	return nil
}
