package proxy

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/Qingluan/merkur"
)

// Server 本地代理服务：SOCKS5 + HTTP CONNECT，出站流量通过 upstream 拨号器转发
type Server struct {
	Dialer    func(ctx context.Context, network, addr string) (net.Conn, error)
	HTTPPort  int
	SOCKSPort int
	httpLn    net.Listener
	socksLn   net.Listener
	mu        sync.Mutex
}

// Listen 开始监听 HTTP 与 SOCKS5 端口
func (s *Server) Listen() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	var err error
	s.httpLn, err = net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", s.HTTPPort))
	if err != nil {
		return fmt.Errorf("http listen: %w", err)
	}
	s.socksLn, err = net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", s.SOCKSPort))
	if err != nil {
		s.httpLn.Close()
		return fmt.Errorf("socks listen: %w", err)
	}
	go s.serveHTTP()
	go s.serveSOCKS5()
	return nil
}

// Close 关闭所有监听
func (s *Server) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	var err1, err2 error
	if s.httpLn != nil {
		err1 = s.httpLn.Close()
		s.httpLn = nil
	}
	if s.socksLn != nil {
		err2 = s.socksLn.Close()
		s.socksLn = nil
	}
	if err1 != nil {
		return err1
	}
	return err2
}

func (s *Server) serveHTTP() {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodConnect {
			s.handleConnect(w, r)
			return
		}
		// 通过 HTTP 代理的普通请求：GET http://host/path → 用上游连 host，转发 GET /path
		if r.URL != nil && r.URL.Host != "" && (r.URL.Scheme == "http" || r.URL.Scheme == "https") {
			s.handleHTTPForward(w, r)
			return
		}
		http.Error(w, "broom: use CONNECT or SOCKS5", http.StatusBadRequest)
	})
	_ = http.Serve(s.httpLn, handler)
}

// handleHTTPForward 处理通过代理的普通 HTTP 请求（GET http(s)://host/path），经上游连目标并转发；支持 HTTP/1.1 与 HTTP/2
func (s *Server) handleHTTPForward(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	// 目标 URL：GET http://host/ 时按 HTTPS 请求，以兼容仅 HTTPS 或仅 HTTP/2 的站点
	targetURL := *r.URL
	if targetURL.Scheme == "" {
		targetURL.Scheme = "https"
	}
	if targetURL.Scheme == "http" && !strings.Contains(targetURL.Host, ":") {
		targetURL.Scheme = "https"
	}
	if targetURL.Host == "" {
		http.Error(w, "missing host", http.StatusBadRequest)
		return
	}
	if !strings.Contains(targetURL.Host, ":") {
		port := "443"
		if targetURL.Scheme == "http" {
			port = "80"
		}
		targetURL.Host = net.JoinHostPort(targetURL.Host, port)
	}
	req := r.Clone(ctx)
	req.URL = &targetURL
	req.RequestURI = ""

	tr := &http.Transport{
		DialContext:     s.Dialer,
		Proxy:           nil,
		IdleConnTimeout: 15 * time.Second,
	}
	client := &http.Client{
		Transport: tr,
		Timeout:   25 * time.Second,
		// 不自动跟随重定向，把 3xx 直接返回给客户端（如 curl），由客户端发新请求；对 https 目标会走 CONNECT 隧道
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	resp, err := client.Do(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	for k, v := range resp.Header {
		for _, vv := range v {
			w.Header().Add(k, vv)
		}
	}
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}

func (s *Server) handleConnect(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	conn, err := s.Dialer(ctx, "tcp", r.Host)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer conn.Close()
	hijacker, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "hijack not supported", http.StatusInternalServerError)
		return
	}
	clientConn, _, err := hijacker.Hijack()
	if err != nil {
		return
	}
	defer clientConn.Close()
	if _, err := clientConn.Write([]byte("HTTP/1.1 200 Connection established\r\n\r\n")); err != nil {
		return
	}
	// 客户端关写端时，通知上游（CloseWrite），避免对端一直等发送导致提前断连
	go func() {
		_, _ = io.Copy(conn, clientConn)
		closeWrite(conn)
	}()
	_, _ = io.Copy(clientConn, conn)
}

// closeWrite 关闭写端，通知对端“本端不再发送”，便于对端发完响应后关闭
func closeWrite(conn net.Conn) {
	type closeWriter interface{ CloseWrite() error }
	if tc, ok := conn.(*tls.Conn); ok {
		conn = tc.NetConn()
	}
	if cw, ok := conn.(closeWriter); ok {
		_ = cw.CloseWrite()
	}
}

func (s *Server) serveSOCKS5() {
	for {
		client, err := s.socksLn.Accept()
		if err != nil {
			return
		}
		go s.handleSOCKS5(client)
	}
}

// 最小 SOCKS5 实现：无认证，仅 CONNECT
func (s *Server) handleSOCKS5(client net.Conn) {
	defer client.Close()
	br := bufio.NewReader(client)
	// 协商：版本 5，1 种方法，0=无认证
	buf := make([]byte, 2)
	if _, err := io.ReadFull(br, buf); err != nil {
		return
	}
	if buf[0] != 5 {
		return
	}
	nmethods := buf[1]
	if nmethods > 0 {
		_ = make([]byte, nmethods)
		if _, err := io.ReadFull(br, make([]byte, nmethods)); err != nil {
			return
		}
	}
	// 回复：版本 5，无需认证
	if _, err := client.Write([]byte{5, 0}); err != nil {
		return
	}
	// 请求：VER CMD RSV ATYP DST.ADDR DST.PORT
	header := make([]byte, 4)
	if _, err := io.ReadFull(br, header); err != nil {
		return
	}
	if header[0] != 5 || header[1] != 1 {
		_, _ = client.Write([]byte{5, 1, 0, 1, 0, 0, 0, 0, 0, 0})
		return
	}
	var addr string
	switch header[3] {
	case 1: // IPv4
		ip := make([]byte, 4)
		if _, err := io.ReadFull(br, ip); err != nil {
			return
		}
		port := make([]byte, 2)
		if _, err := io.ReadFull(br, port); err != nil {
			return
		}
		addr = fmt.Sprintf("%s:%d", net.IP(ip), binary.BigEndian.Uint16(port))
	case 3: // 域名
		lenB := make([]byte, 1)
		if _, err := io.ReadFull(br, lenB); err != nil {
			return
		}
		host := make([]byte, lenB[0])
		if _, err := io.ReadFull(br, host); err != nil {
			return
		}
		port := make([]byte, 2)
		if _, err := io.ReadFull(br, port); err != nil {
			return
		}
		addr = fmt.Sprintf("%s:%d", string(host), binary.BigEndian.Uint16(port))
	case 4: // IPv6
		ip := make([]byte, 16)
		if _, err := io.ReadFull(br, ip); err != nil {
			return
		}
		port := make([]byte, 2)
		if _, err := io.ReadFull(br, port); err != nil {
			return
		}
		addr = fmt.Sprintf("[%s]:%d", net.IP(ip), binary.BigEndian.Uint16(port))
	default:
		_, _ = client.Write([]byte{5, 1, 0, 1, 0, 0, 0, 0, 0, 0})
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	remote, err := s.Dialer(ctx, "tcp", addr)
	if err != nil {
		_, _ = client.Write([]byte{5, 1, 0, 1, 0, 0, 0, 0, 0, 0})
		return
	}
	defer remote.Close()
	// 回复成功：VER REP RSV ATYP BND.ADDR BND.PORT，用 0.0.0.0:0
	if _, err := client.Write([]byte{5, 0, 0, 1, 0, 0, 0, 0, 0, 0}); err != nil {
		return
	}
	go io.Copy(remote, client)
	_, _ = io.Copy(client, remote)
}

// UpstreamDialer 从代理 URI 构造拨号函数。skipTLSVerify 为 true 时 Trojan/VMess 连接不校验服务端证书。
func UpstreamDialer(proxyURI string, skipTLSVerify bool) (func(context.Context, string, string) (net.Conn, error), error) {
	if isTrojanURI(proxyURI) {
		return trojanDialer(proxyURI, skipTLSVerify)
	}
	uri := proxyURI
	if skipTLSVerify && strings.HasPrefix(proxyURI, "vmess://") {
		uri = vmessURIWithInsecure(proxyURI)
	}
	dialer := merkur.NewProxyDialer(uri)
	if dialer == nil {
		return nil, fmt.Errorf("unsupported or invalid proxy: %s", maskURI(proxyURI))
	}
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		return dialer.Dial(network, addr)
	}, nil
}

func maskURI(s string) string {
	if len(s) > 40 {
		return s[:20] + "..." + s[len(s)-10:]
	}
	return s
}
