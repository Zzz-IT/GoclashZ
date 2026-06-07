//go:build windows

package version

import "strings"

// AppVersion 是当前应用的版本号。
// 建议在构建时通过 ldflags 注入，例如：
// go build -ldflags "-X goclashz/core/version.AppVersion=v1.1.4"
var AppVersion = "v1.1.8"

func NormalizeVersion(v string) string {
	v = strings.TrimSpace(v)
	v = strings.TrimPrefix(v, "v")
	v = strings.TrimPrefix(v, "V")
	return v
}

