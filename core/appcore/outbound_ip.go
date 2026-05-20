//go:build windows

package appcore

import (
	"context"
	"fmt"
	"goclashz/core/clash"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// OutboundIPResult 出站 IP 检测结果
type OutboundIPResult struct {
	Preferred string `json:"preferred"` // 最终显示 IP，IPv6 优先
	IPv4      string `json:"ipv4"`
	IPv6      string `json:"ipv6"`
	Mode      string `json:"mode"`    // proxy 或 direct
	Source    string `json:"source"`  // 命中的检测站点
	Message   string `json:"message"` // 失败原因或说明
}

var ipv6Endpoints = []string{
	"https://api6.ipify.org",
	"https://ipv6.icanhazip.com",
	"https://ipv6.seeip.org",
}

var ipv4Endpoints = []string{
	"https://api.ipify.org",
	"https://ipv4.icanhazip.com",
	"https://ipv4.seeip.org",
}

// GetOutboundIP 检测出站 IP
func (c *Controller) GetOutboundIP() (OutboundIPResult, error) {
	state := c.GetAppState()
	proxyActive := state.SystemProxy || state.Tun
	mode := "direct"
	if proxyActive {
		mode = "proxy"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()

	type ipResult struct {
		ip     string
		source string
		err    error
	}

	ipv6Ch := make(chan ipResult, 1)
	ipv4Ch := make(chan ipResult, 1)

	go func() {
		ip, source, err := detectIP(ctx, ipv6Endpoints, "tcp6", proxyActive)
		ipv6Ch <- ipResult{ip: ip, source: source, err: err}
	}()

	go func() {
		ip, source, err := detectIP(ctx, ipv4Endpoints, "tcp4", proxyActive)
		ipv4Ch <- ipResult{ip: ip, source: source, err: err}
	}()

	ipv6Res := <-ipv6Ch
	ipv4Res := <-ipv4Ch

	result := OutboundIPResult{
		IPv4:   ipv4Res.ip,
		IPv6:   ipv6Res.ip,
		Mode:   mode,
		Source: ipv6Res.source,
	}

	if ipv6Res.ip != "" {
		result.Preferred = ipv6Res.ip
		result.Source = ipv6Res.source
	} else if ipv4Res.ip != "" {
		result.Preferred = ipv4Res.ip
		result.Source = ipv4Res.source
	}

	if result.Preferred == "" {
		result.Message = "所有检测站点均不可达"
		if ipv6Res.err != nil {
			result.Message = fmt.Sprintf("IPv6: %v; IPv4: %v", ipv6Res.err, ipv4Res.err)
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

	for _, ep := range endpoints {
		go func(endpoint string) {
			ip, err := fetchIPFromEndpoint(ctx, endpoint, network, useProxy)
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

			ch <- epResult{ip: ip, source: stripScheme(endpoint)}
		}(ep)
	}

	var lastErr error
	for range endpoints {
		res := <-ch
		if res.ip != "" {
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
		port := 7890
		if netCfg, err := clash.GetNetworkConfig(); err == nil && netCfg != nil {
			if netCfg.MixedPort != 0 {
				port = netCfg.MixedPort
			} else if netCfg.Port != 0 {
				port = netCfg.Port
			}
		}
		proxyURL, _ := url.Parse(fmt.Sprintf("http://127.0.0.1:%d", port))
		transport = &http.Transport{
			Proxy: http.ProxyURL(proxyURL),
		}
	} else {
		dialer := &net.Dialer{Timeout: 2500 * time.Millisecond}
		transport = &http.Transport{
			Proxy: nil,
			DialContext: func(ctx context.Context, n, addr string) (net.Conn, error) {
				return dialer.DialContext(ctx, network, addr)
			},
		}
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   2500 * time.Millisecond,
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
