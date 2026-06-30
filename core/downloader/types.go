//go:build windows

package downloader

import (
	"net/http"
)

type Options struct {
	URLs      []string // 🚀 竞速容灾：传入多个下载地址
	DestPath  string
	UserAgent string
	MaxBytes  int64

	Client *http.Client

	Strategy func() DownloadStrategy

	VerifyGitHubSHA    bool
	RequireGitHubSHA   bool // SHA 为可选，Validator 为必须
	InsecureSkipVerify bool // SSL 宽容

	AttemptsPerEndpoint int
	OnResponse          func(resp *http.Response)  // 拦截器
	Validator           func(tmpPath string) error // 防损屏障：替换前执行验证逻辑

	OnProgress func(bytesDone, totalBytes, speedBps int64, etaSec int64)
}

type DownloadStrategy struct {
	ProxyURL    string
	PreferProxy bool
}
