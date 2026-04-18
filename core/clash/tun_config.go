package clash

import (
	"gopkg.in/yaml.v3"
	"os"
)

// InjectRuntimeConfig 强行在用户配置中注入/覆盖运行时参数 (TUN, Mode, Ports)
func InjectRuntimeConfig(configPath string, enableTun bool, mode string) error {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return err
	}

	var configMap map[string]interface{}
	if err := yaml.Unmarshal(data, &configMap); err != nil {
		return err
	}

	// 1. 注入模式 (rule / global / direct)
	if mode != "" {
		configMap["mode"] = mode
	}

	// 2. 注入 TUN 配置
	if enableTun {
		configMap["tun"] = map[string]interface{}{
			"enable":                true,
			"stack":                 "system", // Windows 推荐 system 栈
			"auto-route":            true,
			"auto-detect-interface": true,
			"dns-hijack":            []string{"any:53", "tcp://any:53"},
		}
	} else {
		if tun, ok := configMap["tun"].(map[string]interface{}); ok {
			tun["enable"] = false
		}
	}

	// 3. 注入关键端口和 API 控制器
	configMap["mixed-port"] = 7890
	configMap["external-controller"] = "127.0.0.1:9090"
	configMap["allow-lan"] = true

	// 重新写入
	newData, err := yaml.Marshal(configMap)
	if err != nil {
		return err
	}

	return os.WriteFile(configPath, newData, 0644)
}
