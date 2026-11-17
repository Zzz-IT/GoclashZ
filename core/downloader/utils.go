//go:build windows

package downloader

import (
	"context"
	"crypto/sha256"
	"encoding/hex"

	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

var destLocks sync.Map // map[string]*sync.Mutex

func lockDest(path string) func() {
	clean := filepath.Clean(path)
	abs, err := filepath.Abs(clean)
	if err != nil {
		abs = clean
	}
	abs = strings.ToLower(abs)

	v, _ := destLocks.LoadOrStore(abs, &sync.Mutex{})
	mu := v.(*sync.Mutex)
	mu.Lock()

	return func() {
		mu.Unlock()
	}
}

func ReplaceFile(tmpPath, destPath string) error {
	backupPath := destPath + ".bak"
	_ = os.Remove(backupPath)

	if _, err := os.Stat(destPath); err == nil {
		if err := os.Rename(destPath, backupPath); err != nil {
			return fmt.Errorf("备份旧文件失败: %w", err)
		}
	}

	if err := os.Rename(tmpPath, destPath); err != nil {
		_ = os.Rename(backupPath, destPath)
		return fmt.Errorf("替换目标文件失败: %w", err)
	}

	_ = os.Remove(backupPath)
	return nil
}

func VerifySHA256(path string, expected string) error {
	expected = strings.TrimSpace(strings.ToLower(expected))
	if expected == "" {
		return nil
	}
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	got := hex.EncodeToString(h.Sum(nil))
	if !strings.EqualFold(got, expected) {
		return fmt.Errorf("SHA256 不匹配")
	}
	return nil
}

var githubHostSem = make(chan struct{}, 4)

// acquireHostLimiter 限制同一 GitHub 主机族的并发下载数（4）。
//
// 返回值 release 必须调用。ctx 取消时返回错误而不是无限阻塞在信号量上
// （F-A5-16：之前用户点「暂停」后，协程仍卡在这里，直到前 4 个下载自然超时
// 才轮到它，UI 显示"已暂停"但后台还在排队）。
func acquireHostLimiter(ctx context.Context, rawURL string) (func(), error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return func() {}, nil
	}

	host := strings.ToLower(u.Host)
	if !strings.Contains(host, "github.com") && !strings.Contains(host, "githubusercontent.com") {
		return func() {}, nil
	}

	if ctx == nil {
		githubHostSem <- struct{}{}
		return func() { <-githubHostSem }, nil
	}

	select {
	case githubHostSem <- struct{}{}:
		return func() { <-githubHostSem }, nil
	case <-ctx.Done():
		return nil, fmt.Errorf("等待 GitHub 下载并发额度被取消: %w", ctx.Err())
	}
}
