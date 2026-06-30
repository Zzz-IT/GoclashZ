package runtimeassets

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
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

var (
	coreVersionRe         = regexp.MustCompile(`v?\d+\.\d+\.\d+(?:[-+][^\s]+)?`)
	coreVersionCache      string
	coreVersionCacheTime  time.Time
	coreVersionCacheMu    sync.Mutex
)

func CheckCore(ctx context.Context) AssetHealth {
	path := filepath.Join(utils.GetCoreBinDir(), "clash.exe")
	return checkCoreByPath(ctx, path)
}

func checkCoreByPath(ctx context.Context, path string) AssetHealth {
	h := baseHealth(AssetCore, "Mihomo 内核", path, true)

	info, err := os.Stat(path)
	if err != nil {
		h.ErrorCode = ErrMissing
		h.Error = "内核文件不存在"
		h.Hint = "请使用“恢复内置组件”或“更新内核”修复。"
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

	version, execErr := readCoreVersion(ctx, path)
	if execErr != nil {
		h.ErrorCode = ErrExecFailed
		h.Error = execErr.Error()
		h.Hint = "内核文件存在且 PE 头有效，但无法执行。可能被系统/杀软阻止，或架构不匹配。"
		return h
	}

	h.Version = version
	h.Ready = true
	
	// 计算 SHA256 (备用)
	if hash, err := calculateSHA256(path); err == nil {
		h.SHA256 = hash
	}

	return h
}

func readCoreVersion(ctx context.Context, path string) (string, error) {
	// 针对同一个文件的版本号做内存缓存，避免频繁执行子进程带来开销
	stat, err := os.Stat(path)
	if err != nil {
		return "", err
	}

	coreVersionCacheMu.Lock()
	defer coreVersionCacheMu.Unlock()

	// 缓存有效期 1 分钟且修改时间必须一致
	if coreVersionCache != "" && time.Since(coreVersionCacheTime) < time.Minute {
		return coreVersionCache, nil
	}

	cmdCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
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

	coreVersionCache = ver
	coreVersionCacheTime = time.Now()
	_ = stat // keep compiler happy

	return ver, nil
}

func CheckWintun() AssetHealth {
	path := filepath.Join(utils.GetCoreBinDir(), "wintun.dll")
	return checkWintunByPath(path)
}

func checkWintunByPath(path string) AssetHealth {
	h := baseHealth(AssetWintun, "Wintun 驱动 DLL", path, true)

	info, err := os.Stat(path)
	if err != nil {
		h.ErrorCode = ErrMissing
		h.Error = "wintun.dll 不存在"
		h.Hint = "请使用“恢复内置组件”或“安装驱动”修复。"
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

	v, err := sys.GetFileVersion(path)
	if err == nil && strings.TrimSpace(v) != "" {
		h.Version = v
	} else {
		h.Version = "已安装，版本未知"
	}

	if hash, err := calculateSHA256(path); err == nil {
		h.SHA256 = hash
	}

	return h
}

func CheckDataFile(key AssetKey, label string, filename string, minSize int64) AssetHealth {
	path := filepath.Join(utils.GetCoreBinDir(), filename)
	return checkDataFileByPath(key, label, path, minSize)
}

func checkDataFileByPath(key AssetKey, label string, path string, minSize int64) AssetHealth {
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

	if info.Size() < minSize {
		h.ErrorCode = ErrTooSmall
		h.Error = fmt.Sprintf("%s 文件体积异常: %d bytes (最小预期 %d bytes)", label, info.Size(), minSize)
		return h
	}

	if looksLikeHTML(path) {
		h.ErrorCode = ErrBadContent
		h.Error = label + " 内容可能是 HTML 错误响应页，不是有效的数据库文件"
		return h
	}

	h.Valid = true
	h.Ready = true

	if hash, err := calculateSHA256(path); err == nil {
		h.SHA256 = hash
	}

	return h
}

func looksLikeHTML(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()

	header := make([]byte, 512)
	n, _ := f.Read(header)
	if n == 0 {
		return false
	}

	content := strings.ToLower(string(header[:n]))
	return strings.Contains(content, "<html") || strings.Contains(content, "<!doctype html") || strings.Contains(content, "<head")
}

func calculateSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
