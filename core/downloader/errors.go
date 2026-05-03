//go:build windows

package downloader

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

func isTransientDownloadError(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "eof") ||
		strings.Contains(s, "timeout") ||
		strings.Contains(s, "tls handshake timeout") ||
		strings.Contains(s, "connection reset") ||
		strings.Contains(s, "connection refused") ||
		strings.Contains(s, "unexpected eof") ||
		strings.Contains(s, "use of closed network connection")
}

func SanitizeDownloadError(err error) error {
	if err == nil {
		return nil
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
