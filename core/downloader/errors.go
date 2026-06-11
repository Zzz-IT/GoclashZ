//go:build windows

package downloader

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"regexp"
)

func SanitizeDownloadError(err error) error {
	if err == nil {
		return nil
	}

	if errors.Is(err, context.Canceled) {
		return err
	}

	msg := err.Error()

	// 清洗超长 GitHub Release 资产 Signed URL
	reAsset := regexp.MustCompile(`https://release-assets\.githubusercontent\.com/[^\s"]+`)
	msg = reAsset.ReplaceAllString(msg, "GitHub Release 资产下载地址")

	// 清洗普通 GitHub Release 下载地址，移除 query
	reGithub := regexp.MustCompile(`https://github\.com/[^\s"]+/releases/download/[^\s"]+`)
	msg = reGithub.ReplaceAllStringFunc(msg, func(s string) string {
		if u, err := url.Parse(s); err == nil {
			u.RawQuery = ""
			return u.String()
		}
		return "GitHub Release 下载地址"
	})

	// 移除常见的签名敏感参数
	reQuery := regexp.MustCompile(`([?&](sp|sv|se|sr|sig|skoid|sktid|skt|ske|sks|skv)=[^\s]+)`)
	msg = reQuery.ReplaceAllString(msg, "")

	if len(msg) > 360 {
		msg = msg[:360] + "..."
	}

	return fmt.Errorf("%s", msg)
}
