<template>
  <div class="overview-layout">
    <section class="hero-panel card-panel">
      <div class="status-core">
        <div class="restart-trigger" :class="{ 'is-loading': isRestarting }" @click="handleRestartCore" title="重启内核">
          <div class="orb-visual" v-show="!isRestarting">
            <div class="orb" :class="{ 'active': globalState.isRunning }"></div>
            <div class="orb-glow" v-if="globalState.isRunning"></div>
          </div>
          <!-- 替换为类似测延迟的扫描圈 -->
          <svg class="refresh-icon scanner-svg" :class="{ 'spin': isRestarting }" viewBox="0 0 24 24">
            <circle class="scanner-track" cx="12" cy="12" r="10"></circle>
            <circle class="scanner-bar" cx="12" cy="12" r="10"></circle>
          </svg>
        </div>
        <div class="status-meta">
          <span class="micro-title">引擎状态</span>
          <h2 class="status-heading">{{ isRestarting ? '内核重启中...' : (globalState.isRunning ? '接管中' : '服务停止') }}</h2>
          <span class="version-tag">Mihomo {{ globalState.version || 'Core' }}</span>
        </div>
      </div>

      <div class="traffic-meter">
        <div class="traffic-box" style="margin-right: 24px; text-align: left;">
          <span class="micro-title">活动配置</span>
          <div class="t-val truncate" style="max-width: 150px; font-size: 1rem;" :title="globalState.activeConfigName">
            {{ globalState.activeConfigName || '未选定' }}
          </div>
        </div>
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
      <div class="action-card" :class="{ 'on': globalState.systemProxy }" @click="toggleSysProxy">
        <div class="card-content">
          <div class="icon-ring" v-html="ICONS.sysProxy"></div>
          <div class="text-group">
            <span class="card-title">系统代理</span>
            <span class="card-hint">{{ globalState.systemProxy ? '已修改系统网络层设置' : '未接管系统 HTTP 流量' }}</span>
          </div>
        </div>
        <div class="status-node"></div>
      </div>

      <div class="action-card" :class="{ 'on': globalState.tun }" @click="toggleTun">
        <div class="card-content">
          <div class="icon-ring" v-html="ICONS.tun"></div>
          <div class="text-group">
            <span class="card-title">虚拟网卡 (TUN)</span>
            <span class="card-hint">{{ globalState.tun ? '高优先级虚拟设备已挂载' : '透明代理驱动未加载' }}</span>
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
          :class="{ active: globalState.mode === m.val }"
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
import { ref, computed } from 'vue';
import * as API from '../../wailsjs/go/main/App';
import { globalState, showAlert, showConfirm } from '../store'; // 👈 直接引入唯一的真相来源 globalState
import { ICONS } from '../utils/icons';

defineProps<{
  traffic: { up: string; down: string; }
}>();

const modes = [
  { label: '规则分流', val: 'rule' },
  { label: '全局代理', val: 'global' },
  { label: '直接连接', val: 'direct' }
];

// 👈 1. 彻底删除本地的 status、currentMode、clashVersion 变量！
// 直接使用 globalState 作为唯一真相源，计算属性自动响应
const sliderStyle = computed(() => ({ transform: `translateX(${modes.findIndex(m => m.val === globalState.mode) * 100}%)` }));

const isRestarting = ref(false);

const handleRestartCore = async () => {
  if (isRestarting.value) return;
  const ok = await showConfirm("确定要重新启动内核服务吗？这可能会导致短暂的网络中断。", "重启内核");
  if (!ok) return;

  isRestarting.value = true;
  try {
    await (API as any).RestartCore();
    // 💡 重点：不需要手动 refreshData()，因为 RestartCore 成功后，后端会主动触发 SyncState，store.ts 会自动更新 UI！
    await showAlert("内核服务已成功重启", '成功');
  } catch (e) {
    await showAlert("重启失败: " + e, '错误');
  } finally {
    isRestarting.value = false;
  }
};

// ==========================================
// 状态截断法 (解决快速连点且防崩溃)
// ==========================================

// 后台工作状态与暂存队列
let sysProxyWorkerActive = false;
let pendingSysProxyTarget: boolean | null = null;

let tunWorkerActive = false;
let pendingTunTarget: boolean | null = null;

// 👉 瞬间响应的 UI 开关
const toggleSysProxy = () => {
  // 1. 极致乐观 UI：瞬间改状态，0 延迟变色！
  globalState.systemProxy = !globalState.systemProxy;
  const target = globalState.systemProxy;

  // 2. 如果后台还在干上一个活，只需把“最新目标”贴在门外，直接不管了！
  if (sysProxyWorkerActive) {
    pendingSysProxyTarget = target;
    return;
  }

  // 3. 后台空闲，去干活
  runSysProxyWorker(target);
};

// 👉 后台实际执行的苦力
const runSysProxyWorker = async (target: boolean) => {
  sysProxyWorkerActive = true;
  
  try {
    await API.ToggleSystemProxy(target);
  } catch (err) {
    // 💥 灾难回滚：只有在底层真报错时，才把 UI 掰回来
    globalState.systemProxy = !target;
    console.error("系统代理失败: ", err);
  }

  // 4. 关键点：活干完了，看看这段时间里，用户有没有又狂点了按钮？
  if (pendingSysProxyTarget !== null && pendingSysProxyTarget !== target) {
    // 取出最新的目标，清空暂存
    const nextTarget = pendingSysProxyTarget;
    pendingSysProxyTarget = null;
    
    // 直接拿着最新目标再跑一次，完美跳过了中间所有的无效点击！
    await runSysProxyWorker(nextTarget);
  } else {
    // 没有新目标，彻底休息
    pendingSysProxyTarget = null;
    sysProxyWorkerActive = false;
  }
};


// ==========================================
// 虚拟网卡的同理实现
// ==========================================
const toggleTun = () => {
  globalState.tun = !globalState.tun;
  const target = globalState.tun;

  if (tunWorkerActive) {
    pendingTunTarget = target;
    return;
  }
  runTunWorker(target);
};

const runTunWorker = async (target: boolean) => {
  tunWorkerActive = true;
  
  try {
    await API.ToggleTunMode(target);
  } catch (err) {
    globalState.tun = !target;
    console.error("虚拟网卡失败: ", err);
  }

  if (pendingTunTarget !== null && pendingTunTarget !== target) {
    const nextTarget = pendingTunTarget;
    pendingTunTarget = null;
    await runTunWorker(nextTarget);
  } else {
    pendingTunTarget = null;
    tunWorkerActive = false;
  }
};

const handleModeChange = (val: string) => {
  // 1. 极致乐观 UI：点击瞬间直接改全局状态，滑块和文字立刻变色，绝不等待后端
  globalState.mode = val;
  
  // 2. 异步通知后端执行切换 (后台会处理写盘和内核 API，失败会自动回滚)
  API.UpdateClashMode(val);
};

// 👈 3. 彻底删除了 onMounted 里的 EventsOn 和 onUnmounted 里的 EventsOff！
// 监听权已全部上交给 store.ts
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
.t-arrow { width: 12px; height: 12px; color: var(--text-main); }
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
/* .action-card:hover { background: var(--surface-hover); } */
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