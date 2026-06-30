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

func keyFromName(name string) AssetKey {
	switch name {
	case "clash.exe":
		return AssetCore
	case "wintun.dll":
		return AssetWintun
	case "geoip.metadb":
		return AssetGeoIP
	case "geosite.dat":
		return AssetGeoSite
	case "country.mmdb":
		return AssetMMDB
	case "asn.dat":
		return AssetASN
	default:
		return ""
	}
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
	manifest, err := LoadSeedManifest()
	if err != nil {
		if utils.IsPackagedInstall() {
			return fmt.Errorf("安装包内置资产清单缺失，无法执行自修复: %w", err)
		}
		log.Printf("[runtimeassets] 开发模式未找到 seed manifest，跳过 seed 自修复: %v", err)
		return nil
	}

	state := LoadAssetState()
	changed := false

	manifestData, _ := os.ReadFile(utils.GetSeedAssetManifestPath())
	manifestHash := ""
	if len(manifestData) > 0 {
		h := sha256.New()
		h.Write(manifestData)
		manifestHash = hex.EncodeToString(h.Sum(nil))
	}
	appUpdated := manifestHash != "" && manifestHash != state.SeedManifestSha256

	for _, item := range manifest.Assets {
		key := keyFromName(item.Name)
		if key == "" {
			continue
		}

		// 非需求资产跳过（除非强制模式）
		if !requirementNeedsAsset(req, key) && mode != RepairForce {
			continue
		}

		seedPath := filepath.Join(utils.GetSeedCoreBinDir(), item.Name)
		runtimePath := filepath.Join(utils.GetCoreBinDir(), item.Name)

		// 1. 验证 seed 是否可用
		var seedHealth AssetHealth
		if key == AssetCore {
			seedHealth = checkCoreByPath(ctx, seedPath)
		} else if key == AssetWintun {
			seedHealth = checkWintunByPath(seedPath)
		} else {
			seedHealth = checkDataFileByPath(key, labelForKey(key), seedPath)
		}

		// 如果内置的核心资产 seed 坏了，说明安装包打包有致命问题
		if !seedHealth.Ready && seedHealth.Required {
			log.Printf("[runtimeassets] 内置核心种子不合格: %s path=%s err=%s", item.Name, seedPath, seedHealth.Error)
			// 如果是 core，应该报严重错误
			if key == AssetCore {
				return fmt.Errorf("内置核心资产种子不可用: %s", seedHealth.Error)
			}
		}

		// 2. 验证 runtime 是否可用
		var runtimeHealth AssetHealth
		if key == AssetCore {
			runtimeHealth = CheckCore(ctx)
		} else if key == AssetWintun {
			runtimeHealth = CheckWintun()
		} else {
			runtimeHealth = CheckDataFile(key, labelForKey(key), item.Name)
		}

		shouldCopy := false
		switch mode {
		case RepairForce:
			shouldCopy = true
		case RepairInvalid:
			if isCoreAsset(key) {
				// 核心资产（clash.exe、wintun.dll）：PE无效或版本探测失败时从 seed 替换
				shouldCopy = !runtimeHealth.Ready || !runtimeHealth.VersionProbeOK
			} else {
				// 数据资产（geo 库等）：只补缺失，不覆盖用户自己的版本
				shouldCopy = !runtimeHealth.Exists
			}
		case RepairMissingOnly:
			shouldCopy = !runtimeHealth.Exists
		}

		// 3. 检查升级覆盖策略：如果 runtime 已经就绪，但在 app 升级时需要判断是否被用户魔改过
		if shouldCopy && !forceMode(mode) && runtimeHealth.Ready && appUpdated {
			// 如果记录中已拷过该资产，且当前的哈希等于记录的哈希，说明用户没有碰过它，可以被安全升级覆盖
			if copied, exists := state.CopiedAssets[item.Name]; exists {
				if runtimeHealth.SHA256 == copied.SHA256 {
					shouldCopy = true
				} else {
					log.Printf("[runtimeassets] 资产 %s 已被用户手动更新修改，升级时跳过覆盖", item.Name)
					shouldCopy = false
				}
			} else {
				// 无拷贝记录：首次安装或旧版本升级，允许从 seed 复制
				log.Printf("[runtimeassets] 资产 %s 无拷贝记录，从 seed 同步", item.Name)
				shouldCopy = true
			}
		}

		if shouldCopy {
			if _, err := os.Stat(seedPath); err == nil {
				// 确保目标路径的父目录存在
				_ = os.MkdirAll(filepath.Dir(runtimePath), 0755)

				// 安全覆盖
				if err := utils.CopyFile(seedPath, runtimePath); err != nil {
					return fmt.Errorf("覆盖复制内置资产 %s 失败: %w", item.Name, err)
				}
				log.Printf("[runtimeassets] 成功自愈/同步资产: %s", item.Name)

				state.CopiedAssets[item.Name] = struct {
					SHA256 string `json:"sha256"`
					Source string `json:"source"`
				}{
					SHA256: item.SHA256,
					Source: "seed",
				}
				changed = true
			} else {
				msg := fmt.Sprintf("内置资产实体缺失: %s (%s)", item.Name, seedPath)
				if utils.IsPackagedInstall() {
					return fmt.Errorf(msg)
				}
				log.Printf("[runtimeassets] 警告: %s", msg)
			}
		}
	}

	if changed || appUpdated {
		state.SeedManifestSha256 = manifestHash
		if err := saveAssetState(state); err != nil {
			log.Printf("[runtimeassets] 保存资产记录状态失败: %v", err)
		}
	}

	return nil
}

func forceMode(mode RepairMode) bool {
	return mode == RepairForce
}

// isCoreAsset 判断是否为核心运行时资产（需要版本兼容，应始终从 seed 更新）
func isCoreAsset(key AssetKey) bool {
	return key == AssetCore || key == AssetWintun
}
