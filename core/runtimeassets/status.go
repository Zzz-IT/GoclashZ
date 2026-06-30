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
		AssetGeoIP:   CheckDataFile(AssetGeoIP, "GeoIP", "geoip.metadb", 64*1024),
		AssetGeoSite: CheckDataFile(AssetGeoSite, "GeoSite", "geosite.dat", 64*1024),
		AssetMMDB:    CheckDataFile(AssetMMDB, "MMDB", "country.mmdb", 64*1024),
		AssetASN:     CheckDataFile(AssetASN, "ASN", "asn.dat", 64*1024),
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
func EnsureReady(ctx context.Context, mode RepairMode) (RuntimeAssetStatus, error) {
	// 1. 获取当前最新状态
	status := GetStatus(ctx)

	// 2. 如果核心资产已经完全就绪，并且不处于 Force（强制覆盖）状态，则直接放行
	if status.CoreReady && status.WintunReady && mode != RepairForce {
		return status, nil
	}

	// 3. 从内置 seed 进行同步修复
	if err := RepairFromSeed(ctx, mode); err != nil {
		return GetStatus(ctx), err
	}

	// 4. 同步完成后再次检测最新状态并评估
	status = GetStatus(ctx)
	if !status.CoreReady {
		coreHealth := status.Assets[AssetCore]
		return status, fmt.Errorf("内核仍不可用: %s (%s)", coreHealth.Error, coreHealth.Path)
	}
	if !status.WintunReady {
		wintunHealth := status.Assets[AssetWintun]
		return status, fmt.Errorf("wintun 仍不可用: %s (%s)", wintunHealth.Error, wintunHealth.Path)
	}

	return status, nil
}
