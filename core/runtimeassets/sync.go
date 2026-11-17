package runtimeassets

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	jsonv2 "encoding/json/v2"

	"fmt"
	jsonutil "goclashz/core/jsonutil"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

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
	if err := jsonv2.Unmarshal(data, &manifest); err != nil {
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
		_ = jsonv2.Unmarshal(data, state)
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
	data, err := jsonutil.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return utils.WriteFileAtomic(path, data, 0644)
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

// 修复冷却：EnsureReady 会被 watchdog 每 5s 调一次，一旦 seed 缺失/复制失败，
// 每次都要重新遍历并尝试复制 10~60MB 的资产，表现为"弹窗 + 卡住"。
// 冷却按【资产】独立计（15s），而不是包级单例：这样内核/驱动的失败不会让同一次
// 启动里的 geo 修复连尝试机会都没有（历史问题：geo 阶段直接复用 core 的错误）。
const repairRetryCooldown = 15 * time.Second

type assetRepairFailure struct {
	at  time.Time
	err error
}

var (
	// repairMu 串行化整个修复流程（单飞）：避免两个调用方同时 CopyFile 到同一目标，
	// 也避免"检查→复制→记账"被并发的另一半穿插。冷却检查在锁外先做一次快速失败，
	// 进锁后再复核一次，减少等待者拿到锁之后的重复工作。
	repairMu sync.Mutex

	repairCooldownMu sync.Mutex
	repairFailures   = make(map[AssetKey]assetRepairFailure)
)

// cooledAssetError 返回该资产是否仍在冷却期内（含上次失败原因）。
func cooledAssetError(key AssetKey) (error, bool) {
	repairCooldownMu.Lock()
	defer repairCooldownMu.Unlock()

	item, ok := repairFailures[key]
	if !ok {
		return nil, false
	}
	if time.Since(item.at) >= repairRetryCooldown {
		delete(repairFailures, key)
		return nil, false
	}
	return item.err, true
}

// cooledRepairError 在本次要求的所有资产都仍处于冷却期时，直接复用上次的失败。
func cooledRepairError(req Requirement) (error, bool) {
	defs := requiredAssetDefsFor(req)
	if len(defs) == 0 {
		return nil, false
	}

	msgs := make([]string, 0, len(defs))
	for _, def := range defs {
		err, cooled := cooledAssetError(def.Key)
		if !cooled {
			return nil, false
		}
		msgs = append(msgs, fmt.Sprintf("资产 %s 仍在修复冷却期(%s 内不重试): %v", def.Name, repairRetryCooldown, err))
	}
	return fmt.Errorf("%s", strings.Join(msgs, "; ")), true
}

// recordAssetRepair 记录一次修复结果：失败进入冷却，成功则清除该资产的冷却。
func recordAssetRepair(key AssetKey, err error) {
	repairCooldownMu.Lock()
	defer repairCooldownMu.Unlock()

	if err == nil {
		delete(repairFailures, key)
		return
	}
	repairFailures[key] = assetRepairFailure{at: time.Now(), err: err}
}

// repairLockBudget 是等待"另一个修复在跑"的预算：超过就放弃本次修复并返回
// ErrAssetLocked，让调用方按"稍后重试"处理，而不是无限阻塞 UI/watchdog 线程（F-A1-08）。
const repairLockBudget = 3 * time.Second

// RepairProgressFunc 汇报修复进度（done 从 1 开始）。只有显式传回调的入口才会调用。
type RepairProgressFunc func(assetName string, done, total int)

func RepairFromSeed(ctx context.Context, req Requirement, mode RepairMode) error {
	return RepairFromSeedWithProgress(ctx, req, mode, nil)
}

// RepairFromSeedWithProgress 与 RepairFromSeed 相同，额外支持逐资产进度回调（F-A0-04）。
// 并发语义：整个修复串行化（repairMu），等待有预算、响应 ctx。
func RepairFromSeedWithProgress(ctx context.Context, req Requirement, mode RepairMode, onProgress RepairProgressFunc) error {
	// 本次需要的资产全都在冷却期内：直接复用上次错误，既不进修复流程也不占 repairMu。
	if mode != RepairForce {
		if err, allCooled := cooledRepairError(req); allCooled {
			return err
		}
	}

	if !lockWithContext(ctx, &repairMu, repairLockBudget) {
		if cerr := ctx.Err(); cerr != nil {
			return fmt.Errorf("%w: 等待修复锁时被取消: %v", ErrAssetLocked, cerr)
		}
		return fmt.Errorf("%w: 另一个修复流程仍在进行", ErrAssetLocked)
	}
	defer repairMu.Unlock()

	// 排队期间先到的调用方可能已经修完并把失败记进冷却：进锁后复核一次再决定。
	if mode != RepairForce {
		if err, allCooled := cooledRepairError(req); allCooled {
			return err
		}
	}

	return repairFromSeed(ctx, req, mode, onProgress)
}

func repairFromSeed(ctx context.Context, req Requirement, mode RepairMode, onProgress RepairProgressFunc) error {
	defs := requiredAssetDefsFor(req)
	total := len(defs)

	state := LoadAssetState()
	changed := false

	manifestHash := ""
	if data, err := os.ReadFile(utils.GetSeedAssetManifestPath()); err == nil && len(data) > 0 {
		h := sha256.New()
		h.Write(data)
		manifestHash = hex.EncodeToString(h.Sum(nil))
	}
	appUpdated := manifestHash != "" && manifestHash != state.SeedManifestSha256

	// manifest 元信息（sha256/version）：只用于校验与记账，不作为可信输入。
	catalog := LoadSeedCatalog()

	var errs []string

	for idx, def := range defs {
		name := def.Name
		key := def.Key

		if cerr := ctx.Err(); cerr != nil {
			errs = append(errs, fmt.Sprintf("修复被取消: %v", cerr))
			break
		}

		if onProgress != nil {
			onProgress(name, idx+1, total)
		}

		// 该资产仍在冷却期：跳过（不重复 IO）。只记日志、不追加错误——资产可能已被外部
		// 修好，真正的结论交给 EnsureReady 修复后的二次状态判定，避免伪造失败。
		if mode != RepairForce {
			if cdErr, cooled := cooledAssetError(key); cooled {
				log.Printf("[runtimeassets] 资产 %s 仍在修复冷却期(%s 内不再重试): %v", name, repairRetryCooldown, cdErr)
				continue
			}
		}

		seedPath := seedPathForCanonicalName(name)
		runtimePath := runtimePathForCanonicalName(name)

		// 1. seed 侧：只做快检（stat + PE 头），不 exec、不算哈希（F-A1-14）。
		seedHealth := fastHealthByPath(key, def.Label, seedPath)
		if !seedHealth.Ready {
			if def.Required {
				msg := fmt.Sprintf("seed 必需资产不可用: %s path=%s error=%s", name, seedPath, seedHealth.Error)
				errs = append(errs, msg)
				recordAssetRepair(key, fmt.Errorf("seed 资产不可用: %s", seedHealth.Error))
				log.Printf("[runtimeassets] %s", msg)
			} else {
				log.Printf("[runtimeassets] seed 可选资产不可用，跳过: %s path=%s error=%s", name, seedPath, seedHealth.Error)
			}
			continue
		}

		// 2. runtime 侧：只有"升级覆盖"判断需要哈希，其余情况跳过 95MB 的读盘（F-A1-14）。
		runtimeHealth := checkRuntimeAssetWithHash(ctx, key, def.Label, name, appUpdated)

		// 3+4. 是否需要重拷 + 升级覆盖策略：纯函数判定（便于对矩阵做单测，F-A1-19）。
		copiedSHA, hasRecord := "", false
		if rec, ok := state.CopiedAssets[name]; ok {
			copiedSHA, hasRecord = rec.SHA256, true
		}
		decision := decideRepairCopy(mode, key, runtimeHealth, appUpdated, copiedSHA, hasRecord)
		logDecision(name, decision)

		if !decision.Copy {
			continue
		}

		// 5. 脏状态清理：目标若是目录（ErrIsDir），先移除（只允许 core/bin 下的已知资产名）。
		if err := clearNonFileTarget(utils.GetCoreBinDir(), runtimePath, name); err != nil {
			errs = append(errs, fmt.Sprintf("清理异常目标失败: %s: %v", runtimePath, err))
			recordAssetRepair(key, err)
			continue
		}

		// 6. 清单校验：对**真实 seed 文件**算一次哈希，与 manifest 比对（F-A1-06）。
		//    不一致时以实际文件为准（清单过期不该阻断自愈），但要留下可追的日志。
		seedSHA, hashErr := calculateSHA256Context(ctx, seedPath)
		if hashErr != nil {
			errs = append(errs, fmt.Sprintf("计算 seed 哈希失败: %s: %v", name, hashErr))
			recordAssetRepair(key, hashErr)
			continue
		}
		if meta, ok := catalog.Items[name]; ok && meta.SHA256 != "" && !strings.EqualFold(meta.SHA256, seedSHA) {
			log.Printf("[runtimeassets] 清单与 seed 文件不一致（以实际文件为准）: %s manifest=%s actual=%s",
				name, meta.SHA256, shortSHA(seedSHA))
		}

		_ = os.MkdirAll(filepath.Dir(runtimePath), 0755)

		// 7. 复制：资产级互斥（进程内 + 跨进程，F-A1-13）+ ctx 感知（F-A1-08）。
		copyErr := withAssetLock(ctx, name, 2*time.Second, func() error {
			return utils.CopyFileContext(ctx, seedPath, runtimePath)
		})
		if copyErr != nil {
			errs = append(errs, fmt.Sprintf("复制 seed 资产失败: %s -> %s: %v", seedPath, runtimePath, copyErr))
			recordAssetRepair(key, copyErr)
			continue
		}

		recordAssetRepair(key, nil)

		// 8. 记账：写**实际 seed 文件**的哈希（而不是清单里的），避免"记错哈希 → 升级被判魔改"。
		state.CopiedAssets[name] = struct {
			SHA256 string `json:"sha256"`
			Source string `json:"source"`
		}{
			SHA256: seedSHA,
			Source: "seed",
		}
		changed = true

		log.Printf("[runtimeassets] seed 自修复成功: %s (sha256=%s)", name, shortSHA(seedSHA))
	}

	if changed || appUpdated {
		state.SeedManifestSha256 = manifestHash
		if err := saveAssetState(state); err != nil {
			errs = append(errs, fmt.Sprintf("保存 asset-state 失败: %v", err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("%s", strings.Join(errs, "; "))
	}

	return nil
}

// RepairDecision 是"是否需要从 seed 重拷"的判定结果（F-A1-19：抽成纯函数以便矩阵单测）。
type RepairDecision struct {
	Copy   bool
	Reason string
}

// 判定原因的稳定标识（日志/测试共用）。
const (
	repairReasonForce         = "force"
	repairReasonAlreadyOK     = "already-ok"
	repairReasonUnknownMode   = "unknown-mode"
	repairReasonUpgradeIntact = "upgrade:unmodified-copy"
	repairReasonUpgradeEdited = "upgrade:user-modified-skip"
	repairReasonUpgradeNoRec  = "upgrade:no-record-copy"
	repairReasonNeedsRepair   = "needs-repair"
)

// decideRepairCopy 判定某个资产是否需要从 seed 重拷。**纯函数**：不碰磁盘、不读全局。
//
// 语义要点：
//   - ErrExecFailed/ErrCorrupt/ErrIsDir 都属于"文件在但不可用"，必须能修（F-A1-05/15）；
//   - RepairForce 无条件重拷；
//   - 升级覆盖：runtime 已就绪且 app 清单变了，才用哈希判断"是否被用户魔改"。
func decideRepairCopy(mode RepairMode, key AssetKey, h AssetHealth, appUpdated bool, copiedSHA string, hasRecord bool) RepairDecision {
	if mode == RepairForce {
		return RepairDecision{Copy: true, Reason: repairReasonForce}
	}

	repairNeeded := false
	switch mode {
	case RepairInvalid:
		switch key {
		case AssetCore:
			repairNeeded = !h.Ready || h.ErrorCode == ErrExecFailed || h.ErrorCode == ErrIsDir
		case AssetWintun:
			repairNeeded = !h.Exists || !h.Valid || h.ErrorCode == ErrIsDir
		default:
			repairNeeded = !h.Exists || !h.Valid || h.ErrorCode == ErrIsDir || h.ErrorCode == ErrCorrupt
		}
	case RepairMissingOnly:
		repairNeeded = !h.Exists || h.ErrorCode == ErrIsDir || h.ErrorCode == ErrCorrupt
	default:
		return RepairDecision{Copy: false, Reason: repairReasonUnknownMode}
	}

	if repairNeeded {
		return RepairDecision{Copy: true, Reason: repairReasonNeedsRepair}
	}

	// 应用升级：健康资产也可能需要刷新——旧实现把这段放在"已判定要修"之后，导致
	// 健康内核/驱动永远不会随升级更新（分支不可达）。这里改成独立判断。
	if appUpdated && h.Exists && h.Valid {
		if !hasRecord {
			return RepairDecision{Copy: false, Reason: repairReasonUpgradeNoRec}
		}
		if h.SHA256 == copiedSHA {
			return RepairDecision{Copy: true, Reason: repairReasonUpgradeIntact}
		}
		return RepairDecision{Copy: false, Reason: repairReasonUpgradeEdited}
	}

	return RepairDecision{Copy: false, Reason: repairReasonAlreadyOK}
}

// logDecision 把判定结果翻译成人话日志（只在需要解释的两种情况打，避免刷屏）。
func logDecision(name string, d RepairDecision) {
	switch d.Reason {
	case repairReasonUpgradeEdited:
		log.Printf("[runtimeassets] 资产 %s 已被用户手动修改，升级时跳过覆盖", name)
	case repairReasonUpgradeNoRec:
		log.Printf("[runtimeassets] 资产 %s 无拷贝记录，从 seed 同步", name)
	}
}

// clearNonFileTarget 处理"目标是目录"的历史脏状态（ErrIsDir）：
// 只允许清理 rootDir 下 6 个已知资产名，避免误删用户数据（F-A1-15）。
func clearNonFileTarget(rootDir, runtimePath, name string) error {
	if canonicalAssetName(name) == "" {
		return nil
	}
	if filepath.Dir(runtimePath) != filepath.Clean(rootDir) {
		return nil
	}
	st, err := os.Stat(runtimePath)
	if err != nil || !st.IsDir() {
		return nil
	}
	return os.RemoveAll(runtimePath)
}

func shortSHA(sha string) string {
	if len(sha) <= 12 {
		return sha
	}
	return sha[:12]
}

func RecordSeedCoreCopied(seedPath string) {
	state := LoadAssetState()

	// 记录**真实文件**的哈希，而不是清单里的：清单可能过期（实测清单 size/sha 与实际
	// 二进制不一致），按清单记账会让"升级覆盖"永远把内核判成用户魔改（F-A1-06）。
	target := runtimePathForCanonicalName("clash.exe")
	sha, hashErr := calculateSHA256(target)
	if hashErr != nil {
		sha, hashErr = calculateSHA256(seedPath)
	}

	catalog := LoadSeedCatalog()
	meta, hasMeta := catalog.Items["clash.exe"]
	if hasMeta && meta.SHA256 != "" && sha != "" && !strings.EqualFold(meta.SHA256, sha) {
		log.Printf("[runtimeassets] 清单与已安装内核哈希不一致（以实际文件为准）: manifest=%s actual=%s",
			meta.SHA256, shortSHA(sha))
	}
	if sha == "" && hasMeta {
		sha = meta.SHA256
	}

	state.CopiedAssets["clash.exe"] = struct {
		SHA256 string `json:"sha256"`
		Source string `json:"source"`
	}{
		SHA256: sha,
		Source: "seed",
	}
	_ = saveAssetState(state)
}
