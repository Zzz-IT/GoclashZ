//go:build windows

package downloader

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cavaliergopher/grab/v3"
)

func DownloadLargeAssetAtomic(ctx context.Context, opt Options) error {
	if strings.TrimSpace(opt.DestPath) == "" {
		return fmt.Errorf("DestPath 为空")
	}
	if len(opt.URLs) == 0 {
		return fmt.Errorf("下载地址为空")
	}

	unlock := lockDest(opt.DestPath)
	defer unlock()

	tmpPath := opt.DestPath + ".tmp"
	_ = os.Remove(tmpPath)
	_ = os.Remove(tmpPath + ".meta.json")

	downloadSuccess := false
	defer func() {
		if !downloadSuccess {
			_ = os.Remove(tmpPath)
		}
		_ = os.Remove(tmpPath + ".meta.json")
	}()

	var lastErr error
	ua := opt.UserAgent
	if ua == "" {
		ua = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) GoclashZ/1.0"
	}

	attempts := opt.AttemptsPerEndpoint
	if attempts <= 0 {
		attempts = 3
	}

	var lastStrategy DownloadStrategy
	if opt.Strategy != nil {
		lastStrategy = opt.Strategy()
	}

	// 我们使用一个可中断的大循环来支持策略变更和失败重试
	// 将尝试次数乘以 2，以便在代理切换时有足够的重试次数
	maxAttempts := attempts * 2
	
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		// 每次重试前，都动态获取最新的策略
		if opt.Strategy != nil {
			lastStrategy = opt.Strategy()
		}

		clients := createOrderedClientsFromStrategy(opt, lastStrategy)

		for _, httpClient := range clients {
			grabClient := grab.NewClient()
			grabClient.HTTPClient = httpClient

			for _, rawURL := range opt.URLs {
				release := acquireHostLimiter(rawURL)

				req, err := grab.NewRequest(tmpPath, rawURL)
				if err != nil {
					release()
					lastErr = err
					continue
				}

				req = req.WithContext(ctx)
				req.IgnoreRemoteTime = true
				req.HTTPRequest.Header.Set("User-Agent", ua)
				req.HTTPRequest.Header.Set("Accept", "application/octet-stream,*/*")
				req.HTTPRequest.Header.Set("Cache-Control", "no-cache")

				resp := grabClient.Do(req)

				t := time.NewTicker(500 * time.Millisecond)
				strategyChanged := false
			loop:
				for {
					select {
					case <-t.C:
						// 🚀 核心机制：轮询下载进度时，实时检测代理策略是否发生改变
						if opt.Strategy != nil {
							currentStrategy := opt.Strategy()
							if currentStrategy.PreferProxy != lastStrategy.PreferProxy || currentStrategy.ProxyURL != lastStrategy.ProxyURL {
								// 代理状态或 URL 发生了改变，立刻中断当前的下载！
								resp.Cancel()
								lastStrategy = currentStrategy
								strategyChanged = true
								break loop
							}
						}

						if opt.OnProgress != nil {
							eta := int64(-1)
							if !resp.ETA().IsZero() {
								if d := time.Until(resp.ETA()); d > 0 {
									eta = int64(d.Seconds())
								}
							}
							opt.OnProgress(resp.BytesComplete(), resp.Size(), int64(resp.BytesPerSecond()), eta)
						}
					case <-resp.Done:
						break loop
					}
				}
				t.Stop()
				release()

				if err := resp.Err(); err != nil {
					lastErr = err
					
					// 如果是因为策略改变导致的取消，我们不需要等待，直接跳出双层循环，立即触发外层的全新 attempt！
					if strategyChanged {
						goto NEXT_ATTEMPT
					}

					// 正常的网络失败，等待后重试
					select {
					case <-time.After(time.Duration(attempt) * 800 * time.Millisecond):
					case <-ctx.Done():
						return ctx.Err()
					}
					continue
				}

				if opt.MaxBytes > 0 && resp.BytesComplete() > opt.MaxBytes {
					_ = os.Remove(tmpPath)
					lastErr = fmt.Errorf("下载文件超过大小限制")
					break
				}

				if opt.VerifyGitHubSHA {
					expectedSHA, err := ResolveGitHubAssetSHA256(ctx, httpClient, resp.Request.HTTPRequest.URL.String(), ua)
					if err != nil {
						if opt.RequireGitHubSHA {
							_ = os.Remove(tmpPath)
							lastErr = fmt.Errorf("获取 SHA256 失败: %v", err)
							break
						}
					} else if expectedSHA != "" {
						if err := VerifySHA256(tmpPath, expectedSHA); err != nil {
							_ = os.Remove(tmpPath)
							lastErr = err
							continue // 校验失败允许重试
						}
					}
				}

				if opt.Validator != nil {
					if err := opt.Validator(tmpPath); err != nil {
						_ = os.Remove(tmpPath)
						lastErr = fmt.Errorf("安全校验失败: %v", err)
						break
					}
				}

				if err := os.MkdirAll(filepath.Dir(opt.DestPath), 0755); err != nil {
					_ = os.Remove(tmpPath)
					lastErr = err
					break
				}

				if err := ReplaceFile(tmpPath, opt.DestPath); err != nil {
					_ = os.Remove(tmpPath)
					lastErr = err
					
					select {
					case <-time.After(2 * time.Second):
					case <-ctx.Done():
						return ctx.Err()
					}
					continue
				}

				downloadSuccess = true
				return nil
			}
		}
	NEXT_ATTEMPT:
	}

	if lastErr == nil {
		lastErr = fmt.Errorf("下载失败")
	}

	return SanitizeDownloadError(lastErr)
}
