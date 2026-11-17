<template>
  <div class="settings-page ui-page-shell">
    <div class="sub-header ui-page-header">
      <button class="back-btn" :title="t('common.back')" @click="$emit('navigate', 'main')">
        <span class="back-icon-svg" v-html="ICONS.arrowLeft"></span>
      </button>
      <h3>{{ t('settings.diagnostics.title') }}</h3>
      <span v-if="envLabel" class="env-tag">{{ envLabel }}</span>
      <button class="action-btn" :disabled="loading" @click="load()">
        {{ loading ? t('settings.diagnostics.refreshing') : t('settings.diagnostics.refresh') }}
      </button>
    </div>

    <div class="ui-page-body">
      <!-- 结论：一张卡回答「现在能不能用、有没有要处理的东西」 -->
      <div class="glass-card setting-group ui-surface-group verdict-card">
        <span class="verdict-dot" :class="verdictTone"></span>
        <div class="info">
          <h4>{{ verdictTitle }}</h4>
          <p>{{ verdictHint }}</p>
        </div>
      </div>

      <div class="glass-card setting-group ui-surface-group">
        <h3>{{ t('settings.diagnostics.components') }}</h3>

        <div class="setting-item" id="set-diag-core">
          <div class="info">
            <h4>{{ t('settings.diagnostics.core') }}</h4>
            <p>{{ t('settings.diagnostics.coreDesc') }}</p>
            <p v-if="coreStatus.error" class="row-note is-bad">{{ coreStatus.error }}</p>
            <p v-else-if="autoRepairCore" class="row-note">{{ t('settings.diagnostics.autoRepair') }}</p>
          </div>
          <span class="status-pill" :class="coreStatus.tone">{{ coreStatus.label }}</span>
        </div>

        <div class="divider"></div>

        <div class="setting-item" id="set-diag-wintun">
          <div class="info">
            <h4>{{ t('settings.diagnostics.wintun') }}</h4>
            <p>{{ t('settings.diagnostics.wintunDesc') }}</p>
            <p v-if="wintunStatus.error" class="row-note is-bad">{{ wintunStatus.error }}</p>
            <p v-else-if="autoRepairWintun" class="row-note">{{ t('settings.diagnostics.autoRepair') }}</p>
          </div>
          <span class="status-pill" :class="wintunStatus.tone">{{ wintunStatus.label }}</span>
        </div>

        <div class="divider"></div>

        <div class="setting-item" id="set-diag-seed">
          <div class="info">
            <h4>{{ t('settings.diagnostics.seed') }}</h4>
            <p>{{ t('settings.diagnostics.seedDesc') }}</p>
            <p class="row-note" :class="{ 'is-bad': !seedAllReady }">
              {{ t('settings.diagnostics.seedManifest', { status: seedManifestLabel }) }}
            </p>
          </div>
          <span class="status-pill" :class="seedAllReady ? 'ok' : 'bad'">
            {{ t('settings.diagnostics.seedCount', { ready: seedReadyCount, total: SEED_TOTAL }) }}
          </span>
        </div>
      </div>

      <div class="glass-card setting-group ui-surface-group">
        <h3>{{ t('settings.diagnostics.paths') }}</h3>

        <div class="setting-item" id="set-diag-appDir">
          <div class="info">
            <h4>{{ t('settings.diagnostics.appDir') }}</h4>
            <p class="path-text" :title="dir?.appDir">{{ dir?.appDir || emptyValue }}</p>
          </div>
          <button class="action-btn" @click="copyPath(dir?.appDir)">{{ t('common.copy') }}</button>
        </div>

        <div class="divider"></div>

        <div class="setting-item" id="set-diag-dataDir">
          <div class="info">
            <h4>{{ t('settings.diagnostics.dataDir') }}</h4>
            <p class="path-text" :title="dir?.dataDir">{{ dir?.dataDir || emptyValue }}</p>
          </div>
          <button class="action-btn" @click="copyPath(dir?.dataDir)">{{ t('common.copy') }}</button>
        </div>

        <div class="divider"></div>

        <div class="setting-item" id="set-diag-runtimeDir">
          <div class="info">
            <h4>{{ t('settings.diagnostics.runtimeDir') }}</h4>
            <p class="path-text" :title="dir?.coreBinDir">{{ dir?.coreBinDir || emptyValue }}</p>
          </div>
          <button class="action-btn" @click="copyPath(dir?.coreBinDir)">{{ t('common.copy') }}</button>
        </div>

        <div class="divider"></div>

        <div class="setting-item" id="set-diag-seedDir">
          <div class="info">
            <h4>{{ t('settings.diagnostics.seedDir') }}</h4>
            <p class="path-text" :title="dir?.seedCoreBinDir">{{ dir?.seedCoreBinDir || emptyValue }}</p>
          </div>
          <button class="action-btn" @click="copyPath(dir?.seedCoreBinDir)">{{ t('common.copy') }}</button>
        </div>

        <div class="divider"></div>

        <div class="setting-item" id="set-diag-logDir">
          <div class="info">
            <h4>{{ t('settings.diagnostics.logDir') }}</h4>
            <p class="path-text" :title="diag?.logDir">{{ diag?.logDir || emptyValue }}</p>
          </div>
          <button class="action-btn" @click="copyPath(diag?.logDir)">{{ t('common.copy') }}</button>
        </div>

        <div class="divider"></div>

        <div class="setting-item" id="set-diag-legacyDir">
          <div class="info">
            <h4>{{ t('settings.diagnostics.legacyDir') }}</h4>
            <p class="path-text" :title="dir?.legacyDataDir">{{ dir?.legacyDataDir || emptyValue }}</p>
            <p class="row-note" :class="{ 'is-bad': dir?.legacyExists }">
              {{ dir?.legacyExists ? t('settings.diagnostics.legacyExists') : t('settings.diagnostics.legacyCleaned') }}
            </p>
          </div>
          <button class="action-btn" @click="copyPath(dir?.legacyDataDir)">{{ t('common.copy') }}</button>
        </div>
      </div>

      <div class="glass-card setting-group ui-surface-group">
        <div class="setting-item" id="set-diag-export">
          <div class="info">
            <h4>{{ t('settings.diagnostics.export') }}</h4>
            <p>{{ t('settings.diagnostics.exportDesc') }}</p>
          </div>
          <button class="action-btn accent-btn" :disabled="exporting" @click="handleExport">
            {{ exporting ? t('settings.diagnostics.exporting') : t('settings.diagnostics.export') }}
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue';
import { App } from '../api';
import { showAlert, showToast } from '../store';
import { ICONS } from '../utils/icons';
import { displayErrorMessage, toUserError } from '../utils/errors';
import { reportError } from '../utils/report';
import { t } from '../locales';

defineEmits<{ navigate: [view: 'main'] }>();

const SEED_TOTAL = 6;

const dir = ref<any>(null);
const diag = ref<any>(null);
const loading = ref(true);
const failed = ref(false);
const exporting = ref(false);
const emptyValue = '—';
let inflight = false;

const load = async () => {
  if (inflight) return;
  inflight = true;
  loading.value = true;
  try {
    const [dirInfo, diagnostic] = await Promise.all([App.GetDataDirInfo(), App.GetDiagnosticInfo()]);
    dir.value = dirInfo;
    diag.value = diagnostic;
    failed.value = false;
  } catch (error) {
    failed.value = true;
    reportError(error, 'diagnostics.load');
    const u = toUserError(error);
    void showAlert(displayErrorMessage(error), t('common.error'), true, u.detail);
  } finally {
    loading.value = false;
    inflight = false;
  }
};

const envLabel = computed(() => {
  const version = diag.value?.appVersion ? 'v' + diag.value.appVersion : '';
  const platform = [diag.value?.goos, diag.value?.goarch].filter(Boolean).join('/');
  return [version, platform].filter(Boolean).join(' · ');
});

// 缺失与损坏都读作「需要处理」，靠 danger 色的状态胶囊区分程度
const statusOf = (ready?: boolean, exists?: boolean, error?: string) => {
  if (ready) return { tone: 'ok', label: t('settings.diagnostics.ready'), error: '' };
  return {
    tone: 'bad',
    label: exists ? t('settings.diagnostics.corrupted') : t('settings.diagnostics.missing'),
    error: error || '',
  };
};

const coreStatus = computed(() => statusOf(dir.value?.coreReady, dir.value?.coreExists, dir.value?.coreError));
const wintunStatus = computed(() => statusOf(dir.value?.wintunReady, dir.value?.wintunExists, dir.value?.wintunError));
const autoRepairCore = computed(() => !!dir.value?.canAutoRepairCore);
const autoRepairWintun = computed(() => !!dir.value?.canAutoRepairWintun);

const seedReadyCount = computed(() => {
  const d = dir.value;
  if (!d) return 0;
  return [d.seedCoreReady, d.seedWintunReady, d.seedGeoipReady, d.seedGeositeReady, d.seedMmdbReady, d.seedAsnReady]
    .filter(Boolean).length;
});
const seedAllReady = computed(() => seedReadyCount.value === SEED_TOTAL);
const seedManifestLabel = computed(() =>
  dir.value?.seedManifestOK ? t('settings.diagnostics.seedManifestOk') : t('settings.diagnostics.seedManifestBad')
);

const issueCount = computed(() => {
  const d = dir.value;
  if (!d) return 0;
  return (d.coreReady ? 0 : 1) + (d.wintunReady ? 0 : 1) + (seedAllReady.value ? 0 : 1);
});

const verdictTone = computed(() => {
  if (!dir.value) return failed.value ? 'bad' : 'idle';
  return issueCount.value ? 'bad' : 'ok';
});
const verdictTitle = computed(() => {
  if (!dir.value) return failed.value ? t('settings.diagnostics.summaryFailed') : t('settings.diagnostics.refreshing');
  return issueCount.value
    ? t('settings.diagnostics.summaryIssues', { count: issueCount.value })
    : t('settings.diagnostics.summaryOk');
});
const verdictHint = computed(() => {
  if (!dir.value) return '';
  return issueCount.value ? t('settings.diagnostics.summaryIssueHint') : t('settings.diagnostics.summaryOkHint');
});

const copyPath = async (value?: string) => {
  const text = String(value || '').trim();
  if (!text) {
    showToast(t('common.empty'), 'info');
    return;
  }
  try {
    await navigator.clipboard.writeText(text);
    showToast(t('common.copied'), 'success');
  } catch {
    showToast(t('common.copy') + ' ✗', 'error');
  }
};

const handleExport = async () => {
  if (exporting.value) return;
  exporting.value = true;
  try {
    const path = await App.ExportDiagnostics();
    // 用户取消保存对话框时后端返回空路径，静默返回
    if (!path) return;
    showToast(t('settings.diagnostics.exportSuccess'), 'success');
  } catch (error) {
    reportError(error, 'diagnostics.export');
    const u = toUserError(error);
    void showAlert(displayErrorMessage(error), t('common.error'), true, u.detail);
  } finally {
    exporting.value = false;
  }
};

onMounted(() => { void load(); });
</script>

<style scoped>
.settings-page { flex: 1 1 auto; min-width: 0; }
.setting-group { padding: 20px 24px; margin-bottom: 12px; }

h3 { margin: 0 0 8px 0; color: var(--text-main); font-size: 1.25rem; padding-bottom: 4px; }
h4 { margin: 0 0 6px 0; color: var(--text-main); font-size: 1rem; }
p { margin: 0; font-size: 0.85rem; color: var(--text-sub); max-width: 100%; line-height: 1.5; }

.info { flex: 1; padding-right: 24px; min-width: 0; }
.setting-item { display: flex; justify-content: space-between; align-items: center; padding: 14px 0; }
.divider { height: 1px; background: var(--ui-border-subtle); opacity: 0.5; margin: 0; }

.sub-header { display: flex; align-items: center; gap: 16px; margin-bottom: 12px; padding: 0 0 12px 0; background: transparent; }
.sub-header h3 { margin: 0; border: none; padding: 0; flex: 1 1 auto; min-width: 0; }
.back-btn { background: var(--surface); border: none; color: var(--text-main); width: 36px; height: 36px; border-radius: 50%; display: flex; align-items: center; justify-content: center; cursor: pointer; transition: 0.2s; flex-shrink: 0; }
.back-btn:hover { background: var(--ui-surface-hover); }
.back-icon-svg :deep(svg) { width: 18px; height: 18px; }

/* 环境标签：版本与平台，弱化成一行注脚，不抢标题 */
.env-tag {
  flex-shrink: 0;
  font-family: var(--font-mono);
  font-size: 0.75rem;
  color: var(--text-muted);
  padding: 4px 10px;
  border-radius: 999px;
  background: var(--surface);
  white-space: nowrap;
}

/* 结论卡：一个状态点 + 一句话，承担整页的主判断。加一圈细边把它从「又一组条目」里提出来 */
.verdict-card {
  display: flex;
  align-items: flex-start;
  gap: 14px;
  padding: 16px 18px;
  border: 1px solid var(--ui-border-subtle);
  border-radius: 12px;
}
.verdict-card .info { padding-right: 0; }
.verdict-dot {
  flex-shrink: 0;
  width: 9px; height: 9px;
  margin-top: 7px;
  border-radius: 50%;
  background: currentColor;
  box-shadow: 0 0 0 4px color-mix(in srgb, currentColor 14%, transparent), 0 0 10px currentColor;
}
.verdict-dot.ok { color: var(--text-main); }
.verdict-dot.bad { color: var(--danger); }
.verdict-dot.idle { color: var(--text-muted); }

/* 状态胶囊：控件凹槽 + 呼吸点，沿用订阅页「点 + 文字」的判断语言 */
.status-pill {
  flex-shrink: 0;
  display: inline-flex;
  align-items: center;
  gap: 7px;
  padding: 5px 11px;
  border-radius: 999px;
  font-size: 0.75rem;
  font-weight: 700;
  white-space: nowrap;
  /* 用 --surface 而不是 --ui-surface-card：后者在 :root 处就把 var(--surface) 解析成了
     日间灰值，深色模式下会变成一块亮底（tokens.css 的既有行为） */
  background: var(--surface);
  font-variant-numeric: tabular-nums;
}
.status-pill::before { content: ''; width: 7px; height: 7px; border-radius: 50%; background: currentColor; box-shadow: 0 0 6px currentColor; }
.status-pill.ok { color: var(--text-main); }
.status-pill.bad { color: var(--danger); }

.path-text {
  font-family: var(--font-mono);
  font-size: 0.8rem;
  color: var(--text-sub);
  word-break: break-all;
}
.row-note { margin-top: 4px; font-size: 0.78rem; color: var(--text-muted); }
.row-note.is-bad { color: var(--danger); }

@media (max-width: 720px) {
  .env-tag { display: none; }
}
</style>
