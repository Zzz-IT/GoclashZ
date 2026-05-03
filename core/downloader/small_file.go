//go:build windows

package downloader

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

func FetchSmallFileAtomic(ctx context.Context, opt Options) error {
	if strings.TrimSpace(opt.DestPath) == "" {
		return fmt.Errorf("DestPath 为空")
	}
	unlock := lockDest(opt.DestPath)
	defer unlock()

	clients := createOrderedClients(opt)
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
				body, _ := io.ReadAll(r)
				resultCh <- result{body: body, header: resp.Header, err: nil}
			}(client, u)
		}
	}

	var lastErr error
	for i := 0; i < totalRaces; i++ {
		res := <-resultCh
		if res.err == nil && len(res.body) > 0 {
			tmpPath := opt.DestPath + ".tmp"
			_ = os.WriteFile(tmpPath, res.body, 0644)
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
		lastErr = res.err
	}
	return SanitizeDownloadError(lastErr)
}
