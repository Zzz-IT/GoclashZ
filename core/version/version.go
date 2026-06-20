//go:build windows

package version

import "strings"

// AppVersion 鏄綋鍓嶅簲鐢ㄧ殑鐗堟湰鍙枫€?
// 寤鸿鍦ㄦ瀯寤烘椂閫氳繃 ldflags 娉ㄥ叆锛屼緥濡傦細
// go build -ldflags "-X goclashz/core/version.AppVersion=v1.1.4"
var AppVersion = "v1.2.0"

func NormalizeVersion(v string) string {
	v = strings.TrimSpace(v)
	v = strings.TrimPrefix(v, "v")
	v = strings.TrimPrefix(v, "V")
	return v
}


