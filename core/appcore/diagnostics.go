//go:build windows

package appcore

import (
	"encoding/json"
	"os"
	"path/filepath"

	"goclashz/core/sys"
	"goclashz/core/utils"
)

type DiagnosticAssets struct {
	ClashExe    bool `json:"clash.exe"`
	WintunDll   bool `json:"wintun.dll"`
	Geoip       bool `json:"geoip.metadb"`
	Geosite     bool `json:"geosite.dat"`
	CountryMmdb bool `json:"country.mmdb"`
	AsnDat      bool `json:"asn.dat"`
}

type DiagnosticInfo struct {
	AppDir            string `json:"appDir"`
	DataDir           string `json:"dataDir"`
	SeedCoreBinDir    string `json:"seedCoreBinDir"`
	RuntimeCoreBinDir string `json:"runtimeCoreBinDir"`
	RuntimeConfigPath string `json:"runtimeConfigPath"`
	IsAdmin           bool   `json:"isAdmin"`

	HelperServiceStatus string `json:"helperServiceStatus"`
	HelperBinaryPath    string `json:"helperBinaryPath"`

	Assets DiagnosticAssets `json:"assets"`

	SeedManifest any `json:"seedManifest"`
	AssetState   any `json:"assetState"`
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return false
	}
	return !info.IsDir()
}

func GetDiagnosticInfo() DiagnosticInfo {
	info := DiagnosticInfo{
		AppDir:            utils.GetAppDir(),
		DataDir:           utils.GetDataDir(),
		SeedCoreBinDir:    utils.GetSeedCoreBinDir(),
		RuntimeCoreBinDir: utils.GetCoreBinDir(),
		RuntimeConfigPath: utils.GetRuntimeConfigPath(),
		IsAdmin:           sys.CheckAdmin(),
	}

	// Helper Service Status
	status := sys.CheckHelperService()
	info.HelperServiceStatus = status.Error
	if status.Reachable {
		info.HelperServiceStatus = "Reachable"
	} else if status.Running {
		info.HelperServiceStatus = "Running but not reachable"
	} else if status.Installed {
		info.HelperServiceStatus = "Installed but not running"
	} else if status.Error == "" {
		info.HelperServiceStatus = "Not installed"
	}

	info.HelperBinaryPath = filepath.Join(info.AppDir, "GoclashZHelper.exe")

	// Assets
	info.Assets.ClashExe = fileExists(filepath.Join(info.RuntimeCoreBinDir, "clash.exe"))
	info.Assets.WintunDll = fileExists(filepath.Join(info.RuntimeCoreBinDir, "wintun.dll"))
	info.Assets.Geoip = fileExists(filepath.Join(info.RuntimeCoreBinDir, "geoip.metadb"))
	info.Assets.Geosite = fileExists(filepath.Join(info.RuntimeCoreBinDir, "geosite.dat"))
	info.Assets.CountryMmdb = fileExists(filepath.Join(info.RuntimeCoreBinDir, "country.mmdb"))
	info.Assets.AsnDat = fileExists(filepath.Join(info.RuntimeCoreBinDir, "asn.dat"))

	manifest, _ := utils.LoadSeedManifest()
	info.SeedManifest = manifest

	info.AssetState = utils.LoadAssetState()

	return info
}

func ExportDiagnosticsToFile(path string) error {
	info := GetDiagnosticInfo()
	data, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}
