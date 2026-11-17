//go:build windows

package sys

import (
	"unsafe"

	"github.com/wailsapp/wails/v3/pkg/w32"
)

// ApplyWindowGlass 给主窗口启用/关闭 Windows 原生 Acrylic 玻璃。
// 只有窗口本身以 BackgroundTypeTranslucent 创建、WebView 背景透明时才有效；
// strength 只决定玻璃"染成什么颜色"（0 = 无色），透明度固定，Windows 也不提供可调模糊半径。
func ApplyWindowGlass(hwnd uintptr, enabled bool, strength int) {
	if hwnd == 0 {
		return
	}

	// Windows 11 22621+：直接使用 DWM system backdrop（Acrylic）。
	// Wails 的透明窗口带 WS_EX_NOREDIRECTIONBITMAP，SetWindowCompositionAttribute
	// 在这种 DirectComposition 窗口上不会生效，必须走 DwmSetWindowAttribute。
	if w32.SupportsBackdropTypes() {
		if enabled {
			w32.EnableTranslucency(hwnd, 3) // Acrylic
		} else {
			w32.EnableTranslucency(hwnd, 1) // None
		}
		return
	}

	// 旧系统回退：Accent acrylic（模糊半径固定，tint 可调）。
	var accent w32.ACCENT_POLICY
	if enabled {
		accent.AccentState = w32.ACCENT_ENABLE_ACRYLICBLURBEHIND
		accent.AccentFlags = 2
		accent.GradientColor = acrylicTint(strength)
	} else {
		accent.AccentState = w32.ACCENT_DISABLED
	}

	data := w32.WINDOWCOMPOSITIONATTRIBDATA{
		Attrib: w32.WCA_ACCENT_POLICY,
		PvData: w32.PVOID(&accent),
		CbData: unsafe.Sizeof(accent),
	}
	w32.SetWindowCompositionAttribute(w32.HWND(hwnd), &data)
}

const (
	// 染色固定透明度：0.34 × 255 ≈ 87。与前端 GLASS_TINT_ALPHA 保持一致。
	acrylicTintAlpha = 0x57
	// 染色目标色，与前端 NEUTRAL_GLASS_TINT (#6E8FC7) 对齐。
	tintTargetR = 0x6E
	tintTargetG = 0x8F
	tintTargetB = 0xC7
)

// acrylicTint 生成 AABBGGRR 格式的 tint。
// chroma 0..80 只决定「染多浓」——把主题基色向目标色偏移多少；alpha 固定。
// 因此调浓不会让窗口变实，语义与前端 --glass-tint-bg 的两步合成一致。
func acrylicTint(chroma int) uint32 {
	if chroma < 0 {
		chroma = 0
	}
	if chroma > 80 {
		chroma = 80
	}

	baseR, baseG, baseB := uint32(0x1A), uint32(0x1A), uint32(0x1A)
	if !w32.IsCurrentlyDarkMode() {
		baseR, baseG, baseB = 0xF0, 0xF0, 0xF0
	}

	c := uint32(chroma)
	mix := func(base, target uint32) uint32 {
		return (base*(80-c) + target*c) / 80
	}

	r, g, b := mix(baseR, tintTargetR), mix(baseG, tintTargetG), mix(baseB, tintTargetB)
	return acrylicTintAlpha<<24 | b<<16 | g<<8 | r
}
