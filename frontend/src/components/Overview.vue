<template>
  <div class="overview-layout">
    <section class="hero-panel card-panel">
      <div class="status-core">
        <div class="restart-trigger" :class="{ 'is-loading': isRestarting }" @click="handleRestartCore" :title="t('overview.restartCoreTitle')">
          <div class="orb-visual" v-show="!isRestarting">
            <div class="orb" :class="{ 'active': consoleServiceOn }"></div>
            <div class="orb-glow" v-if="consoleServiceOn"></div>
          </div>
          <svg class="refresh-icon scanner-svg" :class="{ 'spin': isRestarting }" viewBox="0 0 24 24">
            <circle class="scanner-track" cx="12" cy="12" r="10"></circle>
            <circle class="scanner-bar" cx="12" cy="12" r="10"></circle>
          </svg>
        </div>
        <div class="status-meta">
          <span class="micro-title">{{ t('overview.serviceStatus') }}</span>
          <h2 class="status-heading">{{ consoleServiceTitle }}</h2>
          <span class="version-tag">{{ t('overview.coreVersion', { version: globalState.version || 'Core' }) }}</span>
        </div>
      </div>
      <div class="active-config-display">
        <span class="micro-title">{{ t('overview.activeConfig') }}</span>
        <div class="config-name truncate" :title="globalState.activeConfigName">
          {{ globalState.activeConfigName || t('overview.unselected') }}
        </div>
      </div>
    </section>

    <section class="switch-row">
      <div class="action-card" :class="{ 'on': sysProxyCardOn }" @click="toggleSysProxy">
        <div class="card-content">
          <div class="icon-ring" v-html="ICONS.sysProxy"></div>
          <div class="text-group">
            <span class="card-title">{{ globalState.portProxyMode ? t('overview.portProxy') : t('overview.sysProxy') }}</span>
            <span class="card-hint">{{ sysProxyLabel }}</span>
          </div>
        </div>
        <div class="status-node"></div>
      </div>

      <div class="action-card" :class="{ 'on': tunCardOn }" @click="toggleTun">
        <div class="card-content">
          <div class="icon-ring" v-html="ICONS.tun"></div>
          <div class="text-group">
            <span class="card-title">{{ t('overview.tunCard') }}</span>
            <span class="card-hint">{{ tunLabel }}</span>
          </div>
        </div>
        <div class="status-node"></div>
      </div>
    </section>

    <section class="mode-section rules-card">
      <div class="rules-head">
        <h3 class="section-heading">{{ t('overview.outboundRules') }}</h3>
      </div>
      <div class="segmented-control">
        <div v-for="m in modes" :key="m.val" class="seg-item" :class="{ active: globalState.mode === m.val }" @click="handleModeChange(m.val)">
          {{ m.label }}
        </div>
        <div class="seg-slider" :style="sliderStyle"></div>
      </div>
    </section>

    <TrafficCard :traffic="traffic" />
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch, onUnmounted } from 'vue';
import * as API from '../../wailsjs/go/main/App';
import { globalState, showAlert, showConfirm, updateStateFromBackend, settleSystemProxyIntent, settleTunIntent, setSystemProxyIntent, setTunIntent } from '../store';
import { ICONS } from '../utils/icons';
import { t } from '../locales';
import TrafficCard from './TrafficCard.vue';

defineProps<{
  traffic: {
    up: string;
    down: string;
    upRaw?: number;
    downRaw?: number;
    uploadTotal?: string;
    downloadTotal?: string;
    uploadTotalRaw?: number;
    downloadTotalRaw?: number;
  }
}>();

const modes = computed(() => [
  { label: t('overview.modeRule'), val: 'rule' },
  { label: t('overview.modeGlobal'), val: 'global' },
  { label: t('overview.modeDirect'), val: 'direct' }
]);

const sliderStyle = computed(() => ({
  left: `${(modes.value.findIndex(m => m.val === globalState.mode) * 100) / 3}%`
}));

const isRestarting = ref(false);

const desiredActive = computed(() => globalState.systemProxy || globalState.tun || globalState.desiredPortProxy);
const actualActive  = computed(() => globalState.actualSystemProxy || globalState.actualTun);

// Raw computed — never used in the template directly.
const consoleServiceOnRaw = computed(() => desiredActive.value || actualActive.value);

// Stable refs with debounce so transient intermediate backend frames
// (produced by watchdog / restart cycles) don't cause a single-frame flicker.
// "on → off" transitions are deferred 200 ms; "off → on" are instant.
const stableServiceOn = ref(consoleServiceOnRaw.value);
let orbOffTimer: ReturnType<typeof setTimeout> | null = null;
watch(consoleServiceOnRaw, (val) => {
  if (val) {
    if (orbOffTimer !== null) { clearTimeout(orbOffTimer); orbOffTimer = null; }
    stableServiceOn.value = true;
  } else {
    if (orbOffTimer !== null) return;
    orbOffTimer = setTimeout(() => { orbOffTimer = null; stableServiceOn.value = false; }, 200);
  }
});
const consoleServiceOn = stableServiceOn;

const consoleServiceTitleRaw = computed(() => {
  if (isRestarting.value) return t('overview.statusRestarting');

  const wantTun   = globalState.tun;
  const wantProxy = globalState.systemProxy;
  const hasTun    = globalState.actualTun;
  const hasProxy  = globalState.actualSystemProxy;

  if (wantTun) {
    if (hasTun) return t('overview.statusIntercepting');
    // Guard: if nothing is actually active and the core is not running (and
    // no API call is in flight), the desired state is almost certainly stale
    // from a shutdown. Show "停止中..." rather than the misleading "启动中...".
    // In all other cases (startup-restore, config restart, watchdog reconcile
    // with a momentary actualTun=false flash) fall through to "启动中..." so
    // we don't produce a false "停止中..." during legitimate TUN start-up.
    if (!hasProxy && !globalState.isRunning && !globalState.tunPending) return t('overview.statusStopping');
    return t('overview.statusStarting');
  }

  if (wantProxy) {
    if (hasProxy) return t('overview.statusProxying');
    if (!globalState.isRunning && !globalState.systemProxyPending) return t('overview.statusStopping');
    return t('overview.statusStarting');
  }

  // 仅端口代理模式：desiredPortProxy=true 时显示对应状态
  if (globalState.portProxyMode && globalState.desiredPortProxy) {
    if (globalState.isRunning) return t('overview.statusPortProxying');
    return t('overview.statusStarting');
  }

  if (hasTun || hasProxy) return t('overview.statusStopping');
  return t('overview.statusStopped');
});

// Stable title: "停止中..." and "启动中..." (transient transition labels) are
// deferred 200 ms so a watchdog-induced momentary flip doesn't reach the DOM.
// Stable target states ('接管中', '代理中', '服务停止') are applied instantly.
const stableTitle = ref(consoleServiceTitleRaw.value);
let titleTimer: ReturnType<typeof setTimeout> | null = null;
watch(consoleServiceTitleRaw, (newTitle) => {
  if (titleTimer !== null) { clearTimeout(titleTimer); titleTimer = null; }
  stableTitle.value = newTitle;
});
const consoleServiceTitle = stableTitle;

onUnmounted(() => {
  if (orbOffTimer !== null) clearTimeout(orbOffTimer);
  if (titleTimer !== null) clearTimeout(titleTimer);
});

const handleRestartCore = async () => {
  if (isRestarting.value) return;
  const ok = await showConfirm(t('overview.restartConfirmMsg'), t('overview.restartConfirmTitle'));
  if (!ok) return;
  isRestarting.value = true;
  try {
    await (API as any).RestartCore();
    isRestarting.value = false;
    await showAlert(t('overview.restartSuccess'), t('common.success'));
  } catch (e) {
    isRestarting.value = false;
    await showAlert(t('overview.restartError', { error: String(e) }), t('common.error'));
  }
};

// ==========================================
// 切换状态管理（三层模型：intent / actual / pending）
// ==========================================
const sysProxyCardOn = computed(() => globalState.portProxyMode ? globalState.desiredPortProxy : globalState.systemProxy);
const tunCardOn = computed(() => globalState.tun);

const sysProxyLabel = computed(() => {
  if (globalState.portProxyMode) {
    return sysProxyCardOn.value ? t('overview.portProxyHintOn') : t('overview.portProxyHintOff');
  }
  return sysProxyCardOn.value
    ? t('overview.sysProxyHintOn')
    : t('overview.sysProxyHintOff');
});

const tunLabel = computed(() => {
  return tunCardOn.value
    ? t('overview.tunHintOn')
    : t('overview.tunHintOff');
});

const toggleSysProxy = async () => {
  if (globalState.systemProxyPending) return;

  // 端口代理模式用 sysProxyCardOn（反映 desiredPortProxy），
  // 普通模式用 globalState.systemProxy，避免端口代理模式下 target 永远为 true
  const target = !sysProxyCardOn.value;

  setSystemProxyIntent(target);
  globalState.systemProxyPending = true;

  try {
    await API.ToggleSystemProxy(target);
    const latest = await (API as any).GetAppState().catch(() => null);
    if (latest) updateStateFromBackend(latest);
  } catch (err: any) {
    const msg = String(err?.message || err || '');
    if (msg.includes('no active config selected') || msg.includes('ErrNoActiveConfig')) {
      showAlert(`${t('overview.errNoConfigTitle')}\n\n${t('overview.errNoConfigMsg')}`, t('common.notice'));
    } else {
      showAlert(t('overview.errToggleProxy', { error: msg }), t('common.error'), true);
    }
  } finally {
    globalState.systemProxyPending = false;
    if (target === false) {
      if (globalState.portProxyMode) {
        globalState.desiredPortProxy = false;
      } else {
        globalState.desiredSystemProxy = false;
      }
    }
    settleSystemProxyIntent();
  }
};

const toggleTun = async () => {
  if (globalState.tunPending) return;

  const target = !globalState.tun;

  setTunIntent(target);
  globalState.tunPending = true;

  try {
    await API.ToggleTunMode(target);
    const latest = await (API as any).GetAppState().catch(() => null);
    if (latest) updateStateFromBackend(latest);
  } catch (err: any) {
    const msg = String(err?.message || err || '');

    if (msg.includes('helper_install_required') || msg.includes('helper_repair_required')) {
      const confirmed = await showConfirm(
        t('overview.tunHelperConfirm'),
        t('overview.tunHelperTitle')
      );

      if (confirmed) {
        try {
          await (API as any).InstallHelperService();
          await API.ToggleTunMode(target);
          const latest = await (API as any).GetAppState().catch(() => null);
          if (latest) updateStateFromBackend(latest);
          return;
        } catch (e: any) {
          showAlert(t('overview.errInstallHelper', { error: String(e?.message || e) }), t('common.error'), true);
          return;
        }
      }

      return;
    }

    if (msg.includes('no active config selected') || msg.includes('ErrNoActiveConfig')) {
      showAlert(`${t('overview.errNoConfigTitle')}\n\n${t('overview.errNoConfigMsg')}`, t('common.notice'));
    } else if (msg.includes('wintun_missing') || msg.includes('Wintun')) {
      showAlert(t('overview.errWintunMissing'), t('common.error'), true);
    } else {
      showAlert(t('overview.errToggleTun', { error: msg }), t('common.error'), true);
    }
  } finally {
    globalState.tunPending = false;
    if (target === false) globalState.desiredTun = false;
    settleTunIntent();
  }
};

// ==========================================
// 模式切换
// ==========================================
let modeWorkerActive = false;
let pendingModeTarget: string | null = null;
let previousMode = globalState.mode;

// Keep previousMode in sync with external mode changes (e.g., from backend events)
watch(() => globalState.mode, (newVal, oldVal) => {
  if (!modeWorkerActive) {
    previousMode = oldVal;
  }
});

const handleModeChange = (val: string) => {
  if (globalState.mode === val) return;
  if (modeWorkerActive) { pendingModeTarget = val; return; }
  previousMode = globalState.mode;
  runModeWorker(val);
};

const runModeWorker = async (targetMode: string) => {
  modeWorkerActive = true;
  try {
    await API.UpdateClashMode(targetMode);
    const latest = await (API as any).GetAppState();
    updateStateFromBackend(latest);
  } catch (err) {
    globalState.mode = previousMode;
    await showAlert(t('overview.errSwitchMode', { error: String(err) }), t('common.error'));
  } finally {
    if (pendingModeTarget !== null && pendingModeTarget !== targetMode) {
      const next = pendingModeTarget;
      pendingModeTarget = null;
      await runModeWorker(next);
    } else {
      pendingModeTarget = null;
      modeWorkerActive = false;
    }
  }
};
</script>

<style scoped>
.overview-layout { display: flex; flex-direction: column; gap: 24px; min-height: 100%; overflow: visible; }
.hero-panel { padding: 28px 24px; background: var(--surface); border-radius: 20px; display: grid; grid-template-columns: 1fr 1fr; gap: 16px; align-items: center; box-shadow: 0 4px 20px rgba(0,0,0,0.03); }
.status-core { position: relative; display: flex; align-items: center; justify-content: center; width: 100%; }
.restart-trigger { position: absolute; left: 10px; width: 56px; height: 56px; display: flex; align-items: center; justify-content: center; cursor: pointer; border-radius: 50%; transition: 0.3s; }
.restart-trigger:hover { background: var(--surface-hover); }
.restart-trigger.is-loading { cursor: not-allowed; pointer-events: none; }
.orb-visual { position: relative; width: 10px; height: 10px; z-index: 2; transition: all 0.3s cubic-bezier(0.4,0,0.2,1); }
.orb { width: 100%; height: 100%; border-radius: 50%; background: var(--text-muted); transition: 0.4s; }
.orb.active { background: var(--text-main); box-shadow: 0 0 10px var(--text-main); }
.orb-glow { position: absolute; top: 0; left: 0; width: 100%; height: 100%; border-radius: 50%; background: var(--text-main); animation: pulse 2s infinite; }
.refresh-icon { position: absolute; top: 50%; left: 50%; transform: translate(-50%,-50%) rotate(-90deg); width: 44px; height: 44px; opacity: 0.6; transition: all 0.4s cubic-bezier(0.4,0,0.2,1); }
.scanner-track { fill: none; stroke: var(--surface-hover); stroke-width: 2.5; }
.scanner-bar { fill: none; stroke: var(--text-muted); stroke-width: 2.5; transition: all 0.4s cubic-bezier(0.4,0,0.2,1); stroke-dasharray: 62.8; stroke-dashoffset: 62.8; }
.restart-trigger:hover:not(.is-loading) .refresh-icon { opacity: 1; transform: translate(-50%,-50%) rotate(90deg); }
.restart-trigger:hover:not(.is-loading) .scanner-bar { stroke: var(--text-main); stroke-dashoffset: 20; }
.refresh-icon.spin { opacity: 1; animation: spin-centered 1.2s linear infinite; }
.refresh-icon.spin .scanner-bar { stroke: var(--accent); stroke-dasharray: 62.8; animation: scan-dash 1.2s infinite ease-in-out; }
@keyframes spin-centered { 0%{transform:translate(-50%,-50%) rotate(-90deg)} 100%{transform:translate(-50%,-50%) rotate(270deg)} }
@keyframes scan-dash { 0%{stroke-dashoffset:60} 50%{stroke-dashoffset:15} 100%{stroke-dashoffset:60} }
.status-meta { display: flex; flex-direction: column; align-items: center; text-align: center; }
.micro-title { font-size: 0.85rem; font-weight: 700; color: var(--text-sub); letter-spacing: 0.02em; margin-bottom: 8px; }
.status-heading { font-size: 1.6rem; font-weight: 600; margin: 0; line-height: 1.2; color: var(--text-main); }
.version-tag { font-family: var(--font-mono); font-size: 0.75rem; color: var(--text-sub); opacity: 0.8; margin-top: 8px; }
.truncate { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.active-config-display { text-align: center; min-width: 0; display: flex; flex-direction: column; align-items: center; }
.active-config-display .config-name { font-size: 1.1rem; font-weight: 700; color: var(--text-main); }
.switch-row { display: grid; grid-template-columns: 1fr 1fr; gap: 16px; }
.action-card { padding: 20px 24px; background: var(--surface); border: none; border-radius: 16px; display: flex; justify-content: space-between; align-items: center; cursor: pointer; transition: all 0.2s cubic-bezier(0.4,0,0.2,1); }
.action-card.on { background: var(--accent); }
.icon-ring { width: 40px; height: 40px; border-radius: 12px; background: var(--surface-hover); display: flex; align-items: center; justify-content: center; color: var(--text-sub); transition: 0.3s; }
.icon-ring :deep(svg) { width: 22px; height: 22px; }
.on .icon-ring { background: rgba(128,128,128,0.25) !important; color: var(--accent-fg); }
.card-title { display: block; font-size: 1rem; font-weight: 600; margin-bottom: 2px; color: var(--text-main); }
.card-hint { font-size: 0.75rem; color: var(--text-sub); }
.on .card-title { color: var(--accent-fg); }
.on .card-hint { color: var(--accent-fg); opacity: 0.7; }
.status-node { width: 6px; height: 6px; border-radius: 50%; background: var(--text-muted); }
.on .status-node { background: var(--accent-fg); box-shadow: 0 0 8px var(--accent-fg); }
.rules-card { padding: 24px 28px; background: var(--surface); border-radius: 20px; box-shadow: 0 4px 20px rgba(0,0,0,0.03); display: flex; flex-direction: column; gap: 20px; }
.rules-head { display: flex; align-items: center; justify-content: space-between; }
.section-heading { margin: 0; font-size: 1rem; font-weight: 600; color: var(--text-main); }
.segmented-control { background: var(--surface-panel); padding: 4px; border-radius: 12px; display: flex; position: relative; border: none; overflow: hidden; }
.seg-item { flex: 1; text-align: center; padding: 14px 0; font-size: 0.9rem; font-weight: 600; color: var(--text-main); cursor: pointer; z-index: 1; transition: 0.3s; }
.seg-item.active { color: var(--accent-fg); }
.seg-slider { position: absolute; top: 10px; bottom: 10px; width: calc(33.33% - 20px); margin-left: 10px; background: var(--text-main); border-radius: 8px; z-index: 0; box-shadow: 0 2px 10px rgba(0,0,0,0.1); transition: left 0.3s cubic-bezier(0.4,0,0.2,1); }
</style>
