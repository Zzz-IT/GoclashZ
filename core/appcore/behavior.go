//go:build windows

package appcore

import (
	jsonv2 "encoding/json/v2"

	"goclashz/core/clash"
	"goclashz/core/utils"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/wailsapp/wails/v3/pkg/w32"
)

type AppBehavior struct {
	SilentStart        bool   `json:"silentStart"` // 静默启动 (不弹窗，直接进托盘)
	CloseToTray        bool   `json:"closeToTray"` // 点击关闭时隐藏到托盘
	ColorDelay         bool   `json:"colorDelay"`  // 显色彩色延迟数字
	DelayRetention     bool   `json:"delayRetention"`
	DelayRetentionTime string `json:"delayRetentionTime"`
	LogLevel           string `json:"logLevel"`    // 日志等级
	AppLogLevel        string `json:"appLogLevel"` // 软件日志等级
	HideLogs           bool   `json:"hideLogs"`
	SubUA              string `json:"subUA"` // 订阅更新 User-Agent

	ActiveConfig string `json:"activeConfig"`
	ActiveMode   string `json:"activeMode"`

	GeoIpLink   string `json:"geoIpLink"`
	GeoSiteLink string `json:"geoSiteLink"`
	MmdbLink    string `json:"mmdbLink"`
	AsnLink     string `json:"asnLink"`

	AutoUpdate      bool   `json:"autoUpdate"`      // 是否开启自动更新
	UpdateMethod    string `json:"updateMethod"`    // 检查更新方式: "startup" (每次启动) 或 "scheduled" (定时)
	UpdateInterval  int    `json:"updateInterval"`  // 检查间隔时间 (天)
	LastUpdateCheck int64  `json:"lastUpdateCheck"` // 上次检查更新的时间戳 (Unix秒)

	// 👇 新增：自动测速控制
	AutoDelayTest         bool `json:"autoDelayTest"`
	AutoDelayTestInterval int  `json:"autoDelayTestInterval"`

	// 👇 新增：流量统计模式
	ProxyTrafficOnly bool `json:"proxyTrafficOnly"`

	// 👇 新增：开机自启 (Task Scheduler)
	StartupWithOS    bool `json:"startupWithOS"`
	RestoreOnStartup bool `json:"restoreOnStartup"`

	// 👇 新增：仅端口代理模式（开启后系统代理按钮变为端口代理，只启动内核不设置全局代理）
	PortProxyMode bool `json:"portProxyMode"`

	// 👇 新增：长连接保护
	LongConnectionProtection bool `json:"longConnectionProtection"`
	DeferRestartWhenActive   bool `json:"deferRestartWhenActive"`
	LongConnectionMinSeconds int  `json:"longConnectionMinSeconds"`

	// 👇 新增：界面语言 ("zh-CN", "zh-TW", "en-US")
	Language string `json:"language"`

	// 👇 新增：内核对比更新提示
	CoreComparisonPrompt bool `json:"coreComparisonPrompt"`

	// 👇 新增：玻璃模糊（毛玻璃）效果
	// 玻璃层按 3 组独立半径（内容卡片 / 浮层 / 控件凹槽）+ 1 个背景 tint 强度存储，
	// 均为 0..80，不做总控联动：任一组独立生效，互不换算。
	// GlassBlurStrength 是旧版本的单强度字段，仅保留做迁移（语义等价内容卡片组）。
	GlassBlurEnabled         bool `json:"glassBlurEnabled"`         // 是否启用毛玻璃背景
	GlassBlurStrength        int  `json:"glassBlurStrength"`        // 旧字段（迁移用）：单强度 0..80，默认 12
	GlassBlurContentStrength int  `json:"glassBlurContentStrength"` // 内容卡片（卡片/分组/列表容器）模糊：0..80，默认 12
	GlassBlurOverlayStrength int  `json:"glassBlurOverlayStrength"` // 浮层（弹窗/下拉/菜单）模糊：0..80，默认 18
	GlassBlurControlStrength int  `json:"glassBlurControlStrength"` // 控件凹槽（分段控件/图表槽）模糊：0..80，默认 8
	GlassTintStrength        int  `json:"glassTintStrength"`        // 背景 tint 强度：0..80，默认 58
	GlassDepth               int  `json:"glassDepth"`               // 玻璃厚度总控：0..100，同时缩放 tint alpha 与各级面板填充，默认 50

	// 👇 新增：应用主题（与前端 utils/theme.ts 的 ThemeConfig 一一对应）
	// ThemeMode 存的是「用户偏好」（含 system）；theme_setting.txt 仍是「已解析的
	// light/dark」真源，供 main.go 决定窗口底色、备份还原使用，两者由
	// syncThemeSettingFile 保持同步。
	ThemeMode   string `json:"themeMode"`   // 外观模式：light | dark | system，默认 light
	ThemePreset string `json:"themePreset"` // 预设主题 id，默认 graphite（见设计简报第 3 节）
	ThemeAccent string `json:"themeAccent"` // 自定义强调色 #rrggbb；空串 = 跟随预设
	ThemeRadius string `json:"themeRadius"` // 界面圆角：sharp | soft | round，默认 soft
	ThemeAura   bool   `json:"themeAura"`   // 玻璃模式下的背景光晕，默认 true
}

type BehaviorStore struct {
	mu    sync.RWMutex
	cache AppBehavior
}

func NewBehaviorStore() *BehaviorStore {
	// 1. 旧版本配置文件平滑迁移逻辑
	oldPath := filepath.Join(utils.GetDataDir(), "behavior.json")
	if _, err := os.Stat(oldPath); err == nil {
		newPath := filepath.Join(utils.GetSettingsDir(), "user_behavior.json")
		// 如果旧文件存在，且新文件不存在，则将其移动过去
		if _, err := os.Stat(newPath); os.IsNotExist(err) {
			_ = os.MkdirAll(utils.GetSettingsDir(), 0755)
			_ = os.Rename(oldPath, newPath)
		} else {
			// 如果新文件已存在，直接删掉废弃的旧文件
			_ = os.Remove(oldPath)
		}
	}

	store := &BehaviorStore{}
	_ = store.Load()
	return store
}

func (s *BehaviorStore) Get() AppBehavior {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cache
}

func (s *BehaviorStore) SetAndSave(b AppBehavior) error {
	b = normalizeBehavior(b)

	// F-A5-18：先落盘、再提交内存。以前先写 cache，磁盘失败后内存已经是新值，
	// 调用方一旦忽略错误就会出现「进程内是新配置、磁盘还是旧配置」的分叉。
	if err := utils.SaveSetting("behavior", &b); err != nil {
		return err
	}

	s.mu.Lock()
	s.cache = b
	s.mu.Unlock()

	// themeMode 是 light/dark 时同步 theme_setting.txt（文件 IO 放在锁外）。
	syncThemeSettingFile(b)
	return nil
}

// Update 以「锁内读-改-写」的方式原子更新行为配置（F-A5-18）。
//
// 调用方不要再拼「Get() → 改某个字段 → SetAndSave()」：两个并发调用会互相覆盖
// 对方刚改的字段（典型是启动期自启同步 vs 前端保存设置）。
func (s *BehaviorStore) Update(mutate func(b *AppBehavior)) error {
	s.mu.Lock()

	next := normalizeBehavior(s.cache)
	if mutate != nil {
		mutate(&next)
	}
	next = normalizeBehavior(next)

	if err := utils.SaveSetting("behavior", &next); err != nil {
		s.mu.Unlock()
		return err
	}

	s.cache = next
	s.mu.Unlock()

	syncThemeSettingFile(next)
	return nil
}

func (s *BehaviorStore) SetActiveConfig(id string) error {
	// 锁内读-改-写 + 先落盘再提交内存（F-A5-18），与 Update 保持一致。
	s.mu.Lock()
	defer s.mu.Unlock()

	next := s.cache
	next.ActiveConfig = id

	if err := utils.SaveSetting("behavior", &next); err != nil {
		return err
	}
	s.cache = next
	return nil
}

func (s *BehaviorStore) Load() error {
	defaults := s.Default()

	// 升级迁移①：老版本只有 theme_setting.txt、没有 themeMode。这里把文件里已解析的
	// 明暗播种进默认值，随后 LoadSetting 会用 user_behavior.json 覆盖它，
	// 因此已经写过 themeMode 的配置完全不受影响。
	seeded := seedThemeMode(&defaults)

	// 升级迁移②：老版本只有 GlassBlurStrength，3 组模糊 + tint 是后加的键。
	// 按「键是否存在」播种，用户主动保存的全 0 不会被改回默认值。
	_ = migrateGlassStrengths(&defaults)

	// 使用统一的 LoadSetting 机制 (注意这里传入指针)
	cfg, err := utils.LoadSetting("behavior", defaults)
	if err != nil {
		s.mu.Lock()
		s.cache = defaults
		s.mu.Unlock()
		return err
	}

	normalized := normalizeBehavior(*cfg)

	s.mu.Lock()
	s.cache = normalized
	s.mu.Unlock()

	// 启动时补齐 theme_setting.txt，保证 main.go 窗口底色、AppState.Theme 与偏好一致。
	// 播种成功的升级场景跳过：此时 theme_setting.txt 本身就是真源，不必再写一次
	// （升级瞬间零写盘副作用）。
	if !seeded {
		syncThemeSettingFile(normalized)
	}
	return nil
}

func (s *BehaviorStore) Save() error {
	s.mu.RLock()
	cfg := s.cache
	s.mu.RUnlock()

	// 使用统一的 SaveSetting 机制
	return utils.SaveSetting("behavior", &cfg)
}

// 玻璃模糊强度上限：3 组模糊与背景 tint 共用同一量程（0..80），
// 前端 utils/glass.ts 再把 0..80 换算成 px / alpha。
const glassStrengthMax = 80

// 玻璃厚度是一个 0..100 的总控值：它乘在 tint alpha 与四级面板填充上，
// 层次之间的比例保持不变。50 等于历史观感，所以缺少该键的老配置不需要迁移。
const (
	glassDepthMax     = 100
	defaultGlassDepth = 50
)

const (
	defaultGlassBlurContent = 12
	defaultGlassBlurOverlay = 18
	defaultGlassBlurControl = 8
	defaultGlassTint        = 58
)

// normalizeGlassStrength 把玻璃相关强度夹到 [0, glassStrengthMax]；
// 负数视为「未设置」，回落该组默认值；超过上限按上限截断。
func normalizeGlassStrength(v, def int) int {
	if v < 0 {
		return def
	}
	if v > glassStrengthMax {
		return glassStrengthMax
	}
	return v
}

// normalizeGlassDepth 把厚度夹到 [0, glassDepthMax]；负数视为「未设置」，回落默认值。
// 0 是合法值（用户主动要最透），不能用 0 当哨兵。
func normalizeGlassDepth(v, def int) int {
	if v < 0 {
		return def
	}
	if v > glassDepthMax {
		return glassDepthMax
	}
	return v
}

// 主题默认值与合法取值：必须与设计简报第 2 节的数据契约、前端 utils/theme.ts
// 里的 DEFAULT_THEME / THEME_PRESETS / RADII 保持一致。
const (
	defaultThemeMode   = "light"
	defaultThemePreset = "graphite"
	defaultThemeRadius = "soft"
)

// themePresetIDs 是全部合法预设 id（设计简报第 3 节的 12 套）。
// 后端只做合法性校验、不持有配色，具体配色由前端 utils/theme.ts 负责。
var themePresetIDs = map[string]struct{}{
	"graphite": {},
	"midnight": {},
	"ocean":    {},
	"forest":   {},
	"amber":    {},
	"rose":     {},
	"violet":   {},
	"sakura":   {},
	"coffee":   {},
	"nord":     {},
	"lime":     {},
	"crimson":  {},
}

// normalizeThemeAccent 校验强调色：只接受空串（跟随预设）或 #RRGGBB。
// 三位简写 #RGB 会被展开成 #RRGGBB，统一输出小写（与前端 toHex 的输出格式一致）；
// 其它任何脏值一律回落空串，避免把非法颜色写进 user_behavior.json。
func normalizeThemeAccent(v string) string {
	hex := strings.TrimSpace(v)
	if hex == "" {
		return ""
	}
	if !strings.HasPrefix(hex, "#") {
		return ""
	}
	hex = hex[1:]
	if len(hex) == 3 {
		hex = string([]byte{hex[0], hex[0], hex[1], hex[1], hex[2], hex[2]})
	}
	if len(hex) != 6 {
		return ""
	}
	for i := 0; i < 6; i++ {
		c := hex[i]
		isDigit := c >= '0' && c <= '9'
		isLower := c >= 'a' && c <= 'f'
		isUpper := c >= 'A' && c <= 'F'
		if !isDigit && !isLower && !isUpper {
			return ""
		}
	}
	return "#" + strings.ToLower(hex)
}

// syncThemeSettingFile 让 theme_setting.txt（已解析的 light/dark 真源）跟随
// themeMode 的用户偏好：只有用户明确选了 light/dark 才覆盖；
// system 需由前端解析出实际明暗后走 SaveThemePreference 写入，后端不擅自改写。
// 值没变就不写盘：玻璃滑条每次松手都会走一遍 SetAndSave，无条件重写会让
// theme_setting.txt 的 mtime 无意义地抖动。
func syncThemeSettingFile(b AppBehavior) {
	switch b.ThemeMode {
	case "light", "dark":
		if cur, ok := readLegacyThemeMode(filepath.Join(utils.GetDataDir(), "theme_setting.txt")); ok && cur == b.ThemeMode {
			return
		}
		_ = utils.SaveGlobalTheme(b.ThemeMode)
	}
}

// ResolveThemeName 返回当前应当呈现的明暗，唯一权威是用户意图 ThemeMode。
//
// system 必须与前端同源：都用操作系统偏好（这里读注册表 AppsUseLightTheme，
// 前端用 matchMedia），两端各算各的但结论一致。
// 绝对不要在这种分支里回读 theme_setting.txt —— 那是派生缓存，
// system 模式下根本不被写入，读到的必然是过期值。
func ResolveThemeName(b AppBehavior) string {
	switch b.ThemeMode {
	case "light", "dark":
		return b.ThemeMode
	}
	if w32.IsCurrentlyDarkMode() {
		return "dark"
	}
	return "light"
}

// userThemeModeProbe 只用来探测 user_behavior.json 是否**写过** themeMode：
// 指针为 nil 说明是老版本配置（新版本一旦保存过就必定带上该键）。
type userThemeModeProbe struct {
	ThemeMode *string `json:"themeMode"`
}

// hasStoredThemeMode 判断 user_behavior.json 里是否已带 themeMode 字段。
// 文件不存在（全新安装 / 恢复默认后删档）也视为「没有」。
func hasStoredThemeMode(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	var probe userThemeModeProbe
	if jsonv2.Unmarshal(data, &probe) != nil {
		return false
	}
	return probe.ThemeMode != nil
}

// readLegacyThemeMode 读取 theme_setting.txt 里已解析的 light/dark。
// 文件不存在或内容不是这两个值时 ok=false——这里**不能**用 utils.GetGlobalTheme()，
// 它对缺失文件兜底返回 "dark"，会把全新安装误判成暗色。
func readLegacyThemeMode(path string) (string, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	theme := strings.TrimSpace(string(data))
	if theme == "light" || theme == "dark" {
		return theme, true
	}
	return "", false
}

// seedThemeModeFrom 是播种本体（路径可注入，便于单测）：
// 老配置没有 themeMode、且 theme_setting.txt 真实存在时，用它播种用户偏好。
// 返回是否真的播种，调用方据此避免额外写盘。
func seedThemeModeFrom(defaults *AppBehavior, userBehaviorPath, themeSettingPath string) bool {
	if hasStoredThemeMode(userBehaviorPath) {
		return false
	}
	theme, ok := readLegacyThemeMode(themeSettingPath)
	if !ok {
		return false
	}
	defaults.ThemeMode = theme
	return true
}

// seedThemeMode 升级迁移：老版本 user_behavior.json 没有 themeMode，用户真正的
// 明暗偏好只存在于 theme_setting.txt。不播种的话，升级后默认值 light 会把原本的
// 暗色顶掉（前端启动即按 themeMode 应用主题，并会回写 theme_setting.txt）。
// 已有 themeMode 的配置、以及全新安装（无 theme_setting.txt）都不受影响；
// 这里只改内存里的默认值，不产生任何写盘副作用。
func seedThemeMode(defaults *AppBehavior) bool {
	return seedThemeModeFrom(defaults,
		filepath.Join(utils.GetSettingsDir(), "user_behavior.json"),
		filepath.Join(utils.GetDataDir(), "theme_setting.txt"))
}

// glassStrengthProbe 只用来探测 user_behavior.json 里各玻璃字段**是否写过**：
// 指针为 nil 表示老版本配置（该键尚未出现）。注意 0 是合法值（用户可以主动把
// 模糊/背景着色全拉到 0），所以不能用「值是否为 0」当哨兵。
type glassStrengthProbe struct {
	GlassBlurStrength        *int `json:"glassBlurStrength"`
	GlassBlurContentStrength *int `json:"glassBlurContentStrength"`
	GlassBlurOverlayStrength *int `json:"glassBlurOverlayStrength"`
	GlassBlurControlStrength *int `json:"glassBlurControlStrength"`
	GlassTintStrength        *int `json:"glassTintStrength"`
}

// migrateGlassStrengthsFrom 是旧版玻璃迁移本体（路径可注入，便于单测）：
// 只有「新键缺失」的项才用旧单强度播种 3 组模糊（tint 缺失时取默认 58），
// 已存在的键一律不动——包括用户主动保存的全 0。
// 文件不存在（全新安装）直接返回 false，保持 Default() 的 12/18/8/58。
func migrateGlassStrengthsFrom(defaults *AppBehavior, userBehaviorPath string) bool {
	data, err := os.ReadFile(userBehaviorPath)
	if err != nil {
		return false
	}
	var probe glassStrengthProbe
	if jsonv2.Unmarshal(data, &probe) != nil {
		return false
	}

	// 没有任何「旧单强度」可迁移（残缺/手改文件），保持 Default() 不动，
	// 不去凭空造值。老版本一定会写 glassBlurStrength，所以真实升级路径不会漏。
	if probe.GlassBlurStrength == nil {
		return false
	}
	legacy := normalizeGlassStrength(*probe.GlassBlurStrength, defaultGlassBlurContent)

	migrated := false
	if probe.GlassBlurContentStrength == nil {
		defaults.GlassBlurContentStrength = legacy
		migrated = true
	}
	if probe.GlassBlurOverlayStrength == nil {
		defaults.GlassBlurOverlayStrength = legacy
		migrated = true
	}
	if probe.GlassBlurControlStrength == nil {
		defaults.GlassBlurControlStrength = legacy
		migrated = true
	}
	if probe.GlassTintStrength == nil {
		defaults.GlassTintStrength = defaultGlassTint
		migrated = true
	}
	return migrated
}

// migrateGlassStrengths 升级迁移：老版本只持久化过 GlassBlurStrength，新版本拆成
// 3 组独立模糊 + 1 个背景着色。这里按「键是否存在」播种到默认值上（随后 LoadSetting
// 会用 user_behavior.json 覆盖），保证升级前后视觉一致，又不会把用户的全 0 改回默认。
func migrateGlassStrengths(defaults *AppBehavior) bool {
	return migrateGlassStrengthsFrom(defaults,
		filepath.Join(utils.GetSettingsDir(), "user_behavior.json"))
}

func normalizeBehavior(b AppBehavior) AppBehavior {
	b.LogLevel = clash.NormalizeLogLevel(b.LogLevel)

	if b.AppLogLevel == "" {
		b.AppLogLevel = "info"
	}

	if b.DelayRetentionTime == "" {
		b.DelayRetentionTime = "long"
	}

	if b.ActiveMode == "" {
		b.ActiveMode = "rule"
	}

	if b.SubUA == "" {
		b.SubUA = "clash-verge"
	}

	if b.UpdateMethod == "" {
		b.UpdateMethod = "startup"
	}

	if b.UpdateInterval <= 0 {
		b.UpdateInterval = 1
	}

	if b.AutoDelayTest && b.AutoDelayTestInterval <= 0 {
		b.AutoDelayTestInterval = 60
	}

	if b.GeoIpLink == "" {
		b.GeoIpLink = "https://github.com/MetaCubeX/meta-rules-dat/releases/download/latest/geoip.metadb"
	}
	if b.GeoSiteLink == "" {
		b.GeoSiteLink = "https://github.com/MetaCubeX/meta-rules-dat/releases/download/latest/geosite.dat"
	}
	if b.MmdbLink == "" {
		b.MmdbLink = "https://github.com/MetaCubeX/meta-rules-dat/releases/download/latest/country.mmdb"
	}
	if b.AsnLink == "" {
		b.AsnLink = "https://github.com/xishang0128/geoip/releases/download/latest/GeoLite2-ASN.mmdb"
	}

	if b.LongConnectionMinSeconds <= 0 {
		b.LongConnectionMinSeconds = 60
	}

	switch b.Language {
	case "zh-CN", "zh-TW", "zh-HK", "en-US", "ja-JP", "ru-RU", "fr-FR", "de-DE":
		// valid
	default:
		b.Language = "zh-CN"
	}

	// 玻璃模糊：3 组独立半径 + 1 个背景 tint，量程统一 0..80。
	// 负数视为未设置 → 回落各自默认；大于 80 按上限截断。
	b.GlassBlurStrength = normalizeGlassStrength(b.GlassBlurStrength, defaultGlassBlurContent)
	b.GlassBlurContentStrength = normalizeGlassStrength(b.GlassBlurContentStrength, defaultGlassBlurContent)
	b.GlassBlurOverlayStrength = normalizeGlassStrength(b.GlassBlurOverlayStrength, defaultGlassBlurOverlay)
	b.GlassBlurControlStrength = normalizeGlassStrength(b.GlassBlurControlStrength, defaultGlassBlurControl)
	b.GlassTintStrength = normalizeGlassStrength(b.GlassTintStrength, defaultGlassTint)
	b.GlassDepth = normalizeGlassDepth(b.GlassDepth, defaultGlassDepth)

	// 旧版单强度 → 3 组模糊 + tint 的迁移不放在这里：0 是合法值（用户可以把
	// 模糊/着色全拉到 0），不能拿它当「未设置」的哨兵。迁移改由 Load 里
	// migrateGlassStrengths 按 user_behavior.json 中「键是否存在」判断。

	// 应用主题：非法值一律静默回落默认，保证旧配置/手改文件都能正常启动。
	switch b.ThemeMode {
	case "light", "dark", "system":
		// valid
	default:
		b.ThemeMode = defaultThemeMode
	}

	if _, ok := themePresetIDs[b.ThemePreset]; !ok {
		b.ThemePreset = defaultThemePreset
	}

	switch b.ThemeRadius {
	case "sharp", "soft", "round":
		// valid
	default:
		b.ThemeRadius = defaultThemeRadius
	}

	b.ThemeAccent = normalizeThemeAccent(b.ThemeAccent)

	return b
}

func (s *BehaviorStore) Default() AppBehavior {
	return AppBehavior{
		SilentStart:        false,
		CloseToTray:        true,
		ColorDelay:         false,
		DelayRetention:     true,
		DelayRetentionTime: "long",
		LogLevel:           "error",
		AppLogLevel:        "info",
		HideLogs:           false,
		SubUA:              "clash-verge",
		ActiveMode:         "rule",

		AutoUpdate:     true,
		UpdateMethod:   "startup",
		UpdateInterval: 1,

		AutoDelayTest:         false,
		AutoDelayTestInterval: 60,

		ProxyTrafficOnly: false,

		GeoIpLink:   "https://github.com/MetaCubeX/meta-rules-dat/releases/download/latest/geoip.metadb",
		GeoSiteLink: "https://github.com/MetaCubeX/meta-rules-dat/releases/download/latest/geosite.dat",
		MmdbLink:    "https://github.com/MetaCubeX/meta-rules-dat/releases/download/latest/country.mmdb",
		AsnLink:     "https://github.com/xishang0128/geoip/releases/download/latest/GeoLite2-ASN.mmdb",

		StartupWithOS:    false,
		RestoreOnStartup: false,

		PortProxyMode: false,

		LongConnectionProtection: true,
		DeferRestartWhenActive:   true,
		LongConnectionMinSeconds: 60,

		Language: "zh-CN",

		CoreComparisonPrompt: true,

		GlassBlurEnabled:         false,
		GlassBlurStrength:        defaultGlassBlurContent,
		GlassBlurContentStrength: defaultGlassBlurContent,
		GlassBlurOverlayStrength: defaultGlassBlurOverlay,
		GlassBlurControlStrength: defaultGlassBlurControl,
		GlassTintStrength:        defaultGlassTint,
		GlassDepth:               defaultGlassDepth,

		ThemeMode:   defaultThemeMode,
		ThemePreset: defaultThemePreset,
		ThemeAccent: "",
		ThemeRadius: defaultThemeRadius,
		ThemeAura:   true,
	}
}
