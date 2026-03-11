package main

import (
	"context"
	"fmt"
	"net"
	"os"
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
	rootCmd.AddCommand(subCmd, startCmd, stopCmd)
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

var global, autoSelect, skipTLSVerify bool

var startCmd = &cobra.Command{
	Use:   "start",
	Short: "启动代理。默认代理模式（仅本机端口）；加 --global 为全局模式（设置系统代理）",
	RunE:  runStart,
}

func init() {
	startCmd.Flags().BoolVar(&global, "global", false, "全局模式：设置系统代理，使所有流量走代理")
	startCmd.Flags().BoolVar(&autoSelect, "auto-select", false, "自动选择节点：对全部节点测速，选用延迟最低的节点")
	startCmd.Flags().BoolVar(&skipTLSVerify, "insecure", false, "跳过 TLS 证书校验（部分机场证书与域名不一致时使用，有安全风险）")
}

func runStart(cmd *cobra.Command, args []string) error {
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

	fmt.Printf("broom 已启动，按 Ctrl+C 退出\n")
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh
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
