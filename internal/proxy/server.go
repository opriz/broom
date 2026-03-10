package proxy

import (
	"bufio"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/Qingluan/merkur"
)

// Server 本地代理服务：SOCKS5 + HTTP CONNECT，出站流量通过 upstream 拨号器转发
type Server struct {
	Dialer   func(ctx context.Context, network, addr string) (net.Conn, error)
	HTTPPort int
	SOCKSPort int
	httpLn   net.Listener
	socksLn  net.Listener
	mu       sync.Mutex
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
		// 普通 HTTP 请求也通过 CONNECT 语义：用上游代理去连目标
		http.Error(w, "broom: use CONNECT or SOCKS5", http.StatusBadRequest)
	})
	_ = http.Serve(s.httpLn, handler)
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
	_, _ = clientConn.Write([]byte("HTTP/1.1 200 Connection established\r\n\r\n"))
	go io.Copy(conn, clientConn)
	_, _ = io.Copy(clientConn, conn)
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

// UpstreamDialer 从代理 URI 构造一个可用于 Server.Dialer 的拨号函数（支持 ss://、vmess://、ssr://）
func UpstreamDialer(proxyURI string) (func(context.Context, string, string) (net.Conn, error), error) {
	dialer := merkur.NewProxyDialer(proxyURI)
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
