//go:build windows

package sys

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"goclashz/core/utils"
	"golang.org/x/sys/windows/registry"
)

type ownedProxyState struct {
	Host      string `json:"host"`
	Port      int    `json:"port"`
	EnabledAt int64  `json:"enabledAt"`
}

func proxyStatePath() string {
	return filepath.Join(utils.GetDataDir(), "system_proxy_state.json")
}

// MarkSystemProxyOwned 标记当前系统的代理所有权归属于本程序
func MarkSystemProxyOwned(host string, port int) {
	state := ownedProxyState{
		Host:      host,
		Port:      port,
		EnabledAt: time.Now().Unix(),
	}

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return
	}

	_ = os.WriteFile(proxyStatePath(), data, 0644)
}

// UnmarkSystemProxyOwned 清除代理所有权标记
func UnmarkSystemProxyOwned() {
	_ = os.Remove(proxyStatePath())
}

func readOwnedProxyState() (ownedProxyState, bool) {
	data, err := os.ReadFile(proxyStatePath())
	if err != nil {
		return ownedProxyState{}, false
	}

	var state ownedProxyState
	if err := json.Unmarshal(data, &state); err != nil {
		return ownedProxyState{}, false
	}

	if state.Host == "" || state.Port <= 0 {
		return ownedProxyState{}, false
	}

	return state, true
}

func currentProxyMatches(host string, port int) bool {
	key, err := registry.OpenKey(
		registry.CURRENT_USER,
		`Software\Microsoft\Windows\CurrentVersion\Internet Settings`,
		registry.QUERY_VALUE,
	)
	if err != nil {
		return false
	}
	defer key.Close()

	enabled, _, err := key.GetIntegerValue("ProxyEnable")
	if err != nil || enabled == 0 {
		return false
	}

	proxyServer, _, err := key.GetStringValue("ProxyServer")
	if err != nil {
		return false
	}

	target := fmt.Sprintf("%s:%d", host, port)

	// 兼容：
	// 127.0.0.1:7890
	// http=127.0.0.1:7890;https=127.0.0.1:7890
	return strings.Contains(proxyServer, target)
}

// ClearOwnedSystemProxy 仅在检测到代理是由本程序设置且仍匹配时，执行清理
func ClearOwnedSystemProxy() {
	state, ok := readOwnedProxyState()
	if !ok {
		return
	}

	if currentProxyMatches(state.Host, state.Port) {
		_ = DisableSystemProxy()
	}

	UnmarkSystemProxyOwned()
}
