<template>
  <div class="overview-layout">
    <section class="status-hero glass-panel">
      <div class="hero-main">
        <div class="orb-wrapper">
          <div class="status-orb" :class="{ 'is-active': isRunning }"></div>
          <div class="orb-ring" :class="{ 'is-active': isRunning }"></div>
        </div>
        <div class="status-details">
          <span class="micro-title">内核状态报告</span>
          <h2 class="status-text">{{ isRunning ? '网络接管中' : '服务待命' }}</h2>
          <div class="version-badge">
            <span class="icon" v-html="ICONS.cpu"></span>
            {{ clashVersion || 'Mihomo Core' }}
          </div>
        </div>
      </div>

      <div class="traffic-dashboard">
        <div class="traffic-stat">
          <div class="stat-header">
            <span class="icon up" v-html="ICONS.arrowUp"></span>
            <span class="micro-title">发送</span>
          </div>
          <span class="stat-value">{{ traffic.up }}</span>
        </div>
        <div class="stat-divider"></div>
        <div class="traffic-stat">
          <div class="stat-header">
            <span class="icon down" v-html="ICONS.arrowDown"></span>
            <span class="micro-title">接收</span>
          </div>
          <span class="stat-value">{{ traffic.down }}</span>
        </div>
      </div>
    </section>

    <section class="control-grid">
      <div 
        class="switch-card" 
        :class="{ 'is-on': status.systemProxy }" 
        @click="toggleSysProxy"
      >
        <div class="card-icon" v-html="ICONS.globe"></div>
        <div class="card-info">
          <span class="card-title">系统代理</span>
          <span class="card-desc">{{ status.systemProxy ? '已修改系统 HTTP 代理设置' : '未接管系统流量' }}</span>
        </div>
        <div class="indicator-dot"></div>
      </div>

      <div 
        class="switch-card" 
        :class="{ 'is-on': status.tun }" 
        @click="toggleTun"
      >
        <div class="card-icon" v-html="ICONS.zap"></div>
        <div class="card-info">
          <span class="card-title">虚拟网卡 (TUN)</span>
          <span class="card-desc">{{ status.tun ? '高优先级虚拟网卡已挂载' : '透明代理模式未启动' }}</span>
        </div>
        <div class="indicator-dot"></div>
      </div>
    </section>

    <section class="config-bar glass-panel">
      <div class="config-item">
        <div class="item-label">
          <span class="icon" v-html="ICONS.layers"></span>
          <span class="micro-title">路由模式</span>
        </div>
        <div class="select-box">
          <select v-model="currentMode" @change="handleModeChange">
            <option value="rule">规则模式 (Rule)</option>
            <option value="global">全局模式 (Global)</option>
            <option value="direct">直连模式 (Direct)</option>
          </select>
        </div>
      </div>
      
      <div class="v-divider"></div>

      <div class="config-item">
        <div class="item-label">
          <span class="icon" v-html="ICONS.file"></span>
          <span class="micro-title">活跃配置文件</span>
        </div>
        <div class="config-name truncate" :title="activeConfigName">
          {{ activeConfigName || 'default_config.yaml' }}
        </div>
      </div>
    </section>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted, computed } from 'vue';
import * as API from '../../wailsjs/go/main/App';

const ICONS = {
  cpu: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="4" y="4" width="16" height="16" rx="2"/><path d="M9 9h6v6H9zM15 2v2M9 2v2M20 15h2M20 9h2M15 20v2M9 20v2M2 15h2M2 9h2"/></svg>`,
  arrowUp: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5"><path d="M12 19V5M5 12l7-7 7 7"/></svg>`,
  arrowDown: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5"><path d="M12 5v14M5 12l7 7 7-7"/></svg>`,
  globe: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8"><circle cx="12" cy="12" r="10"/><path d="M2 12h20M12 2a15.3 15.3 0 0 1 4 10 15.3 15.3 0 0 1-4 10 15.3 15.3 0 0 1-4-10 15.3 15.3 0 0 1 4-10z"/></svg>`,
  zap: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8"><path d="M13 2L3 14h9l-1 8 10-12h-9l1-8z"/></svg>`,
  layers: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M12 2L2 7l10 5 10-5-10-5zM2 17l10 5 10-5M2 12l10 5 10-5"/></svg>`,
  file: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M13 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V9z"/><polyline points="13 2 13 9 20 9"/></svg>`
};

const status = ref({ systemProxy: false, tun: false });
const activeConfigName = ref('');
const currentMode = ref('rule');
const clashVersion = ref('');
const traffic = ref({ up: '0 B/s', down: '0 B/s' });

const isRunning = computed(() => status.value.systemProxy || status.value.tun);

const refreshAllData = async () => {
  try {
    const data: any = await API.GetInitialData();
    if (data) {
      if (data.activeConfig) activeConfigName.value = data.activeConfig;
      if (data.mode) currentMode.value = data.mode;
      clashVersion.value = data.version || '';
    }
    status.value = await API.GetProxyStatus() as any;
  } catch (e) { console.error(e); }
};

const toggleSysProxy = async () => {
  await API.ToggleSystemProxy(!status.value.systemProxy);
  status.value = await API.GetProxyStatus() as any;
};

const toggleTun = async () => {
  await API.ToggleTunMode(!status.value.tun);
  status.value = await API.GetProxyStatus() as any;
};

const handleModeChange = () => API.UpdateClashMode(currentMode.value);

const onConfigChanged = (newName: string) => {
  activeConfigName.value = newName;
  refreshAllData();
};

onMounted(() => {
  refreshAllData();
  (window as any).runtime.EventsOn("config-changed", onConfigChanged);
  (window as any).runtime.EventsOn("traffic-data", (data: any) => traffic.value = data);
});

onUnmounted(() => {
  (window as any).runtime.EventsOff("config-changed");
  (window as any).runtime.EventsOff("traffic-data");
});
</script>

<style scoped>
.overview-layout {
  display: flex;
  flex-direction: column;
  gap: 24px;
  animation: slideUp 0.4s ease-out;
  font-family: "Microsoft YaHei", -apple-system, sans-serif !important;
}

/* 顶部状态卡片 */
.status-hero {
  padding: 32px;
  display: flex;
  justify-content: space-between;
  align-items: center;
  background: var(--glass-panel);
}

.hero-main {
  display: flex;
  align-items: center;
  gap: 28px;
}

.orb-wrapper {
  position: relative;
  width: 14px;
  height: 14px;
}

.status-orb {
  width: 100%;
  height: 100%;
  border-radius: 50%;
  background: var(--text-muted);
  transition: all 0.6s ease;
}

.orb-ring {
  position: absolute;
  top: -6px; left: -6px; right: -6px; bottom: -6px;
  border: 1px solid var(--text-muted);
  border-radius: 50%;
  opacity: 0.2;
}

.status-orb.is-active {
  background: #10b981;
  box-shadow: 0 0 20px rgba(16, 185, 129, 0.4);
}

.orb-ring.is-active {
  border-color: #10b981;
  animation: pulse 2.5s infinite;
}

.status-text {
  font-size: 1.8rem;
  font-weight: 600;
  letter-spacing: -0.01em;
  margin: 4px 0;
  color: var(--text-main);
}

.version-badge {
  font-family: var(--font-mono);
  font-size: 0.75rem;
  color: var(--text-sub);
  display: flex;
  align-items: center;
  gap: 6px;
  opacity: 0.8;
}

.version-badge .icon { width: 14px; height: 14px; }

/* 流量显示 */
.traffic-dashboard {
  display: flex;
  gap: 40px;
}

.traffic-stat {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.stat-header {
  display: flex;
  align-items: center;
  gap: 6px;
}

.stat-header .icon { width: 12px; height: 12px; }
.stat-header .icon.up { color: #3b82f6; }
.stat-header .icon.down { color: #10b981; }

.stat-value {
  font-family: var(--font-mono);
  font-size: 1.25rem;
  font-weight: 500;
  color: var(--text-main);
}

.stat-divider {
  width: 1px;
  height: 40px;
  background: var(--glass-border);
}

/* 控制切换区 */
.control-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 16px;
}

.switch-card {
  padding: 24px;
  border-radius: 16px;
  background: var(--surface);
  border: 1px solid transparent;
  display: flex;
  align-items: flex-start;
  gap: 16px;
  cursor: pointer;
  transition: all 0.2s cubic-bezier(0.4, 0, 0.2, 1);
  position: relative;
}

.switch-card:hover {
  background: var(--surface-hover);
  border-color: var(--glass-border);
}

.switch-card.is-on {
  background: var(--accent);
}

.card-icon {
  width: 40px;
  height: 40px;
  display: flex;
  align-items: center;
  justify-content: center;
  background: var(--surface-hover);
  border-radius: 12px;
  color: var(--text-sub);
  transition: 0.3s;
}

.is-on .card-icon {
  background: rgba(255, 255, 255, 0.1);
  color: var(--accent-fg);
}

.card-title {
  font-size: 1rem;
  font-weight: 600;
  display: block;
  margin-bottom: 4px;
  color: var(--text-main);
}

.card-desc {
  font-size: 0.8rem;
  color: var(--text-sub);
  line-height: 1.4;
}

.is-on .card-title { color: var(--accent-fg); }
.is-on .card-desc { color: var(--accent-fg); opacity: 0.7; }

.indicator-dot {
  position: absolute;
  top: 24px;
  right: 24px;
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: var(--text-muted);
}
.is-on .indicator-dot { background: #10b981; box-shadow: 0 0 10px #10b981; }

/* 底部工具条 */
.config-bar {
  display: flex;
  padding: 16px 24px;
  align-items: center;
  gap: 24px;
}

.config-item {
  flex: 1;
}

.item-label {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 6px;
  color: var(--text-muted);
}

.item-label .icon { width: 14px; height: 14px; }

.select-box select {
  background: transparent;
  border: none;
  color: var(--text-main);
  font-size: 0.9rem;
  font-weight: 500;
  outline: none;
  cursor: pointer;
  padding: 0;
  width: 100%;
}

.config-name {
  font-size: 0.9rem;
  font-weight: 500;
  color: var(--text-main);
}

.v-divider { width: 1px; height: 32px; background: var(--glass-border); }

@keyframes pulse {
  0% { transform: scale(1); opacity: 0.3; }
  50% { transform: scale(1.5); opacity: 0; }
  100% { transform: scale(1); opacity: 0.3; }
}

@keyframes slideUp {
  from { opacity: 0; transform: translateY(12px); }
  to { opacity: 1; transform: translateY(0); }
}

.truncate { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }

.micro-title {
  font-size: 0.7rem;
  text-transform: uppercase;
  letter-spacing: 0.1em;
  font-weight: 600;
  color: var(--text-muted);
}
</style>