//go:build windows

package sys

import (
	"encoding/json"
	"os"
	"path/filepath"

	"goclashz/core/utils"
)

type DataDirInfo struct {
	AppDir        string `json:"appDir"`
	DataDir       string `json:"dataDir"`
	LegacyDataDir string `json:"legacyDataDir"`
	LegacyExists  bool   `json:"legacyExists"`
	Migrated      bool   `json:"migrated"`
	LastError     string `json:"lastError"`
}

func GetDataDirInfo() DataDirInfo {
	info := DataDirInfo{
		AppDir:        utils.GetAppDir(),
		DataDir:       utils.GetDataDir(),
		LegacyDataDir: utils.GetLegacyDataDir(),
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

	return info
}
