package runtimeassets

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"goclashz/core/sys"
	"goclashz/core/utils"
)

type coreVersionCacheItem struct {
	version   string
	size      int64
	modTime   int64
	timestamp time.Time
}

var (
	coreVersionRe       = regexp.MustCompile(`v?\d+\.\d+\.\d+(?:[-+][^\s]+)?`)
	coreVersionCacheMap = make(map[string]coreVersionCacheItem)
	coreVersionCacheMu  sync.Mutex
)

// fallbackBinDirs 返回同机上可能存放运行组件的其他目录。
//
// 历史坑：程序在 <repo>/bin 运行、而资产在 <repo>/data/core/bin（或助手目录）时，
// 自检只看 <dataDir>/core/bin，于是"wintun 明明装了也在用"却被判成缺少驱动。
// 这里把这些布局都收集起来，发现可用文件就迁入运行目录（自愈）。
func fallbackBinDirs() []string {
	appDir := utils.GetAppDir()
	parent := filepath.Dir(appDir)
	primary := filepath.Clean(utils.GetCoreBinDir())

	dirs := []string{
		filepath.Join(appDir, "core", "bin"),
		filepath.Join(appDir, "data", "core", "bin"),
		filepath.Join(parent, "data", "core", "bin"),
		filepath.Join(parent, "core", "bin"),
	}

	out := make([]string, 0, len(dirs))
	seen := map[string]bool{primary: true}
	for _, dir := range dirs {
		clean := filepath.Clean(dir)
		if seen[clean] {
			continue
		}
		seen[clean] = true
		out = append(out, clean)
	}
	return out
}

// AdoptFallback 在运行目录缺少某资产时，从同机其它已知布局把它复制进来（**会写盘**）。
//
// 只读检查路径（CheckCore / CheckWintun / GetStatus 系列）不再隐式调用它：写盘只允许
// 发生在 EnsureReady 进入修复分支之前的这一次显式采纳（见 adoptFallbackAssets，每资产
// 每进程至多一次）。
// TODO(待 W3)：候选来源仍只做 PE 魔数+体积校验，不比对 seed manifest 的 sha256；
// 目标形态是把来源收敛到 seed 目录并校验哈希。
func AdoptFallback(name string) {
	// 与 repair/migrate 共用同一把资产锁：采纳也是写运行目录（F-A1-13）。
	_ = withAssetLock(context.Background(), name, 2*time.Second, func() error {
		adoptFallbackLocked(name)
		return nil
	})
}

func adoptFallbackLocked(name string) {
	primaryDir := utils.GetCoreBinDir()
	primary := filepath.Join(primaryDir, name)
	if st, err := os.Stat(primary); err == nil && !st.IsDir() {
		return
	}

	for _, dir := range fallbackBinDirs() {
		src := filepath.Join(dir, name)
		st, err := os.Stat(src)
		if err != nil || st.IsDir() {
			continue
		}
		if err := os.MkdirAll(primaryDir, 0755); err != nil {
			return
		}
		if err := utils.CopyFile(src, primary); err != nil {
			log.Printf("[runtimeassets] 资产自愈复制失败: %s -> %s: %v", src, primary, err)
			continue
		}
		log.Printf("[runtimeassets] 资产已从其他布局迁入运行目录: %s -> %s", src, primary)
		return
	}
}

// CheckCore 是只读全量检查：不写盘、不采纳 fallback 目录（采纳见 AdoptFallback）。
// 不计算 SHA256——哈希只在自愈/升级比对时按需计算（F-A1-14）。
func CheckCore(ctx context.Context) AssetHealth {
	path := filepath.Join(utils.GetCoreBinDir(), "clash.exe")
	return checkCoreByPath(ctx, path, false)
}

// checkCoreByPath 是内核的深检：PE 头 + 版本探测（-v），withHash 时才额外算 SHA256。
//
// 可执行性判定（F-A1-05）——三种结果必须分开：
//  1. 探测成功：Ready=true、VersionProbeOK=true；
//  2. 探测超时（杀软/冷盘扫秒级拖慢是常态）：Ready 保持 true，只降级版本文案。
//     绝不因为一次超时就重拷 61 MB；
//  3. 探测明确失败（能读 PE，但进程起不来/退出即错）：Ready=false + ErrExecFailed，
//     让 EnsureReady 走自愈重拷——这才是"损坏内核"。
func checkCoreByPath(ctx context.Context, path string, withHash bool) AssetHealth {
	h := baseHealth(AssetCore, "Mihomo 内核", path, true)

	info, err := os.Stat(path)
	if err != nil {
		h.ErrorCode = ErrMissing
		h.Error = "内核文件不存在"
		h.Hint = "请在「设置 → 组件与库更新」点「更新内核」修复。"
		return h
	}
	if info.IsDir() {
		h.Exists = true
		h.ErrorCode = ErrIsDir
		h.Error = "内核路径是目录，不是可执行文件"
		h.Hint = "运行目录里 clash.exe 被建成了目录；自愈会尝试清理该目录后重装内核。"
		return h
	}

	h.Exists = true
	h.Size = info.Size()
	h.ModTime = info.ModTime().Unix()

	if err := utils.ValidateWindowsPE(path, 5*1024*1024, 300*1024*1024); err != nil {
		h.ErrorCode = ErrInvalidPE
		h.Error = err.Error()
		h.Hint = "当前 clash.exe 已损坏或不是 Windows amd64 可执行文件；请点「更新内核」用内置版本覆盖。"
		return h
	}

	h.Valid = true
	h.Ready = true

	if withHash {
		if hash, herr := calculateSHA256Context(ctx, path); herr == nil {
			h.SHA256 = hash
		}
	}

	if cerr := ctx.Err(); cerr != nil {
		h.Version = "已安装，版本未知"
		h.Hint = "检查预算已用尽（跳过 -v 探测）；内核格式有效，可下次再检。"
		return h
	}

	version, execErr := ReadCoreVersionContext(ctx, path)
	if execErr != nil {
		h.Version = "已安装，版本未知"
		if errors.Is(execErr, ErrProbeTimeout) {
			h.Hint = "版本探测超时（多半是杀软/磁盘扫描拖慢），内核格式有效，可直接启动。"
			return h
		}
		h.Ready = false
		h.ErrorCode = ErrExecFailed
		h.Error = fmt.Sprintf("内核无法执行: %v", execErr)
		h.Hint = "文件格式有效但无法运行；请点「更新内核」用内置版本覆盖。"
		return h
	}

	h.Version = version
	h.VersionProbeOK = true
	return h
}

// 内核版本探测的预算与错误语义。
const coreVersionProbeTimeout = 5 * time.Second

// ErrProbeTimeout 表示内核 -v 探测超时。它必须与"文件损坏"区分开：
// 冷启动/杀软扫描下 5s 不够是常态，据此重拷 61 MB 才是真故障。
var ErrProbeTimeout = errors.New("core version probe timeout")

// coreVersionProbeMu 只串行化"起子进程"这一段，不保护缓存（F-A1-09：
// 历史实现在 spawn 全程持有缓存锁，一次 5s 探针会把所有 GetStatus 串行拖死）。
var coreVersionProbeMu sync.Mutex

// ReadCoreVersion 兼容既有签名（内部按 Background 处理，不响应取消）。
func ReadCoreVersion(path string) (string, error) {
	return ReadCoreVersionContext(context.Background(), path)
}

// ReadCoreVersionContext 读取内核版本：
//   - 命中 (size,mtime) 且 60s 内的缓存 → 直接返回，不 spawn；
//   - spawn 期间不持缓存锁，只持 probe 锁；
//   - 超时返回 ErrProbeTimeout（可 errors.Is 区分超时与执行失败）。
func ReadCoreVersionContext(ctx context.Context, path string) (string, error) {
	stat, statErr := os.Stat(path)
	if statErr == nil {
		if v, ok := cachedCoreVersion(path, stat, true); ok {
			return v, nil
		}
	}

	if !lockWithContext(ctx, &coreVersionProbeMu, coreVersionProbeTimeout) {
		if cerr := ctx.Err(); cerr != nil {
			return "", fmt.Errorf("%w: %v", ErrProbeTimeout, cerr)
		}
		return "", fmt.Errorf("%w: 等待版本探测锁超时", ErrProbeTimeout)
	}
	defer coreVersionProbeMu.Unlock()

	// 拿到锁后复核缓存：先到的调用方可能已经填好。
	if statErr == nil {
		if v, ok := cachedCoreVersion(path, stat, true); ok {
			return v, nil
		}
	}

	cmdCtx, cancel := context.WithTimeout(ctx, coreVersionProbeTimeout)
	defer cancel()

	cmd := exec.CommandContext(cmdCtx, path, "-v")
	cmd.Dir = filepath.Dir(path)
	utils.HideCommandWindow(cmd, 0)

	out, err := cmd.CombinedOutput()
	s := strings.TrimSpace(string(out))

	if err != nil {
		if s != "" {
			return s, nil
		}
		if cmdCtx.Err() != nil {
			return "", fmt.Errorf("%w: 超过 %s", ErrProbeTimeout, coreVersionProbeTimeout)
		}
		return "", fmt.Errorf("执行 clash.exe -v 失败: %w", err)
	}

	if s == "" {
		return "已安装，版本未知", nil
	}

	ver := s
	if m := coreVersionRe.FindString(s); m != "" {
		if strings.HasPrefix(m, "v") {
			ver = m
		} else {
			ver = "v" + m
		}
	}

	if statErr == nil {
		storeCoreVersion(path, stat, ver)
	}
	return ver, nil
}

// cachedCoreVersion 读版本缓存。freshOnly=true 时套 60s TTL（用于"避免 spawn"）；
// freshOnly=false 时只校验 (size,mtime)（用于"只读展示"）。
func cachedCoreVersion(path string, stat os.FileInfo, freshOnly bool) (string, bool) {
	coreVersionCacheMu.Lock()
	defer coreVersionCacheMu.Unlock()

	item, ok := coreVersionCacheMap[path]
	if !ok {
		return "", false
	}
	if item.size != stat.Size() || item.modTime != stat.ModTime().UnixNano() {
		return "", false
	}
	if freshOnly && time.Since(item.timestamp) >= time.Minute {
		return "", false
	}
	if strings.TrimSpace(item.version) == "" {
		return "", false
	}
	return item.version, true
}

func storeCoreVersion(path string, stat os.FileInfo, version string) {
	coreVersionCacheMu.Lock()
	coreVersionCacheMap[path] = coreVersionCacheItem{
		version:   version,
		size:      stat.Size(),
		modTime:   stat.ModTime().UnixNano(),
		timestamp: time.Now(),
	}
	coreVersionCacheMu.Unlock()
}

// CheckWintun 是只读全量检查：不写盘、不采纳 fallback 目录（采纳见 AdoptFallback）。
//
// 只认运行目录 <dataDir>/core/bin/wintun.dll：旁路目录里即使有可用副本也**不算 Ready**，
// 只在 Hint 里提示"检测到旁路副本"——因为内核只会从自己的 binDir 加载 DLL，把旁路副本
// 判成就绪会造成"自检通过、TUN 起不来"（F-A1-07）。
func CheckWintun() AssetHealth {
	primary := filepath.Join(utils.GetCoreBinDir(), "wintun.dll")
	health := checkWintunByPath(primary, false)
	if health.Ready {
		return health
	}
	if alt := findAltWintun(primary); alt != "" {
		health.Hint = fmt.Sprintf("检测到旁路副本但运行目录没有：%s；请点「重装驱动」迁入运行目录。", alt)
	}
	return health
}

// altWintunPaths 列出历史布局中可能出现 wintun.dll 的旁路目录。
func altWintunPaths(primary string) []string {
	appDir := utils.GetAppDir()
	candidates := []string{
		filepath.Join(appDir, "core", "bin", "wintun.dll"),
		filepath.Join(appDir, "data", "core", "bin", "wintun.dll"),
	}
	out := make([]string, 0, len(candidates))
	for _, alt := range candidates {
		if alt != primary {
			out = append(out, alt)
		}
	}
	return out
}

// findAltWintun 仅用于生成提示：返回第一个"看起来可用"的旁路副本路径（不改变 Ready）。
func findAltWintun(primary string) string {
	for _, alt := range altWintunPaths(primary) {
		if info, err := os.Stat(alt); err != nil || info.IsDir() {
			continue
		}
		if err := utils.ValidateWindowsPE(alt, 32*1024, 5*1024*1024); err != nil {
			continue
		}
		return alt
	}
	return ""
}

func checkWintunByPath(path string, withHash bool) AssetHealth {
	h := baseHealth(AssetWintun, "Wintun 驱动 DLL", path, true)

	info, err := os.Stat(path)
	if err != nil {
		h.ErrorCode = ErrMissing
		h.Error = "wintun.dll 不存在"
		h.Hint = "请在「设置 → 组件与库更新」点「重装驱动」修复。"
		return h
	}
	if info.IsDir() {
		h.Exists = true
		h.ErrorCode = ErrIsDir
		h.Error = "wintun 路径是目录，不是 DLL 文件"
		h.Hint = "运行目录里 wintun.dll 被建成了目录；自愈会尝试清理该目录后重装驱动。"
		return h
	}

	h.Exists = true
	h.Size = info.Size()
	h.ModTime = info.ModTime().Unix()

	if err := utils.ValidateWindowsPE(path, 32*1024, 5*1024*1024); err != nil {
		h.ErrorCode = ErrInvalidPE
		h.Error = err.Error()
		h.Hint = "当前 wintun.dll 已损坏或不是有效 Windows DLL；请点「重装驱动」修复。"
		return h
	}

	h.Valid = true
	h.Ready = true

	v, verr := sys.GetFileVersion(path)
	if verr == nil && strings.TrimSpace(v) != "" {
		h.Version = v
	} else {
		h.Version = "已安装，版本未知"
	}

	if withHash {
		if hash, herr := calculateSHA256Context(context.Background(), path); herr == nil {
			h.SHA256 = hash
		}
	}

	return h
}

func CheckDataFile(key AssetKey, label string, filename string) AssetHealth {
	path := filepath.Join(utils.GetCoreBinDir(), filename)
	return checkDataFileByPath(key, label, path, false)
}

// minDataFileBytes 是规则库的最小合理体积：0 字节/被截断的规则库必须判为损坏，
// 否则 Ready 恒为 true、自愈永远不会被触发（F-A1-05）。
const minDataFileBytes = 1024

func checkDataFileByPath(key AssetKey, label string, path string, withHash bool) AssetHealth {
	h := baseHealth(key, label, path, false)

	info, err := os.Stat(path)
	if err != nil {
		h.ErrorCode = ErrMissing
		h.Error = label + " 文件不存在"
		return h
	}
	if info.IsDir() {
		h.Exists = true
		h.ErrorCode = ErrIsDir
		h.Error = label + " 路径是目录"
		return h
	}

	h.Exists = true
	h.Size = info.Size()
	h.ModTime = info.ModTime().Unix()

	if info.Size() < minDataFileBytes {
		h.ErrorCode = ErrCorrupt
		h.Error = fmt.Sprintf("%s 文件不完整（%d 字节）", label, info.Size())
		h.Hint = "文件为空或被截断；可在「组件与库更新」重新下载该规则库。"
		return h
	}

	h.Valid = true
	h.Ready = true

	if withHash {
		if hash, herr := calculateSHA256Context(context.Background(), path); herr == nil {
			h.SHA256 = hash
		}
	}

	return h
}

// --- 快检（L0/L1） -------------------------------------------------------
//
// 快检只做 stat + PE 头 + 文件版本（Win32 版本 API）：**不跑 clash.exe -v、不算 SHA256**，
// 因此可以安全地挂在 UI 与 watchdog 上（单次成本从"读满 95 MB"降到若干次 stat）。
// 需要 SHA256 或真实内核版本时走全量检查（CheckCore / GetStatus）。

// peekCoreVersion 只读版本缓存，绝不 spawn 子进程。
// 只要 (size, mtime) 与缓存一致，就认为版本仍然有效——内核文件没变，版本就不会变；
// 这里刻意不套 60s TTL：TTL 的目的是"别重复跑进程"，而读缓存不需要。
func peekCoreVersion(path string) (string, bool) {
	info, err := os.Stat(path)
	if err != nil {
		return "", false
	}
	return cachedCoreVersion(path, info, false)
}

func checkCoreByPathFast(path string) AssetHealth {
	h := baseHealth(AssetCore, "Mihomo 内核", path, true)

	info, err := os.Stat(path)
	if err != nil {
		h.ErrorCode = ErrMissing
		h.Error = "内核文件不存在"
		h.Hint = "请在「设置 → 组件与库更新」点「更新内核」修复。"
		return h
	}
	if info.IsDir() {
		h.Exists = true
		h.ErrorCode = ErrIsDir
		h.Error = "内核路径是目录，不是可执行文件"
		return h
	}

	h.Exists = true
	h.Size = info.Size()
	h.ModTime = info.ModTime().Unix()

	if err := utils.ValidateWindowsPE(path, 5*1024*1024, 300*1024*1024); err != nil {
		h.ErrorCode = ErrInvalidPE
		h.Error = err.Error()
		h.Hint = "当前 clash.exe 已损坏或不是 Windows amd64 可执行文件。"
		return h
	}

	h.Valid = true
	h.Ready = true

	// 版本：先看 PE 版本资源；没有再退回"最近一次 -v 的结果缓存"（只读缓存，不 spawn）。
	if v, verr := sys.GetFileVersion(path); verr == nil && strings.TrimSpace(v) != "" {
		h.Version = v
	} else if v, ok := peekCoreVersion(path); ok {
		h.Version = v
	}
	return h
}

func checkCoreFast() AssetHealth {
	return checkCoreByPathFast(filepath.Join(utils.GetCoreBinDir(), "clash.exe"))
}

func checkWintunByPathFast(path string) AssetHealth {
	h := baseHealth(AssetWintun, "Wintun 驱动 DLL", path, true)

	info, err := os.Stat(path)
	if err != nil {
		h.ErrorCode = ErrMissing
		h.Error = "wintun.dll 不存在"
		h.Hint = "请在「设置 → 组件与库更新」点「重装驱动」修复。"
		return h
	}
	if info.IsDir() {
		h.Exists = true
		h.ErrorCode = ErrIsDir
		h.Error = "wintun 路径是目录，不是 DLL 文件"
		return h
	}

	h.Exists = true
	h.Size = info.Size()
	h.ModTime = info.ModTime().Unix()

	if err := utils.ValidateWindowsPE(path, 32*1024, 5*1024*1024); err != nil {
		h.ErrorCode = ErrInvalidPE
		h.Error = err.Error()
		h.Hint = "当前 wintun.dll 已损坏或不是有效 Windows DLL。"
		return h
	}

	h.Valid = true
	h.Ready = true

	if v, verr := sys.GetFileVersion(path); verr == nil && strings.TrimSpace(v) != "" {
		h.Version = v
	}
	return h
}

// checkWintunFast 与 CheckWintun 语义一致：只认运行目录；旁路副本只进 Hint（F-A1-07）。
func checkWintunFast() AssetHealth {
	primary := filepath.Join(utils.GetCoreBinDir(), "wintun.dll")
	health := checkWintunByPathFast(primary)
	if health.Ready {
		return health
	}
	if alt := findAltWintun(primary); alt != "" {
		health.Hint = fmt.Sprintf("检测到旁路副本但运行目录没有：%s；请点「重装驱动」迁入运行目录。", alt)
	}
	return health
}

func checkDataFileByPathFast(key AssetKey, label string, path string) AssetHealth {
	h := baseHealth(key, label, path, false)

	info, err := os.Stat(path)
	if err != nil {
		h.ErrorCode = ErrMissing
		h.Error = label + " 文件不存在"
		return h
	}
	if info.IsDir() {
		h.Exists = true
		h.ErrorCode = ErrIsDir
		h.Error = label + " 路径是目录"
		return h
	}

	h.Exists = true
	h.Size = info.Size()
	h.ModTime = info.ModTime().Unix()

	if info.Size() < minDataFileBytes {
		h.ErrorCode = ErrCorrupt
		h.Error = fmt.Sprintf("%s 文件不完整（%d 字节）", label, info.Size())
		h.Hint = "文件为空或被截断；可在「组件与库更新」重新下载该规则库。"
		return h
	}

	h.Valid = true
	h.Ready = true
	return h
}

func checkDataFileFast(key AssetKey, label string, filename string) AssetHealth {
	return checkDataFileByPathFast(key, label, filepath.Join(utils.GetCoreBinDir(), filename))
}

// fastHealthFor 按资产键选择快检实现（运行目录），供 GetStatusFast / GetStatusCached 复用。
func fastHealthFor(key AssetKey, label string, name string) AssetHealth {
	switch key {
	case AssetCore:
		return checkCoreFast()
	case AssetWintun:
		return checkWintunFast()
	default:
		return checkDataFileFast(key, label, name)
	}
}

// fastHealthByPath 对任意路径做快检（种子库诊断用，不触碰运行目录）。
func fastHealthByPath(key AssetKey, label string, path string) AssetHealth {
	switch key {
	case AssetCore:
		return checkCoreByPathFast(path)
	case AssetWintun:
		return checkWintunByPathFast(path)
	default:
		return checkDataFileByPathFast(key, label, path)
	}
}

// calculateSHA256 兼容既有调用（不响应取消）。
func calculateSHA256(path string) (string, error) {
	return calculateSHA256Context(context.Background(), path)
}

// calculateSHA256Context 分块计算哈希（1MB/块），每块前检查 ctx：
// 大文件哈希是"重检查"的主体，必须能被预算打断（F-A1-08）。
func calculateSHA256Context(ctx context.Context, path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	buf := make([]byte, 1<<20)
	for {
		if cerr := ctx.Err(); cerr != nil {
			return "", cerr
		}
		n, readErr := f.Read(buf)
		if n > 0 {
			_, _ = h.Write(buf[:n]) // hash.Writer 永不返回错误
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return "", readErr
		}
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
