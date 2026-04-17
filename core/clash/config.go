package clash

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// ClashConfig 映射完整的 YAML 结构
type ClashConfig struct {
	Mode        string                   `yaml:"mode"`
	ProxyGroups []map[string]interface{} `yaml:"proxy-groups"`
}

// OfflineGroup 专供前端在“未启动”状态下展示的节点组结构
type OfflineGroup struct {
	Name    string   `json:"name"`
	Type    string   `json:"type"`
	Proxies []string `json:"proxies"` // 组内包含的所有节点名称
}

// GetStaticNodes 从本地 config.yaml 读取节点，用于启动前展示和合规性校验
func GetStaticNodes() (mode string, groups []OfflineGroup, err error) {
	pwd, _ := os.Getwd()
	configPath := filepath.Join(pwd, "core", "bin", "config.yaml")

	// 1. 如果文件还不存在（比如第一次打开软件），不报错，直接返回空
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return "rule", nil, nil
	}

	// 2. 读取文件
	data, err := os.ReadFile(configPath)
	if err != nil {
		return "", nil, fmt.Errorf("读取配置文件失败: %v", err)
	}

	// 3. ✨ 核心：合规性校验
	var conf ClashConfig
	err = yaml.Unmarshal(data, &conf)
	if err != nil {
		return "", nil, fmt.Errorf("YAML 格式错误，请检查缩进或语法: %v", err)
	}

	// 4. 提取策略组和其下属的节点列表
	for _, g := range conf.ProxyGroups {
		name, _ := g["name"].(string)
		gType, _ := g["type"].(string)

		// 提取组内的 proxies 列表
		var proxyList []string
		if pList, ok := g["proxies"].([]interface{}); ok {
			for _, p := range pList {
				if pStr, ok := p.(string); ok {
					proxyList = append(proxyList, pStr)
				}
			}
		}

		// 过滤掉没用的底层策略组，只发给前端有用的
		if gType == "select" || gType == "url-test" || gType == "fallback" || gType == "load-balance" {
			groups = append(groups, OfflineGroup{
				Name:    name,
				Type:    gType,
				Proxies: proxyList,
			})
		}
	}

	mode = conf.Mode
	if mode == "" {
		mode = "rule" // 如果没写 mode，默认为 rule
	}

	return mode, groups, nil
}
