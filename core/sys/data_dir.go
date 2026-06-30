//go:build windows

package sys

import (
	"encoding/json"
	"os"
	"path/filepath"

	"goclashz/core/utils"
)

type DataDirInfo struct {
	AppDir             string `json:"appDir"`
	DataDir            string `json:"dataDir"`
	CoreBinDir         string `json:"coreBinDir"`
	SeedCoreBinDir     string `json:"seedCoreBinDir"`
	SeedManifestExists bool   `json:"seedManifestExists"`

	SeedManifestOK    bool   `json:"seedManifestOK"`
	SeedManifestError string `json:"seedManifestError,omitempty"`
	SeedCoreReady     bool   `json:"seedCoreReady"`
	SeedWintunReady   bool   `json:"seedWintunReady"`
	SeedGeoIPReady    bool   `json:"seedGeoipReady"`
	SeedGeoSiteReady  bool   `json:"seedGeositeReady"`
	SeedMMDBReady     bool   `json:"seedMmdbReady"`
	SeedASNReady      bool   `json:"seedAsnReady"`

	CanAutoRepairCore   bool `json:"canAutoRepairCore"`
	CanAutoRepairWintun bool `json:"canAutoRepairWintun"`

	CoreExePath  string `json:"coreExePath"`
	CoreExists   bool   `json:"coreExists"`
	CoreReady    bool   `json:"coreReady"`
	CoreError    string `json:"coreError,omitempty"`
	WintunExists bool   `json:"wintunExists"`
	WintunReady  bool   `json:"wintunReady"`
	WintunError  string `json:"wintunError,omitempty"`

	LayoutMode string `json:"layoutMode"`
	LayoutOK   bool   `json:"layoutOK"`

	LegacyDataDir    string `json:"legacyDataDir"`
	LegacyExists     bool   `json:"legacyExists"`
	LegacyCoreExists bool   `json:"legacyCoreExists"`
	Migrated         bool   `json:"migrated"`
	LastError        string `json:"lastError"`
}

func GetDataDirInfo() DataDirInfo {
	info := DataDirInfo{
		AppDir:         utils.GetAppDir(),
		DataDir:        utils.GetDataDir(),
		CoreBinDir:     utils.GetCoreBinDir(),
		SeedCoreBinDir: utils.GetSeedCoreBinDir(),
		LegacyDataDir:  utils.GetLegacyDataDir(),
	}

	// seed 清单
	manifestPath := utils.GetSeedAssetManifestPath()
	if _, err := os.Stat(manifestPath); err == nil {
		info.SeedManifestExists = true
	}

	// core 状态（基础 stat 检查，由调用方用 runtimeassets 状态覆盖）
	info.CoreExePath = filepath.Join(info.CoreBinDir, "clash.exe")
	if st, err := os.Stat(info.CoreExePath); err == nil && !st.IsDir() && st.Size() > 0 {
		info.CoreExists = true
		info.CoreReady = true
	}

	// wintun 状态
	wintunPath := filepath.Join(info.CoreBinDir, "wintun.dll")
	if st, err := os.Stat(wintunPath); err == nil && !st.IsDir() && st.Size() > 0 {
		info.WintunExists = true
		info.WintunReady = true
	}

	// 布局判断
	info.LayoutMode = "unknown"
	if utils.IsPackagedInstall() {
		info.LayoutMode = "standard"
	} else {
		info.LayoutMode = "dev"
	}
	info.LayoutOK = info.CoreExists

	// 旧版目录
	if info.LegacyDataDir != "" {
		if _, err := os.Stat(info.LegacyDataDir); err == nil {
			info.LegacyExists = true
		}
	}
	legacyCorePath := filepath.Join(utils.GetLegacyDataCoreBinDir(), "clash.exe")
	if _, err := os.Stat(legacyCorePath); err == nil {
		info.LegacyCoreExists = true
	}

	// 迁移记录
	metaPath := filepath.Join(info.DataDir, ".migration.json")
	if data, err := os.ReadFile(metaPath); err == nil {
		var meta struct {
			SourceDeleted bool   `json:"sourceDeleted"`
			DeleteError   string `json:"deleteError"`
		}
		if json.Unmarshal(data, &meta) == nil {
			info.Migrated = true
			if !meta.SourceDeleted {
				info.LastError = meta.DeleteError
			}
		}
	}

	errPath := filepath.Join(info.DataDir, ".migration_error.json")
	if data, err := os.ReadFile(errPath); err == nil {
		var errMeta struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(data, &errMeta) == nil && errMeta.Error != "" {
			info.LastError = "迁移失败: " + errMeta.Error
		}
	}

	return info
}
