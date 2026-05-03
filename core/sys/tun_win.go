//go:build windows

package sys

import (
	"io"
	"os"
	"path/filepath"

	"goclashz/core/utils"
)

func GetWintunPath() string {
	return filepath.Join(utils.GetCoreBinDir(), "wintun.dll")
}

func IsWintunInstalled() bool {
	path := GetWintunPath()
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return false
	}

	// 校验1：大小在合理范围内 (32KB ~ 5MB)，防止 0 字节损坏文件
	if info.Size() < 32*1024 || info.Size() > 5*1024*1024 {
		return false
	}

	// 校验2：验证 PE 文件的特征码 (MZ 标识)
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	header := make([]byte, 2)
	if _, err := io.ReadFull(f, header); err != nil {
		return false
	}
	return header[0] == 'M' && header[1] == 'Z'
}
