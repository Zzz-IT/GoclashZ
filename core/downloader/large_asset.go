//go:build windows

package downloader

import (
	"context"
	"encoding/json"
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

	downloadSuccess := false
	defer func() {
		if downloadSuccess {
			_ = os.Remove(tmpPath + ".meta.json")
		}
	}()

	var lastErr error

	// slowAbandoned 标记本轮是否已经主动放弃过一个慢速源。
	// 放弃过一次后就不再触发 tooSlow，让剩下的候选跑到底——否则"所有源都偏慢"
	// 时会把每个候选都判死，用户只看到失败，而不是"慢但能下完"。
	slowAbandoned := false

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

				if metaData, err := os.ReadFile(tmpPath + ".meta.json"); err == nil {
					var meta struct {
						URL string `json:"URL"`
					}
					if err := json.Unmarshal(metaData, &meta); err == nil {
						if meta.URL != rawURL {
							_ = os.Remove(tmpPath)
							_ = os.Remove(tmpPath + ".meta.json")
						}
					}
				}

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

				t := time.NewTicker(400 * time.Millisecond)
				strategyChanged := false
				tooSlow := false
				dlStart := time.Now()
				var smoothedSpeed float64
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

						// 🚀 慢速主动放弃：起步一段时间（已建立连接）后，若速度持续过低且
						// 完成度还很小，说明当前通道（尤其镜像）质量差，主动 Cancel 换下一个候选，
						// 避免在低速源上空等整个超时。
						// 只放弃一次：之后的候选即使慢也让它跑完，保证总有兜底能出结果，
						// 否则所有源都偏慢时会被逐个判死，用户只看到失败。
						if elapsed := time.Since(dlStart); elapsed > 6*time.Second {
							speed := resp.BytesPerSecond()
							progress := float64(0)
							if size := resp.Size(); size > 0 {
								progress = float64(resp.BytesComplete()) / float64(size)
							}
							if speed > 0 && speed < 100*1024 && progress < 0.1 && !slowAbandoned {
								resp.Cancel()
								tooSlow = true
								slowAbandoned = true
								break loop
							}
						}

						if opt.OnProgress != nil {
							instSpeed := resp.BytesPerSecond()
							if smoothedSpeed <= 0 {
								smoothedSpeed = instSpeed
							} else if instSpeed > 0 {
								smoothedSpeed = smoothedSpeed*0.7 + instSpeed*0.3
							}

							eta := int64(-1)
							remBytes := resp.Size() - resp.BytesComplete()
							if smoothedSpeed > 1024 && remBytes > 0 {
								eta = int64(float64(remBytes) / smoothedSpeed)
							} else if !resp.ETA().IsZero() {
								if d := time.Until(resp.ETA()); d > 0 {
									eta = int64(d.Seconds())
								}
							}
							opt.OnProgress(resp.BytesComplete(), resp.Size(), int64(smoothedSpeed), eta)
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

					// 慢速主动放弃：立即换下一个候选 URL，不做退避等待
					if tooSlow {
						continue
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
