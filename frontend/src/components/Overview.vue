<template>
  <div class="overview-layout">
    <section class="hero-panel card-panel">
      <div class="status-core">
        <div class="orb-visual">
          <div class="orb" :class="{ 'active': isRunning }"></div>
          <div class="orb-glow" v-if="isRunning"></div>
        </div>
        <div class="status-meta">
          <span class="micro-title">引擎状态</span>
          <h2 class="status-heading">{{ isRunning ? '接管中' : '服务停止' }}</h2>
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
        <div class="v-line"></div>
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
        <span class="micro-title">流量分配规则</span>
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
import { ref, onMounted, computed } from 'vue';
import * as API from '../../wailsjs/go/main/App';
import { showAlert } from '../store';
import { ICONS } from '../utils/icons';

defineProps<{
  traffic: { up: string; down: string; }
}>();

const status = ref({ systemProxy: false, tun: false });
const currentMode = ref('rule');
const clashVersion = ref('');

const modes = [
  { label: '规则分流', val: 'rule' },
  { label: '全局代理', val: 'global' },
  { label: '直接连接', val: 'direct' }
];

const isRunning = computed(() => status.value.systemProxy || status.value.tun);
const sliderStyle = computed(() => ({ transform: `translateX(${modes.findIndex(m => m.val === currentMode.value) * 100}%)` }));

const refreshData = async () => {
  try {
    const data: any = await API.GetInitialData();
    if (data?.mode) currentMode.value = data.mode;
    if (data?.version) clashVersion.value = data.version;
    status.value = await API.GetProxyStatus() as any;
  } catch (e) { console.error(e); }
};

const toggleSysProxy = async () => {
  const originalValue = status.value.systemProxy;
  try {
    await API.ToggleSystemProxy(!originalValue);
    status.value = await API.GetProxyStatus() as any;
    window.dispatchEvent(new CustomEvent('proxy-status-sync', { detail: status.value }));
  } catch (err) {
    status.value.systemProxy = originalValue;
    await showAlert("操作系统代理失败: " + err, '错误');
  }
};

const toggleTun = async () => {
  const originalValue = status.value.tun;
  try {
    await API.ToggleTunMode(!originalValue);
    status.value = await API.GetProxyStatus() as any;
    window.dispatchEvent(new CustomEvent('proxy-status-sync', { detail: status.value }));
  } catch (err) {
    status.value.tun = originalValue;
    await showAlert("操作虚拟网卡失败: " + err, '错误');
  }
};

const handleModeChange = (val: string) => {
  currentMode.value = val;
  API.UpdateClashMode(val);
};

onMounted(() => {
  refreshData();
});
</script>

<style scoped>
.overview-layout {
  display: flex;
  flex-direction: column;
  gap: 24px;
  animation: fadeIn 0.4s ease-out;
}

/* 顶部面板 */
.hero-panel {
  padding: 24px 32px;
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.status-core { display: flex; align-items: center; gap: 24px; }
.orb-visual { position: relative; width: 12px; height: 12px; }
.orb { width: 100%; height: 100%; border-radius: 50%; background: var(--text-muted); transition: 0.4s; }
.orb.active { background: var(--text-main); box-shadow: 0 0 12px var(--text-main); }
.orb-glow { position: absolute; top: 0; left: 0; width: 100%; height: 100%; border-radius: 50%; background: var(--text-main); animation: pulse 2s infinite; }

.status-heading { font-size: 1.6rem; font-weight: 600; margin: 4px 0; color: var(--text-main); }
.version-tag { font-family: var(--font-mono); font-size: 0.75rem; color: var(--text-sub); opacity: 0.8; }

.traffic-meter { display: flex; gap: 40px; }
.traffic-box { text-align: right; }
.t-header { display: flex; align-items: center; gap: 6px; justify-content: flex-end; margin-bottom: 4px; }
.t-arrow { width: 12px; height: 12px; }
.t-arrow.up { color: var(--text-sub); }
.t-arrow.down { color: var(--text-main); }
.t-val { font-family: var(--font-mono); font-size: 1.15rem; font-weight: 500; color: var(--text-main); }
.v-line { width: 1px; height: 32px; background: var(--surface-hover); align-self: center; }

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
@keyframes fadeIn { from { opacity: 0; transform: translateY(10px); } to { opacity: 1; transform: translateY(0); } }

.micro-title { font-size: 0.7rem; text-transform: uppercase; letter-spacing: 0.12em; font-weight: 700; color: var(--text-muted); }
</style>