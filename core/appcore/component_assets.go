//go:build windows

package appcore

import (
	"goclashz/core/clash"
	"goclashz/core/utils"
	"os"
	"path/filepath"
)

// ComponentFileInfo 定义组件资产的文件属性
type ComponentFileInfo struct {
	Exists  bool      `json:"exists"`
	Size    int64     `json:"size"`
	ModTime int64     `json:"modTime"`
	Path    string    `json:"path"`
}

// GetComponentFileInfo 统一获取所有组件资产的状态
func GetComponentFileInfo() map[string]ComponentFileInfo {
	results := make(map[string]ComponentFileInfo)

	// 1. 内核资产
	results["clash"] = getSingleFileInfo(filepath.Join(utils.GetCoreBinDir(), "clash.exe"))
	
	// 2. 驱动资产
	results["wintun"] = getSingleFileInfo(filepath.Join(utils.GetCoreBinDir(), "wintun.dll"))

	// 3. Geo 数据库资产
	geoKeys := []string{"geoip", "geosite", "mmdb", "asn"}
	for _, key := range geoKeys {
		path, err := clash.GeoDBPath(key)
		if err == nil {
			results[key] = getSingleFileInfo(path)
		}
	}

	return results
}

func getSingleFileInfo(path string) ComponentFileInfo {
	info, err := os.Stat(path)
	if err != nil {
		return ComponentFileInfo{Exists: false, Path: path}
	}
	return ComponentFileInfo{
		Exists:  true,
		Size:    info.Size(),
		ModTime: info.ModTime().Unix(),
		Path:    path,
	}
}
