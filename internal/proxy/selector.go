package proxy

import (
	"context"
	"fmt"
	"sync"
	"time"
)

const (
	DefaultTestURL   = "1.1.1.1:443"
	DefaultTestLimit = 20 * time.Second
)

// 默认测速失败时依次尝试的备用目标（部分网络/节点对单一目标不通）
var fallbackTestURLs = []string{"1.1.1.1:443", "www.gstatic.com:443", "connect.qq.com:443", "www.bing.com:443"}

// SelectBest 对多个代理节点测速，返回延迟最低且可用的节点 URI；若均失败则返回错误。
// 自动跳过服务器为 127.0.0.1/localhost 的节点。skipTLSVerify 为 true 时 Trojan 连接不校验证书。
func SelectBest(proxyURLs []string, testURL string, timeout time.Duration, skipTLSVerify bool) (bestURI string, latency time.Duration, err error) {
	if timeout <= 0 {
		timeout = DefaultTestLimit
	}
	// 过滤掉代理服务器为本机的节点，避免 dial 127.0.0.1:xxx connection refused
	candidates := make([]string, 0, len(proxyURLs))
	for _, u := range proxyURLs {
		if !isProxyServerLocalhost(u) {
			candidates = append(candidates, u)
		}
	}
	if len(candidates) == 0 {
		return "", 0, fmt.Errorf("无可用节点（已跳过 %d 个本地/127.0.0.1 节点）", len(proxyURLs))
	}
	targets := []string{testURL}
	if testURL == "" {
		targets = fallbackTestURLs
	}
	var firstErr error
	for _, target := range targets {
		bestURI, latency, err = selectBestOne(candidates, target, timeout, skipTLSVerify)
		if err == nil {
			return bestURI, latency, nil
		}
		if firstErr == nil {
			firstErr = err
		}
	}
	if firstErr != nil {
		return "", 0, firstErr
	}
	return "", 0, fmt.Errorf("所有节点测速均失败或超时")
}

func selectBestOne(proxyURLs []string, testURL string, timeout time.Duration, skipTLSVerify bool) (bestURI string, latency time.Duration, err error) {
	type result struct {
		uri     string
		latency time.Duration
		err     error
	}
	ch := make(chan result, len(proxyURLs))
	var wg sync.WaitGroup
	for _, u := range proxyURLs {
		u := u
		wg.Add(1)
		go func() {
			defer wg.Done()
			dialer, dialErr := UpstreamDialer(u, skipTLSVerify)
			if dialErr != nil {
				ch <- result{uri: u, err: dialErr}
				return
			}
			// 每个节点单独 20s 超时；底层 Dial 可能不支持 context，用带超时的 context 至少能避免无限等
			ctx, cancel := context.WithTimeout(context.Background(), timeout)
			defer cancel()
			start := time.Now()
			conn, dialErr := dialer(ctx, "tcp", testURL)
			elapsed := time.Since(start)
			if dialErr != nil {
				ch <- result{uri: u, err: dialErr}
				return
			}
			conn.Close()
			ch <- result{uri: u, latency: elapsed}
		}()
	}
	go func() {
		wg.Wait()
		close(ch)
	}()
	var best result
	best.latency = time.Duration(1<<63 - 1)
	var firstErr error
	ok := false
	for r := range ch {
		if r.err != nil {
			if firstErr == nil {
				firstErr = r.err
			}
			continue
		}
		ok = true
		if r.latency < best.latency {
			best = r
		}
	}
	if !ok {
		if firstErr != nil {
			return "", 0, fmt.Errorf("所有节点测速均失败，首个错误: %w", firstErr)
		}
		return "", 0, fmt.Errorf("所有节点测速均失败或超时")
	}
	return best.uri, best.latency, nil
}
