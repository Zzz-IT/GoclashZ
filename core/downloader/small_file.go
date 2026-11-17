//go:build windows

package downloader

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// smallFileFetchTimeout 是「小文件」（订阅 / YAML / 清单）整次抓取的总预算。
//
// 之前这里只有 http.Client.Timeout = 10 分钟，而 UpdateSub / UpdateSingleSub 是
// 同步 UI 调用，用户点一次「更新订阅」最坏要等 10 分钟且没有任何进度或取消入口
// （F-A5-08）。这里给整次抓取一个硬上限：超时直接返回可读错误，让调用方重试，
// 而不是把 UI 挂在那里。
var smallFileFetchTimeout = 3 * time.Minute

func FetchSmallFileAtomic(ctx context.Context, opt Options) error {
	if strings.TrimSpace(opt.DestPath) == "" {
		return fmt.Errorf("DestPath 为空")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	// 整次抓取的硬上限（见 smallFileFetchTimeout）：必须在发请求之前就把 ctx 兜住，
	// 后续所有并发竞速请求都会随它一起取消。
	if smallFileFetchTimeout > 0 {
		var cancelTimeout context.CancelFunc
		ctx, cancelTimeout = context.WithTimeout(ctx, smallFileFetchTimeout)
		defer cancelTimeout()
	}
	unlock := lockDest(opt.DestPath)
	defer unlock()

	var lastStrategy DownloadStrategy
	if opt.Strategy != nil {
		lastStrategy = opt.Strategy()
	}
	clients := createOrderedClientsFromStrategy(opt, lastStrategy)
	urls := opt.URLs
	totalRaces := len(clients) * len(urls)

	type result struct {
		body   []byte
		header http.Header
		err    error
	}
	resultCh := make(chan result, totalRaces)
	raceCtx, cancelRace := context.WithCancel(ctx)
	defer cancelRace()

	ua := opt.UserAgent
	if ua == "" {
		ua = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) GoclashZ/1.0"
	}

	for _, client := range clients {
		for _, u := range urls {
			go func(c *http.Client, target string) {
				req, err := http.NewRequestWithContext(raceCtx, http.MethodGet, target, nil)
				if err != nil {
					resultCh <- result{err: err}
					return
				}
				req.Header.Set("User-Agent", ua)
				resp, err := c.Do(req)
				if err != nil {
					resultCh <- result{err: err}
					return
				}
				defer resp.Body.Close()
				if resp.StatusCode < 200 || resp.StatusCode >= 300 {
					resultCh <- result{err: fmt.Errorf("HTTP %d", resp.StatusCode)}
					return
				}
				var r io.Reader = resp.Body
				if opt.MaxBytes > 0 {
					r = io.LimitReader(resp.Body, opt.MaxBytes+1)
				}
				// 读错误不得吞：连接中途断开会得到半截 body，当成成功写盘就是
				// 「订阅更新成功但内容残缺」这类事故的源头。
				body, readErr := io.ReadAll(r)
				if readErr != nil {
					resultCh <- result{err: fmt.Errorf("读取响应体失败: %w", readErr)}
					return
				}
				// 超限必须显式报错：LimitReader 只会截断，不会自己报错。
				if opt.MaxBytes > 0 && int64(len(body)) > opt.MaxBytes {
					resultCh <- result{err: fmt.Errorf("响应体超过 %d 字节上限", opt.MaxBytes)}
					return
				}
				// 200 + 空 body 是机场过期 / 被墙时最常见的形态，必须当失败上报，
				// 否则调用方拿到 nil 会把旧文件当作「新下载的内容」继续用。
				if len(body) == 0 {
					resultCh <- result{err: fmt.Errorf("响应体为空")}
					return
				}
				resultCh <- result{body: body, header: resp.Header}
			}(client, u)
		}
	}

	var lastErr error
	for i := 0; i < totalRaces; i++ {
		res := <-resultCh
		if res.err == nil && len(res.body) > 0 {
			tmpPath := opt.DestPath + ".tmp"
			if err := os.WriteFile(tmpPath, res.body, 0644); err != nil {
				lastErr = fmt.Errorf("写入临时文件失败: %w", err)
				continue
			}
			if opt.Validator != nil {
				if err := opt.Validator(tmpPath); err != nil {
					_ = os.Remove(tmpPath)
					lastErr = err
					continue
				}
			}
			cancelRace()
			if opt.OnResponse != nil {
				opt.OnResponse(&http.Response{Header: res.header})
			}
			return ReplaceFile(tmpPath, opt.DestPath)
		}
		if res.err != nil {
			lastErr = res.err
		}
	}
	if totalRaces == 0 {
		lastErr = fmt.Errorf("下载地址为空")
	} else if lastErr == nil {
		lastErr = fmt.Errorf("所有下载候选均返回空响应")
	}
	// 总预算耗尽要给用户一句人话，而不是裸的 "context deadline exceeded"。
	if errors.Is(lastErr, context.DeadlineExceeded) {
		return fmt.Errorf("下载超时（超过 %s），请检查网络或稍后重试: %w", smallFileFetchTimeout, lastErr)
	}
	if errors.Is(lastErr, context.Canceled) {
		return fmt.Errorf("下载已取消: %w", lastErr)
	}
	return SanitizeDownloadError(lastErr)
}
