//go:build windows

package appcore

import (
	"context"
	"fmt"
	"goclashz/core/clash"
	"goclashz/core/downloader"
	"goclashz/core/logger"
	"goclashz/core/runtimeassets"
	"goclashz/core/sys"
	"goclashz/core/utils"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// 检查与下载拆成两个任务名：
// 之前两者共用 "app-update-flow"，导致"上一次自动检查还没释放任务名"时
// 用户的手动点击被误判成"正在下载"而弹 busy。拆开后手动检查可以安全抢占，
// 又不会影响真正的下载任务。
const (
	appUpdateCheckTask    = "app-update-check"
	appUpdateDownloadTask = "app-update-download"
)

func (c *Controller) updateGeoDatabase(ctx context.Context, key string, onProgress func(bytesDone, totalBytes, speedBps, etaSec int64)) error {
	var url string
	behavior := c.Behavior.Get()

	switch key {
	case "geoip":
		url = behavior.GeoIpLink
	case "geosite":
		url = behavior.GeoSiteLink
	case "mmdb":
		url = behavior.MmdbLink
	case "asn":
		url = behavior.AsnLink
	default:
		return fmt.Errorf("unknown geo key: %s", key)
	}

	if url == "" {
		return fmt.Errorf("下载链接未配置")
	}

	return clash.UpdateGeoDB(ctx, key, url, c.getDynamicStrategy, onProgress)
}

func (c *Controller) UpdateGeoDatabaseAsync(ctx context.Context, key string) {
	c.GeoUpdates.UpdateOneAsync(ctx, key)
}

func (c *Controller) UpdateAllGeoDatabasesAsync(ctx context.Context) {
	c.GeoUpdates.UpdateAllAsync(ctx)
}

func (c *Controller) IsCoreBinWritable() bool {
	testFile := filepath.Join(utils.GetCoreBinDir(), ".write_test")
	err := os.WriteFile(testFile, []byte("test"), 0644)
	if err != nil {
		return false
	}
	os.Remove(testFile)
	return true
}

func (c *Controller) UpdateCoreComponentAsync(ctx context.Context) {
	// 如果 core\bin 不可写，检查 helper 是否可用
	if !c.IsCoreBinWritable() {
		_ = c.EnsureHelperReady("core-update")
		helperStatus := sys.CheckHelperService()
		if !helperStatus.Reachable {
			errStr := "需要管理员权限进行内核更新 (组件被 Windows 安全策略保护)。请安装后台服务或以管理员身份重启软件后重试。"
			c.setLastError(errStr)
			c.UpdateTasks.Set("core-update", UpdateTaskState{
				Key:    "core-update",
				Title:  "Mihomo 内核更新",
				Status: "error",
				Error:  errStr,
			})
			return
		}
	}

	c.runComponentUpdateTransaction(ctx, "core-update", ComponentUpdateOptions{
		Name:        "Mihomo 内核更新",
		StopCore:    true,
		RestartCore: true,
		Prepare: func(ctx context.Context, onProgress func(int64, int64, int64, int64)) (map[string]string, error) {
			assetURL := ""
			// 优先使用前端检查更新时缓存的下载地址
			c.mu.RLock()
			cachedURL := c.pendingCoreUpdateAssetURL
			c.mu.RUnlock()

			if cachedURL != "" {
				assetURL = cachedURL
			} else {
				_, discoveredURL, _, err := clash.CheckLatestCore(ctx, c.getDynamicStrategy)
				if err != nil {
					return nil, err
				}
				assetURL = discoveredURL
			}

			return clash.PrepareCoreUpdate(ctx, assetURL, c.getDynamicStrategy, onProgress)
		},
		Commit: func(ctx context.Context, prepared map[string]string) (map[string]string, error) {
			version, err := clash.CommitCoreUpdate(ctx, prepared)
			if err != nil {
				return nil, err
			}
			return map[string]string{
				"version": version,
			}, nil
		},
		AfterSuccess: func(result map[string]string) {
			if version := result["version"]; version != "" {
				c.events.Emit("core-version-updated", map[string]string{
					"version": version,
				})
			}

			// 内核二进制更新后，连接和延迟状态不应沿用旧进程
			c.events.Emit("delay-cache-clear", "core-update")

			c.mu.Lock()
			c.pendingCoreUpdateAssetURL = ""
			c.pendingCoreUpdateVersion = ""
			c.mu.Unlock()
		},
	})
}

func (c *Controller) CheckCoreUpdateAsync(ctx context.Context) {
	c.Tasks.Run(ctx, "core-update-check", true, func(ctx context.Context) error {
		timeoutCtx, cancel := context.WithTimeout(ctx, 40*time.Second)
		defer cancel()

		local := clash.GetLocalCoreVersion(timeoutCtx)

		remote, assetURL, releaseURL, err := clash.CheckLatestCore(timeoutCtx, c.getDynamicStrategy)
		if err != nil {
			return err
		}

		cmp, err := clash.CompareCoreVersion(remote, local)
		if err != nil {
			return err
		}

		if cmp <= 0 {
			c.mu.Lock()
			c.pendingCoreUpdateAssetURL = ""
			c.pendingCoreUpdateVersion = ""
			c.mu.Unlock()

			c.events.Emit("core-update-none", map[string]string{
				"local":  local,
				"remote": remote,
			})
			return nil
		}

		c.mu.Lock()
		c.pendingCoreUpdateAssetURL = assetURL
		c.pendingCoreUpdateVersion = remote
		c.mu.Unlock()

		c.events.Emit("core-update-available", map[string]string{
			"local":      local,
			"remote":     remote,
			"assetUrl":   assetURL,
			"releaseUrl": releaseURL,
		})

		return nil
	})
}

func (c *Controller) InstallTunDriverAsync(ctx context.Context) {
	c.runComponentUpdateTransaction(ctx, "driver-install", ComponentUpdateOptions{
		Name:        "Wintun 重装",
		StopCore:    true,
		RestartCore: true,
		Prepare: func(ctx context.Context, onProgress func(int64, int64, int64, int64)) (map[string]string, error) {
			return clash.PrepareWintunRuntime(ctx, c.getDynamicStrategy, onProgress)
		},
		Commit: func(ctx context.Context, prepared map[string]string) (map[string]string, error) {
			version, err := clash.CommitWintunRuntime(ctx, prepared)
			if err != nil {
				return nil, err
			}
			return map[string]string{
				"version": version,
			}, nil
		},
		AfterSuccess: func(result map[string]string) {
			if version := result["version"]; version != "" {
				c.events.Emit("wintun-version-updated", map[string]string{
					"version": version,
				})
			}
		},
	})
}

func (c *Controller) GetCoreVersion(ctx context.Context) string {
	return clash.GetLocalCoreVersion(ctx)
}

type CoreSeedCheckResult struct {
	CurrentVersion string `json:"currentVersion"`
	SeedVersion    string `json:"seedVersion"`
	IsCurrentLower bool   `json:"isCurrentLower"`
	IsUnknown      bool   `json:"isUnknown"`
	ShouldPrompt   bool   `json:"shouldPrompt"`
}

func isVersionUnknown(ver string) bool {
	v := strings.TrimSpace(ver)
	if v == "" || v == "未安装" || strings.Contains(v, "未知") {
		return true
	}
	_, err := clash.ParseVersionParts(v)
	return err != nil
}

func (c *Controller) GetSeedCoreVersion(ctx context.Context) string {
	seedPath := filepath.Join(utils.GetSeedCoreBinDir(), "clash.exe")
	if stat, err := os.Stat(seedPath); err == nil && !stat.IsDir() {
		if ver, err := runtimeassets.ReadCoreVersion(seedPath); err == nil && ver != "" && !strings.Contains(ver, "未知") {
			return ver
		}
	}

	catalog := runtimeassets.LoadSeedCatalog()
	if meta, ok := catalog.Items["clash.exe"]; ok && meta.Version != "" {
		v := strings.TrimSpace(meta.Version)
		if strings.HasPrefix(v, "v") || strings.HasPrefix(v, "V") {
			return v
		}
		return "v" + v
	}

	return ""
}

func (c *Controller) CheckCoreAgainstSeed(ctx context.Context) CoreSeedCheckResult {
	currentVer := clash.GetLocalCoreVersion(ctx)
	seedVer := c.GetSeedCoreVersion(ctx)

	behavior := c.Behavior.Get()
	result, newPromptEnabled := EvaluateCoreComparison(currentVer, seedVer, behavior.CoreComparisonPrompt)

	if result.IsUnknown && behavior.CoreComparisonPrompt != newPromptEnabled {
		behavior.CoreComparisonPrompt = newPromptEnabled
		_ = c.Behavior.SetAndSave(behavior)
	}

	return result
}

func EvaluateCoreComparison(currentVer, seedVer string, promptEnabled bool) (CoreSeedCheckResult, bool) {
	result := CoreSeedCheckResult{
		CurrentVersion: currentVer,
		SeedVersion:    seedVer,
	}

	// 1. 若当前内核版本不明，不提示，并自动关闭“内核对比更新提示”设置
	if isVersionUnknown(currentVer) {
		result.IsUnknown = true
		result.ShouldPrompt = false
		return result, false
	}

	// 2. 若种子版本不明，无法对比，不提示
	if isVersionUnknown(seedVer) {
		result.ShouldPrompt = false
		return result, promptEnabled
	}

	// 3. 对比版本
	// CompareCoreVersion(seedVer, currentVer): 若 seedVer > currentVer 返回 1
	cmp, err := clash.CompareCoreVersion(seedVer, currentVer)
	if err != nil || cmp <= 0 {
		// 当前内核大于或等于种子内核，不提示
		result.IsCurrentLower = false
		result.ShouldPrompt = false
		return result, promptEnabled
	}

	// 4. 当前内核版本低于种子版本
	result.IsCurrentLower = true
	result.ShouldPrompt = promptEnabled

	return result, promptEnabled
}

func (c *Controller) ReplaceCoreWithSeed(ctx context.Context) (string, error) {
	seedPath := filepath.Join(utils.GetSeedCoreBinDir(), "clash.exe")
	if _, err := os.Stat(seedPath); err != nil {
		return "", fmt.Errorf("种子内核文件不存在: %s", seedPath)
	}

	if err := utils.ValidateWindowsPE(seedPath, 5*1024*1024, 300*1024*1024); err != nil {
		return "", fmt.Errorf("种子内核文件校验失败: %w", err)
	}

	c.componentUpdateMu.Lock()
	defer c.componentUpdateMu.Unlock()

	targetPath := filepath.Join(utils.GetCoreBinDir(), "clash.exe")
	if !c.IsCoreBinWritable() {
		_ = c.EnsureHelperReady("core-update")
		helperStatus := sys.CheckHelperService()
		if !helperStatus.Reachable {
			return "", fmt.Errorf("需要管理员权限进行内核替换。请安装后台服务或以管理员身份重启软件后重试")
		}
	}

	c.coreLifecycleMu.Lock()
	wasRunning := clash.IsRunning()
	c.mu.RLock()
	wantSysProxy := c.sysProxyActive
	wantTun := c.tunActive
	c.mu.RUnlock()
	shouldRestart := wasRunning || wantSysProxy || wantTun

	if wasRunning {
		c.stopCoreProcessLocked()
		c.coreLifecycleMu.Unlock()
		c.SyncState()
	} else {
		c.coreLifecycleMu.Unlock()
	}

	var replaceErr error
	if !c.IsCoreBinWritable() {
		client := sys.NewHelperClient()
		replaceErr = client.ReplaceCoreFile(sys.ReplaceCoreFileParams{
			Source: seedPath,
			Target: targetPath,
		})
	} else {
		if err := clash.WaitFileReleased(targetPath, 5*time.Second); err != nil {
			replaceErr = err
		} else {
			replaceErr = clash.ReplaceFileWithBackup(seedPath, targetPath)
		}
	}

	if replaceErr != nil {
		if shouldRestart {
			_ = c.EnsureCoreRunning(ctx)
		}
		c.SyncState()
		return "", fmt.Errorf("替换内核文件失败: %w", replaceErr)
	}

	clash.ClearLocalCoreVersionCache()
	newVer := clash.GetLocalCoreVersion(ctx)

	runtimeassets.RecordSeedCoreCopied(seedPath)

	if shouldRestart {
		if err := c.EnsureCoreRunning(ctx); err != nil {
			c.SyncState()
			return newVer, fmt.Errorf("内核替换成功，但恢复启动失败: %w", err)
		}
	}

	c.events.Emit("core-version-updated", map[string]string{
		"version": newVer,
	})
	c.events.Emit("delay-cache-clear", "core-replace-seed")
	c.SyncState()

	return newVer, nil
}

func (c *Controller) DisableCoreComparisonPrompt() error {
	behavior := c.Behavior.Get()
	behavior.CoreComparisonPrompt = false
	return c.Behavior.SetAndSave(behavior)
}

func (c *Controller) CheckAppUpdateAsync(ctx context.Context, currentVersion string, manual bool) {
	// 手动点击必须抢占：不能因为上一次启动自动检查还没释放任务名就把用户的
	// 点击挡回去弹 busy。检查与下载用不同任务名，所以抢占只会取消上一次"检查"，
	// 绝不会误伤正在跑的 40MB 下载。
	if manual {
		c.Tasks.Run(ctx, appUpdateCheckTask, false, func(ctx context.Context) error {
			return c.runAppUpdateCheck(ctx, currentVersion, true)
		})
		return
	}

	// 自动检查：已有任务在跑就安静跳过，不打扰用户。
	_ = c.Tasks.RunIfIdle(ctx, appUpdateCheckTask, false, func(ctx context.Context) error {
		return c.runAppUpdateCheck(ctx, currentVersion, false)
	})
}

func (c *Controller) runAppUpdateCheck(ctx context.Context, currentVersion string, manual bool) error {
	if manual {
		c.events.Emit("app-update-check-start")
	}

	// 40s 预算：覆盖 2 client × 3 API 地址、单次 6s 的最坏遍历，
	// 不留出"候选还没试完就被掐断"的窗口。
	timeoutCtx, cancel := context.WithTimeout(ctx, 40*time.Second)
	defer cancel()

	info, err := downloader.CheckAppUpdate(timeoutCtx, currentVersion, c.getDynamicStrategy)
	if err != nil {
		if manual {
			c.events.Emit("app-update-error", "检查软件更新失败: "+err.Error())
		}
		return nil
	}

	if info == nil || !info.HasUpdate {
		if manual {
			c.events.Emit("app-update-none", map[string]string{
				"message": "当前已经是最新版本。",
			})
		}
		return nil
	}

	if info.DownloadURL == "" {
		if manual {
			c.events.Emit("app-update-error", fmt.Sprintf(
				"发现新版本 %s，但 Release 中没有匹配的软件本体安装包",
				info.Version,
			))
		}
		return nil
	}

	c.mu.Lock()
	c.pendingAppUpdateInfo = info
	c.mu.Unlock()

	c.events.Emit("app-update-available", map[string]any{
		"version": info.Version,
		"manual":  manual,
	})

	return nil
}

func (c *Controller) DownloadPendingAppUpdateAsync(ctx context.Context) {
	// 下载是真正的重型任务：这里必须保留 RunIfIdle。
	// 真有下载在跑时弹 busy 是正确行为，不能让第二次点击把它取消掉。
	ok := c.Tasks.RunIfIdle(ctx, appUpdateDownloadTask, false, func(ctx context.Context) error {
		c.mu.RLock()
		info := c.pendingAppUpdateInfo
		c.mu.RUnlock()

		if info == nil || !info.HasUpdate || info.DownloadURL == "" {
			c.events.Emit("app-update-error", "没有可下载的软件更新，请重新检查更新。")
			return nil
		}

		return c.downloadAppUpdateWithInfo(ctx, info)
	})

	if !ok {
		c.events.Emit("app-update-busy")
	}
}

func (c *Controller) CheckAndDownloadAppUpdateAsync(ctx context.Context, currentVersion string) {
	// 兼容接口：现在改为仅检查
	c.CheckAppUpdateAsync(ctx, currentVersion, true)
}

func (c *Controller) AutoCheckAndDownloadAppUpdateAsync(ctx context.Context, currentVersion string) {
	// 启动自动检查也改为仅检查
	c.CheckAppUpdateAsync(ctx, currentVersion, false)
}

func (c *Controller) downloadAppUpdateWithInfo(ctx context.Context, info *downloader.AppUpdateInfo) error {
	c.SetUpdateStatus(true, info.Version)
	c.setAppUpdateDownloading(true)
	defer c.setAppUpdateDownloading(false)

	c.events.Emit("app-update-start", map[string]string{
		"version": info.Version,
	})

	destDir := filepath.Join(utils.GetDataDir(), "updates")

	onProgress := func(bytesDone, totalBytes, speedBps, etaSec int64) {
		c.events.Emit("app-update-progress", map[string]interface{}{
			"bytesDone":  bytesDone,
			"totalBytes": totalBytes,
			"speedBps":   speedBps,
			"etaSec":     etaSec,
		})
	}

	// 🚀 这里只调用一次，底层的 DownloadLargeAssetAtomic 会通过 Strategy() 回调动态感知代理切换并自动进行内部无感断点续传！
	path, err := downloader.DownloadAppUpdate(ctx, info, destDir, onProgress, c.getDynamicStrategy)
	if err != nil {
		logger.Errorf("软件更新下载失败: %v", err)
		c.events.Emit("app-update-error", userFacingAppUpdateDownloadError(err))
		return err
	}

	c.SetDownloadedAppUpdate(path, info.Version)
	c.events.Emit("app-update-downloaded", map[string]string{
		"version": info.Version,
		"path":    path,
	})
	return nil
}

func (c *Controller) getDynamicStrategy() downloader.DownloadStrategy {
	c.mu.RLock()
	preferProxy := c.sysProxyActive || c.tunActive
	c.mu.RUnlock()

	// 注意：resolveLocalProxyURL() 只是获取格式化字符串，它不启动内核。
	// 如果用户当前没有开启代理（preferProxy 为 false），且直连失败了，
	// 下载机将会尝试走代理兜底。如果此时内核未启动，代理请求也会迅速失败（connection refused）。
	// 这是合理的行为，因为我们不希望在用户完全没有开启代理时偷偷在后台启动内核耗电。
	return downloader.DownloadStrategy{
		ProxyURL:    resolveLocalProxyURL(),
		PreferProxy: preferProxy,
	}
}

func userFacingAppUpdateDownloadError(err error) string {
	if err == nil {
		return "下载软件更新失败，请稍后重试。"
	}

	msg := strings.ToLower(err.Error())

	// 映射网络类错误
	if strings.Contains(msg, "wsarecv") ||
		strings.Contains(msg, "connection was aborted") ||
		strings.Contains(msg, "timeout") ||
		strings.Contains(msg, "context deadline exceeded") ||
		strings.Contains(msg, "所有网络环境") ||
		strings.Contains(msg, "github") {
		return "下载软件更新失败：当前网络无法连接下载地址，请检查网络或切换可用节点后重试。"
	}

	// 映射文件效验类错误
	if strings.Contains(msg, "mz") ||
		strings.Contains(msg, "不是有效的 windows 可执行文件") ||
		strings.Contains(msg, "体积异常") {
		return "下载的软件安装包无效，请稍后重新下载。"
	}

	return "下载软件更新失败，请稍后重试。"
}
