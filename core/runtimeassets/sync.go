package runtimeassets

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"goclashz/core/utils"
)

type SeedManifest struct {
	GeneratedAt string `json:"generatedAt"`
	Assets      []struct {
		Name    string `json:"name"`
		Path    string `json:"path"`
		Source  string `json:"source"`
		Version string `json:"version"`
		Size    int64  `json:"size"`
		SHA256  string `json:"sha256"`
	} `json:"assets"`
}

type AssetState struct {
	SeedManifestSha256 string `json:"seedManifestSha256"`
	CopiedAssets       map[string]struct {
		SHA256 string `json:"sha256"`
		Source string `json:"source"`
	} `json:"copiedAssets"`
}

func getAssetStatePath() string {
	return filepath.Join(utils.GetCoreBinDir(), "asset-state.json")
}

func LoadSeedManifest() (*SeedManifest, error) {
	path := utils.GetSeedAssetManifestPath()
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var manifest SeedManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, err
	}
	return &manifest, nil
}

func LoadAssetState() *AssetState {
	path := getAssetStatePath()
	data, err := os.ReadFile(path)
	state := &AssetState{
		CopiedAssets: make(map[string]struct {
			SHA256 string `json:"sha256"`
			Source string `json:"source"`
		}),
	}
	if err == nil {
		_ = json.Unmarshal(data, state)
	}
	if state.CopiedAssets == nil {
		state.CopiedAssets = make(map[string]struct {
			SHA256 string `json:"sha256"`
			Source string `json:"source"`
		})
	}
	return state
}

func saveAssetState(state *AssetState) error {
	path := getAssetStatePath()
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return utils.WriteFileAtomic(path, data, 0644)
}

func labelForKey(key AssetKey) string {
	switch key {
	case AssetCore:
		return "Mihomo 内核"
	case AssetWintun:
		return "Wintun 驱动"
	case AssetGeoIP:
		return "GeoIP"
	case AssetGeoSite:
		return "GeoSite"
	case AssetMMDB:
		return "MMDB"
	case AssetASN:
		return "ASN"
	default:
		return string(key)
	}
}

func RepairFromSeed(ctx context.Context, req Requirement, mode RepairMode) error {
	state := LoadAssetState()
	changed := false

	manifestHash := ""
	if data, err := os.ReadFile(utils.GetSeedAssetManifestPath()); err == nil && len(data) > 0 {
		h := sha256.New()
		h.Write(data)
		manifestHash = hex.EncodeToString(h.Sum(nil))
	}
	appUpdated := manifestHash != "" && manifestHash != state.SeedManifestSha256

	// 用 manifest 元信息补充 sha256/version（可选）
	catalog := LoadSeedCatalog()

	var errs []string

	for _, def := range requiredAssetDefsFor(req) {
		name := def.Name
		key := def.Key

		seedPath := seedPathForCanonicalName(name)
		runtimePath := runtimePathForCanonicalName(name)

		// 1. 验证 seed 文件是否真实存在且可用
		seedHealth := checkAssetByPath(ctx, key, def.Label, seedPath)
		if !seedHealth.Ready {
			if def.Required {
				msg := fmt.Sprintf("seed 必需资产不可用: %s path=%s error=%s", name, seedPath, seedHealth.Error)
				errs = append(errs, msg)
				log.Printf("[runtimeassets] %s", msg)
			} else {
				log.Printf("[runtimeassets] seed 可选资产不可用，跳过: %s path=%s error=%s", name, seedPath, seedHealth.Error)
			}
			continue
		}

		// 2. 验证 runtime 是否可用
		runtimeHealth := checkRuntimeAsset(ctx, key, def.Label, name)

		shouldCopy := false
		switch mode {
		case RepairForce:
			shouldCopy = true
		case RepairInvalid:
			if isCoreAsset(key) {
				shouldCopy = !runtimeHealth.Ready || !runtimeHealth.VersionProbeOK
			} else {
				shouldCopy = !runtimeHealth.Exists
			}
		case RepairMissingOnly:
			shouldCopy = !runtimeHealth.Exists
		}

		// 3. 升级覆盖策略：runtime 已就绪时判断是否被用户魔改
		if shouldCopy && !forceMode(mode) && runtimeHealth.Ready && appUpdated {
			if copied, exists := state.CopiedAssets[name]; exists {
				if runtimeHealth.SHA256 == copied.SHA256 {
					shouldCopy = true
				} else {
					log.Printf("[runtimeassets] 资产 %s 已被用户手动修改，升级时跳过覆盖", name)
					shouldCopy = false
				}
			} else {
				log.Printf("[runtimeassets] 资产 %s 无拷贝记录，从 seed 同步", name)
				shouldCopy = true
			}
		}

		if !shouldCopy {
			continue
		}

		_ = os.MkdirAll(filepath.Dir(runtimePath), 0755)

		if err := utils.CopyFile(seedPath, runtimePath); err != nil {
			errs = append(errs, fmt.Sprintf("复制 seed 资产失败: %s -> %s: %v", seedPath, runtimePath, err))
			continue
		}

		log.Printf("[runtimeassets] seed 自修复成功: %s", name)

		// 优先用 manifest 元信息的 sha256，fallback 到 seed 实际 sha256
		copySHA256 := seedHealth.SHA256
		if meta, ok := catalog.Items[name]; ok && meta.SHA256 != "" {
			copySHA256 = meta.SHA256
		}

		state.CopiedAssets[name] = struct {
			SHA256 string `json:"sha256"`
			Source string `json:"source"`
		}{
			SHA256: copySHA256,
			Source: "seed",
		}
		changed = true
	}

	if changed || appUpdated {
		state.SeedManifestSha256 = manifestHash
		if err := saveAssetState(state); err != nil {
			errs = append(errs, fmt.Sprintf("保存 asset-state 失败: %v", err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("%s", strings.Join(errs, "; "))
	}

	return nil
}

func forceMode(mode RepairMode) bool {
	return mode == RepairForce
}

func isCoreAsset(key AssetKey) bool {
	return key == AssetCore || key == AssetWintun
}
