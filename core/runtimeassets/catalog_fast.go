package runtimeassets

import (
	"context"
	"os"

	"goclashz/core/utils"
)

// GetSeedCatalogStatusFast 返回内置种子库的资产可用性，但**不跑任何子进程、不算 SHA256**：
//   - 可用性：快检（stat + PE 头）
//   - 版本/哈希：只取 asset-manifest.json 的元信息（清单里没有就留空）
//
// 背景：GetSeedCatalogStatus 会对 seed 的 clash.exe 跑一次 `-v`（超时 5s）、再对 6 个文件
// 算哈希；它挂在 UI 的"数据目录诊断"按钮上，用户感知就是点一下卡几秒。诊断只需要回答
// "种子库能不能用"，所以这里改用清单元信息 + 快检。
func GetSeedCatalogStatusFast(ctx context.Context) SeedCatalogStatus {
	status := SeedCatalogStatus{
		ManifestExists: false,
		ManifestOK:     false,
	}

	manifestPath := utils.GetSeedAssetManifestPath()
	if _, err := os.Stat(manifestPath); err == nil {
		status.ManifestExists = true
	}

	catalog := LoadSeedCatalog()
	status.ManifestOK = catalog.ManifestOK
	status.ManifestError = catalog.ManifestError

	for _, def := range RequiredAssets {
		seedPath := seedPathForCanonicalName(def.Name)
		h := fastHealthByPath(def.Key, def.Label, seedPath)

		item := SeedAssetHealth{
			Key:    def.Key,
			Name:   def.Name,
			Label:  def.Label,
			Path:   seedPath,
			Ready:  h.Ready,
			Error:  h.Error,
			SHA256: "",
		}
		if meta, ok := catalog.Items[def.Name]; ok {
			item.SHA256 = meta.SHA256
		}
		status.Assets = append(status.Assets, item)
	}

	return status
}
