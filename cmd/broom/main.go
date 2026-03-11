package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/zhujian/broom/internal/config"
	"github.com/zhujian/broom/internal/daemon"
	"github.com/zhujian/broom/internal/proxy"
	"github.com/zhujian/broom/internal/subscription"
	"github.com/zhujian/broom/internal/sysproxy"
)

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

var rootCmd = &cobra.Command{
	Use:   "broom",
	Short: "命令行代理工具，使用订阅实现代理（类似 Clash，不依赖 Clash）",
}

func init() {
	rootCmd.AddCommand(subCmd, startCmd, stopCmd, envCmd)
}

var subCmd = &cobra.Command{
	Use:   "sub",
	Short: "管理订阅",
}

var subAddCmd = &cobra.Command{
	Use:   "add [订阅URL]",
	Short: "添加或覆盖订阅地址",
	Args:  cobra.ExactArgs(1),
	RunE:  runSubAdd,
}

var subUpdateCmd = &cobra.Command{
	Use:   "update",
	Short: "拉取订阅并更新本地代理节点列表",
	RunE:  runSubUpdate,
}

func init() {
	subCmd.AddCommand(subAddCmd, subUpdateCmd)
}

func runSubAdd(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	cfg.SubscriptionURL = args[0]
	if err := config.Save(cfg); err != nil {
		return err
	}
	fmt.Println("订阅地址已保存:", args[0])
	return nil
}

func runSubUpdate(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if cfg.SubscriptionURL == "" {
		return fmt.Errorf("请先使用 broom sub add <订阅URL> 配置订阅地址")
	}
	urls, err := subscription.GetProxyURLs(cfg.SubscriptionURL)
	if err != nil {
		return err
	}
	if err := config.SaveProxies(urls); err != nil {
		return err
	}
	fmt.Printf("订阅已更新，共 %d 个节点，已写入本地\n", len(urls))
	return nil
}

var global, autoSelect, skipTLSVerify, daemonMode bool

var startCmd = &cobra.Command{
	Use:   "start",
	Short: "启动代理。默认代理模式（仅本机端口）；加 --global 为全局模式（设置系统代理）",
	RunE:  runStart,
}

func init() {
	startCmd.Flags().BoolVar(&global, "global", false, "全局模式：设置系统代理，使所有流量走代理")
	startCmd.Flags().BoolVar(&autoSelect, "auto-select", false, "自动选择节点：对全部节点测速，选用延迟最低的节点")
	startCmd.Flags().BoolVar(&skipTLSVerify, "insecure", false, "跳过 TLS 证书校验（部分机场证书与域名不一致时使用，有安全风险）")
	startCmd.Flags().BoolVar(&daemonMode, "daemon", false, "后台服务方式运行（脱离终端，可用 broom stop 停止）")
}

func runStart(cmd *cobra.Command, args []string) error {
	// 若以 --daemon 启动且当前不是已拉起的子进程，则 re-exec 为后台进程后退出
	if daemonMode && os.Getenv("BROOM_DAEMON_CHILD") != "1" {
		return runAsDaemonParent()
	}
	// 从环境恢复 daemon 子进程的启动参数
	if os.Getenv("BROOM_DAEMON_CHILD") == "1" {
		global = os.Getenv("BROOM_GLOBAL") == "1"
		autoSelect = os.Getenv("BROOM_AUTO_SELECT") == "1"
		skipTLSVerify = os.Getenv("BROOM_INSECURE") == "1"
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	cfg.EnsurePorts()
	if autoSelect {
		cfg.AutoSelectNode = true
	}
	if skipTLSVerify {
		cfg.SkipTLSVerify = true
	}

	// 加载代理列表（若没有则尝试从订阅拉取）
	urls, err := config.LoadProxies()
	if err != nil || len(urls) == 0 {
		if cfg.SubscriptionURL == "" {
			return fmt.Errorf("请先执行 broom sub add <订阅URL> 和 broom sub update")
		}
		urls, err = subscription.GetProxyURLs(cfg.SubscriptionURL)
		if err != nil {
			return fmt.Errorf("拉取订阅失败: %w", err)
		}
		_ = config.SaveProxies(urls)
	}

	// 选用上游代理：自动选择时测速取最快，否则取第一个可用
	var dialer func(ctx context.Context, network, addr string) (net.Conn, error)
	var chosenURI string
	if cfg.AutoSelectNode {
		testURL := cfg.TestURL
		if testURL == "" {
			testURL = proxy.DefaultTestURL
		}
		best, latency, err := proxy.SelectBest(urls, testURL, 20*time.Second, cfg.SkipTLSVerify)
		if err != nil {
			return fmt.Errorf("自动选择节点失败: %w", err)
		}
		chosenURI = best
		dialer, err = proxy.UpstreamDialer(best, cfg.SkipTLSVerify)
		if err != nil {
			return fmt.Errorf("使用选中节点失败: %w", err)
		}
		fmt.Printf("自动选择节点完成，延迟 %v，共 %d 个节点\n", latency.Round(time.Millisecond), len(urls))
	} else {
		for _, u := range urls {
			d, err := proxy.UpstreamDialer(u, cfg.SkipTLSVerify)
			if err != nil {
				continue
			}
			dialer = d
			chosenURI = u
			break
		}
	}
	if dialer == nil {
		return fmt.Errorf("订阅中无受支持的代理节点（broom 支持 ss://、vmess://、ssr://、trojan://）")
	}
	_ = chosenURI

	svc := &proxy.Server{
		Dialer:    dialer,
		HTTPPort:  cfg.HTTPPort,
		SOCKSPort: cfg.SOCKSPort,
	}
	if err := svc.Listen(); err != nil {
		return err
	}
	defer svc.Close()

	configDir, _ := config.ConfigDirPath()
	if err := daemon.SavePID(configDir, os.Getpid()); err != nil {
		return err
	}
	defer os.Remove(filepath.Join(configDir, "broom.pid"))

	if global {
		if err := sysproxy.On("127.0.0.1", cfg.HTTPPort); err != nil {
			fmt.Fprintf(os.Stderr, "设置系统代理失败（macOS 可能需 sudo）: %v\n", err)
		} else {
			fmt.Println("已开启系统代理（全局模式）")
		}
		defer sysproxy.Off()
	} else {
		fmt.Printf("代理模式：请将应用代理设为 HTTP 127.0.0.1:%d 或 SOCKS5 127.0.0.1:%d\n", cfg.HTTPPort, cfg.SOCKSPort)
	}

	if os.Getenv("BROOM_DAEMON_CHILD") != "1" {
		fmt.Printf("broom 已启动，按 Ctrl+C 退出\n")
	}
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh
	return nil
}

// runAsDaemonParent 以当前进程拉起后台子进程后退出；子进程通过环境变量继承 global/auto-select/insecure。
func runAsDaemonParent() error {
	executable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("获取可执行路径失败: %w", err)
	}
	configDir, err := config.ConfigDirPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return err
	}
	logPath := filepath.Join(configDir, "daemon.log")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("打开日志文件失败: %w", err)
	}
	defer logFile.Close()

	env := append(os.Environ(),
		"BROOM_DAEMON_CHILD=1",
		"BROOM_GLOBAL="+boolEnv(global),
		"BROOM_AUTO_SELECT="+boolEnv(autoSelect),
		"BROOM_INSECURE="+boolEnv(skipTLSVerify),
	)
	c := exec.Cmd{
		Path:   executable,
		Args:   []string{filepath.Base(executable), "start"},
		Env:    env,
		Dir:    configDir,
		Stdout: logFile,
		Stderr: logFile,
	}
	if err := c.Start(); err != nil {
		return fmt.Errorf("后台启动失败: %w", err)
	}
	// 子进程已脱离，本进程退出；停止用 broom stop
	fmt.Printf("broom 已在后台启动，PID %d\n", c.Process.Pid)
	fmt.Printf("日志: %s\n", logPath)
	fmt.Println("停止: broom stop")
	os.Exit(0)
	return nil
}

func boolEnv(b bool) string {
	if b {
		return "1"
	}
	return "0"
}

var envCmd = &cobra.Command{
	Use:   "env",
	Short: "Output shell export for proxy (eval $(broom env) to enable in current shell)",
	RunE:  runEnv,
}

func runEnv(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	cfg.EnsurePorts()
	fmt.Printf("export http_proxy=http://127.0.0.1:%d https_proxy=http://127.0.0.1:%d all_proxy=socks5://127.0.0.1:%d\n",
		cfg.HTTPPort, cfg.HTTPPort, cfg.SOCKSPort)
	return nil
}

var stopCmd = &cobra.Command{
	Use:   "stop",
	Short: "停止已运行的代理并关闭系统代理（若曾开启全局模式）",
	RunE:  runStop,
}

func runStop(cmd *cobra.Command, args []string) error {
	configDir, err := config.ConfigDirPath()
	if err != nil {
		return err
	}
	_ = sysproxy.Off()
	if err := daemon.Stop(configDir); err != nil {
		return err
	}
	fmt.Println("代理已停止")
	return nil
}
