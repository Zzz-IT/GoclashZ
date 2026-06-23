//go:build windows

package sys

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"goclashz/core/utils"
)

type DataDirInfo struct {
	AppDir           string `json:"appDir"`
	DataDir          string `json:"dataDir"`
	CoreBinDir       string `json:"coreBinDir"`
	CoreExePath      string `json:"coreExePath"`
	CoreExists       bool   `json:"coreExists"`
	CoreExecutable   bool   `json:"coreExecutable"`
	CoreInDataDir    bool   `json:"coreInDataDir"`
	LegacyCoreExists bool   `json:"legacyCoreExists"`
	LegacyDataDir    string `json:"legacyDataDir"`
	LegacyExists     bool   `json:"legacyExists"`
	Migrated         bool   `json:"migrated"`
	LastError        string `json:"lastError"`
}

func GetDataDirInfo() DataDirInfo {
	info := DataDirInfo{
		AppDir:        utils.GetAppDir(),
		DataDir:       utils.GetDataDir(),
		CoreBinDir:    utils.GetCoreBinDir(),
		LegacyDataDir: utils.GetLegacyDataDir(),
	}

	info.CoreExePath = filepath.Join(info.CoreBinDir, "clash.exe")

	if st, err := os.Stat(info.CoreExePath); err == nil {
		info.CoreExists = true
		if !st.IsDir() && st.Size() > 0 {
			info.CoreExecutable = true
		}
	}

	info.CoreInDataDir = strings.HasPrefix(
		strings.ToLower(filepath.Clean(info.CoreExePath)),
		strings.ToLower(filepath.Clean(info.DataDir))+string(filepath.Separator),
	)

	legacyCorePath := filepath.Join(utils.GetLegacyDataCoreBinDir(), "clash.exe")
	if _, err := os.Stat(legacyCorePath); err == nil {
		info.LegacyCoreExists = true
	}

	if info.LegacyDataDir != "" {
		if _, err := os.Stat(info.LegacyDataDir); err == nil {
			info.LegacyExists = true
		}
	}

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
