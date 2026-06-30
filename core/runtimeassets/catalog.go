package runtimeassets

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"goclashz/core/utils"
)

type RequiredAssetDef struct {
	Key      AssetKey
	Name     string
	Label    string
	Required bool
}

var RequiredAssets = []RequiredAssetDef{
	{Key: AssetCore, Name: "clash.exe", Label: "Mihomo 内核", Required: true},
	{Key: AssetWintun, Name: "wintun.dll", Label: "Wintun 驱动 DLL", Required: true},
	{Key: AssetGeoIP, Name: "geoip.metadb", Label: "GeoIP", Required: false},
	{Key: AssetGeoSite, Name: "geosite.dat", Label: "GeoSite", Required: false},
	{Key: AssetMMDB, Name: "country.mmdb", Label: "MMDB", Required: false},
	{Key: AssetASN, Name: "asn.dat", Label: "ASN", Required: false},
}

func requiredAssetDefsFor(req Requirement) []RequiredAssetDef {
	var out []RequiredAssetDef
	for _, def := range RequiredAssets {
		switch def.Key {
		case AssetCore:
			if req.NeedCore {
				out = append(out, def)
			}
		case AssetWintun:
			if req.NeedWintun {
				out = append(out, def)
			}
		case AssetGeoIP, AssetGeoSite, AssetMMDB, AssetASN:
			if req.NeedGeo {
				out = append(out, def)
			}
		}
	}
	return out
}

type SeedAssetMeta struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	Source  string `json:"source"`
	Version string `json:"version"`
	Size    int64  `json:"size"`
	SHA256  string `json:"sha256"`
}

type SeedCatalog struct {
	ManifestPath  string                   `json:"manifestPath"`
	ManifestOK    bool                     `json:"manifestOK"`
	ManifestError string                   `json:"manifestError,omitempty"`
	Items         map[string]SeedAssetMeta `json:"items"`
}

type SeedAssetHealth struct {
	Key   AssetKey    `json:"key"`
	Name  string      `json:"name"`
	Label string      `json:"label"`
	Path  string      `json:"path"`
	Ready bool        `json:"ready"`
	Error string      `json:"error,omitempty"`
	SHA256 string     `json:"sha256,omitempty"`
}

type SeedCatalogStatus struct {
	ManifestExists bool            `json:"manifestExists"`
	ManifestOK     bool            `json:"manifestOK"`
	ManifestError  string          `json:"manifestError,omitempty"`
	Assets         []SeedAssetHealth `json:"assets"`
}

func LoadSeedCatalog() SeedCatalog {
	c := SeedCatalog{
		ManifestPath: utils.GetSeedAssetManifestPath(),
		Items:        make(map[string]SeedAssetMeta),
	}

	manifest, err := LoadSeedManifest()
	if err != nil {
		c.ManifestOK = false
		c.ManifestError = err.Error()
		return c
	}

	c.ManifestOK = true

	for _, item := range manifest.Assets {
		name := canonicalAssetName(item.Name)
		if name == "" && item.Path != "" {
			name = canonicalAssetName(item.Path)
		}
		if name == "" {
			continue
		}

		c.Items[name] = SeedAssetMeta{
			Name:    name,
			Path:    item.Path,
			Source:  item.Source,
			Version: item.Version,
			Size:    item.Size,
			SHA256:  item.SHA256,
		}
	}

	return c
}

func GetSeedCatalogStatus(ctx context.Context) SeedCatalogStatus {
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
		seedHealth := checkAssetByPath(ctx, def.Key, def.Label, seedPath)

		sh := SeedAssetHealth{
			Key:   def.Key,
			Name:  def.Name,
			Label: def.Label,
			Path:  seedPath,
			Ready: seedHealth.Ready,
			Error: seedHealth.Error,
			SHA256: seedHealth.SHA256,
		}
		status.Assets = append(status.Assets, sh)
	}

	return status
}

func checkAssetByPath(ctx context.Context, key AssetKey, label, path string) AssetHealth {
	switch key {
	case AssetCore:
		return checkCoreByPath(ctx, path)
	case AssetWintun:
		return checkWintunByPath(path)
	default:
		return checkDataFileByPath(key, label, path)
	}
}

func checkRuntimeAsset(ctx context.Context, key AssetKey, label, name string) AssetHealth {
	switch key {
	case AssetCore:
		return CheckCore(ctx)
	case AssetWintun:
		return CheckWintun()
	default:
		return CheckDataFile(key, label, name)
	}
}

func seedPathForCanonicalName(name string) string {
	return filepath.Join(utils.GetSeedCoreBinDir(), name)
}

func runtimePathForCanonicalName(name string) string {
	return filepath.Join(utils.GetCoreBinDir(), name)
}

func normalizeAssetName(s string) string {
	s = strings.TrimSpace(s)
	s = filepath.Base(filepath.Clean(s))
	return strings.ToLower(s)
}

func canonicalAssetName(name string) string {
	switch normalizeAssetName(name) {
	case "clash.exe", "mihomo.exe", "mihomo-windows-amd64.exe":
		return "clash.exe"
	case "wintun.dll":
		return "wintun.dll"
	case "geoip.metadb":
		return "geoip.metadb"
	case "geosite.dat":
		return "geosite.dat"
	case "country.mmdb":
		return "country.mmdb"
	case "asn.dat", "geolite2-asn.mmdb":
		return "asn.dat"
	default:
		return ""
	}
}
