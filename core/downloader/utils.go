//go:build windows

package downloader

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
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

func metaPath(tmpPath string) string {
	return tmpPath + ".meta.json"
}

func readResumeMeta(path string) (resumeMeta, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return resumeMeta{}, false
	}

	var meta resumeMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		return resumeMeta{}, false
	}
	return meta, true
}

func writeResumeMeta(path string, meta resumeMeta) error {
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
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

func acquireHostLimiter(rawURL string) func() {
	u, err := url.Parse(rawURL)
	if err != nil {
		return func() {}
	}

	host := strings.ToLower(u.Host)
	if strings.Contains(host, "github.com") ||
		strings.Contains(host, "githubusercontent.com") {
		githubHostSem <- struct{}{}
		return func() { <-githubHostSem }
	}

	return func() {}
}
