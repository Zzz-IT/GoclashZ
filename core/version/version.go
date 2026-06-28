//go:build windows

package version

import "strings"

// AppVersion 閺勵垰缍嬮崜宥呯安閻劎娈戦悧鍫熸拱閸欐灚鈧?// 瀵ら缚顔呴崷銊︾€鐑樻闁俺绻?ldflags 濞夈劌鍙嗛敍灞肩伐婵″偊绱?// go build -ldflags "-X goclashz/core/version.AppVersion=v1.2.0"
var AppVersion = "v1.2.1"

func NormalizeVersion(v string) string {
	v = strings.TrimSpace(v)
	v = strings.TrimPrefix(v, "v")
	v = strings.TrimPrefix(v, "V")
	return v
}
