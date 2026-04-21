<template>
  <div class="overview-layout">
    <section class="hero-panel card-panel">
      <div class="status-core">
        <div class="restart-trigger" :class="{ 'is-loading': isRestarting }" @click="handleRestartCore" title="重启内核">
          <div class="orb-visual" v-show="!isRestarting">
            <div class="orb" :class="{ 'active': isRunning }"></div>
            <div class="orb-glow" v-if="isRunning"></div>
          </div>
          <!-- 替换为类似测延迟的扫描圈 -->
          <svg class="refresh-icon scanner-svg" :class="{ 'spin': isRestarting }" viewBox="0 0 24 24">
            <circle class="scanner-track" cx="12" cy="12" r="10"></circle>
            <circle class="scanner-bar" cx="12" cy="12" r="10"></circle>
          </svg>
        </div>
        <div class="status-meta">
          <span class="micro-title">引擎状态</span>
          <h2 class="status-heading">{{ isRestarting ? '内核重启中...' : (isRunning ? '接管中' : '服务停止') }}</h2>
          <span class="version-tag">{{ clashVersion || 'Mihomo Core' }}</span>
        </div>
      </div>

      <div class="traffic-meter">
        <div class="traffic-box">
          <div class="t-header">
            <span class="t-arrow up" v-html="ICONS.arrowUp"></span>
            <span class="micro-title">上传</span>
          </div>
          <div class="t-val">{{ traffic.up }}</div>
        </div>
        <div class="traffic-box">
          <div class="t-header">
            <span class="t-arrow down" v-html="ICONS.arrowDown"></span>
            <span class="micro-title">下载</span>
          </div>
          <div class="t-val">{{ traffic.down }}</div>
        </div>
      </div>
    </section>

    <section class="switch-row">
      <div class="action-card" :class="{ 'on': status.systemProxy }" @click="toggleSysProxy">
        <div class="card-content">
          <div class="icon-ring" v-html="ICONS.sysProxy"></div>
          <div class="text-group">
            <span class="card-title">系统代理</span>
            <span class="card-hint">{{ status.systemProxy ? '已修改系统网络层设置' : '未接管系统 HTTP 流量' }}</span>
          </div>
        </div>
        <div class="status-node"></div>
      </div>

      <div class="action-card" :class="{ 'on': status.tun }" @click="toggleTun">
        <div class="card-content">
          <div class="icon-ring" v-html="ICONS.tun"></div>
          <div class="text-group">
            <span class="card-title">虚拟网卡 (TUN)</span>
            <span class="card-hint">{{ status.tun ? '高优先级虚拟设备已挂载' : '透明代理驱动未加载' }}</span>
          </div>
        </div>
        <div class="status-node"></div>
      </div>
    </section>

    <section class="mode-section">
      <div class="section-title">
        <h3 class="section-heading">出站路由规则</h3>
      </div>
      <div class="segmented-control">
        <div 
          v-for="m in modes" 
          :key="m.val" 
          class="seg-item"
          :class="{ active: currentMode === m.val }"
          @click="handleModeChange(m.val)"
        >
          {{ m.label }}
        </div>
        <div class="seg-slider" :style="sliderStyle"></div>
      </div>
    </section>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted, computed } from 'vue';
import * as API from '../../wailsjs/go/main/App';
// 👇 新增：引入 Wails 运行时的事件监听机制
import { EventsOn, EventsOff } from '../../wailsjs/runtime/runtime';
import { showAlert, showConfirm } from '../store';
import { ICONS } from '../utils/icons';

defineProps<{
  traffic: { up: string; down: string; }
}>();

const status = ref({ systemProxy: false, tun: false });
const currentMode = ref('rule');
const clashVersion = ref('');

// 状态调和队列：分离 UI 的"目标状态"与后台的"实际状态"
const sysProxyQueue = { isProcessing: false, target: false, actual: false };
const tunQueue = { isProcessing: false, target: false, actual: false };

const modes = [
  { label: '规则分流', val: 'rule' },
  { label: '全局代理', val: 'global' },
  { label: '直接连接', val: 'direct' }
];

const isRunning = computed(() => status.value.systemProxy || status.value.tun);
const sliderStyle = computed(() => ({ transform: `translateX(${modes.findIndex(m => m.val === currentMode.value) * 100}%)` }));

const isRestarting = ref(false);

const handleRestartCore = async () => {
  if (isRestarting.value) return;

  const ok = await showConfirm("确定要重新启动内核服务吗？这可能会导致短暂的网络中断。", "重启内核");
  if (!ok) return;

  isRestarting.value = true;
  try {
    await (API as any).RestartCore();
    await refreshData();
    await showAlert("内核服务已成功重启", '成功');
  } catch (e) {
    await showAlert("重启失败: " + e, '错误');
  } finally {
    isRestarting.value = false;
  }
};

const refreshData = async () => {
  try {
    const data: any = await API.GetInitialData();
    if (data?.mode) currentMode.value = data.mode;
    if (data?.version) clashVersion.value = data.version;
    const st: any = await API.GetProxyStatus();
    status.value = st;

    // 初始化对齐
    sysProxyQueue.actual = st.systemProxy;
    sysProxyQueue.target = st.systemProxy;
    tunQueue.actual = st.tun;
    tunQueue.target = st.tun;
  } catch (e) { console.error(e); }
};

const toggleSysProxy = () => {
  // 🚀 极致乐观 UI：无视后台，开关瞬间响应用户点击
  status.value.systemProxy = !status.value.systemProxy;
  sysProxyQueue.target = status.value.systemProxy;
  processSysProxy();
};

const processSysProxy = async () => {
  if (sysProxyQueue.isProcessing) return; // 后台正在处理，直接返回
  sysProxyQueue.isProcessing = true;

  // 只要真实状态还没追上目标状态，就继续干活
  while (sysProxyQueue.actual !== sysProxyQueue.target) {
    const nextState = sysProxyQueue.target; // 锁定此时的目标
    try {
      await API.ToggleSystemProxy(nextState);
      sysProxyQueue.actual = nextState; // 成功到达！
    } catch (err) {
      // 灾难回滚：把 UI 和目标重置为崩溃前的真实状态
      sysProxyQueue.target = sysProxyQueue.actual;
      status.value.systemProxy = sysProxyQueue.actual;
      await showAlert("操作系统代理失败: " + err, '错误');
      break;
    }
  }

  // 队列清空，推送一次全局同步
  const finalStatus = await API.GetProxyStatus() as any;
  status.value = finalStatus;
  window.dispatchEvent(new CustomEvent('proxy-status-sync', { detail: finalStatus }));
  sysProxyQueue.isProcessing = false;
};

const toggleTun = () => {
  status.value.tun = !status.value.tun;
  tunQueue.target = status.value.tun;
  processTun();
};

const processTun = async () => {
  if (tunQueue.isProcessing) return;
  tunQueue.isProcessing = true;

  while (tunQueue.actual !== tunQueue.target) {
    const nextState = tunQueue.target;
    try {
      await API.ToggleTunMode(nextState);
      tunQueue.actual = nextState;
    } catch (err) {
      tunQueue.target = tunQueue.actual;
      status.value.tun = tunQueue.actual;
      await showAlert("操作虚拟网卡失败: " + err, '错误');
      break;
    }
  }

  const finalStatus = await API.GetProxyStatus() as any;
  status.value = finalStatus;
  window.dispatchEvent(new CustomEvent('proxy-status-sync', { detail: finalStatus }));
  tunQueue.isProcessing = false;
};

const handleModeChange = (val: string) => {
  currentMode.value = val;
  API.UpdateClashMode(val);
};

onMounted(() => {
  refreshData();

  // 👇 新增：监听后端(如托盘菜单)触发的状态同步事件
  EventsOn("app-state-sync", (state: any) => {
    // 1. 同步出站路由模式
    if (state.mode) {
      currentMode.value = state.mode;
    }

    // 👈 核心：如果在狂点处理中，忽略后端的全局广播，防止 UI 被扯回过去
    if (!sysProxyQueue.isProcessing && !tunQueue.isProcessing) {
      API.GetProxyStatus().then((res: any) => {
        status.value = res;
        sysProxyQueue.actual = res.systemProxy;
        sysProxyQueue.target = res.systemProxy;
        tunQueue.actual = res.tun;
        tunQueue.target = res.tun;
      });
    }
  });
});

// 👇 新增：当页面被销毁时注销监听，防止事件重复绑定和内存泄漏
onUnmounted(() => {
  EventsOff("app-state-sync");
});
</script>

<style scoped>
.overview-layout {
  display: flex;
  flex-direction: column;
  gap: 24px;
}

/* 顶部面板 */
.hero-panel {
  padding: 28px 36px;
  background: var(--surface);
  border-radius: 20px;
  display: flex;
  justify-content: space-between;
  align-items: center;
  box-shadow: 0 4px 20px rgba(0, 0, 0, 0.03);
}

.status-core { display: flex; align-items: center; gap: 20px; }

/* ⬇️ 重置的呼吸灯交互区域 ⬇️ */
.restart-trigger { 
  position: relative; width: 56px; height: 56px; /* 进一步加大 */
  display: flex; align-items: center; justify-content: center; 
  cursor: pointer; border-radius: 50%; transition: 0.3s;
}
.restart-trigger:hover { background: var(--surface-hover); }
.restart-trigger.is-loading { cursor: not-allowed; pointer-events: none; }

.orb-visual { position: relative; width: 10px; height: 10px; z-index: 2; transition: all 0.3s cubic-bezier(0.4, 0, 0.2, 1); }
.orb { width: 100%; height: 100%; border-radius: 50%; background: var(--text-muted); transition: 0.4s; }
.orb.active { background: var(--text-main); box-shadow: 0 0 10px var(--text-main); }
.orb-glow { position: absolute; top: 0; left: 0; width: 100%; height: 100%; border-radius: 50%; background: var(--text-main); animation: pulse 2s infinite; }

/* 刷新图标：使用扫面圈样式 */
.refresh-icon { 
  position: absolute; top: 50%; left: 50%;
  transform: translate(-50%, -50%) rotate(-90deg);
  width: 44px; height: 44px; /* 加大图标尺寸 */
  opacity: 0.6; /* 默认半透明 */
  transition: all 0.4s cubic-bezier(0.4, 0, 0.2, 1);
}

.scanner-track { fill: none; stroke: var(--surface-hover); stroke-width: 2.5; }
.scanner-bar {
  fill: none; stroke: var(--text-muted); stroke-width: 2.5;
  transition: all 0.4s cubic-bezier(0.4, 0, 0.2, 1);
  stroke-dasharray: 62.8; 
  stroke-dashoffset: 62.8; /* 默认不显示 */
}

/* 交互逻辑 1：普通悬停时，显示部分圆环并旋转 */
.restart-trigger:hover:not(.is-loading) .refresh-icon { 
  opacity: 1; 
  transform: translate(-50%, -50%) rotate(90deg); 
}
.restart-trigger:hover:not(.is-loading) .scanner-bar {
  stroke: var(--text-main);
  stroke-dashoffset: 20; /* 显示一部分圆环 */
}

/* 交互逻辑 2：请求后端加载时，刷新图标持续旋转 */
.refresh-icon.spin { 
  opacity: 1; 
  animation: spin-centered 1.2s linear infinite; 
}
.refresh-icon.spin .scanner-bar {
  stroke: var(--accent);
  stroke-dasharray: 62.8;
  animation: scan-dash 1.2s infinite ease-in-out;
}

@keyframes spin-centered { 
  0% { transform: translate(-50%, -50%) rotate(-90deg); }
  100% { transform: translate(-50%, -50%) rotate(270deg); } 
}
@keyframes scan-dash {
  0% { stroke-dashoffset: 60; }
  50% { stroke-dashoffset: 15; }
  100% { stroke-dashoffset: 60; }
}
/* ⬆️ 呼吸灯交互样式结束 ⬆️ */

.status-heading { font-size: 1.6rem; font-weight: 600; margin: 4px 0; color: var(--text-main); }
.version-tag { font-family: var(--font-mono); font-size: 0.75rem; color: var(--text-sub); opacity: 0.8; }

.traffic-meter { display: flex; gap: 32px; }
.traffic-box { text-align: right; }
.t-header { display: flex; align-items: center; gap: 6px; justify-content: flex-end; margin-bottom: 4px; }
.t-arrow { width: 12px; height: 12px; }
.t-arrow.up { color: var(--text-sub); }
.t-arrow.down { color: var(--text-main); }
.t-val { font-family: var(--font-mono); font-size: 1.15rem; font-weight: 500; color: var(--text-main); }

/* 开关卡片 */
.switch-row { display: grid; grid-template-columns: 1fr 1fr; gap: 16px; }
.action-card {
  padding: 20px 24px;
  background: var(--surface);
  border: none;
  border-radius: 16px;
  display: flex;
  justify-content: space-between;
  align-items: center;
  cursor: pointer;
  transition: all 0.2s cubic-bezier(0.4, 0, 0.2, 1);
}
.action-card:hover { background: var(--surface-hover); }
.action-card.on { background: var(--accent); }

.icon-ring { 
  width: 40px; height: 40px; border-radius: 12px; background: var(--surface-hover);
  display: flex; align-items: center; justify-content: center; color: var(--text-sub);
  transition: 0.3s;
}
.icon-ring :deep(svg) { width: 22px; height: 22px; }
.on .icon-ring { 
  background: rgba(128, 128, 128, 0.25) !important; 
  color: var(--accent-fg); 
}

.card-title { display: block; font-size: 1rem; font-weight: 600; margin-bottom: 2px; color: var(--text-main); }
.card-hint { font-size: 0.75rem; color: var(--text-sub); }
.on .card-title { color: var(--accent-fg); }
.on .card-hint { color: var(--accent-fg); opacity: 0.7; }

.status-node { width: 6px; height: 6px; border-radius: 50%; background: var(--text-muted); }
.on .status-node { background: var(--accent-fg); box-shadow: 0 0 8px var(--accent-fg); }

/* 分段选择器 */
.segmented-control {
  background: var(--surface);
  padding: 4px; border-radius: 14px; display: flex; position: relative;
  border: none; overflow: hidden;
}
.seg-item {
  flex: 1; text-align: center; padding: 12px 0; font-size: 0.9rem; font-weight: 500;
  color: var(--text-sub); cursor: pointer; z-index: 1; transition: 0.3s;
}
.seg-item.active { color: var(--text-main); font-weight: 600; }
.seg-slider {
  position: absolute; top: 4px; left: 4px; width: calc(33.33% - 8px); height: calc(100% - 8px);
  background: var(--surface-panel); border-radius: 10px; z-index: 0;
  box-shadow: 0 2px 10px rgba(0, 0, 0, 0.08); transition: transform 0.3s cubic-bezier(0.4, 0, 0.2, 1);
}

@keyframes pulse { 0% { transform: scale(1); opacity: 0.5; } 100% { transform: scale(2.5); opacity: 0; } }

.micro-title { font-size: 0.7rem; text-transform: uppercase; letter-spacing: 0.12em; font-weight: 700; color: var(--text-muted); }
.section-heading { font-size: 1.1rem; font-weight: 600; color: var(--text-main); margin: 0 0 12px 4px; }

</style>