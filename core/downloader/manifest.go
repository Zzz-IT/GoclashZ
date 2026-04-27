package downloader

var ComponentSHA256 = map[string]string{
	// 示例：在这里预填已知稳定的组件哈希
	"wintun-0.14.1.zip": "1f33f0b005be7f6f70a1a457492982d610111586790938f73111586790938f7", // 示例值
}

func LookupKnownSHA256(fileName string) string {
	return ComponentSHA256[fileName]
}
