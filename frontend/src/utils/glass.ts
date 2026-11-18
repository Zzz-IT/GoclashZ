// 玻璃模糊（毛玻璃）效果工具。
// 职责收敛为两件事：在根元素上打 data-glass 标记 + 写 CSS 变量；
// 具体视觉（底色多透、染什么色）全部由样式层消费 --glass-tint-bg / --glass-fill-*。
// 模糊半径不再是可调项：三级半径是写死的设计常量（见 styles/tokens.css），
// 其中只有浮层真的挂了 backdrop-filter，另外两级写了也没人消费（已移除）。
// 关键约束：这里不写任何内联 background（旧实现会盖死 token 与主题切换）。
export const GLASS_BLUR_MIN = 0;
export const GLASS_BLUR_MAX = 80;

/** 内容卡片（卡片/分组/列表容器）默认模糊半径（px） */
export const GLASS_BLUR_DEFAULT = 12;
/** 浮层（弹窗/下拉/菜单）默认模糊半径（px） */
export const GLASS_BLUR_OVERLAY_DEFAULT = 28;
/** 控件凹槽（分段控件/图表槽）默认模糊半径（px） */
export const GLASS_BLUR_CONTROL_DEFAULT = 8;
/** 背景 tint 默认强度（0..80；换算成 alpha 后与旧版 0.58 一致） */
export const GLASS_TINT_DEFAULT = 58;

/** 玻璃厚度：一个滑条同时改 tint alpha 与四级面板填充。50 = 改造前的观感 */
export const GLASS_DEPTH_MIN = 0;
export const GLASS_DEPTH_MAX = 100;
export const GLASS_DEPTH_DEFAULT = 50;

/** 背景 tint + 厚度总控；缺省项各自回落到默认值 */
export interface GlassStrengthGroups {
	tint?: number;
	depth?: number;
}

/** 把任意输入夹到 [GLASS_BLUR_MIN, GLASS_BLUR_MAX] 的整数；非法值（含 undefined/null）回落到 fallback */
export function normalizeGlassStrength(
	value: number | null | undefined,
	fallback: number = GLASS_BLUR_DEFAULT
): number {
	// 注意 Number(null) === 0：null 会被当成"合法值 0"，从而把缺失配置读成"不模糊"。
	// 缺字段（null / undefined）必须整体回落 fallback，只有真正的数字才参与夹取。
	if (value === null || value === undefined) return fallback;
	const n = Number(value);
	if (!Number.isFinite(n)) return fallback;
	return Math.min(GLASS_BLUR_MAX, Math.max(GLASS_BLUR_MIN, Math.round(n)));
}

/** 兼容旧调用方：单强度归一化（语义等同「内容卡片」组） */
export function normalizeGlassBlurStrength(strength?: number): number {
	return normalizeGlassStrength(strength, GLASS_BLUR_DEFAULT);
}

/** 厚度归一化：夹到 [GLASS_DEPTH_MIN, GLASS_DEPTH_MAX]；缺字段回落默认 50 */
export function normalizeGlassDepth(value: number | null | undefined): number {
	if (value === null || value === undefined) return GLASS_DEPTH_DEFAULT;
	const n = Number(value);
	if (!Number.isFinite(n)) return GLASS_DEPTH_DEFAULT;
	return Math.min(GLASS_DEPTH_MAX, Math.max(GLASS_DEPTH_MIN, Math.round(n)));
}

/**
 * 0..80 的 tint 强度 → 染色色相的混合百分比。
 *
 * 语义与旧版不同：这个值**只改玻璃的颜色**，不再碰透明度。
 * 旧实现把它换算成 alpha，于是「加浓」等于「变实」，玻璃感被一并吃掉；
 * 现在 alpha 固定为 GLASS_TINT_ALPHA，滑条只在"无色玻璃 → 染色玻璃"之间移动，
 * 背景透光比例全程不变。
 * 0 → 完全无色（等同主题面板色）；58 → 向染色目标色偏移 58%；80 → 偏移 80%。
 */
export function glassTintChroma(tint: number): number {
	return normalizeGlassStrength(tint, GLASS_TINT_DEFAULT);
}

/**
 * 染色的兜底透明度：只有直接调用 glassTintColor 且不传 alpha 时才会用到。
 *
 * 正常路径（applyGlassEffect）写入的值由「玻璃厚度」在 DEPTH_ANCHORS.tintAlpha
 * 的三档之间插值决定，与这里的 0.34 无关。
 * 这一层叠在 DWM Acrylic 之上，实际透光 = 1 − (1 − α_dwm)(1 − α_app)。
 */
export const GLASS_TINT_ALPHA = 0.34;

/* ---------------------------------------------------------------------------
 * 厚度总控
 *
 * 面板「看起来多实」是四级叠出来的，不是四个独立偏好：浮层最实（要盖住正文）、
 * 卡片次之、控件凹槽再次、页面大面板最透。
 * 给每一级单独一条滑条，用户随手就能拖出「凹槽比卡片还实」这种自相矛盾的组合，
 * 所以对外只暴露一个厚度值：四级填充与 tint alpha 乘同一个系数，层次比例恒定。
 * ------------------------------------------------------------------------- */

/**
 * 厚度 0 / 50 / 100 三档下的取值，中间线性插值。
 *
 * 四级填充的三档与改造前 tokens.css 的常量一致（50 那一档就是 86/62/48/44），
 * 三档都保持「浮层最实 → 卡片 → 凹槽 → 大面板最透」的次序，
 * 拖到两端也不会出现某一级被夹死、滑条后半段失效的情况。
 *
 * tintAlpha 例外：它比改造前的 0.34 整体抬高了一档。原因不是审美，
 * 而是背景亮度不可控 —— 窗口着色层是唯一能"把背景拉回亮区间"的东西，
 * 0.34 时背后只要是一块中灰壁纸，窗口就落到 #909090 一带，
 * 压在裸背景上的 --text-sub 只有 1.7:1，直接看不见。
 */
const DEPTH_ANCHORS = {
	tintAlpha: [0.38, 0.58, 0.78],
	card: [34, 62, 92],
	soft: [26, 48, 76],
	content: [24, 44, 70]
} as const;

/**
 * 浮层填充：**不走厚度总控**，是独立常量。
 *
 * 实测（无头 Chrome，同一层叠结构下 5 档填充对比）：
 *   15% / 30% —— 背后内容被清楚糊开，能看到平滑的明暗过渡
 *   45%       —— 只剩一点痕迹
 *   60% / 82% —— 完全均匀，backdrop-filter 等于不存在
 * 也就是说浮层的可读性不能靠"加厚填充"来换：一旦超过 ~45%，
 * 28px 的模糊就完全没有产出，浮层退化成一块半透明色板。
 * 它也不该跟着厚度滑条走 —— 厚度是给主界面的，浮层有自己的物理约束。
 */
const GLASS_OVERLAY_FILL = 46;

/** 在三档锚点之间线性插值；depth 缺省按 50 处理 */
function depthLerp(anchors: readonly [number, number, number], depth: number | null | undefined): number {
	const d = normalizeGlassDepth(depth);
	if (d <= GLASS_DEPTH_DEFAULT) {
		return anchors[0] + ((anchors[1] - anchors[0]) * d) / GLASS_DEPTH_DEFAULT;
	}
	return (
		anchors[1] +
		((anchors[2] - anchors[1]) * (d - GLASS_DEPTH_DEFAULT)) / (GLASS_DEPTH_MAX - GLASS_DEPTH_DEFAULT)
	);
}

/**
 * 生成一级填充的 color-mix。
 * 引用 var(--surface-panel) 而不是算成色值 —— 这样主题切换时这一级自动跟随，
 * 不需要额外重算（--glass-tint-bg 是 HSL 字面量，做不到这一点）。
 */
function fillMix(percent: number): string {
	const pct = Math.min(100, Math.max(0, Math.round(percent)));
	return `color-mix(in srgb, var(--surface-panel) ${pct}%, transparent)`;
}

/** alpha 统一保留三位小数，避免 0.34000000000000008 这种串进 CSS */
function formatAlpha(alpha: number): string {
	return String(Math.round(Math.min(1, Math.max(0, alpha)) * 1000) / 1000);
}

/* ---------------------------------------------------------------------------
 * 染色颜色计算
 *
 * 不能直接把面板色向目标色做 RGB 混合：目标色和面板色的明度差太远，
 * 混出来的颜色亮度会随滑条剧烈变化（深色主题 #1A1A1A → #6E8FC7 亮度涨 5.4 倍），
 * 观感就是"拖滑条在黑白之间翻"。
 *
 * 所以改成在 HSL 里操作：色相取目标色、饱和度随滑条上升，
 * 而明度只在「面板色明度 → 夹紧后的可显色区间」之间按比例移动。
 * 纯白/纯黑面板（L=100 / L=0）下任何色相都会被压成同色，
 * 所以把目标明度夹进 [TINT_MIN_L, TINT_MAX_L]，保证染色始终可见、又不至于变成明暗滑条。
 * ------------------------------------------------------------------------- */

/** 染色时允许到达的最低/最高明度：越界后色相就显不出来了 */
const TINT_MIN_L = 22;
const TINT_MAX_L = 82;
/** 饱和度的上限：留一点余地，纯饱和会显得刺眼 */
const TINT_MAX_S = 85;
/** 低于这个饱和度就算"没有色相可染"：目标色是黑白灰时滑条应当完全无效 */
const TINT_NEUTRAL_S = 6;

function hexToRgb(hex: string): [number, number, number] | null {
	const raw = String(hex || '').trim().replace(/^#/, '');
	const full = raw.length === 3 ? raw[0] + raw[0] + raw[1] + raw[1] + raw[2] + raw[2] : raw;
	if (!/^[0-9a-fA-F]{6}$/.test(full)) return null;
	return [parseInt(full.slice(0, 2), 16), parseInt(full.slice(2, 4), 16), parseInt(full.slice(4, 6), 16)];
}

function rgbToHsl(rgb: [number, number, number]): { h: number; s: number; l: number } {
	const r = rgb[0] / 255;
	const g = rgb[1] / 255;
	const b = rgb[2] / 255;
	const max = Math.max(r, g, b);
	const min = Math.min(r, g, b);
	const l = (max + min) / 2;
	if (max === min) return { h: 0, s: 0, l: l * 100 };
	const d = max - min;
	const s = l > 0.5 ? d / (2 - max - min) : d / (max + min);
	let h: number;
	if (max === r) h = ((g - b) / d + (g < b ? 6 : 0)) / 6;
	else if (max === g) h = ((b - r) / d + 2) / 6;
	else h = ((r - g) / d + 4) / 6;
	return { h: h * 360, s: s * 100, l: l * 100 };
}

/** 沿最短弧插值色相，避免 350°→10° 绕一大圈 */
function lerpHue(from: number, to: number, t: number): number {
	let diff = ((to - from + 540) % 360) - 180;
	return (from + diff * t + 360) % 360;
}

/**
 * 计算玻璃染色层的颜色（含固定 alpha）。
 * @param panelHex 当前主题的面板色（--surface-panel）
 * @param targetHex 染色目标色（--glass-tint-target）
 * @param chroma 0..80 的滑条值；0 时结果严格等于面板色
 * @param alpha 该层的透明度；由厚度总控换算，省略时取基准值
 * @returns 可直接用作 background 的颜色字符串；解析失败返回 null（调用方回退到 color-mix 写法）
 */
export function glassTintColor(
	panelHex: string,
	targetHex: string,
	chroma: number,
	alpha: number = GLASS_TINT_ALPHA
): string | null {
	const panel = hexToRgb(panelHex);
	const target = hexToRgb(targetHex);
	if (!panel || !target) return null;
	const a = formatAlpha(alpha);

	const base = rgbToHsl(panel);
	const tgt = rgbToHsl(target);
	const ratio = normalizeGlassStrength(chroma, GLASS_TINT_DEFAULT) / GLASS_BLUR_MAX;

	// 0 必须严格等于当前面板色，否则默认状态（未开染色）会和现在不一样
	if (ratio <= 0) {
		return `hsl(${base.h.toFixed(1)} ${base.s.toFixed(1)}% ${base.l.toFixed(1)}% / ${a})`;
	}

	/* 目标色是黑白灰时没有色相可染，直接返回面板色本身。
	   不能只跳过色相插值：下面的 s 会把饱和度从 base.s 推向 TINT_MAX_S，
	   而中性色的 h 是 0（红），灰色面板会被染成红色 —— 这正是「单色主题
	   染色」必须整体短路的原因。主题页用同一个判据禁用滑条。 */
	if (tgt.s <= TINT_NEUTRAL_S) {
		return `hsl(${base.h.toFixed(1)} ${base.s.toFixed(1)}% ${base.l.toFixed(1)}% / ${a})`;
	}

	const lTarget = Math.min(TINT_MAX_L, Math.max(TINT_MIN_L, base.l));
	const l = base.l + (lTarget - base.l) * ratio;
	const s = base.s + (TINT_MAX_S - base.s) * ratio;
	/* 面板色本身无彩度时（纯白/纯黑）它的色相是 rgbToHsl 给的占位 0（红），
	   拿它当插值起点会把目标色拉到半路 —— 海洋青 193° 会被染成 239° 的蓝。
	   无色玻璃染向彩色时直接取目标色相；此时 s 也很小，看不出跳变。 */
	const h = base.s <= TINT_NEUTRAL_S ? tgt.h : lerpHue(base.h, tgt.h, ratio);

	return `hsl(${h.toFixed(1)} ${s.toFixed(1)}% ${l.toFixed(1)}% / ${a})`;
}

/**
 * 最近一次 applyGlassEffect 的入参。
 *
 * --glass-tint-bg 是由 glassTintColor() 算出的**字面量**（不是 color-mix 那种
 * 会跟着变量自动求值的表达式），唯一的输入是 --surface-panel。主题一换，
 * --surface-panel 变了，但没有任何人重算它 —— 玻璃开着时 .app-shell 的前景
 * 就靠这一条 tint 撑着，于是整窗停在旧主题的面板色上，观感就是"切暗色没生效"。
 * 所以 applyThemeToApp 之后必须调用 refreshGlassTint() 重算一次。
 */
let lastApplied: { enabled: boolean; groups?: GlassStrengthGroups } | null = null;

/** 主题切换后重算玻璃层（当前只有 --glass-tint-bg 依赖主题变量） */
export function refreshGlassTint(): void {
	if (!lastApplied) return;
	applyGlassEffect(lastApplied.enabled, lastApplied.groups);
}

/**
 * 应用/关闭玻璃模糊：只写 data-glass 与填充/染色变量，不碰任何元素的 background。
 * 模糊半径不在这里写 —— 它是 tokens.css 里的设计常量。
 * @param enabled 是否启用（写成 html 的 data-glass = on/off）
 * @param groups 背景 tint + 厚度总控
 */
export function applyGlassEffect(enabled: boolean, groups?: GlassStrengthGroups): void {
	if (typeof document === 'undefined') return;
	lastApplied = { enabled, groups: groups ? { ...groups } : undefined };
	const root = document.documentElement;
	root.dataset.glass = enabled ? 'on' : 'off';

	const tint = normalizeGlassStrength(groups?.tint, GLASS_TINT_DEFAULT);

	// 厚度总控：四级填充与 tint alpha 走同一套锚点，层次比例恒定。
	// 四级底色统一取 --surface-panel（改造前凹槽那级用的是 --surface，比面板色深一档，
	// 浅色主题下就是一个发闷的灰块），这样四级只剩透明度差异，拖动滑条时是单调变化。
	const depth = groups?.depth;
	root.style.setProperty('--glass-fill-overlay', fillMix(GLASS_OVERLAY_FILL));
	root.style.setProperty('--glass-fill-card', fillMix(depthLerp(DEPTH_ANCHORS.card, depth)));
	root.style.setProperty('--glass-fill-soft', fillMix(depthLerp(DEPTH_ANCHORS.soft, depth)));
	root.style.setProperty('--glass-fill-content', fillMix(depthLerp(DEPTH_ANCHORS.content, depth)));

	// 染色层：优先用 HSL 计算（明度被约束住，拖滑条不会出现明暗跳动）。
	// 面板色 / 目标色取不到可解析的 hex 时，退回 color-mix 写法 —— 虽然会带上
	// 明度漂移，但至少染色本身还有效。
	const tintAlpha = depthLerp(DEPTH_ANCHORS.tintAlpha, depth);
	const computed = getComputedStyle(root);
	const tintColor = glassTintColor(
		computed.getPropertyValue('--surface-panel'),
		computed.getPropertyValue('--glass-tint-target'),
		tint,
		tintAlpha
	);
	root.style.setProperty(
		'--glass-tint-bg',
		tintColor ??
			`color-mix(in srgb, color-mix(in srgb, var(--glass-tint-target) ${glassTintChroma(tint)}%, var(--surface-panel)) ${(tintAlpha * 100).toFixed(0)}%, transparent)`
	);
}
