//go:build windows

package utils

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
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
	return filepath.Join(GetCoreBinDir(), "asset-state.json")
}

func LoadSeedManifest() (*SeedManifest, error) {
	path := GetSeedAssetManifestPath()
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
	return WriteFileAtomic(path, data, 0644)
}

func calculateFileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// SyncSeedAssetsToRuntime checks and copies seed assets to runtime directory if necessary.
// force flag forces the sync regardless of existing user modifications.
func SyncSeedAssetsToRuntime(force bool) error {
	manifest, err := LoadSeedManifest()
	if err != nil {
		// If seed manifest doesn't exist, this might be a dev environment or portable without seed.
		log.Printf("内置资产清单不存在或读取失败: %v, 跳过资产同步", err)
		return nil
	}

	state := LoadAssetState()
	changed := false

	// Check manifest sha256 to detect if app was updated
	manifestData, _ := os.ReadFile(GetSeedAssetManifestPath())
	manifestHash := ""
	if len(manifestData) > 0 {
		h := sha256.New()
		h.Write(manifestData)
		manifestHash = hex.EncodeToString(h.Sum(nil))
	}

	appUpdated := manifestHash != "" && manifestHash != state.SeedManifestSha256

	for _, asset := range manifest.Assets {
		seedPath := filepath.Join(GetSeedCoreBinDir(), asset.Name)
		runtimePath := filepath.Join(GetCoreBinDir(), asset.Name)

		shouldCopy := force
		if !shouldCopy {
			// Check if file is missing or empty
			if stat, err := os.Stat(runtimePath); os.IsNotExist(err) || stat.Size() == 0 {
				shouldCopy = true
			} else if appUpdated {
				// App updated, check if runtime file hash matches the OLD seed hash.
				// If yes, it means the user hasn't modified it, we can safely overwrite it with the NEW seed.
				// If no, it means user manually updated it, do NOT overwrite.
				if copied, exists := state.CopiedAssets[asset.Name]; exists {
					currentHash, _ := calculateFileSHA256(runtimePath)
					if currentHash == copied.SHA256 {
						shouldCopy = true // Safe to overwrite
					} else {
						log.Printf("资产 %s 已被用户修改，跳过覆盖更新", asset.Name)
					}
				} else {
					// We don't have a record of copying this, maybe user put it there manually.
					// Leave it alone.
					log.Printf("资产 %s 存在但无历史记录，跳过覆盖更新", asset.Name)
				}
			}
		}

		if shouldCopy {
			if _, err := os.Stat(seedPath); err == nil {
				if err := CopyFile(seedPath, runtimePath); err != nil {
					return fmt.Errorf("同步内置资产 %s 失败: %w", asset.Name, err)
				}
				log.Printf("成功同步内置资产: %s", asset.Name)
				
				state.CopiedAssets[asset.Name] = struct {
					SHA256 string `json:"sha256"`
					Source string `json:"source"`
				}{
					SHA256: asset.SHA256,
					Source: "seed",
				}
				changed = true
			} else {
				log.Printf("内置资产 %s 不存在，跳过同步", asset.Name)
			}
		}
	}

	if changed || appUpdated {
		state.SeedManifestSha256 = manifestHash
		if err := saveAssetState(state); err != nil {
			log.Printf("保存资产状态失败: %v", err)
		}
	}

	return nil
}
