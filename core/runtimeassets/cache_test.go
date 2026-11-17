package runtimeassets

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestAssetCacheHitMtimeTTL 覆盖快检缓存的三种命运：命中 / mtime 变化失效 / TTL 过期失效（F-A1-19）。
func TestAssetCacheHitMtimeTTL(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "asset.bin")
	if err := os.WriteFile(path, []byte("v1"), 0644); err != nil {
		t.Fatal(err)
	}

	storeCachedHealth(AssetHealth{Key: AssetCore, Path: path, Exists: true, Valid: true, Ready: true, Version: "v1"})
	if h, ok := loadCachedHealth(path); !ok || h.Version != "v1" {
		t.Fatalf("应命中缓存: ok=%v version=%q", ok, h.Version)
	}

	// mtime/size 变化 -> 失效
	time.Sleep(10 * time.Millisecond)
	if err := os.WriteFile(path, []byte("v2-longer"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, ok := loadCachedHealth(path); ok {
		t.Fatal("文件变化后缓存应失效")
	}

	// TTL 过期 -> 失效
	storeCachedHealth(AssetHealth{Key: AssetCore, Path: path, Exists: true, Valid: true, Ready: true})
	assetCacheMu.Lock()
	entry := assetCache[path]
	entry.at = time.Now().Add(-assetCacheTTL - time.Second)
	assetCache[path] = entry
	assetCacheMu.Unlock()
	if _, ok := loadCachedHealth(path); ok {
		t.Fatal("超过 TTL 应失效")
	}
}

// TestAssetCacheSkipsMissingAndCarriesVersion 覆盖"缺失不缓存"与"版本号不被快检擦掉"。
func TestAssetCacheSkipsMissingAndCarriesVersion(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "missing.bin")
	storeCachedHealth(AssetHealth{Key: AssetGeoIP, Path: missing, Exists: false})
	if _, ok := loadCachedHealth(missing); ok {
		t.Fatal("不存在的资产不应进入缓存")
	}

	path := filepath.Join(dir, "clash.exe")
	if err := os.WriteFile(path, []byte("MZ"), 0755); err != nil {
		t.Fatal(err)
	}
	storeCachedHealth(AssetHealth{Key: AssetCore, Path: path, Exists: true, Valid: true, Ready: true, Version: "v1.19.33"})
	storeCachedHealth(AssetHealth{Key: AssetCore, Path: path, Exists: true, Valid: true, Ready: true})
	if h, ok := loadCachedHealth(path); !ok || h.Version != "v1.19.33" {
		t.Fatalf("快检不应擦掉已知版本: ok=%v version=%q", ok, h.Version)
	}
}
