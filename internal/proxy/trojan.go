package proxy

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// trojanDialer 解析 trojan:// 并返回拨号函数。skipTLSVerify 为 true 时不校验服务端证书（兼容证书与域名不一致的节点）。
func trojanDialer(proxyURI string, skipTLSVerify bool) (func(context.Context, string, string) (net.Conn, error), error) {
	u, err := url.Parse(proxyURI)
	if err != nil || u.Scheme != "trojan" || u.Host == "" {
		return nil, fmt.Errorf("invalid trojan URI")
	}
	var password string
	if u.User != nil {
		password, _ = url.QueryUnescape(u.User.Username())
		if password == "" {
			if p, ok := u.User.Password(); ok {
				password, _ = url.QueryUnescape(p)
			}
		}
	}
	if password == "" {
		password = u.User.Username()
	}
	host := u.Hostname()
	portStr := u.Port()
	if portStr == "" {
		portStr = "443"
	}
	port, err := strconv.Atoi(portStr)
	if err != nil || port <= 0 {
		return nil, fmt.Errorf("invalid trojan port")
	}
	sni := host
	if u.Query().Get("sni") != "" {
		sni = u.Query().Get("sni")
	}
	serverAddr := net.JoinHostPort(host, strconv.Itoa(port))
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		if network != "tcp" {
			return nil, fmt.Errorf("trojan only supports tcp")
		}
		tlsCfg := &tls.Config{
			ServerName:         sni,
			InsecureSkipVerify: skipTLSVerify,
		}
		d := &net.Dialer{Timeout: 15 * time.Second}
		tcpConn, err := d.DialContext(ctx, "tcp", serverAddr)
		if err != nil {
			return nil, err
		}
		tlsConn := tls.Client(tcpConn, tlsCfg)
		if err := tlsConn.HandshakeContext(ctx); err != nil {
			tcpConn.Close()
			return nil, err
		}
		// Trojan 首包: hex(SHA224(password)) + CRLF + Request + CRLF
		hash := sha256.Sum224([]byte(password))
		hexHash := hex.EncodeToString(hash[:])
		req := buildTrojanRequest(addr)
		firstPacket := []byte(hexHash + "\r\n")
		firstPacket = append(firstPacket, req...)
		firstPacket = append(firstPacket, []byte("\r\n")...)
		if _, err := tlsConn.Write(firstPacket); err != nil {
			tlsConn.Close()
			return nil, err
		}
		return tlsConn, nil
	}, nil
}

// buildTrojanRequest 构造 Trojan 请求：CMD(1) + ATYP(1) + DST.ADDR + DST.PORT
func buildTrojanRequest(addr string) []byte {
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return nil
	}
	port, _ := strconv.Atoi(portStr)
	if port <= 0 {
		port = 80
	}
	var req []byte
	req = append(req, 1) // CMD: 1 = connect
	if ip := net.ParseIP(host); ip != nil {
		if ip4 := ip.To4(); ip4 != nil {
			req = append(req, 1) // ATYP IPv4
			req = append(req, ip4...)
		} else {
			req = append(req, 4) // ATYP IPv6
			req = append(req, ip.To16()...)
		}
	} else {
		req = append(req, 3) // ATYP domain
		req = append(req, byte(len(host)))
		req = append(req, host...)
	}
	p := make([]byte, 2)
	binary.BigEndian.PutUint16(p, uint16(port))
	req = append(req, p...)
	return req
}

func isTrojanURI(s string) bool {
	return strings.HasPrefix(s, "trojan://")
}
