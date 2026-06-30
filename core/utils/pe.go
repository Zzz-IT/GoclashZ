package utils

import (
	"fmt"
	"io"
	"os"
)

// ValidateWindowsPE 校验路径上的文件是否为有效的 Windows 可执行文件或 DLL
func ValidateWindowsPE(path string, minSize int64, maxSize int64) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	header := make([]byte, 2)
	if _, err := io.ReadFull(f, header); err != nil {
		return err
	}

	if header[0] != 'M' || header[1] != 'Z' {
		return fmt.Errorf("不是有效的 Windows PE 文件 (缺少 MZ 特征)")
	}

	info, err := f.Stat()
	if err != nil {
		return err
	}

	if minSize > 0 && info.Size() < minSize {
		return fmt.Errorf("文件体积过小: %d bytes (最小期望 %d bytes)", info.Size(), minSize)
	}

	if maxSize > 0 && info.Size() > maxSize {
		return fmt.Errorf("文件体积过大: %d bytes (最大期望 %d bytes)", info.Size(), maxSize)
	}

	return nil
}
