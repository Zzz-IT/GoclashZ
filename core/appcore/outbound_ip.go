//go:build windows

package appcore

import (
	"context"
	"fmt"
	"goclashz/core/clash"
	"goclashz/core/logger"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
)

type OutboundIPResult struct {
	Preferred string `json:"preferred"` // 最终显示 IP，IPv6 优先
	IPv4      string `json:"ipv4"`
	IPv6      string `json:"ipv6"`
	Mode      string `json:"mode"`   // proxy 或 direct
	Source    string `json:"source"` // (旧字段，可能废弃或保留用于主显)
	Source4   string `json:"source4"`
	Source6   string `json:"source6"`
	Message   string `json:"message"` // (整体消息)
	Message4  string `json:"message4"`
	Message6  string `json:"message6"`
	Complete  bool   `json:"complete"`
}

var ipv6ProxyEndpoints = []string{
	"https://6.ipw.cn",
	"https://api6.ipify.org",
	"https://ipv6.ddnspod.com",
	"https://api-ipv6.ip.sb/ip",
	"https://ipv6.icanhazip.com",
}

var ipv4ProxyEndpoints = []string{
	"https://4.ipw.cn",
	"https://api.ipify.org",
	"https://api.ip.sb/ip",
	"http://ip.3322.net",
	"https://ipv4.icanhazip.com",
}

var ipv6DirectEndpoints = []string{
	"https://6.ipw.cn",
	"https://ipv6.ddnspod.com",
	"https://api-ipv6.ip.sb/ip",
	"https://api6.ipify.org",
}

var ipv4DirectEndpoints = []string{
	"https://4.ipw.cn",
	"http://ip.3322.net",
	"https://api.ip.sb/ip",
	"https://api.ipify.org",
}

type cachedOutboundIP struct {
	result    OutboundIPResult
	expiresAt time.Time
}

type outboundIPCache struct {
	mu    sync.Mutex
	value map[string]cachedOutboundIP
}

func (c *outboundIPCache) cleanupLocked(now time.Time) {
	for k, v := range c.value {
		if now.After(v.expiresAt) {
			delete(c.value, k)
		}
	}
}

var (
	ipCache         = outboundIPCache{value: make(map[string]cachedOutboundIP)}
	outboundIPGroup singleflight.Group
)

func (c *Controller) outboundIPCacheKey() string {
	state := c.GetAppState()

	mode := "direct"
	switch {
	case state.Tun:
		mode = "tun-route"
	case state.SystemProxy:
		mode = "proxy"
	}

	if (mode == "proxy" || mode == "tun-route") && !clash.IsRunning() {
		mode = "direct"
	}

	return fmt.Sprintf(
		"%s|proxy=%t|tun=%t|sys=%t|config=%s",
		mode,
		state.SystemProxy,
		state.Tun,
		false,
		state.ActiveConfig,
	)
}

// GetOutboundIP 检测出站 IP
func (c *Controller) GetOutboundIP(force bool) (OutboundIPResult, error) {
	isForce := force
	key := c.outboundIPCacheKey()

	if !isForce {
		ipCache.mu.Lock()
		now := time.Now()
		ipCache.cleanupLocked(now)
		if cached, ok := ipCache.value[key]; ok && now.Before(cached.expiresAt) && cached.result.Preferred != "" {
			ipCache.mu.Unlock()
			return cached.result, nil
		}
		ipCache.mu.Unlock()
	}

	v, err, _ := outboundIPGroup.Do(key, func() (any, error) {
		res, err := c.getOutboundIPInternal()
		if err == nil && res.Preferred != "" {
			ipCache.mu.Lock()
			ipCache.value[key] = cachedOutboundIP{
				result:    res,
				expiresAt: time.Now().Add(10 * time.Second),
			}
			ipCache.mu.Unlock()
		}
		return res, err
	})

	if err != nil {
		return OutboundIPResult{}, err
	}
	return v.(OutboundIPResult), nil
}

func (c *Controller) getOutboundIPInternal() (OutboundIPResult, error) {
	state := c.GetAppState()

	// TUN 模式：不走 HTTP proxy，直接检测（TUN 在网络层透明接管）
	// System Proxy 模式：通过 HTTP proxy 检测
	// Direct：直连检测
	mode := "direct"
	useHTTPProxy := false

	switch {
	case state.Tun:
		mode = "tun-route"
		useHTTPProxy = false
	case state.SystemProxy:
		mode = "proxy"
		useHTTPProxy = true
	}

	if useHTTPProxy && !clash.IsRunning() {
		useHTTPProxy = false
		mode = "direct"
	}

	if useHTTPProxy {
		port := clash.GetProxyPort()
		if port > 0 {
			conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 500*time.Millisecond)
			if err != nil {
				useHTTPProxy = false
				mode = "direct"
			} else {
				conn.Close()
			}
		} else {
			useHTTPProxy = false
			mode = "direct"
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	type ipResult struct {
		family string // "ipv4" or "ipv6"
		ip     string
		source string
		err    error
	}

	var endpointsV6, endpointsV4 []string
	if useHTTPProxy {
		endpointsV6 = ipv6ProxyEndpoints
		endpointsV4 = ipv4ProxyEndpoints
	} else {
		endpointsV6 = ipv6DirectEndpoints
		endpointsV4 = ipv4DirectEndpoints
	}

	resultCh := make(chan ipResult, 2)

	// 并发检测双栈
	go func() {
		ip, source, err := detectIP(ctx, endpointsV6, "tcp6", useHTTPProxy)
		resultCh <- ipResult{family: "ipv6", ip: ip, source: source, err: err}
	}()
	go func() {
		ip, source, err := detectIP(ctx, endpointsV4, "tcp4", useHTTPProxy)
		resultCh <- ipResult{family: "ipv4", ip: ip, source: source, err: err}
	}()

	result := OutboundIPResult{Mode: mode}
	deadline := time.NewTimer(3500 * time.Millisecond)
	defer deadline.Stop()

	var count int
	for count < 2 {
		select {
		case r := <-resultCh:
			count++
			if r.err == nil && r.ip != "" {
				if r.family == "ipv6" {
					result.IPv6 = r.ip
					result.Source6 = r.source
					result.Source = r.source
				} else {
					result.IPv4 = r.ip
					result.Source4 = r.source
					if result.Source == "" {
						result.Source = r.source
					}
				}
			} else if r.err != nil {
				if r.family == "ipv6" {
					result.Message6 = r.err.Error()
				} else {
					result.Message4 = r.err.Error()
				}
			}
		case <-deadline.C:
			count = 2 // Timeout, break the loop
			result.Message = "检测超时"
		}
	}

	result.Complete = true

	if result.IPv6 != "" {
		result.Preferred = result.IPv6
	} else if result.IPv4 != "" {
		result.Preferred = result.IPv4
	}

	if result.Preferred == "" {
		result.Message = "所有检测站点均不可达或检测超时"
		if c.appLogger != nil {
			c.appLogger.AddApp(logger.LogEntry{
				Type:    "error",
				Payload: "出站 IP 检测失败: " + result.Message,
			})
		}
	}

	return result, nil
}

func detectIP(ctx context.Context, endpoints []string, network string, useProxy bool) (string, string, error) {
	type epResult struct {
		ip     string
		source string
		err    error
	}

	ch := make(chan epResult, len(endpoints))
	reqCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	var wg sync.WaitGroup

	go func() {
		defer func() {
			go func() {
				wg.Wait()
				close(ch)
			}()
		}()

		for i, ep := range endpoints {
			wg.Add(1)
			go func(endpoint string) {
				defer wg.Done()

				// 给单个请求一个合理的超时，避免无限期挂起
				fetchCtx, fetchCancel := context.WithTimeout(reqCtx, 3500*time.Millisecond)
				defer fetchCancel()

				ip, err := fetchIPFromEndpoint(fetchCtx, endpoint, network, useProxy)
				if err != nil {
					ch <- epResult{err: err}
					return
				}
				ip = strings.TrimSpace(ip)
				parsed := net.ParseIP(ip)
				if parsed == nil {
					ch <- epResult{err: fmt.Errorf("invalid IP: %s", ip)}
					return
				}
				if network == "tcp4" && parsed.To4() == nil {
					ch <- epResult{err: fmt.Errorf("expected IPv4 but got IPv6: %s", ip)}
					return
				}
				if network == "tcp6" && parsed.To4() != nil {
					ch <- epResult{err: fmt.Errorf("expected IPv6 but got IPv4: %s", ip)}
					return
				}
				ch <- epResult{ip: ip, source: stripScheme(endpoint), err: nil}
			}(ep)

			if i < len(endpoints)-1 {
				select {
				case <-reqCtx.Done():
					return // 如果已经拿到正确结果被 cancel，则停止继续分发后续请求
				case <-time.After(80 * time.Millisecond):
					// 错峰延时
				}
			}
		}
	}()

	var lastErr error
	for res := range ch {
		if res.err == nil && res.ip != "" {
			return res.ip, res.source, nil
		}
		if res.err != nil {
			lastErr = res.err
		}
	}

	return "", "", lastErr
}

func fetchIPFromEndpoint(ctx context.Context, endpoint string, network string, useProxy bool) (string, error) {
	var transport *http.Transport

	if useProxy {
		port := clash.GetProxyPort()
		proxyURL, _ := url.Parse(fmt.Sprintf("http://127.0.0.1:%d", port))
		transport = &http.Transport{
			Proxy: http.ProxyURL(proxyURL),
		}
	} else {
		dialer := &net.Dialer{Timeout: 3500 * time.Millisecond}
		transport = &http.Transport{
			Proxy: nil,
			DialContext: func(ctx context.Context, n, addr string) (net.Conn, error) {
				return dialer.DialContext(ctx, network, addr)
			},
		}
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   3500 * time.Millisecond,
	}

	req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if err != nil {
		return "", err
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 64))
	if err != nil {
		return "", err
	}

	return string(body), nil
}

func stripScheme(rawURL string) string {
	return strings.TrimPrefix(strings.TrimPrefix(rawURL, "https://"), "http://")
}
