//go:build windows

package version

// AppVersion 是当前应用的版本号。
// 建议在构建时通过 ldflags 注入，例如：
// go build -ldflags "-X goclashz/core/version.AppVersion=v1.1.3"
var AppVersion = "v1.1.3"
