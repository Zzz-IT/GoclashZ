//go:build windows

package downloader

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"strings"
)

// ExtractFileFromZip 从 zip 压缩包中提取指定名称的文件
func ExtractFileFromZip(zipPath, targetName, destPath string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer r.Close()

	var targetFile *zip.File
	for _, f := range r.File {
		if strings.EqualFold(f.Name, targetName) || strings.HasSuffix(strings.ToLower(f.Name), "/"+strings.ToLower(targetName)) {
			targetFile = f
			break
		}
	}

	if targetFile == nil {
		return fmt.Errorf("zip 中未找到文件: %s", targetName)
	}

	rc, err := targetFile.Open()
	if err != nil {
		return err
	}
	defer rc.Close()

	f, err := os.OpenFile(destPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0755)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = io.Copy(f, rc)
	return err
}
