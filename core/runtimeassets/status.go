package runtimeassets

import (
	"context"
	"fmt"

	"goclashz/core/utils"
)

type RepairMode string

const (
	RepairMissingOnly RepairMode = "missing-only"
	RepairInvalid     RepairMode = "invalid"
	RepairForce       RepairMode = "force"
)

// GetStatus 获取当前所有运行期组件资产的完整健康度状态
func GetStatus(ctx context.Context) RuntimeAssetStatus {
	assets := map[AssetKey]AssetHealth{
		AssetCore:    CheckCore(ctx),
		AssetWintun:  CheckWintun(),
		AssetGeoIP:   CheckDataFile(AssetGeoIP, "GeoIP", "geoip.metadb"),
		AssetGeoSite: CheckDataFile(AssetGeoSite, "GeoSite", "geosite.dat"),
		AssetMMDB:    CheckDataFile(AssetMMDB, "MMDB", "country.mmdb"),
		AssetASN:     CheckDataFile(AssetASN, "ASN", "asn.dat"),
	}

	coreReady := assets[AssetCore].Ready
	wintunReady := assets[AssetWintun].Ready

	return RuntimeAssetStatus{
		AppDir:         utils.GetAppDir(),
		DataDir:        utils.GetDataDir(),
		CoreBinDir:     utils.GetCoreBinDir(),
		SeedCoreBinDir: utils.GetSeedCoreBinDir(),
		Assets:         assets,
		CoreReady:      coreReady,
		WintunReady:    wintunReady,
		Ready:          coreReady && wintunReady,
	}
}

// EnsureReady 检查并尝试在资产状态不满足要求时，自动从只读种子库拉起覆盖修复
func EnsureReady(ctx context.Context, req Requirement, mode RepairMode) (RuntimeAssetStatus, error) {
	// 1. 获取当前最新状态
	status := GetStatus(ctx)

	// 2. 如果所需资产已经完全就绪，并且不处于 Force（强制覆盖）状态，则直接放行
	if requirementSatisfied(status, req) && mode != RepairForce {
		return status, nil
	}

	// 3. 从内置 seed 进行同步修复（只修当前场景需要的资产）
	if err := RepairFromSeed(ctx, req, mode); err != nil {
		return GetStatus(ctx), err
	}

	// 4. 同步完成后再次检测最新状态并评估
	status = GetStatus(ctx)

	if req.NeedCore && !status.CoreReady {
		coreHealth := status.Assets[AssetCore]
		return status, fmt.Errorf("内核仍不可用: %s (%s)", coreHealth.Error, coreHealth.Path)
	}

	if req.NeedWintun && !status.WintunReady {
		wintunHealth := status.Assets[AssetWintun]
		return status, fmt.Errorf("wintun 仍不可用: %s (%s)", wintunHealth.Error, wintunHealth.Path)
	}

	return status, nil
}

func requirementSatisfied(status RuntimeAssetStatus, req Requirement) bool {
	if req.NeedCore && !status.CoreReady {
		return false
	}
	if req.NeedWintun && !status.WintunReady {
		return false
	}
	if req.NeedGeo {
		for _, key := range []AssetKey{AssetGeoIP, AssetGeoSite, AssetMMDB, AssetASN} {
			if h, ok := status.Assets[key]; ok && !h.Ready {
				return false
			}
		}
	}
	return true
}

func MergeStatus(statuses ...RuntimeAssetStatus) RuntimeAssetStatus {
	merged := RuntimeAssetStatus{
		Assets: make(map[AssetKey]AssetHealth),
	}

	for _, s := range statuses {
		if s.AppDir != "" {
			merged.AppDir = s.AppDir
		}
		if s.DataDir != "" {
			merged.DataDir = s.DataDir
		}
		if s.CoreBinDir != "" {
			merged.CoreBinDir = s.CoreBinDir
		}
		if s.SeedCoreBinDir != "" {
			merged.SeedCoreBinDir = s.SeedCoreBinDir
		}

		for k, v := range s.Assets {
			merged.Assets[k] = v
		}

		if s.CoreReady {
			merged.CoreReady = true
		}
		if s.WintunReady {
			merged.WintunReady = true
		}
	}

	merged.Ready = merged.CoreReady && merged.WintunReady
	return merged
}
