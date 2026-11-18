<template>
  <div class="settings-page ui-page-shell theme-page" :class="{ 'no-anim': loading }">
    <!-- 页面头：返回 + 标题 + 恢复默认 -->
    <div class="sub-header section-header ui-page-header">
      <button class="back-btn" type="button" :aria-label="t('common.back')" @click="$emit('navigate', 'main')">
        <svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" stroke-width="2"><line x1="19" y1="12" x2="5" y2="12"></line><polyline points="12 19 5 12 12 5"></polyline></svg>
      </button>
      <h3>{{ t('settings.theme.title') }}</h3>
      <button class="action-btn accent-btn mini-btn-reset" type="button" @click="handleReset">
        <span class="btn-icon" v-html="THEME_ICONS.reset"></span>
        {{ t('settings.theme.resetBtn') }}
      </button>
    </div>

    <!-- 页面唯一的滚动区 -->
    <div class="ui-page-body theme-body">
      <!-- ② 外观模式 -->
      <section class="glass-card setting-group ui-surface-group scrollable theme-group">
        <div class="group-head">
          <div class="group-text">
            <h3>{{ t('settings.theme.modeGroup') }}</h3>
          </div>
        </div>
        <div class="seg" id="set-theme-mode" role="radiogroup" :aria-label="t('settings.theme.modeGroup')">
          <button
            v-for="mode in MODE_OPTIONS"
            :key="mode.id"
            type="button"
            class="seg-btn"
            role="radio"
            :aria-checked="config.mode === mode.id"
            :class="{ active: config.mode === mode.id }"
            @click="setMode(mode.id)"
          >
            <span class="seg-icon" v-html="mode.icon"></span>
            <span>{{ t(mode.labelKey) }}</span>
          </button>
        </div>
      </section>

      <!-- ③ 预设主题网格 -->
      <section class="glass-card setting-group ui-surface-group scrollable theme-group">
        <div class="group-head">
          <div class="group-text">
            <h3>{{ t('settings.theme.presetGroup') }}</h3>
          </div>
        </div>
        <div class="preset-grid" id="set-theme-preset" role="radiogroup" :aria-label="t('settings.theme.presetGroup')">
          <button
            v-for="preset in presetTiles"
            :key="preset.id"
            type="button"
            class="preset-tile"
            role="radio"
            :aria-checked="preset.active"
            :class="{ active: preset.active }"
            @click="setPreset(preset.id)"
          >
            <span class="preset-swatches">
              <i v-for="(color, index) in preset.colors" :key="index" :style="{ background: color }"></i>
            </span>
            <span class="preset-meta">
              <span class="preset-name">{{ preset.name }}</span>
              <span v-if="preset.active" class="preset-check" v-html="THEME_ICONS.check"></span>
            </span>
          </button>
        </div>
      </section>

      <!-- ④ 强调色 -->
      <section class="glass-card setting-group ui-surface-group scrollable theme-group">
        <div class="group-head">
          <div class="group-text">
            <h3>{{ t('settings.theme.accentGroup') }}</h3>
          </div>
        </div>
        <div class="accent-block" id="set-theme-accent">
          <div class="accent-row">
            <div class="accent-dots" role="group" :aria-label="t('settings.theme.accentGroup')">
              <button
                v-for="hex in THEME_ACCENTS"
                :key="hex"
                type="button"
                class="accent-dot"
                :class="{ active: isAccentActive(hex) }"
                :style="{ background: hex }"
                :aria-label="hex"
                :aria-pressed="isAccentActive(hex)"
                @click="setAccent(hex)"
              >
                <span v-if="isAccentActive(hex)" class="accent-dot__check" :style="{ color: contrastForeground(hex) }" v-html="THEME_ICONS.check"></span>
              </button>
            </div>
            <div class="accent-custom">
              <span class="accent-custom__label">{{ t('settings.theme.accentCustom') }}</span>
              <span class="accent-picker">
                <input
                  class="accent-color"
                  type="color"
                  :value="nativeColorValue"
                  :aria-label="t('settings.theme.accentCustom')"
                  @input="onNativePick"
                  @change="commitConfig"
                />
                <span class="accent-picker__badge" aria-hidden="true" v-html="THEME_ICONS.plus"></span>
              </span>
              <input
                class="hex-input"
                :class="{ invalid: hexError }"
                type="text"
                maxlength="7"
                spellcheck="false"
                autocomplete="off"
                placeholder="#3B5BDB"
                :aria-invalid="hexError"
                :aria-label="t('settings.theme.accentCustom')"
                :value="hexDraft"
                @focus="hexFocused = true"
                @input="onHexInput"
                @blur="onHexBlur"
                @keydown.enter.prevent="onHexEnter"
              />
              <button
                type="button"
                class="follow-btn"
                :class="{ active: isAccentFollow }"
                :aria-pressed="isAccentFollow"
                @click="setAccent('')"
              >
                <span class="follow-btn__icon" v-html="THEME_ICONS.reset"></span>
                {{ t('settings.theme.accentFollow') }}
              </button>
            </div>
          </div>
          <p v-if="hexError" class="accent-error" role="alert">{{ t('settings.theme.accentInvalid') }}</p>
        </div>
      </section>

      <!-- ⑤ 界面圆角 -->
      <section class="glass-card setting-group ui-surface-group scrollable theme-group">
        <div class="group-head">
          <div class="group-text">
            <h3>{{ t('settings.theme.radiusGroup') }}</h3>
          </div>
        </div>
        <div class="seg" id="set-theme-radius" role="radiogroup" :aria-label="t('settings.theme.radiusGroup')">
          <button
            v-for="option in RADIUS_OPTIONS"
            :key="option.id"
            type="button"
            class="seg-btn"
            role="radio"
            :aria-checked="config.radius === option.id"
            :class="{ active: config.radius === option.id }"
            @click="setRadius(option.id)"
          >
            <span class="radius-demo" :style="{ borderRadius: option.demo }"></span>
            <span>{{ t(option.labelKey) }}</span>
          </button>
        </div>
      </section>

      <!-- ⑥ 玻璃质感：开关 + 模糊 + 染色 + 厚度总控 -->
      <section class="glass-card setting-group ui-surface-group scrollable theme-group">
        <div class="group-head" id="set-theme-glass">
          <div class="group-text">
            <h3>{{ t('settings.theme.glassGroup') }}</h3>
            <p>{{ t('settings.theme.glassDesc') }}</p>
          </div>
          <label class="modern-switch" :title="t('settings.behavior.glassBlur')">
            <input
              type="checkbox"
              v-model="behavior.glassBlurEnabled"
              :aria-label="t('settings.behavior.glassBlur')"
              @change="onGlassToggle"
            />
            <span class="slider"></span>
          </label>
        </div>

        <Transition name="dropdown">
          <div v-if="glassEnabled" class="glass-body">
            <div class="blur-grid">
              <!-- 厚度总控：一条滑条同时缩放窗口着色层与四级面板填充。
                   层次比例（浮层最实 → 卡片 → 凹槽 → 大面板最透）由代码固定，
                   不给每一级单独开滑条，避免拖出「凹槽比卡片还实」这种组合。 -->
              <div class="blur-cell blur-cell--wide" id="set-theme-glassDepth">
                <div class="blur-cell__head">
                  <span class="blur-cell__name">{{ t('settings.behavior.glassDepth') }}</span>
                  <span class="blur-cell__value">{{ glassDepth }}</span>
                </div>
                <input
                  class="blur-range"
                  type="range"
                  :min="GLASS_DEPTH_MIN"
                  :max="GLASS_DEPTH_MAX"
                  step="1"
                  :aria-label="t('settings.behavior.glassDepth')"
                  :style="trackStyle(glassDepth, GLASS_DEPTH_MAX)"
                  v-model.number="behavior.glassDepth"
                  @input="previewGlass"
                  @change="commitGlass"
                />
                <p class="blur-cell__desc">{{ t('settings.behavior.glassDepthDesc') }}</p>
              </div>

              <div class="blur-cell blur-cell--wide" id="set-theme-glassTint">
                <div class="blur-cell__head">
                  <span class="blur-cell__name">{{ t('settings.behavior.glassTint') }}</span>
                  <span class="blur-cell__value">{{ glassTint }}</span>
                </div>
                <input
                  class="blur-range"
                  type="range"
                  :min="GLASS_BLUR_MIN"
                  :max="GLASS_BLUR_MAX"
                  step="1"
                  :aria-label="t('settings.behavior.glassTint')"
                  :style="trackStyle(glassTint)"
                  :disabled="tintDisabled"
                  v-model.number="behavior.glassTintStrength"
                  @input="previewGlass"
                  @change="commitGlass"
                />
                <p class="blur-cell__desc">{{ tintDisabled ? t('settings.behavior.glassTintNeutral') : t('settings.behavior.glassTintDesc') }}</p>
              </div>
            </div>
          </div>
        </Transition>
      </section>

      <!-- ⑦ 数据显示：彩色延迟 + 色阶示例 -->
      <section class="glass-card setting-group ui-surface-group scrollable theme-group">
        <div class="group-head">
          <div class="group-text">
            <h3>{{ t('settings.theme.dataGroup') }}</h3>
            <p>{{ t('settings.theme.dataDesc') }}</p>
          </div>
        </div>
        <div class="switch-row" id="set-theme-colorDelay">
          <div class="switch-row__text">
            <h4>{{ t('settings.behavior.colorDelay') }}</h4>
            <p>{{ t('settings.behavior.colorDelayDesc') }}</p>
          </div>
          <div class="delay-demo" :class="{ 'is-color': !!behavior.colorDelay }" aria-hidden="true">
            <span class="delay-pill fast">118ms</span>
            <span class="delay-pill mid">352ms</span>
            <span class="delay-pill slow">782ms</span>
          </div>
          <label class="modern-switch">
            <input
              type="checkbox"
              v-model="behavior.colorDelay"
              :aria-label="t('settings.behavior.colorDelay')"
              @change="commitConfig"
            />
            <span class="slider"></span>
          </label>
        </div>
      </section>

    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue';
import { t } from '../locales';
import { applyThemeToApp, showConfirm, showToast, themeState } from '../store';
import {
  applyGlassEffect,
  normalizeGlassDepth,
  normalizeGlassStrength,
  GLASS_BLUR_MAX,
  GLASS_BLUR_MIN,
  GLASS_DEPTH_DEFAULT,
  GLASS_DEPTH_MAX,
  GLASS_DEPTH_MIN,
  GLASS_TINT_DEFAULT
} from '../utils/glass';
import {
  DEFAULT_THEME,
  THEME_ACCENTS,
  THEME_PRESETS,
  computeTheme,
  contrastForeground,
  getPreset,
  isNeutralColor,
  normalizeThemeConfig,
  parseHex,
  resolveAccent,
  resolvePalette,
  themeConfigFromBehavior,
  toHex,
  type ThemeConfig,
  type ThemeMode,
  type ThemePreset,
  type ThemeRadiusId
} from '../utils/theme';

const props = defineProps<{
  behavior: Record<string, any>;
  resetKey: number;
  loading?: boolean;
}>();

const emit = defineEmits(['navigate', 'reset', 'save']);

/* ------------------------------------------------------------ 图标（内联 SVG） */
const THEME_ICONS = {
  reset: '<svg viewBox="0 0 24 24" width="15" height="15" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="1 4 1 10 7 10"/><path d="M3.51 15a9 9 0 1 0 2.13-9.36L1 10"/></svg>',
  mode: '<svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="9"/><path d="M12 3a9 9 0 0 1 0 18z" fill="currentColor" stroke="none"/></svg>',
  preset: '<svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2"><rect x="3" y="4" width="18" height="3.5" rx="1.75"/><rect x="3" y="10.25" width="18" height="3.5" rx="1.75"/><rect x="3" y="16.5" width="18" height="3.5" rx="1.75"/></svg>',
  accent: '<svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2"><path d="M12 3s6 6.4 6 10a6 6 0 0 1-12 0c0-3.6 6-10 6-10z"/></svg>',
  radius: '<svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><path d="M4 20V11a7 7 0 0 1 7-7h9"/><path d="M4 20h16"/></svg>',
  glass: '<svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2"><rect x="3" y="8.5" width="12.5" height="12.5" rx="2.5"/><rect x="8.5" y="3" width="12.5" height="12.5" rx="2.5"/></svg>',
  data: '<svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><line x1="4" y1="20" x2="20" y2="20"/><rect x="6" y="11" width="3" height="6" rx="1"/><rect x="11" y="7" width="3" height="10" rx="1"/><rect x="16" y="13" width="3" height="4" rx="1"/></svg>',
  check: '<svg viewBox="0 0 24 24" width="13" height="13" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"><polyline points="20 6 9 17 4 12"/></svg>',
  plus: '<svg viewBox="0 0 24 24" width="9" height="9" fill="none" stroke="currentColor" stroke-width="3" stroke-linecap="round"><line x1="12" y1="5" x2="12" y2="19"/><line x1="5" y1="12" x2="19" y2="12"/></svg>',
  sun: '<svg viewBox="0 0 24 24" width="15" height="15" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><circle cx="12" cy="12" r="4.4"/><path d="M12 2.2v2.1M12 19.7v2.1M2.2 12h2.1M19.7 12h2.1M5 5l1.5 1.5M17.5 17.5L19 19M19 5l-1.5 1.5M6.5 17.5L5 19"/></svg>',
  moon: '<svg viewBox="0 0 24 24" width="15" height="15" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 12.8A9 9 0 1 1 11.2 3a7 7 0 0 0 9.8 9.8z"/></svg>',
  system: '<svg viewBox="0 0 24 24" width="15" height="15" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="2.5" y="4" width="19" height="12.5" rx="2"/><path d="M9 20.5h6M12 16.5v4"/></svg>'
};

/* ------------------------------------------------------------ 选项常量 */
const MODE_OPTIONS: { id: ThemeMode; labelKey: string; icon: string }[] = [
  { id: 'light', labelKey: 'settings.theme.modeLight', icon: THEME_ICONS.sun },
  { id: 'dark', labelKey: 'settings.theme.modeDark', icon: THEME_ICONS.moon },
  { id: 'system', labelKey: 'settings.theme.modeSystem', icon: THEME_ICONS.system }
];

const RADIUS_OPTIONS: { id: ThemeRadiusId; labelKey: string; demo: string }[] = [
  { id: 'sharp', labelKey: 'settings.theme.radiusSharp', demo: '2px' },
  { id: 'soft', labelKey: 'settings.theme.radiusSoft', demo: '6px' },
  { id: 'round', labelKey: 'settings.theme.radiusRound', demo: '16px' }
];

/** 缩略预览用的圆角（按 shape 比例缩小，不等于真实 --ui-radius-*） */
const PREVIEW_RADIUS: Record<ThemeRadiusId, string> = { sharp: '5px', soft: '9px', round: '15px' };

/* ------------------------------------------------------------ 主题状态 */
/**
 * 主题状态的真源是 store.themeState（侧栏快捷明暗、跟随系统、启动后的后端校准都会更新它）。
 * 不能拿 props.behavior 的快照初始化：用户先点侧栏切了明暗、再进设置页时，
 * Settings 里的 behavior 可能还是上一次 loadBehavior 的旧对象，页面会显示成旧模式。
 */
const config = ref<ThemeConfig>(normalizeThemeConfig(themeState.config));

/** 只算不写：预览卡与色块全部从这里取色，不触碰全局 */
const applied = computed(() => computeTheme(config.value));

const previewStyle = computed(() => {
  const palette = applied.value.palette;
  const accent = applied.value.accent;
  return [
    '--pv-bg:' + palette.appBg,
    '--pv-surface:' + palette.surface,
    '--pv-hover:' + palette.surfaceHover,
    '--pv-panel:' + palette.panel,
    '--pv-text:' + palette.textMain,
    '--pv-sub:' + palette.textSub,
    '--pv-accent:' + accent,
    '--pv-accent-fg:' + contrastForeground(accent),
    '--pv-radius:' + PREVIEW_RADIUS[config.value.radius]
  ].join(';');
});

const presetName = computed(() => t(getPreset(config.value.preset).nameKey));
const actualModeLabel = computed(() => t(applied.value.isDark ? 'settings.theme.modeDark' : 'settings.theme.modeLight'));
const selectedModeLabel = computed(() => {
  const hit = MODE_OPTIONS.filter((option) => option.id === config.value.mode)[0] || MODE_OPTIONS[0];
  return t(hit.labelKey);
});
const previewChip = computed(() => presetName.value + ' · ' + actualModeLabel.value);
const summaryText = computed(() => t('settings.theme.summary', { preset: presetName.value, mode: selectedModeLabel.value }));

/** 12 张预设卡片的 4 个色块：底色 / 次表面 / 强调色 / 正文色 */
const presetTiles = computed(() =>
  THEME_PRESETS.map((preset: ThemePreset) => {
    const local: ThemeConfig = {
      mode: config.value.mode,
      preset: preset.id,
      accent: '',
      radius: config.value.radius,
      aura: config.value.aura
    };
    const palette = resolvePalette(local, applied.value.isDark);
    const accent = resolveAccent(local, applied.value.isDark);
    return {
      id: preset.id,
      name: t(preset.nameKey),
      active: config.value.preset === preset.id,
      colors: [palette.appBg, palette.surfaceHover, accent, palette.textMain]
    };
  })
);

/* ------------------------------------------------------------ 高频操作的防抖 / 合帧 */
// 拖滑条 / 拖动取色器时不要每帧都写全局变量，也不要每帧落库：
// input 只做热预览（rAF 或 120ms 合并），change（松手）才 emit('save')。
let themePreviewTimer: number | undefined;
let glassFrame = 0;

const applyThemePreview = () => applyThemeToApp(config.value, props.behavior);

/** 高频：120ms 合并一次 applyThemeToApp */
const previewConfig = () => {
  if (themePreviewTimer !== undefined) return;
  themePreviewTimer = window.setTimeout(() => {
    themePreviewTimer = undefined;
    applyThemePreview();
  }, 120);
};

/** 立即应用（有待执行的预览先撤销） */
const flushThemePreview = () => {
  if (themePreviewTimer !== undefined) {
    window.clearTimeout(themePreviewTimer);
    themePreviewTimer = undefined;
  }
  applyThemePreview();
};

/** 单击类操作：立即应用 + 落库 */
const commitConfig = () => {
  flushThemePreview();
  emit('save');
};

/* ------------------------------------------------------------ 外观模式 / 预设 / 圆角 */
const setMode = (mode: ThemeMode) => {
  if (config.value.mode === mode) return;
  config.value.mode = mode;
  commitConfig();
};

const setPreset = (id: string) => {
  if (config.value.preset === id) return;
  config.value.preset = id;
  commitConfig();
};

const setRadius = (radius: ThemeRadiusId) => {
  if (config.value.radius === radius) return;
  config.value.radius = radius;
  commitConfig();
};

/* ------------------------------------------------------------ 强调色 */
const accentHex = computed(() => config.value.accent || applied.value.accent);
const nativeColorValue = computed(() => accentHex.value.toLowerCase());
const isAccentFollow = computed(() => config.value.accent === '');
const isAccentActive = (hex: string) => config.value.accent.toLowerCase() === hex.toLowerCase();

const hexDraft = ref(config.value.accent || applied.value.accent);
const hexError = ref(false);
const hexFocused = ref(false);

const setAccent = (hex: string) => {
  const rgb = parseHex(hex);
  const next = rgb ? toHex(rgb) : '';
  if (next === config.value.accent) {
    hexError.value = false;
    hexDraft.value = next || accentHex.value;
    return;
  }
  config.value.accent = next;
  hexError.value = false;
  hexDraft.value = next || accentHex.value;
  commitConfig();
};

/** 原生取色器：拖动过程只热预览，松手（change）才落库 */
const onNativePick = (e: Event) => {
  const rgb = parseHex((e.target as HTMLInputElement).value);
  if (!rgb) return;
  config.value.accent = toHex(rgb);
  hexDraft.value = config.value.accent;
  hexError.value = false;
  previewConfig();
};

const onHexInput = (e: Event) => {
  const raw = (e.target as HTMLInputElement).value.trim();
  hexDraft.value = raw;
  const body = raw.replace(/^#/, '');
  if (body === '') {
    hexError.value = false;
    config.value.accent = '';
    previewConfig();
    return;
  }
  if (/^[0-9a-fA-F]{6}$/.test(body)) {
    hexError.value = false;
    config.value.accent = '#' + body.toLowerCase();
    previewConfig();
    return;
  }
  // 未输完（1-5 位）先不报错，输满 6 位仍非法才提示
  hexError.value = body.length >= 6;
};

const onHexEnter = (e: KeyboardEvent) => {
  (e.target as HTMLInputElement).blur();
};

const onHexBlur = () => {
  hexFocused.value = false;
  const body = hexDraft.value.trim().replace(/^#/, '');
  if (body === '') {
    setAccent('');
    return;
  }
  // 支持 #RGB 简写，统一归一化成 #RRGGBB
  if (/^[0-9a-fA-F]{3}$/.test(body)) {
    setAccent(body);
    return;
  }
  if (!/^[0-9a-fA-F]{6}$/.test(body)) {
    hexError.value = true;
    return;
  }
  setAccent(body);
};

// 预设 / 明暗变化时同步输入框显示（正在输入时不打扰）
watch(
  () => [config.value.preset, config.value.accent, applied.value.isDark],
  () => {
    if (hexFocused.value) return;
    hexError.value = false;
    hexDraft.value = config.value.accent || accentHex.value;
  }
);

/* ------------------------------------------------------------ 玻璃质感 */
const glassEnabled = computed(() => !!props.behavior.glassBlurEnabled);
const glassTint = computed(() => normalizeGlassStrength(props.behavior.glassTintStrength, GLASS_TINT_DEFAULT));
const glassDepth = computed(() => normalizeGlassDepth(props.behavior.glassDepth));

/**
 * 当前强调色是黑白灰时没有色相可染，染色滑条整体禁用并换成说明文案。
 * 判据用强调色本身（而非预设 id）：用户手填了彩色强调色时，滑条自动恢复可用。
 */
const tintDisabled = computed(() => isNeutralColor(applied.value.accent));

/** 旧配置可能缺字段：补默认值（随下一次 save 一起持久化） */
const ensureGlassDefaults = () => {
  const behavior = props.behavior;
  if (behavior.glassTintStrength == null) behavior.glassTintStrength = GLASS_TINT_DEFAULT;
  if (behavior.glassDepth == null) behavior.glassDepth = GLASS_DEPTH_DEFAULT;
};

const applyGlass = () => {
  applyGlassEffect(glassEnabled.value, {
    tint: props.behavior.glassTintStrength,
    depth: props.behavior.glassDepth
  });
};

/** 拖动滑条：合到一帧里只写 --glass-* 变量，不落库、不动主题变量 */
const previewGlass = () => {
  if (glassFrame) return;
  glassFrame = window.requestAnimationFrame(() => {
    glassFrame = 0;
    applyGlass();
  });
};

const commitGlass = () => {
  if (glassFrame) {
    window.cancelAnimationFrame(glassFrame);
    glassFrame = 0;
  }
  applyGlass();
  emit('save');
};

const onGlassToggle = () => commitGlass();

/** 滑条已填充轨道（用强调色，视觉反馈更直接） */
const trackStyle = (value: number, max: number = GLASS_BLUR_MAX) => {
  const percent = Math.max(0, Math.min(100, (value / max) * 100));
  return {
    background:
      'linear-gradient(to right, var(--ui-accent) 0%, var(--ui-accent) ' +
      percent +
      '%, var(--ui-surface-hover) ' +
      percent +
      '%, var(--ui-surface-hover) 100%)'
  };
};

/* ------------------------------------------------------------ 外部同步 / 恢复默认 */
/**
 * 只同步「玻璃」这部分（玻璃字段只存在 behavior 里）。
 * 主题配置不同步：真源是 themeState，主题字段由下面的 themeState watcher 负责，
 * 否则过期的 behavior 快照会把用户刚在侧栏做的切换顶回去。
 */
const syncFromBehavior = () => {
  ensureGlassDefaults();
  applyGlass();
};

watch(() => [props.resetKey, props.behavior], syncFromBehavior);

// 主题引擎的真源是 store.themeState：侧边栏快捷明暗、跟随系统的 OS 切换、
// 以及启动后的后端校准都会改它。这里必须回灌页面 config，
// 否则页面拿着旧快照，用户随便点一下预设就会把外部改动顶回去（实测复现过的 P1）。
const themeFingerprint = (c: ThemeConfig) =>
  [c.mode, c.preset, c.accent, c.radius, c.aura ? '1' : '0'].join('|');

watch(
  () => themeState.config,
  (next) => {
    if (themeFingerprint(next) === themeFingerprint(config.value)) return;
    config.value = normalizeThemeConfig(next);
    hexError.value = false;
    if (!hexFocused.value) hexDraft.value = config.value.accent || accentHex.value;
  },
  { deep: true }
);

ensureGlassDefaults();

const handleReset = async () => {
  const confirmed = await showConfirm(
    t('settings.resetConfirmMsg', { name: t('settings.theme.title') }),
    t('settings.resetConfirmTitle')
  );
  if (!confirmed) return;

  config.value = normalizeThemeConfig(DEFAULT_THEME);
  hexError.value = false;
  const behavior = props.behavior;
  behavior.glassBlurEnabled = false;
  behavior.glassTintStrength = GLASS_TINT_DEFAULT;
  behavior.glassDepth = GLASS_DEPTH_DEFAULT;

  flushThemePreview();
  if (glassFrame) {
    window.cancelAnimationFrame(glassFrame);
    glassFrame = 0;
  }
  applyGlass();
  hexDraft.value = accentHex.value;
  emit('save');
  showToast(t('settings.theme.savedToast'), 'success');
};

onBeforeUnmount(() => {
  if (themePreviewTimer !== undefined) {
    window.clearTimeout(themePreviewTimer);
    themePreviewTimer = undefined;
    // 卸载前把最后一次预览落到界面上，避免配置与界面不一致
    applyThemeToApp(config.value, props.behavior);
  }
  if (glassFrame) {
    window.cancelAnimationFrame(glassFrame);
    glassFrame = 0;
  }
});
</script>

<style scoped>
.settings-page { flex: 1 1 auto; min-width: 0; }
.theme-body {
  display: flex;
  flex-direction: column;
}


.setting-group { padding: 20px 24px; margin-bottom: 12px; }
.setting-group.scrollable { padding-bottom: 20px; }

/* 延迟色阶：彩色模式的三档状态色（与节点列表 .c-fast/.c-mid/.c-slow 同源） */
.theme-page {
  --delay-fast: #22c55e;
  --delay-mid: #f59e0b;
  --delay-slow: #ef4444;
}

/* ---------------------------------------------------------- 页面头 */
.sub-header { display: flex; align-items: center; gap: 16px; margin-bottom: 12px; padding: 0 0 12px 0; background: transparent; }
.sub-header.section-header { justify-content: space-between; }
.sub-header.section-header h3 { flex: 1; min-width: 0; margin: 0 0 0 12px; border: none; padding: 0; color: var(--text-main); font-size: 1.25rem; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.back-btn { background: var(--surface); border: none; color: var(--text-main); width: 36px; height: 36px; border-radius: 50%; display: flex; align-items: center; justify-content: center; cursor: pointer; transition: background-color 0.18s cubic-bezier(0.4, 0, 0.2, 1); }
.back-btn:hover { background: var(--ui-surface-hover); }
.mini-btn-reset { flex: 0 0 auto; height: 36px !important; padding: 0 14px !important; font-size: 0.85rem !important; border-radius: var(--ui-radius-sm) !important; white-space: nowrap; }
.mini-btn-reset :deep(.btn-icon) svg { width: 15px; height: 15px; }

/* ---------------------------------------------------------- 分组外壳 */
.theme-group { padding: 18px 24px 20px; }
.group-head { display: flex; align-items: flex-start; gap: 12px; margin-bottom: 14px; }
.group-icon {
  flex: 0 0 auto;
  display: flex;
  align-items: center;
  justify-content: center;
  width: 30px;
  height: 30px;
  border-radius: var(--ui-radius-sm);
  background: var(--ui-accent-soft);
  color: var(--ui-accent);
}
.group-text { flex: 1; min-width: 0; }
.group-text h3 { margin: 0; font-size: 1.05rem; font-weight: 600; color: var(--text-main); }
.group-text p { margin: 3px 0 0; font-size: 0.8rem; line-height: 1.5; color: var(--text-sub); }
.group-head .modern-switch { flex: 0 0 auto; margin-top: 4px; margin-left: 4px; }

/* ---------------------------------------------------------- 分段控件 */
.seg {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
  padding: 4px;
  border-radius: var(--ui-radius-md);
  background: var(--ui-surface-control);
  border: 1px solid var(--ui-border-subtle);
}
.seg-btn {
  flex: 1 1 132px;
  min-width: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 7px;
  height: 34px;
  padding: 0 10px;
  border: none;
  border-radius: calc(var(--ui-radius-md) - 3px);
  background: transparent;
  color: var(--text-sub);
  font-family: inherit;
  font-size: 0.84rem;
  font-weight: 500;
  white-space: nowrap;
  cursor: pointer;
  transition: background-color 0.16s cubic-bezier(0.4, 0, 0.2, 1), color 0.16s cubic-bezier(0.4, 0, 0.2, 1), box-shadow 0.16s;
}
.seg-btn:hover { color: var(--text-main); background: var(--ui-surface-hover); }
.seg-btn.active { background: var(--ui-accent-block); color: var(--ui-accent-fg); box-shadow: var(--ui-shadow-1); }
.seg-btn:focus-visible { outline: 2px solid var(--ui-accent-ring); outline-offset: 1px; }
.seg-icon { flex: 0 0 auto; display: flex; align-items: center; }
/* 长语言（ru/fr/de）下标签允许省略，不再把分段控件撑爆 */
.seg-btn > span:last-child { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.radius-demo { flex: 0 0 auto; width: 20px; height: 20px; border: 2px solid currentColor; opacity: 0.9; transition: border-radius 0.16s; }

/* ---------------------------------------------------------- 预设网格 */
.preset-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(138px, 1fr));
  gap: 8px;
}
/* 12 张卡片永远整除，避免出现 5+5+2 这种孤行 */
@media (max-width: 1240px) {
  .preset-grid { grid-template-columns: repeat(4, 1fr); }
}
@media (max-width: 1000px) {
  .preset-grid { grid-template-columns: repeat(3, 1fr); }
}
.preset-tile {
  display: flex;
  flex-direction: column;
  gap: 8px;
  padding: 8px;
  border: 1px solid var(--ui-border-subtle);
  border-radius: var(--ui-radius-md);
  background: var(--ui-surface-control);
  cursor: pointer;
  font-family: inherit;
  text-align: left;
  transition: border-color 0.16s cubic-bezier(0.4, 0, 0.2, 1), background-color 0.16s, transform 0.16s, box-shadow 0.16s;
}
.preset-tile:hover { border-color: var(--ui-border-strong); box-shadow: var(--ui-shadow-1); transform: translateY(-1px); }
.preset-tile:focus-visible { outline: 2px solid var(--ui-accent-ring); outline-offset: 2px; }
.preset-tile.active { border-color: var(--ui-accent); background: var(--ui-accent-soft); box-shadow: 0 0 0 1px var(--ui-accent); }
.preset-swatches {
  display: flex;
  height: 26px;
  border-radius: 8px;
  overflow: hidden;
  border: 1px solid color-mix(in srgb, var(--text-main) 10%, transparent);
}
.preset-swatches i { flex: 1; }
.preset-meta { display: flex; align-items: flex-start; gap: 6px; padding: 0 2px 1px; }
/* 预设名最多两行：德/法语长名称不撑破卡片，也不整体截成一行 */
.preset-name {
  flex: 1;
  min-width: 0;
  font-size: 0.76rem;
  font-weight: 500;
  line-height: 1.35;
  color: var(--text-main);
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
  overflow-wrap: break-word;
  hyphens: auto;
  -webkit-hyphens: auto;
}
.preset-check { flex: 0 0 auto; display: flex; align-items: center; margin-top: 1px; color: var(--ui-accent); }

/* ---------------------------------------------------------- 强调色 */
.accent-row { display: flex; flex-wrap: wrap; align-items: center; gap: 10px 14px; }
.accent-dots { display: flex; flex-wrap: wrap; gap: 7px; }
.accent-dot {
  position: relative;
  width: 26px;
  height: 26px;
  padding: 0;
  border-radius: 50%;
  border: 1px solid color-mix(in srgb, var(--text-main) 14%, transparent);
  display: flex;
  align-items: center;
  justify-content: center;
  cursor: pointer;
  transition: transform 0.15s cubic-bezier(0.4, 0, 0.2, 1), box-shadow 0.15s;
}
.accent-dot:hover { transform: scale(1.1); }
.accent-dot:focus-visible { outline: 2px solid var(--ui-accent-ring); outline-offset: 2px; }
.accent-dot.active { box-shadow: 0 0 0 2px var(--surface-panel), 0 0 0 3.5px var(--ui-accent); }
.accent-dot__check { display: flex; align-items: center; justify-content: center; }
/* 自定义强调色：整组包成一颗药丸，避免看起来像「第 13 个色点」 */
.accent-custom {
  margin-left: auto;
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 5px 8px 5px 12px;
  border: 1px solid var(--ui-border-subtle);
  border-radius: 999px;
  background: var(--ui-surface-control);
}
.accent-custom__label { font-size: 0.74rem; color: var(--text-sub); white-space: nowrap; padding-right: 2px; }
.accent-picker { position: relative; display: inline-flex; flex: 0 0 auto; }
.accent-picker__badge {
  position: absolute;
  right: -4px;
  bottom: -4px;
  width: 15px;
  height: 15px;
  border-radius: 50%;
  display: flex;
  align-items: center;
  justify-content: center;
  background: var(--text-main);
  color: var(--surface-panel);
  border: 1.5px solid var(--surface-panel);
  pointer-events: none;
}
.accent-color {
  width: 30px;
  height: 30px;
  padding: 0;
  border: none;
  border-radius: 50%;
  background: none;
  cursor: pointer;
}
.accent-color::-webkit-color-swatch-wrapper { padding: 0; }
.accent-color::-webkit-color-swatch { border: 1px solid var(--ui-border-strong); border-radius: 50%; }
.hex-input {
  width: 98px;
  height: 30px;
  padding: 0 10px;
  border: 1px solid var(--ui-border-subtle);
  border-radius: var(--ui-radius-sm);
  background: var(--ui-surface-control);
  color: var(--text-main);
  font-family: var(--font-mono);
  font-size: 0.78rem;
  text-align: center;
  outline: none;
  transition: border-color 0.16s, box-shadow 0.16s;
}
.hex-input:focus { border-color: var(--ui-accent); box-shadow: 0 0 0 3px var(--ui-accent-ring); }
.hex-input.invalid { border-color: var(--ui-danger); }
.follow-btn {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  height: 30px;
  padding: 0 12px;
  border: 1px solid var(--ui-border-subtle);
  border-radius: 999px;
  background: transparent;
  color: var(--text-sub);
  font-family: inherit;
  font-size: 0.76rem;
  cursor: pointer;
  transition: color 0.16s, border-color 0.16s, background-color 0.16s;
}
.follow-btn:hover { color: var(--text-main); border-color: var(--ui-border-strong); }
.follow-btn.active { color: var(--ui-accent); border-color: var(--ui-accent-ring); background: var(--ui-accent-soft); }
.follow-btn:focus-visible { outline: 2px solid var(--ui-accent-ring); outline-offset: 2px; }
.follow-btn__icon { display: flex; align-items: center; }
.accent-error { margin: 8px 0 0; font-size: 0.76rem; color: var(--ui-danger); }

/* ---------------------------------------------------------- 玻璃质感 */
.glass-body { border-top: 1px solid var(--ui-border-subtle); padding-top: 6px; }
.blur-grid { display: grid; grid-template-columns: 1fr 1fr; gap: 0 24px; }
.blur-cell { display: flex; flex-direction: column; gap: 7px; padding: 10px 0; }
/* 厚度是总控，占满整行单独一排，和下面两条细分滑条区分开 */
.blur-cell--wide { grid-column: 1 / -1; }
.blur-cell__head { display: flex; align-items: baseline; gap: 8px; }
.blur-cell__name { font-size: 0.84rem; font-weight: 500; color: var(--text-main); }
.blur-cell__value { margin-left: auto; font-family: var(--font-mono); font-size: 0.76rem; font-weight: 500; color: var(--text-sub); }
.blur-cell__desc { margin: 0; font-size: 0.78rem; line-height: 1.45; color: var(--text-sub); }

.blur-range {
  -webkit-appearance: none;
  appearance: none;
  width: 100%;
  height: 6px;
  border-radius: 3px;
  background: var(--ui-surface-hover);
  /* 深色下轨道与卡片同色，靠这道内侧描边保证「看得见、抓得住」 */
  box-shadow: inset 0 0 0 1px var(--glass-hairline);
  outline: none;
  cursor: pointer;
}
.blur-range:disabled { cursor: not-allowed; opacity: 0.45; }
.blur-range:disabled::-webkit-slider-thumb { background: var(--text-muted); transform: none; }
.blur-range:disabled::-moz-range-thumb { background: var(--text-muted); }
.blur-range:focus-visible { outline: 2px solid var(--ui-accent-ring); outline-offset: 3px; }
.blur-range::-webkit-slider-thumb {
  -webkit-appearance: none;
  appearance: none;
  width: 16px;
  height: 16px;
  border-radius: 50%;
  background: var(--ui-accent);
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.3);
  transition: transform 0.15s ease;
}
.blur-range::-webkit-slider-thumb:hover { transform: scale(1.15); }
.blur-range::-moz-range-thumb {
  width: 16px;
  height: 16px;
  border: none;
  border-radius: 50%;
  background: var(--ui-accent);
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.3);
}

.switch-row { display: flex; align-items: center; gap: 14px; padding: 12px 0 4px; }
.switch-row__text { flex: 1; min-width: 0; }
.switch-row__text h4 { margin: 0 0 3px; font-size: 0.88rem; font-weight: 500; color: var(--text-main); }
.switch-row__text p { margin: 0; font-size: 0.76rem; line-height: 1.45; color: var(--text-sub); }

/* ---------------------------------------------------------- 数据显示 */
.delay-demo { display: flex; align-items: center; gap: 6px; }
.delay-pill {
  font-family: var(--font-mono);
  font-size: 0.72rem;
  font-weight: 700;
  line-height: 1;
  padding: 4px 8px;
  border-radius: 999px;
  background: var(--ui-surface-hover);
  color: var(--text-main);
  transition: color 0.18s cubic-bezier(0.4, 0, 0.2, 1);
}
.delay-demo.is-color .delay-pill.fast { color: var(--delay-fast); }
.delay-demo.is-color .delay-pill.mid { color: var(--delay-mid); }
.delay-demo.is-color .delay-pill.slow { color: var(--delay-slow); }

/* ---------------------------------------------------------- 底部摘要 */

/* ---------------------------------------------------------- 开关（沿用设置页样式） */
.modern-switch { position: relative; display: inline-block; width: 44px; height: 24px; }
.modern-switch input { opacity: 0; width: 0; height: 0; }
.slider {
  position: absolute;
  cursor: pointer;
  top: 0;
  left: 0;
  right: 0;
  bottom: 0;
  background-color: var(--ui-surface-hover);
  transition: 0.3s;
  border-radius: 24px;
  box-shadow: inset 0 1px 3px rgba(0, 0, 0, 0.1);
}
.slider:before {
  position: absolute;
  content: "";
  height: 18px;
  width: 18px;
  left: 3px;
  bottom: 3px;
  background-color: white;
  transition: 0.3s;
  border-radius: 50%;
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.3);
}
input:disabled + .slider { opacity: 0.5; cursor: not-allowed; }
input:checked + .slider { background-color: var(--ui-accent-block); }
input:checked + .slider:before { transform: translateX(20px); background-color: var(--ui-accent-fg); }

/* 窄窗口：滑条与色板改为单列，避免挤压 */
@media (max-width: 780px) {
  .blur-grid { grid-template-columns: 1fr; gap: 0; }
  .accent-custom { margin-left: 0; }
}
</style>
