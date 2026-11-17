package runtimeassets

import (
	"context"
	"fmt"
	"sync"
	"time"

	"goclashz/core/utils"
)

type RepairMode string

const (
	RepairMissingOnly RepairMode = "missing-only"
	RepairInvalid     RepairMode = "invalid"
	RepairForce       RepairMode = "force"
)

// GetStatus 获取当前所有运行期组件资产的完整健康度状态（全量深检，语义保持不变）。
//
// 成本：6 个文件全量 SHA256（≈95 MB 读盘）+ 内核 clash.exe -v（≤5s 超时）。
// 因此**不要把它挂在 UI 或 watchdog 上**：那两条路径用 GetStatusCached / GetStatusFast。
// 只读语义：不再写盘（历史行为见 AdoptFallback）。
func GetStatus(ctx context.Context) RuntimeAssetStatus {
	assets := fullAssets(ctx)
	for _, h := range assets {
		// 只把"版本号"这类轻量结论带进快检缓存：快照一律不携带 SHA256，
		// 避免 UI 拿到"半深检"的混合体；VersionProbeOK 同样不对外暴露。
		light := h
		light.SHA256 = ""
		light.VersionProbeOK = false
		storeCachedHealth(light)
	}
	return buildStatus(assets)
}

// GetStatusFast 做 L0/L1 快检：只 stat + PE 头 + 文件版本 API，
// **不跑 clash.exe -v、不算 SHA256**（AssetHealth.SHA256 一律留空），成本是若干次 stat。
// 结果写入 60s 快检缓存，供 GetStatusCached 复用；watchdog 可安全地高频调用。
func GetStatusFast(ctx context.Context) RuntimeAssetStatus {
	assets := fastAssets()
	for _, h := range assets {
		storeCachedHealth(h)
	}
	return buildStatus(assets)
}

// GetStatusCached 是 UI / 首屏 / 设置页的推荐入口：命中 60s 内的快检缓存就直接返回，
// 未命中（或文件 size/mtime 变了）时退回 GetStatusFast 并回填缓存。
// 保证：不跑 -v、不算 SHA256、不写盘。
func GetStatusCached(ctx context.Context) RuntimeAssetStatus {
	assets := make(map[AssetKey]AssetHealth, len(RequiredAssets))
	for _, def := range RequiredAssets {
		if h, ok := loadCachedHealth(runtimePathForCanonicalName(def.Name)); ok {
			assets[def.Key] = h
			continue
		}
		h := fastHealthFor(def.Key, def.Label, def.Name)
		storeCachedHealth(h)
		assets[def.Key] = h
	}
	return buildStatus(assets)
}

// fullAssets 走全量检查（CheckCore 含 -v 与 SHA256），仅自愈/诊断路径使用。
func fullAssets(ctx context.Context) map[AssetKey]AssetHealth {
	assets := make(map[AssetKey]AssetHealth, len(RequiredAssets))
	for _, def := range RequiredAssets {
		assets[def.Key] = checkRuntimeAsset(ctx, def.Key, def.Label, def.Name)
	}
	return assets
}

// fastAssets 走快检（stat + PE 头 + 版本 API），不产生子进程与哈希。
func fastAssets() map[AssetKey]AssetHealth {
	assets := make(map[AssetKey]AssetHealth, len(RequiredAssets))
	for _, def := range RequiredAssets {
		assets[def.Key] = fastHealthFor(def.Key, def.Label, def.Name)
	}
	return assets
}

func buildStatus(assets map[AssetKey]AssetHealth) RuntimeAssetStatus {
	coreReady := assets[AssetCore].Ready
	wintunReady := assets[AssetWintun].Ready
	geoReady := assets[AssetGeoIP].Ready &&
		assets[AssetGeoSite].Ready &&
		assets[AssetMMDB].Ready &&
		assets[AssetASN].Ready

	return RuntimeAssetStatus{
		AppDir:         utils.GetAppDir(),
		DataDir:        utils.GetDataDir(),
		CoreBinDir:     utils.GetCoreBinDir(),
		SeedCoreBinDir: utils.GetSeedCoreBinDir(),
		Assets:         assets,
		CoreReady:      coreReady,
		WintunReady:    wintunReady,
		GeoReady:       geoReady,
		Ready:          coreReady && wintunReady,
		ReadyForAll:    coreReady && wintunReady && geoReady,
	}
}

// annotateRequirement 把"本次场景真正需要哪些资产"写进状态：
//   - Required 列表（场景相关）；
//   - 每个 AssetHealth.Required 标记（让单资产展示也知道"这个场景缺它就阻塞"）。
//
// 这样 geo 不再是"恒 false"的 Required（F-A1-20）。
func annotateRequirement(status *RuntimeAssetStatus, req Requirement) {
	if status == nil {
		return
	}
	if status.Assets == nil {
		return
	}

	defs := requiredAssetDefsFor(req)
	keys := make([]AssetKey, 0, len(defs))
	for _, def := range defs {
		keys = append(keys, def.Key)
		if h, ok := status.Assets[def.Key]; ok {
			h.Required = true
			status.Assets[def.Key] = h
		}
	}
	status.Required = keys
}

// deepStatusTTL：一次用户动作会连续触发 3~4 次 EnsureReady（prepare → reconcile →
// ensureCore → PrepareEnv）。深检（PE + -v）没必要每次都做：2s 内复用同一份结果，
// 把"一次动作 3~4 次深检"收敛成 1~2 次（F-A1-10）。
const deepStatusTTL = 2 * time.Second

var (
	deepStatusMu    sync.Mutex
	deepStatusAt    time.Time
	deepStatusValue RuntimeAssetStatus
	deepStatusValid bool
)

// sharedDeepStatus 返回"短窗内共享"的深检结果。并发调用可能各算一次（只读、无副作用），
// 但不写盘的语义必须保持——它是复用的前提。
func sharedDeepStatus(ctx context.Context) RuntimeAssetStatus {
	deepStatusMu.Lock()
	if deepStatusValid && time.Since(deepStatusAt) < deepStatusTTL {
		st := deepStatusValue
		deepStatusMu.Unlock()
		return st
	}
	deepStatusMu.Unlock()

	st := GetStatus(ctx)

	deepStatusMu.Lock()
	deepStatusValue = st
	deepStatusAt = time.Now()
	deepStatusValid = true
	deepStatusMu.Unlock()
	return st
}

// invalidateDeepStatus 在发生写操作（采纳/修复）后调用，避免复用写前的旧结论。
func invalidateDeepStatus() {
	deepStatusMu.Lock()
	deepStatusValid = false
	deepStatusMu.Unlock()
}

// EnsureReady 检查并尝试在资产状态不满足要求时，自动从只读种子库拉起覆盖修复
func EnsureReady(ctx context.Context, req Requirement, mode RepairMode) (RuntimeAssetStatus, error) {
	return EnsureReadyWithProgress(ctx, req, mode, nil)
}

// EnsureReadyWithProgress 与 EnsureReady 语义完全相同，额外把逐资产修复进度传给
// onProgress（供 UI 异步修复展示进度，F-A0-04）。签名是新增，不动既有调用方。
func EnsureReadyWithProgress(ctx context.Context, req Requirement, mode RepairMode, onProgress RepairProgressFunc) (RuntimeAssetStatus, error) {
	// 1. 获取当前状态（短窗内复用，避免一次动作反复深检）
	status := sharedDeepStatus(ctx)

	// 2. 如果所需资产已经完全就绪，并且不处于 Force（强制覆盖）状态，则直接放行
	if requirementSatisfied(status, req) && mode != RepairForce {
		annotateRequirement(&status, req)
		return status, nil
	}

	// 3. 进入修复分支前，先把同机其它布局里的同名资产"采纳"进运行目录。
	//    写盘只发生在这条显式路径上（只读检查已经不再触发写盘）；每资产每进程至多一次。
	adoptFallbackAssets(req)
	invalidateDeepStatus() // 采纳会改文件：之前的深检结论作废

	// 4. 从内置 seed 进行同步修复（只修当前场景需要的资产）
	if err := RepairFromSeedWithProgress(ctx, req, mode, onProgress); err != nil {
		invalidateDeepStatus()
		failed := GetStatus(ctx)
		annotateRequirement(&failed, req)
		return failed, err
	}

	// 5. 同步完成后再次检测最新状态并评估（结果进入 2s 共享窗，供后续几层复用）
	invalidateDeepStatus()
	status = sharedDeepStatus(ctx)

	annotateRequirement(&status, req)

	if req.NeedCore && !status.CoreReady {
		coreHealth := status.Assets[AssetCore]
		return status, fmt.Errorf("内核仍不可用: %s (%s)", coreHealth.Error, coreHealth.Path)
	}

	if req.NeedWintun && !status.WintunReady {
		wintunHealth := status.Assets[AssetWintun]
		return status, fmt.Errorf("wintun 仍不可用: %s (%s)", wintunHealth.Error, wintunHealth.Path)
	}

	// 规则库缺失/损坏过去是静默的（F-A1-15）：NeedGeo 场景下必须显式失败，
	// 否则用户会看到"一切就绪"却在用降级规则。
	if req.NeedGeo {
		for _, key := range []AssetKey{AssetGeoIP, AssetGeoSite, AssetMMDB, AssetASN} {
			if h, ok := status.Assets[key]; ok && !h.Ready {
				return status, fmt.Errorf("规则库不可用: %s (%s): %s", h.Label, h.Path, h.Error)
			}
		}
	}

	return status, nil
}

var (
	adoptOnceMu sync.Mutex
	adoptOnce   = make(map[string]*sync.Once)
)

func adoptOnceFor(name string) *sync.Once {
	adoptOnceMu.Lock()
	defer adoptOnceMu.Unlock()

	once, ok := adoptOnce[name]
	if !ok {
		once = &sync.Once{}
		adoptOnce[name] = once
	}
	return once
}

// adoptFallbackAssets 按本次需求，把同机其它布局中的同名资产采纳进运行目录（每资产仅一次）。
// 只在 EnsureReady 进入修复分支之前调用——检查路径不允许有写副作用（F-A1-04）。
func adoptFallbackAssets(req Requirement) {
	for _, def := range requiredAssetDefsFor(req) {
		name := def.Name
		adoptOnceFor(name).Do(func() { AdoptFallback(name) })
	}
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
		if s.GeoReady {
			merged.GeoReady = true
		}
		if len(s.Required) > 0 {
			merged.Required = s.Required
		}
	}

	merged.Ready = merged.CoreReady && merged.WintunReady
	merged.ReadyForAll = merged.CoreReady && merged.WintunReady && merged.GeoReady
	return merged
}
