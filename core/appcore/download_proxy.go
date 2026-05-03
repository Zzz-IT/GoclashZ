//go:build windows

package appcore

import (
	"fmt"
	"goclashz/core/clash"
)

func resolveLocalProxyURL() string {
	if !clash.IsRunning() {
		return ""
	}

	netCfg, err := clash.GetNetworkConfig()
	if err != nil || netCfg == nil {
		return ""
	}

	port := netCfg.MixedPort
	if port == 0 {
		port = netCfg.Port
	}
	if port == 0 {
		port = 7890
	}

	return fmt.Sprintf("http://127.0.0.1:%d", port)
}
