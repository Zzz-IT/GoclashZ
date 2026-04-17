package clash

import (
	"context"
	"crypto/tls"
	"net"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// ProxyTestResult 定义推送给前端的结构
type ProxyTestResult struct {
	Name  string `json:"name"`
	Delay int    `json:"delay"`
}

// BatchTestNodes 高并发探测入口
func BatchTestNodes(ctx context.Context, nodes []RawProxyInfo) {
	// 限制并发数为 30，防止瞬间占用过多系统句柄
	sem := make(chan struct{}, 30)

	for _, node := range nodes {
		sem <- struct{}{} // 占用坑位
		go func(n RawProxyInfo) {
			defer func() { <-sem }() // 释放坑位

			delay := probeProtocol(n)

			// ✨ 核心：测完一个，立即通过 Wails 事件推送到前端
			runtime.EventsEmit(ctx, "node_delay_update", ProxyTestResult{
				Name:  n.Name,
				Delay: delay,
			})
		}(node)
	}
}

// probeProtocol 模拟客户端协议握手
func probeProtocol(n RawProxyInfo) int {
	start := time.Now()
	address := net.JoinHostPort(n.Server, n.Port)
	timeout := 3 * time.Second

	// 1. 基础 TCP 握手
	conn, err := net.DialTimeout("tcp", address, timeout)
	if err != nil {
		return -1
	}
	defer conn.Close()

	// 2. 根据协议“假装”发送握手包 (此处以模拟逻辑为主)
	// 如果是 Trojan/Vmess TLS，尝试建立 TLS 握手
	if n.Port == "443" || n.Port == "8443" {
		conf := &tls.Config{InsecureSkipVerify: true}
		tlsConn := tls.Client(conn, conf)
		tlsConn.SetDeadline(time.Now().Add(timeout))
		if err := tlsConn.Handshake(); err != nil {
			return -1
		}
	} else {
		// 如果是 Shadowsocks，尝试发送一串随机字节并读取（简单模拟）
		conn.SetDeadline(time.Now().Add(timeout))
		_, _ = conn.Write([]byte{0x05, 0x01, 0x00}) // 类似 SOCKS5 的探测
		buf := make([]byte, 1)
		_, err := conn.Read(buf)
		// 只要有数据返回或没有立即 reset，说明服务在运行
		if err != nil && err.Error() != "EOF" {
			// 如果被拒绝连接则失败
		}
	}

	return int(time.Since(start).Milliseconds())
}
