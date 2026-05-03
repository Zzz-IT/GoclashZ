//go:build windows

package downloader

import (
	"net/http"
)

type Mode string

const (
	ModeLargeAsset Mode = "large_asset"
	ModeResume     Mode = "resume"
	ModeSmallFile  Mode = "small_file"
)

type Options struct {
	URLs      []string // 🚀 竞速容灾：传入多个下载地址
	DestPath  string
	UserAgent string
	MaxBytes  int64

	Client   *http.Client
	ProxyURL string

	Mode Mode

	Resume           bool
	VerifyGitHubSHA  bool
	RequireGitHubSHA bool                       // SHA 为可选，Validator 为必须
	InsecureSkipVerify bool                     // SSL 宽容

	AttemptsPerEndpoint int
	PreferProxy         bool

	OnResponse func(resp *http.Response)  // 拦截器
	Validator  func(tmpPath string) error // 防损屏障：替换前执行验证逻辑
}

type resumeMeta struct {
	URL          string `json:"url"`
	ETag         string `json:"etag"`
	LastModified string `json:"lastModified"`
	TotalSize    int64  `json:"totalSize"`
}
