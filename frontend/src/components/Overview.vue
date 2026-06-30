<template>
  <div class="overview-layout">
    <section class="hero-panel card-panel">
      <div class="status-core">
        <div class="restart-trigger" :class="{ 'is-loading': isRestarting }" @click="handleRestartCore" title="重启内核">
          <div class="orb-visual" v-show="!isRestarting">
            <div class="orb" :class="{ 'active': globalState.isRunning }"></div>
            <div class="orb-glow" v-if="globalState.isRunning"></div>
          </div>
          <svg class="refresh-icon scanner-svg" :class="{ 'spin': isRestarting }" viewBox="0 0 24 24">
            <circle class="scanner-track" cx="12" cy="12" r="10"></circle>
            <circle class="scanner-bar" cx="12" cy="12" r="10"></circle>
          </svg>
        </div>
        <div class="status-meta">
          <span class="micro-title">服务状态</span>
          <h2 class="status-heading">{{ isRestarting ? '内核重启中...' : (globalState.isRunning ? '接管中' : '服务停止') }}</h2>
          <span class="version-tag">Mihomo {{ globalState.version || 'Core' }}</span>
        </div>
      </div>
      <div class="active-config-display">
        <span class="micro-title">活动配置</span>
        <div class="config-name truncate" :title="globalState.activeConfigName">
          {{ globalState.activeConfigName || '未选定' }}
        </div>
      </div>
    </section>

    <section class="switch-row">
      <div class="action-card" :class="{ 'on': sysProxyCardOn }" @click="toggleSysProxy">
        <div class="card-content">
          <div class="icon-ring" v-html="ICONS.sysProxy"></div>
          <div class="text-group">
            <span class="card-title">系统代理</span>
            <span class="card-hint">{{ sysProxyLabel }}</span>
          </div>
        </div>
        <div class="status-node"></div>
      </div>

      <div class="action-card" :class="{ 'on': tunCardOn }" @click="toggleTun">
        <div class="card-content">
          <div class="icon-ring" v-html="ICONS.tun"></div>
          <div class="text-group">
            <span class="card-title">虚拟网卡 (TUN)</span>
            <span class="card-hint">{{ tunLabel }}</span>
          </div>
        </div>
        <div class="status-node"></div>
      </div>
    </section>

    <section class="mode-section rules-card">
      <div class="rules-head">
        <h3 class="section-heading">出站路由规则</h3>
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
import { ref, computed } from 'vue';
import * as API from '../../wailsjs/go/main/App';
import { globalState, showAlert, showConfirm, updateStateFromBackend, scheduleOutboundIPRefresh } from '../store';
import { ICONS } from '../utils/icons';
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

const modes = [
  { label: '规则分流', val: 'rule' },
  { label: '全局代理', val: 'global' },
  { label: '直接连接', val: 'direct' }
];

const sliderStyle = computed(() => ({
  left: `${(modes.findIndex(m => m.val === globalState.mode) * 100) / 3}%`
}));

const isRestarting = ref(false);

const handleRestartCore = async () => {
  if (isRestarting.value) return;
  const ok = await showConfirm("确定要重新启动内核服务吗？这可能会导致短暂的网络中断。", "重启内核");
  if (!ok) return;
  isRestarting.value = true;
  try {
    await (API as any).RestartCore();
    isRestarting.value = false;
    await showAlert("内核服务已成功重启", '成功');
  } catch (e) {
    isRestarting.value = false;
    await showAlert("重启失败: " + e, '错误');
  }
};

// ==========================================
// 切换状态管理（乐观 UI）
// ==========================================
const sysProxySwitching = ref(false);
const optimisticSysProxy = ref<boolean | null>(null);

const tunSwitching = ref(false);
const optimisticTun = ref<boolean | null>(null);

const sysProxyCardOn = computed(() => {
  if (sysProxySwitching.value && optimisticSysProxy.value !== null) {
    return optimisticSysProxy.value;
  }
  return globalState.systemProxy;
});

const tunCardOn = computed(() => {
  if (tunSwitching.value && optimisticTun.value !== null) {
    return optimisticTun.value;
  }
  return globalState.tun;
});

const sysProxyLabel = computed(() => {
  return sysProxyCardOn.value
    ? '已修改系统网络层设置'
    : '未接管系统 HTTP 流量';
});

const tunLabel = computed(() => {
  return tunCardOn.value
    ? '高优先级虚拟设备已挂载'
    : '透明代理驱动未加载';
});

const toggleSysProxy = async () => {
  if (sysProxySwitching.value) return;

  const target = !sysProxyCardOn.value;
  optimisticSysProxy.value = target;
  sysProxySwitching.value = true;
  globalState.systemProxy = target;
  // 操作期间屏蔽 TUN 状态推送，防止 Reconcile 中间态导致 TUN 卡片闪烁
  globalState.suppressTunSync = true;

  try {
    await API.ToggleSystemProxy(target);
  } catch (err: any) {
    globalState.systemProxy = !target;
    const msg = String(err?.message || err || '');
    if (msg.includes('no active config selected') || msg.includes('ErrNoActiveConfig')) {
      showAlert('尚未添加配置\n\n启用系统代理前，请先添加并应用一个配置文件。', '提示');
    } else {
      showAlert('系统代理启用失败: ' + msg, '错误', true);
    }
  } finally {
    sysProxySwitching.value = false;
    optimisticSysProxy.value = null;
    globalState.suppressTunSync = false;
  }
};

const toggleTun = async () => {
  if (tunSwitching.value) return;

  const target = !tunCardOn.value;
  optimisticTun.value = target;
  tunSwitching.value = true;
  globalState.tun = target;
  // 操作期间屏蔽系统代理状态推送，防止 Reconcile 中间态导致系统代理卡片闪烁
  globalState.suppressSystemProxySync = true;

  try {
    await API.ToggleTunMode(target);
  } catch (err: any) {
    globalState.tun = !target;
    const msg = String(err?.message || err || '');

    if (msg.includes('no active config selected') || msg.includes('ErrNoActiveConfig')) {
      showAlert('尚未添加配置\n\n启用虚拟网卡前，请先添加并应用一个配置文件。', '提示');
    } else if (msg.includes('helper_install_required') || msg.includes('helper_repair_required')) {
      const confirmed = await showConfirm(
        'TUN 模式需要初始化后台服务 (GoclashZHelper)\n\n此操作只需管理员确认一次，之后可无感启用 TUN 和开机恢复。',
        '需要初始化后台服务'
      );

      if (confirmed) {
        try {
          optimisticTun.value = target;
          globalState.tun = target;
          await (API as any).InstallHelperService();
          await API.ToggleTunMode(target);
        } catch (e: any) {
          globalState.tun = !target;
          showAlert('初始化后台服务失败: ' + String(e?.message || e), '错误', true);
        }
      } else {
        globalState.tun = !target;
      }
    } else if (msg.includes('wintun_missing') || msg.includes('Wintun')) {
      showAlert('缺少 Wintun 驱动，请在「组件与库更新」页面安装 Wintun 驱动。', '缺少依赖', true);
    } else {
      showAlert('虚拟网卡启用失败: ' + msg, '错误', true);
    }
  } finally {
    tunSwitching.value = false;
    optimisticTun.value = null;
    globalState.suppressSystemProxySync = false;
  }
};

// ==========================================
// 模式切换
// ==========================================
let modeWorkerActive = false;
let pendingModeTarget: string | null = null;
let previousMode = globalState.mode;

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
    await API.FlushFakeIP().catch(() => {});
    await API.CloseAllConnections().catch(() => {});
    if (targetMode === 'global') {
      const data: any = await API.GetInitialData();
      if (data?.groups) {
        const keys = data.groupOrder?.length > 0 ? data.groupOrder : Object.keys(data.groups);
        for (const name of keys) {
          const item = data.groups[name];
          if (!item) continue;
          if (item.type === 'Selector' && !['GLOBAL', 'DIRECT', 'REJECT'].includes(name)) {
            if (item.now) await API.SelectProxy('GLOBAL', item.now).catch(() => {});
            break;
          }
        }
      }
    }
    const latest = await (API as any).GetAppState();
    updateStateFromBackend(latest);
  } catch (err) {
    globalState.mode = previousMode;
    await showAlert("模式切换失败: " + err, '错误');
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
