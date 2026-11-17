package runtimeassets

import (
	"os"
	"sync"
	"time"
)

// 快检结果缓存：由 GetStatusFast（以及 GetStatus 全量检查）填充，GetStatusCached 读取。
//
// 失效条件（任一满足即视为未命中）：
//  1. 文件的 (size, mtime) 与缓存不一致 —— 资产被自愈替换/在线更新后立即失效；
//  2. 距写入超过 assetCacheTTL（60s）—— 保证错误码之类的判断不会长期陈旧。
const assetCacheTTL = 60 * time.Second

type assetCacheEntry struct {
	size    int64
	modTime int64
	health  AssetHealth
	at      time.Time
}

var (
	assetCacheMu sync.Mutex
	assetCache   = make(map[string]assetCacheEntry)
)

// loadCachedHealth 尝试命中缓存（缓存键是资产文件的绝对路径）。
// 命中前会先 stat 一次，用 size/mtime 双向校验，避免"文件已换但缓存还活着"。
func loadCachedHealth(path string) (AssetHealth, bool) {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return AssetHealth{}, false
	}

	assetCacheMu.Lock()
	defer assetCacheMu.Unlock()

	item, ok := assetCache[path]
	if !ok {
		return AssetHealth{}, false
	}
	if item.size != info.Size() || item.modTime != info.ModTime().UnixNano() {
		delete(assetCache, path)
		return AssetHealth{}, false
	}
	if time.Since(item.at) >= assetCacheTTL {
		delete(assetCache, path)
		return AssetHealth{}, false
	}
	return item.health, true
}

// storeCachedHealth 写入缓存。不存在的资产不缓存（下次调用仍应如实观测"缺失"）。
//
// 若新结果没带版本号、而旧缓存有版本号且 (size, mtime) 一致，则保留旧版本号：
// 快检不跑 -v，不能因为一次快检就把 UI 上已经显示的版本擦成"未知"。
func storeCachedHealth(h AssetHealth) {
	if h.Path == "" || !h.Exists || h.ErrorCode == ErrIsDir {
		return
	}

	info, err := os.Stat(h.Path)
	if err != nil || info.IsDir() {
		return
	}
	size, modTime := info.Size(), info.ModTime().UnixNano()

	assetCacheMu.Lock()
	defer assetCacheMu.Unlock()

	if h.Version == "" {
		if old, ok := assetCache[h.Path]; ok && old.size == size && old.modTime == modTime {
			h.Version = old.health.Version
		}
	}
	assetCache[h.Path] = assetCacheEntry{
		size:    size,
		modTime: modTime,
		health:  h,
		at:      time.Now(),
	}
}
