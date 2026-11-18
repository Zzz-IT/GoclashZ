// 主题引擎（App Theme engine）
// 职责：把「模式 + 预设 + 强调色 + 圆角 + 光晕」这一组用户偏好，翻译成根元素上的
// data-* 标记与 CSS 变量，供 tokens.css / surfaces.css / style.css 消费。
// 关键约束：
//   1) 这里是唯一写主题变量的地方（玻璃的 --glass-* 仍由 utils/glass.ts 负责）；
//   2) 不改任何元素的 background 内联样式，避免盖死 token；
//   3) 启动首屏用 localStorage 缓存同步应用，随后再由后端 behavior 校准，杜绝闪烁。
export type ThemeMode = 'light' | 'dark' | 'system';
export type ThemeRadiusId = 'sharp' | 'soft' | 'round';

export interface ThemePalette {
	/** 窗口底色 */
	appBg: string;
	/** 输入框 / 按钮 / 次级填充 */
	surface: string;
	surfaceHover: string;
	/** 内容面板 */
	panel: string;
	textMain: string;
	textSub: string;
	textMuted: string;
}

export interface ThemePreset {
	id: string;
	/** 名称 i18n key：settings.theme.presets.<id> */
	nameKey: string;
	accent: { light: string; dark: string };
	/** 可选：在基准调色上做冷暖/色相偏移（省略则用 BASE_PALETTE） */
	tone?: { light?: Partial<ThemePalette>; dark?: Partial<ThemePalette> };
}

export interface ThemeConfig {
	mode: ThemeMode;
	preset: string;
	/** 空字符串 = 跟随预设 */
	accent: string;
	radius: ThemeRadiusId;
	/** 玻璃模式下的背景光晕 */
	aura: boolean;
}

/** 基准调色：与改造前的黑白灰体系完全一致，保证默认主题零视觉回归 */
export const BASE_PALETTE: { light: ThemePalette; dark: ThemePalette } = {
	light: {
		appBg: '#F0F0F0',
		surface: '#EBEBEB',
		surfaceHover: '#E2E2E2',
		panel: '#FFFFFF',
		textMain: '#1A1A1A',
		// 次要文字：比旧版 #777777 略深，让 --text-sub 在浅色底上达到 4.5:1（旧值只有 3.93:1）
		textSub: '#6B6B6B',
		textMuted: '#AAAAAA'
	},
	dark: {
		appBg: '#111111',
		surface: '#222222',
		surfaceHover: '#2A2A2A',
		panel: '#1A1A1A',
		textMain: '#E8E8E8',
		textSub: '#9A9A9A',
		textMuted: '#555555'
	}
};

export const THEME_PRESETS: ThemePreset[] = [
	{
		id: 'graphite',
		nameKey: 'settings.theme.presets.graphite',
		accent: { light: '#1A1A1A', dark: '#E8E8E8' }
	},
	{
		id: 'midnight',
		nameKey: 'settings.theme.presets.midnight',
		accent: { light: '#3B5BDB', dark: '#7C93FF' },
		tone: {
			light: { appBg: '#EEF1F8', surface: '#E7EBF5', surfaceHover: '#DDE3F1', panel: '#FFFFFF' },
			dark: { appBg: '#0E1320', surface: '#1B2333', surfaceHover: '#232D40', panel: '#151C2A' }
		}
	},
	{
		id: 'ocean',
		nameKey: 'settings.theme.presets.ocean',
		accent: { light: '#0E7490', dark: '#38BDF8' },
		tone: {
			light: { appBg: '#EDF4F6', surface: '#E4EFF2', surfaceHover: '#D9E8EC', panel: '#FFFFFF' },
			dark: { appBg: '#0C1418', surface: '#16242B', surfaceHover: '#1D2F38', panel: '#111C22' }
		}
	},
	{
		id: 'forest',
		nameKey: 'settings.theme.presets.forest',
		accent: { light: '#157F5B', dark: '#34D399' },
		tone: {
			light: { appBg: '#EFF4F0', surface: '#E6EFE8', surfaceHover: '#DBE8DE', panel: '#FFFFFF' },
			dark: { appBg: '#0D1411', surface: '#18241E', surfaceHover: '#1F2F27', panel: '#121C18' }
		}
	},
	{
		id: 'amber',
		nameKey: 'settings.theme.presets.amber',
		accent: { light: '#B45309', dark: '#F59E0B' },
		tone: {
			light: { appBg: '#F6F1EA', surface: '#F0E9DF', surfaceHover: '#E8DFD2', panel: '#FFFDFA' },
			dark: { appBg: '#16110C', surface: '#241C14', surfaceHover: '#2E241A', panel: '#1B150F' }
		}
	},
	{
		id: 'rose',
		nameKey: 'settings.theme.presets.rose',
		accent: { light: '#BE123C', dark: '#FB7185' },
		tone: {
			light: { appBg: '#F7EFF1', surface: '#F2E6E9', surfaceHover: '#EBD9DE', panel: '#FFFCFD' },
			dark: { appBg: '#150F12', surface: '#241A1E', surfaceHover: '#2E2127', panel: '#1A1316' }
		}
	},
	{
		id: 'violet',
		nameKey: 'settings.theme.presets.violet',
		accent: { light: '#6D28D9', dark: '#A78BFA' },
		tone: {
			light: { appBg: '#F2EFFA', surface: '#EBE6F6', surfaceHover: '#E1DAF2', panel: '#FFFFFF' },
			dark: { appBg: '#110E1B', surface: '#1E1929', surfaceHover: '#262034', panel: '#161221' }
		}
	},
	{
		id: 'sakura',
		nameKey: 'settings.theme.presets.sakura',
		accent: { light: '#DB2777', dark: '#F472B6' },
		tone: {
			light: { appBg: '#F9F0F5', surface: '#F4E7EF', surfaceHover: '#EFDBE7', panel: '#FFFDFE' },
			dark: { appBg: '#150F14', surface: '#241A21', surfaceHover: '#2E202A', panel: '#1A1318' }
		}
	},
	{
		id: 'coffee',
		nameKey: 'settings.theme.presets.coffee',
		accent: { light: '#7C4A2D', dark: '#C89F81' },
		tone: {
			light: { appBg: '#F5F0EA', surface: '#EFE8E0', surfaceHover: '#E7DDD2', panel: '#FFFDFB' },
			dark: { appBg: '#14100D', surface: '#221B16', surfaceHover: '#2C231C', panel: '#191410' }
		}
	},
	{
		id: 'nord',
		nameKey: 'settings.theme.presets.nord',
		accent: { light: '#475569', dark: '#94A3B8' },
		tone: {
			light: { appBg: '#EFF2F5', surface: '#E7ECF1', surfaceHover: '#DDE4EB', panel: '#FFFFFF' },
			dark: { appBg: '#101418', surface: '#1B2128', surfaceHover: '#232B34', panel: '#151A20' }
		}
	},
	{
		id: 'lime',
		nameKey: 'settings.theme.presets.lime',
		accent: { light: '#4D7C0F', dark: '#A3E635' },
		tone: {
			light: { appBg: '#F3F5EC', surface: '#EDF1E2', surfaceHover: '#E4EAD6', panel: '#FEFFF9' },
			dark: { appBg: '#12150D', surface: '#1F2418', surfaceHover: '#28301E', panel: '#171B11' }
		}
	},
	{
		id: 'crimson',
		nameKey: 'settings.theme.presets.crimson',
		accent: { light: '#B91C1C', dark: '#F87171' },
		tone: {
			light: { appBg: '#F7EFEF', surface: '#F2E6E6', surfaceHover: '#EBDADA', panel: '#FFFCFC' },
			dark: { appBg: '#150E0E', surface: '#241A1A', surfaceHover: '#2E2121', panel: '#1A1313' }
		}
	}
];

/** 强调色快捷色板（自定义配色之外的「一键换色」） */
export const THEME_ACCENTS: string[] = [
	'#1A1A1A',
	'#3B5BDB',
	'#0E7490',
	'#157F5B',
	'#B45309',
	'#BE123C',
	'#6D28D9',
	'#DB2777',
	'#7C4A2D',
	'#475569',
	'#4D7C0F',
	'#B91C1C'
];

const RADIUS_VALUES: Record<ThemeRadiusId, { sm: string; md: string; lg: string }> = {
	sharp: { sm: '6px', md: '8px', lg: '10px' },
	soft: { sm: '8px', md: '10px', lg: '12px' },
	round: { sm: '12px', md: '16px', lg: '20px' }
};

export const DEFAULT_THEME: ThemeConfig = {
	mode: 'light',
	preset: 'graphite',
	accent: '',
	radius: 'soft',
	aura: true
};

export const THEME_CACHE_KEY = 'goclashz_theme_v2';

/* ---------------------------------------------------------------- 颜色工具 */

const clamp255 = (n: number) => Math.max(0, Math.min(255, Math.round(n)));

/** 解析 #RGB / #RRGGBB；失败返回 null */
export function parseHex(input: string | null | undefined): [number, number, number] | null {
	if (!input) return null;
	let hex = String(input).trim().replace(/^#/, '');
	if (hex.length === 3) hex = hex[0] + hex[0] + hex[1] + hex[1] + hex[2] + hex[2];
	if (!/^[0-9a-fA-F]{6}$/.test(hex)) return null;
	return [parseInt(hex.slice(0, 2), 16), parseInt(hex.slice(2, 4), 16), parseInt(hex.slice(4, 6), 16)];
}

export function toHex(rgb: [number, number, number]): string {
	const part = (v: number) => clamp255(v).toString(16).padStart(2, '0');
	return '#' + part(rgb[0]) + part(rgb[1]) + part(rgb[2]);
}

/**
 * 判断一个色是不是"中性灰"（r/g/b 三通道几乎相等）。
 * 中性强调色没有色相可染：玻璃染色滑条据此置为禁用态。
 */
export function isNeutralColor(hex: string): boolean {
	const rgb = parseHex(hex);
	if (!rgb) return true;
	return Math.max(...rgb) - Math.min(...rgb) < 12;
}

/** sRGB 相对亮度（WCAG） */
export function relativeLuminance(hex: string): number {
	const rgb = parseHex(hex);
	if (!rgb) return 0;
	const channel = (v: number) => {
		const s = v / 255;
		return s <= 0.03928 ? s / 12.92 : Math.pow((s + 0.055) / 1.055, 2.4);
	};
	return 0.2126 * channel(rgb[0]) + 0.7152 * channel(rgb[1]) + 0.0722 * channel(rgb[2]);
}

/**
 * 给定底色挑一个可读的前景色（白/黑）。
 * 分界点不是 0.45：白字对比度 = 1.05/(L+0.05)，黑字 = (L+0.05)/0.05，
 * 两者相等时 L = sqrt(0.0525) - 0.05 ≈ 0.179。用 0.45 会让浅亮强调色
 * （如 #38BDF8）配上白字，对比度只有 2.1:1，直接不可读。
 */
export function contrastForeground(bgHex: string): string {
	return relativeLuminance(bgHex) > 0.179 ? '#111111' : '#FFFFFF';
}

/** 混合两个十六进制颜色，weight 是 a 的占比（0..1） */
export function mixHex(a: string, b: string, weight: number): string {
	const ca = parseHex(a) || [0, 0, 0];
	const cb = parseHex(b) || [255, 255, 255];
	const w = Math.max(0, Math.min(1, weight));
	return toHex([
		ca[0] * w + cb[0] * (1 - w),
		ca[1] * w + cb[1] * (1 - w),
		ca[2] * w + cb[2] * (1 - w)
	]);
}

/** 变亮 / 变暗（amount>0 变亮，<0 变暗） */
export function shadeHex(hex: string, amount: number): string {
	return amount >= 0 ? mixHex('#ffffff', hex, amount) : mixHex('#000000', hex, -amount);
}

/* ------------------------------------------------------------ 配置解析 */

const MODES: ThemeMode[] = ['light', 'dark', 'system'];
const RADII: ThemeRadiusId[] = ['sharp', 'soft', 'round'];

export function normalizeThemeConfig(raw: Partial<ThemeConfig> | null | undefined): ThemeConfig {
	const src = raw || {};
	const mode = MODES.indexOf(src.mode as ThemeMode) >= 0 ? (src.mode as ThemeMode) : DEFAULT_THEME.mode;
	const preset = THEME_PRESETS.some((p) => p.id === src.preset) ? String(src.preset) : DEFAULT_THEME.preset;
	const radius = RADII.indexOf(src.radius as ThemeRadiusId) >= 0 ? (src.radius as ThemeRadiusId) : DEFAULT_THEME.radius;
	const parsedAccent = parseHex(src.accent);
	return {
		mode,
		preset,
		accent: parsedAccent ? toHex(parsedAccent) : '',
		radius,
		aura: src.aura === undefined || src.aura === null ? DEFAULT_THEME.aura : !!src.aura
	};
}

/** 从后端 AppBehavior 读出主题配置（旧配置缺字段时回落默认） */
export function themeConfigFromBehavior(behavior: any): ThemeConfig {
	return normalizeThemeConfig({
		mode: behavior && behavior.themeMode,
		preset: behavior && behavior.themePreset,
		accent: behavior && behavior.themeAccent,
		radius: behavior && behavior.themeRadius,
		aura: behavior ? behavior.themeAura : undefined
	});
}

export function getPreset(id: string): ThemePreset {
	return THEME_PRESETS.filter((p) => p.id === id)[0] || THEME_PRESETS[0];
}

/** 主题实际生效的强调色（自定义优先，否则用预设对应明暗的值） */
export function resolveAccent(config: ThemeConfig, isDark: boolean): string {
	if (config.accent) return config.accent;
	const preset = getPreset(config.preset);
	return isDark ? preset.accent.dark : preset.accent.light;
}

/** 主题实际生效的调色（基准 + 预设 tone 覆盖） */
export function resolvePalette(config: ThemeConfig, isDark: boolean): ThemePalette {
	const base = isDark ? BASE_PALETTE.dark : BASE_PALETTE.light;
	const tone = getPreset(config.preset).tone;
	const override = (isDark ? (tone && tone.dark) : (tone && tone.light)) || {};
	return Object.assign({}, base, override);
}

/* ------------------------------------------------------------ 系统明暗 */

export function systemPrefersDark(): boolean {
	if (typeof window === 'undefined' || !window.matchMedia) return false;
	return window.matchMedia('(prefers-color-scheme: dark)').matches;
}

export function resolveIsDark(mode: ThemeMode): boolean {
	if (mode === 'system') return systemPrefersDark();
	return mode === 'dark';
}

/** 监听系统明暗变化；返回取消函数 */
export function onSystemThemeChange(handler: () => void): () => void {
	if (typeof window === 'undefined' || !window.matchMedia) return function () {};
	const mq = window.matchMedia('(prefers-color-scheme: dark)');
	const listener = () => handler();
	try {
		mq.addEventListener('change', listener);
		return () => mq.removeEventListener('change', listener);
	} catch {
		// 旧版 WebView 回退
		mq.addListener(listener);
		return () => mq.removeListener(listener);
	}
}

/* ------------------------------------------------------------ 应用主题 */

export interface AppliedTheme {
	config: ThemeConfig;
	isDark: boolean;
	accent: string;
	palette: ThemePalette;
}

/** 只算不写：给预览 UI 用 */
export function computeTheme(config: ThemeConfig): AppliedTheme {
	const normalized = normalizeThemeConfig(config);
	const isDark = resolveIsDark(normalized.mode);
	return {
		config: normalized,
		isDark,
		accent: resolveAccent(normalized, isDark),
		palette: resolvePalette(normalized, isDark)
	};
}

/**
 * 应用主题：写 data-* 标记 + CSS 变量。
 * 不改变 globalState.theme（明暗真源在 store，由调用方决定是否同步）。
 */
export function applyTheme(config: ThemeConfig): AppliedTheme {
	const applied = computeTheme(config);
	if (typeof document === 'undefined') return applied;
	const root = document.documentElement;

	root.dataset.themePreset = applied.config.preset;
	root.dataset.themeMode = applied.isDark ? 'dark' : 'light';
	root.dataset.themeRadius = applied.config.radius;
	root.dataset.themeAura = applied.config.aura ? 'on' : 'off';
	root.classList.toggle('dark', applied.isDark);

	const palette = applied.palette;
	const accent = applied.accent;
	const fg = contrastForeground(accent);
	const s = root.style;
	s.setProperty('--app-bg', palette.appBg);
	s.setProperty('--surface', palette.surface);
	s.setProperty('--surface-hover', palette.surfaceHover);
	s.setProperty('--surface-panel', palette.panel);
	s.setProperty('--text-main', palette.textMain);
	s.setProperty('--text-sub', palette.textSub);
	s.setProperty('--text-muted', palette.textMuted);

	s.setProperty('--ui-accent', accent);
	s.setProperty('--ui-accent-fg', fg);
	s.setProperty('--ui-accent-hover', applied.isDark ? shadeHex(accent, 0.18) : shadeHex(accent, -0.16));
	s.setProperty('--ui-accent-soft', 'color-mix(in srgb, ' + accent + ' 12%, transparent)');
	s.setProperty('--ui-accent-ring', 'color-mix(in srgb, ' + accent + ' 26%, transparent)');
	// 旧变量同步（部分历史组件仍直接读 --accent）
	s.setProperty('--accent', accent);
	s.setProperty('--accent-fg', fg);
	s.setProperty('--pulse-ring', 'color-mix(in srgb, ' + accent + ' 45%, transparent)');
	s.setProperty('--status-online', accent);

	// 玻璃染色目标色：直接跟随强调色。强调色本身是黑白灰时没有色相可染 ——
	// 染色层会退化成面板色本身（见 glass.ts 的中性目标判断），
	// 主题页据 isNeutralColor 把「染色」滑条置为禁用态，而不是偷偷塞一个冷色进去。
	s.setProperty('--glass-tint-target', accent);

	const r = RADIUS_VALUES[applied.config.radius];
	s.setProperty('--ui-radius-sm', r.sm);
	s.setProperty('--ui-radius-md', r.md);
	s.setProperty('--ui-radius-lg', r.lg);

	return applied;
}

/** 写入本地缓存（首屏防闪烁） */
export function cacheTheme(config: ThemeConfig): void {
	try {
		localStorage.setItem(THEME_CACHE_KEY, JSON.stringify(normalizeThemeConfig(config)));
	} catch {
		// 隐私模式 / 配额不足时静默失败
	}
}

/** 读缓存；没有缓存时返回默认值 */
export function readCachedTheme(): ThemeConfig {
	try {
		const raw = localStorage.getItem(THEME_CACHE_KEY);
		if (!raw) return Object.assign({}, DEFAULT_THEME);
		return normalizeThemeConfig(JSON.parse(raw));
	} catch {
		return Object.assign({}, DEFAULT_THEME);
	}
}

/** 启动期同步应用（main.ts 调用，早于 Vue 挂载，避免主题闪烁） */
export function bootstrapTheme(): ThemeConfig {
	const config = readCachedTheme();
	applyTheme(config);
	return config;
}

/** 把主题写回 behavior 对象的字段（页面保存时用） */
export function writeThemeToBehavior(behavior: any, config: ThemeConfig): void {
	const normalized = normalizeThemeConfig(config);
	behavior.themeMode = normalized.mode;
	behavior.themePreset = normalized.preset;
	behavior.themeAccent = normalized.accent;
	behavior.themeRadius = normalized.radius;
	behavior.themeAura = normalized.aura;
}
