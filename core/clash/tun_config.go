package clash

import (
	"gopkg.in/yaml.v3"
	"os"
)

// InjectTunConfig 强行在用户配置中注入/覆盖最佳实践的 TUN 设置
func InjectTunConfig(configPath string, enableTun bool) error {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return err
	}

	var configMap map[string]interface{}
	if err := yaml.Unmarshal(data, &configMap); err != nil {
		return err
	}

	// Stelliberty 风格的默认最优 TUN 配置
	if enableTun {
		configMap["tun"] = map[string]interface{}{
			"enable":                true,
			"stack":                 "system", // Windows 推荐 system 栈
			"auto-route":            true,
			"auto-detect-interface": true,
			"dns-hijack":            []string{"any:53", "tcp://any:53"},
		}
	} else {
		// 如果关闭，直接删除 tun 节点或设为 false
		if tun, ok := configMap["tun"].(map[string]interface{}); ok {
			tun["enable"] = false
		}
	}

	// 👇 新增：无论是 TUN 还是普通系统代理模式，都必须确保这几个关键端口开放
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
