<template>
  <div class="overview-layout ui-page-shell">
    <!-- 唯一滚动区：以下所有面板都在 body 内滚动，页面头由外层 App 提供 -->
    <div class="overview-body ui-page-body">
    <section class="hero-panel ui-surface-card">
      <div class="status-core">
        <div class="restart-trigger ui-hover-surface" :class="{ 'is-loading': isRestarting }" @click="handleRestartCore" :title="t('overview.restartCoreTitle')">
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
          <!-- 启动超时兜底：与其让"启动中..."无限转，不如明确说清"还没完成，可点击重试" -->
          <button v-if="startStuck" type="button" class="start-stuck-hint" @click="retryPendingStart">
            {{ t('overview.startStuckHint') }}
          </button>
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
      <div class="action-card ui-surface-card" :class="{ 'on': sysProxyCardOn }" @click="toggleSysProxy">
        <div class="card-content">
          <div class="icon-ring" v-html="ICONS.sysProxy"></div>
          <div class="text-group">
            <span class="card-title">{{ globalState.portProxyMode ? t('overview.portProxy') : t('overview.sysProxy') }}</span>
            <span class="card-hint">{{ sysProxyLabel }}</span>
          </div>
        </div>
        <div class="status-node"></div>
      </div>

      <div class="action-card ui-surface-card" :class="{ 'on': tunCardOn }" @click="toggleTun">
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

    <section class="mode-section rules-card ui-surface-card">
      <div class="rules-head">
        <h3 class="section-heading">{{ t('overview.outboundRules') }}</h3>
      </div>
      <div class="segmented-control ui-surface-control">
        <div v-for="m in modes" :key="m.val" class="seg-item" :class="{ active: globalState.mode === m.val }" @click="handleModeChange(m.val)">
          {{ m.label }}
        </div>
        <div v-show="modeIndex >= 0" class="seg-slider" :style="sliderStyle"></div>
      </div>
    </section>

    <TrafficCard :traffic="traffic" />
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch, onUnmounted } from 'vue';
import { App } from '../api';
import { globalState, showAlert, showToast, showConfirm, updateStateFromBackend, settleSystemProxyIntent, setSystemProxyIntent } from '../store';
import { ICONS } from '../utils/icons';
import { classifyError, displayErrorMessage, toUserError } from '../utils/errors';
import { t } from '../locales';
import { useTunToggle } from '../composables/useTunToggle';
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

// H-16 / C-19：findIndex 未命中返回 -1，会让滑块 left 变成 -33.33%（滑出控件左侧），
// 且三个 seg-item 都不高亮 —— 用户看不到任何选中态。这里单独暴露下标：
// 命中时按真实下标定位，未知 mode 时兜底 0 并在模板里隐藏滑块，避免画出「选中第一项」的错觉。
const modeIndex = computed(() => modes.value.findIndex(m => m.val === globalState.mode));

const sliderStyle = computed(() => ({
  left: `${(Math.max(modeIndex.value, 0) * 100) / modes.value.length}%`
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
// 只有瞬时态（启动中/停止中/重启中）才延迟 200ms 上屏，终态一律立即生效：
// 这样 watchdog 抖动造成的 actualTun 瞬时 false 不会闪进 DOM，也不会拖慢「已接管」的反馈。
const TITLE_SETTLE_MS = 200;
const isTransientTitle = (label: string) =>
  label === t('overview.statusStarting') ||
  label === t('overview.statusStopping') ||
  label === t('overview.statusRestarting');
watch(consoleServiceTitleRaw, (newTitle) => {
  if (titleTimer !== null) { clearTimeout(titleTimer); titleTimer = null; }
  if (!isTransientTitle(newTitle)) {
    stableTitle.value = newTitle;
    return;
  }
  titleTimer = setTimeout(() => {
    titleTimer = null;
    stableTitle.value = newTitle;
  }, TITLE_SETTLE_MS);
});
const consoleServiceTitle = stableTitle;

/* 启动态超时降级（2026-11-18 实测事故的直接修复）。
   事故现场：点开 TUN 后内核 20s 内没建出网卡，探测又误认了一个别的 Wintun 适配器，
   watchdog 每 5s 空转，直到 15s 静默阶梯才重启内核 —— 全程 46 秒界面一直是"启动中..."，
   既没有原因也没有操作入口，用户只能"取消再点一次"绕过。
   这里给启动态加一道人肉可见的兜底：超过 START_TIMEOUT_MS 仍未就绪，就明确提示
   "启动尚未完成 + 点击重试"，而不是让"启动中"永远转下去。
   阈值取后端同步预算（tunAdapterReadyTimeout=20s）+ 内核启动开销。 */
const START_TIMEOUT_MS = 25_000;

const startPending = computed(() => {
  if (isRestarting.value) return false;
  const wantTun = globalState.tun;
  const wantProxy = globalState.systemProxy;
  const wantPort = globalState.portProxyMode && globalState.desiredPortProxy;
  if (wantTun) return !globalState.actualTun;
  if (wantProxy) return !globalState.actualSystemProxy;
  if (wantPort) return !globalState.isRunning;
  return false;
});

const startStuck = ref(false);
let startStuckTimer: ReturnType<typeof setTimeout> | null = null;
const clearStartStuckTimer = () => {
  if (startStuckTimer !== null) {
    clearTimeout(startStuckTimer);
    startStuckTimer = null;
  }
};

watch(startPending, (pending) => {
  if (!pending) {
    clearStartStuckTimer();
    startStuck.value = false;
    return;
  }
  if (startStuckTimer !== null || startStuck.value) return;
  startStuckTimer = setTimeout(() => {
    startStuckTimer = null;
    // 到点仍处于启动中才标记：避免刚好就绪时闪一下"超时"
    if (startPending.value) startStuck.value = true;
  }, START_TIMEOUT_MS);
}, { immediate: true });

onUnmounted(() => {
  clearStartStuckTimer();
  if (orbOffTimer !== null) clearTimeout(orbOffTimer);
  if (titleTimer !== null) clearTimeout(titleTimer);
});

// H-03：防重入闸门必须早于 showConfirm 的 await —— 模态是队列，连点两次会排两个确认框，
// 用户各点一次「确定」后 RestartCore 会真执行两次（第一次刚起来就被第二次打断，连接全断）。
// 闸门用独立标记（而不是 isRestarting）：确认框还开着时不该显示「内核重启中」。
let restartInFlight = false;

const handleRestartCore = async () => {
  if (restartInFlight) return;
  restartInFlight = true;
  try {
    const ok = await showConfirm(t('overview.restartConfirmMsg'), t('overview.restartConfirmTitle'));
    if (!ok) return;
    isRestarting.value = true;
    try {
      await App.RestartCore();
      showToast(t('overview.restartSuccess'), 'success');
    } catch (e) {
      const u = toUserError(e);
      await showAlert(u.message, t('common.error'), true, u.detail);
    }
  } finally {
    restartInFlight = false;
    isRestarting.value = false;
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
    await App.ToggleSystemProxy(target);
    const latest = await App.GetAppState().catch(() => null);
    if (latest) updateStateFromBackend(latest);
  } catch (err: any) {
    // 语义判定走 classifyError，不再用文案 includes 猜
    const cls = classifyError(err);
    // H-14：失败后不再由前端猜期望值，先对一次后端真值 —— 后端硬失败会用 CAS 回滚 desired 并广播，
    // 瞬时失败则保留新意图转后台重试；两种语义只有后端知道，前端照抄即可。
    const latest = await App.GetAppState().catch(() => null);
    if (latest) updateStateFromBackend(latest);
    if (cls.kind === 'no_active_config') {
      showAlert(`${t('overview.errNoConfigTitle')}\n\n${t('overview.errNoConfigMsg')}`, t('common.notice'));
    } else {
      // 文案走 errors.ts 的人话映射；用现成的 errToggleProxy 句位补上「系统代理切换失败」上下文
      const u = toUserError(err);
      showAlert(t('overview.errToggleProxy', { error: displayErrorMessage(err) }), t('common.error'), true, u.detail);
    }
  } finally {
    globalState.systemProxyPending = false;
    // H-14：原来这里会把 desiredSystemProxy / desiredPortProxy 直写成 false，抵消后端刚做的回滚
    // （卡片显示「已关闭」而系统代理实际还开着）。只清本地乐观覆盖层，期望值以后端为准。
    settleSystemProxyIntent();
  }
};

// TUN 开关：统一实现（与「设置→TUN」共用），见 composables/useTunToggle.ts
// pending 管理、classifyError 分支、helper/驱动引导、错误展示全部收敛在那里，
// 这里只保留页面自己的视觉（卡片亮灭由 globalState.tun 驱动）。
const { toggle: toggleTunState } = useTunToggle();

const toggleTun = async () => {
  await toggleTunState();
};

/* 超时提示上的"点击重试"：把当前意图再下发一次，让后端重新收敛。
   刻意不复用 toggle*（那会先反转意图）；这里的目标是"幂等重放"，与用户手动关再开等价，
   但不闪中间状态。失败只记日志 —— 提示条本身已经说明了状态。 */
const retryPendingStart = async () => {
  if (startPending.value === false) return;
  if (globalState.tun) {
    await toggleTunState(true);
    return;
  }
  if (globalState.systemProxy || (globalState.portProxyMode && globalState.desiredPortProxy)) {
    if (globalState.systemProxyPending) return;
    const target = globalState.portProxyMode ? globalState.desiredPortProxy : globalState.systemProxy;
    setSystemProxyIntent(target);
    globalState.systemProxyPending = true;
    try {
      await App.ToggleSystemProxy(target);
      const latest = await App.GetAppState().catch(() => null);
      if (latest) updateStateFromBackend(latest);
    } catch {
      /* 静默：提示条仍然在，用户可以再点一次 */
    } finally {
      globalState.systemProxyPending = false;
      settleSystemProxyIntent();
    }
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
    try {
      await App.UpdateClashMode(targetMode);
    } catch (err) {
      // H-15：后端的失败分两类 —— A 类没落盘（SyncState 广播的还是旧 mode，回滚正确）；
      // B 类已经写进 behavior、只是内核热切失败（广播的是新 mode），无条件回滚反而把 UI 拉离真相。
      // 所以先拉一次真值以后端为准，只有拿不到快照时才回滚到 previousMode。
      const latest = await App.GetAppState().catch(() => null);
      if (latest) updateStateFromBackend(latest);
      else globalState.mode = previousMode;
      // F-A4-13：人话进 i18n 槽位、原始文本进 detail，标题不再被长错误撑开
      const u = toUserError(err);
      await showAlert(t('overview.errSwitchMode', { error: displayErrorMessage(err) }), t('common.error'), true, u.detail);
      return;
    }
    // H-15：成功后的快照失败不再算「切换失败」—— 原来 GetAppState 抛错会落进同一个 catch，
    // 把一次成功的切换回滚掉并弹一条错误框。
    const latest = await App.GetAppState().catch(() => null);
    if (latest) updateStateFromBackend(latest);
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
.overview-layout {
  /* PageShell：外层视图容器已提供确定高度，这里只负责撑满 */
  flex: 1 1 auto;
  min-width: 0;
}
.overview-body {
  display: flex;
  flex-direction: column;
  gap: 24px;
}
.hero-panel { padding: 28px 24px; display: grid; grid-template-columns: 1fr 1fr; gap: 16px; align-items: center; }
/* 状态块：按钮回到正常文档流，用两列网格与文字并排。
   之前按钮是绝对定位、文字居中，状态文字一长（"内核重启中…"）就会滑到圆环下面压字。 */
/* 状态块做成"图标 + 文字"的居中整体：
   1) 按钮回到正常文档流，文字不会滑到圆环下面（重叠的根因）；
   2) 整组在左半格里水平居中，与右侧「活动配置」的居中基线一致；
   3) 文字块给一个最小宽度，状态文案变化时图标不会左右跳动。 */
.status-core {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 18px;
  width: 100%;
}
.restart-trigger { position: relative; width: 56px; height: 56px; display: flex; align-items: center; justify-content: center; cursor: pointer; border-radius: 50%; transition: 0.3s; }
.restart-trigger.is-loading { cursor: not-allowed; pointer-events: none; }
.orb-visual { position: relative; width: 10px; height: 10px; z-index: 2; transition: all 0.3s cubic-bezier(0.4,0,0.2,1); }
.orb { width: 100%; height: 100%; border-radius: 50%; background: var(--text-muted); transition: 0.4s; }
/* 圆点用稀释版强调色：玻璃下满强度会让它从材质里"跳"出来。
   运行/熄灭的区分靠颜色档位 + 静态辉光 + 脉冲扩散三件一起。 */
.orb.active { background: var(--ui-accent-block); box-shadow: 0 0 10px var(--ui-accent); }
/* 呼吸灯的晕圈：实心色块 + 外缘渐隐，而不是整块均匀圆盘。
   关键是渐变长度 —— radial-gradient(circle, ...) 的百分比基准是 farthest-corner
   （10px 见方约 7.07px），所以 60% 只到 4.2px：元素放大 2.5 倍后是 10.6px，
   仍大于圆点自身 5px 的半径。也就是说「圆点之外那一段」承载的是满 alpha 的强调色，
   虚环才有实体感；再把 100% 处收成透明，边缘就不会像原来那样硬切。
   （第一版把透明点写在 72%，正好压在圆点边缘上，圆点外面全是透明的，环就没了。）
   z-index:-1 让晕圈从圆点背后扩散，圆点本身不被染色，起始那一帧也不会闪。
   渐变用的是纯强调色，深浅主题都成立：深色下是浅色光环，浅色下是深色涟漪。 */
.orb-glow { position: absolute; top: 0; left: 0; width: 100%; height: 100%; border-radius: 50%; z-index: -1; background: radial-gradient(circle, var(--ui-accent) 0%, var(--ui-accent) 60%, transparent 100%); animation: pulse 2s infinite; pointer-events: none; }
/* 评审修复：U-16 删掉了 style.css 里的全局 @keyframes pulse（当时判定"消费方都有同名定义"），
   但本文件的 .orb-glow 仍在用 —— 光晕的呼吸/扩散动画会静默失效。
   这里补一份 scoped 定义（scoped 会把 keyframes 名一起哈希，正好只服务本组件）。
   数值与 TrafficCard.vue 的 pulse 保持一致。 */
@keyframes pulse {
  0% {
    transform: scale(1);
    opacity: 0.5;
  }

  100% {
    transform: scale(2.5);
    opacity: 0;
  }
}
.refresh-icon { position: absolute; top: 50%; left: 50%; transform: translate(-50%,-50%) rotate(-90deg); width: 44px; height: 44px; opacity: 0.6; transition: all 0.4s cubic-bezier(0.4,0,0.2,1); }
.scanner-track { fill: none; stroke: var(--ui-surface-hover); stroke-width: 2.5; }
.scanner-bar { fill: none; stroke: var(--text-muted); stroke-width: 2.5; transition: all 0.4s cubic-bezier(0.4,0,0.2,1); stroke-dasharray: 62.8; stroke-dashoffset: 62.8; }
.restart-trigger:hover:not(.is-loading) .refresh-icon { opacity: 1; transform: translate(-50%,-50%) rotate(90deg); }
.restart-trigger:hover:not(.is-loading) .scanner-bar { stroke: var(--text-main); stroke-dashoffset: 20; }
.refresh-icon.spin { opacity: 1; animation: spin-centered 1.2s linear infinite; }
.refresh-icon.spin .scanner-bar { stroke: var(--ui-accent); stroke-dasharray: 62.8; animation: scan-dash 1.2s infinite ease-in-out; }
@keyframes spin-centered { 0%{transform:translate(-50%,-50%) rotate(-90deg)} 100%{transform:translate(-50%,-50%) rotate(270deg)} }
@keyframes scan-dash { 0%{stroke-dashoffset:60} 50%{stroke-dashoffset:15} 100%{stroke-dashoffset:60} }
/* 与右侧「活动配置」同一条左基线：两栏文字都左对齐，视觉上才是一个整体 */
.status-meta { display: flex; flex-direction: column; align-items: flex-start; text-align: left; min-width: 216px; }
.micro-title { font-size: 0.85rem; font-weight: 700; color: var(--text-sub); letter-spacing: 0.02em; margin-bottom: 8px; }
.status-heading { font-size: 1.6rem; font-weight: 600; margin: 0; line-height: 1.2; color: var(--text-main); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; max-width: 100%; }
.version-tag { font-family: var(--font-mono); font-size: 0.75rem; color: var(--text-sub); opacity: 0.8; margin-top: 8px; }
/* 启动超时提示：低对比、可点击，不能盖过主状态标题 */
.start-stuck-hint {
  margin-top: 8px;
  padding: 0;
  border: none;
  background: transparent;
  font: inherit;
  font-size: 0.78rem;
  color: var(--ui-accent, var(--text-sub));
  text-decoration: underline dotted;
  cursor: pointer;
  text-align: left;
}
.start-stuck-hint:hover { opacity: 0.8; }
.start-stuck-hint:focus-visible { outline: 2px solid var(--ui-accent-ring); outline-offset: 3px; border-radius: 4px; }
.truncate { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.active-config-display { text-align: center; min-width: 0; display: flex; flex-direction: column; align-items: center; }
.active-config-display .config-name { font-size: 1.1rem; font-weight: 700; color: var(--text-main); }
.switch-row { display: grid; grid-template-columns: 1fr 1fr; gap: 16px; }
.action-card { padding: 20px 24px; border: none; display: flex; justify-content: space-between; align-items: center; cursor: pointer; transition: all 0.2s cubic-bezier(0.4,0,0.2,1); }
.action-card.on { background: var(--ui-accent-block); }
.icon-ring { width: 40px; height: 40px; border-radius: var(--ui-radius-lg); background: var(--ui-surface-hover); display: flex; align-items: center; justify-content: center; color: var(--ui-text-secondary); transition: 0.3s; }
.icon-ring :deep(svg) { width: 22px; height: 22px; }
.on .icon-ring { background: color-mix(in srgb, var(--ui-accent-fg) 25%, transparent) !important; color: var(--ui-accent-fg); }
.card-title { display: block; font-size: 1rem; font-weight: 600; margin-bottom: 2px; color: var(--text-main); }
.card-hint { font-size: 0.75rem; color: var(--text-sub); }
.on .card-title { color: var(--ui-accent-fg); }
.on .card-hint { color: var(--ui-accent-fg); opacity: 0.7; }
.status-node { width: 6px; height: 6px; border-radius: 50%; background: var(--text-muted); }
.on .status-node { background: var(--ui-accent-fg); box-shadow: 0 0 8px var(--ui-accent-fg); }
.rules-card { padding: 24px 28px; display: flex; flex-direction: column; gap: 20px; }
.rules-head { display: flex; align-items: center; justify-content: space-between; }
/* 分段控件：控件凹槽 surface 由 .ui-surface-control 提供（无圆角），这里只负责布局与形状 */
.segmented-control { padding: 4px; border-radius: var(--ui-radius-lg); display: flex; position: relative; border: none; overflow: hidden; }
.seg-item { flex: 1; text-align: center; padding: 10px 4px; min-height: 40px; display: flex; align-items: center; justify-content: center; font-size: 0.9rem; font-weight: 600; color: var(--text-main); cursor: pointer; z-index: 1; transition: 0.3s; word-break: keep-all; }
.seg-item.active { color: var(--ui-accent-fg); }
.seg-slider { position: absolute; top: 4px; bottom: 4px; width: calc(33.33% - 8px); margin-left: 4px; background: var(--ui-accent-block); border-radius: var(--ui-radius-sm); z-index: 0; transition: left 0.3s cubic-bezier(0.4,0,0.2,1); }
</style>
