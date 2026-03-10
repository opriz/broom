package daemon

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
)

const pidFileName = "broom.pid"

func pidPath(configDir string) string {
	return filepath.Join(configDir, pidFileName)
}

// SavePID 保存当前进程 PID 到配置目录，供 stop 命令使用
func SavePID(configDir string, pid int) error {
	return os.WriteFile(pidPath(configDir), []byte(strconv.Itoa(pid)), 0644)
}

// LoadPID 读取保存的 PID，若进程存在则返回 true
func LoadPID(configDir string) (int, bool) {
	data, err := os.ReadFile(pidPath(configDir))
	if err != nil {
		return 0, false
	}
	pid, err := strconv.Atoi(string(data))
	if err != nil {
		return 0, false
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return pid, false
	}
	// 发送 signal 0 检查进程是否存活
	err = proc.Signal(syscall.Signal(0))
	return pid, err == nil
}

// Stop 向保存的 PID 发送 SIGTERM，并删除 pid 文件
func Stop(configDir string) error {
	pid, ok := LoadPID(configDir)
	if !ok {
		return fmt.Errorf("no running broom process found (or pid file missing)")
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	if err := proc.Signal(syscall.SIGTERM); err != nil {
		return err
	}
	_ = os.Remove(pidPath(configDir))
	return nil
}
