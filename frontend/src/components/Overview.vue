<template>
  <div class="overview">
    <div class="status-grid">
      <div class="status-card glass-panel" :class="{ 'is-active': isRunning }">
        <div class="card-icon" v-html="isRunning ? ICONS.powerOn : ICONS.powerOff"></div>
        <div class="card-content">
          <span class="label">系统服务状态</span>
          <h3 class="value">{{ isRunning ? '正在运行' : '已停止' }}</h3>
        </div>
      </div>

      <div class="status-card glass-panel">
        <div class="card-icon" v-html="ICONS.mode"></div>
        <div class="card-content">
          <span class="label">路由模式控制</span>
          <select class="mode-select" v-model="currentMode" @change="handleModeChange">
            <option value="rule">规则 (Rule)</option>
            <option value="global">全局 (Global)</option>
            <option value="direct">直连 (Direct)</option>
          </select>
        </div>
      </div>

      <div class="status-card glass-panel">
        <div class="card-icon" v-html="ICONS.config"></div>
        <div class="card-content">
          <span class="label">当前活跃配置</span>
          <h3 class="value truncate" :title="activeConfigName">
            {{ activeConfigName || '未选择配置' }}
          </h3>
        </div>
      </div>
    </div>

    <div class="traffic-section glass-panel">
      <div class="section-header">
        <h3 class="section-title">实时流量监控</h3>
        <div class="traffic-indicators">
          <div class="indicator up">
            <span class="dot"></span> 上传: {{ traffic.up }}
          </div>
          <div class="indicator down">
            <span class="dot"></span> 下载: {{ traffic.down }}
          </div>
        </div>
      </div>
      <div class="chart-placeholder">
        <p class="hint">内核：{{ clashVersion || 'Mihomo Core' }}</p>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted } from 'vue';
import * as API from '../../wailsjs/go/main/App';

// 图标定义
const ICONS = {
  powerOn: `<svg viewBox="0 0 24 24" fill="none" stroke="#10b981" stroke-width="2"><path d="M18.36 6.64a9 9 0 1 1-12.73 0"></path><line x1="12" y1="2" x2="12" y2="12"></line></svg>`,
  powerOff: `<svg viewBox="0 0 24 24" fill="none" stroke="#64748b" stroke-width="2"><path d="M18.36 6.64a9 9 0 1 1-12.73 0"></path><line x1="12" y1="2" x2="12" y2="12"></line></svg>`,
  config: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"></path><polyline points="14 2 14 8 20 8"></polyline><line x1="16" y1="13" x2="8" y2="13"></line><line x1="16" y1="17" x2="8" y2="17"></line><polyline points="10 9 9 9 8 9"></polyline></svg>`,
  mode: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="10"></circle><polyline points="12 6 12 12 16 14"></polyline></svg>`
};

const isRunning = ref(false);
const activeConfigName = ref('');
const currentMode = ref('rule');
const clashVersion = ref('');
const traffic = ref({ up: '0 B/s', down: '0 B/s' });

// 获取所有初始化数据
const refreshAllData = async () => {
  try {
    const data: any = await API.GetInitialData();
    if (data) {
      // 1. 同步运行状态
      isRunning.value = await API.GetProxyStatus();

      // 2. 同步配置名称
      if (data.activeConfig) {
        activeConfigName.value = data.activeConfig;
      } else if (data.configPath) {
        activeConfigName.value = data.configPath.split(/[\\/]/).pop() || 'config.yaml';
      }

      // 3. 同步模式
      if (data.mode) {
          currentMode.value = data.mode;
      }

      clashVersion.value = data.version || '';
    }
  } catch (e) {
    console.error("加载概览数据失败:", e);
  }
};

// 处理模式切换
const handleModeChange = async () => {
    try {
        // 调用后端统一的更新接口 (兼顾运行时与持久化)
        await API.UpdateClashMode(currentMode.value);
    } catch (e) {
        console.error("模式切换失败:", e);
    }
};

// 事件监听回调：当配置发生改变时
const onConfigChanged = (newName: string) => {
  console.log("概览页同步新配置:", newName);
  activeConfigName.value = newName;
  refreshAllData(); // 重新拉取一次完整状态
};

onMounted(() => {
  refreshAllData();

  // 注册全局监听事件
  (window as any).runtime.EventsOn("config-changed", onConfigChanged);

  // 监听流量数据
  (window as any).runtime.EventsOn("traffic-data", (data: any) => {
    traffic.value = data;
  });
});

onUnmounted(() => {
  // 销毁监听，防止切换页面重复注册
  (window as any).runtime.EventsOff("config-changed");
  (window as any).runtime.EventsOff("traffic-data");
});
</script>

<style scoped>
.overview {
  display: flex;
  flex-direction: column;
  gap: 24px;
}

.status-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(240px, 1fr));
  gap: 20px;
}

.status-card {
  display: flex;
  align-items: center;
  gap: 16px;
  padding: 24px;
  border-radius: 16px;
  background: var(--surface);
  border: 1px solid var(--glass-border);
}

.status-card.is-active {
  border-color: rgba(16, 185, 129, 0.3);
  box-shadow: 0 4px 20px rgba(16, 185, 129, 0.05);
}

.card-icon {
  width: 48px;
  height: 48px;
  display: flex;
  align-items: center;
  justify-content: center;
  background: var(--surface-hover);
  border-radius: 12px;
}

.card-icon :deep(svg) {
  width: 24px;
  height: 24px;
}

.card-content .label {
  font-size: 0.8rem;
  color: var(--text-sub);
  display: block;
  margin-bottom: 4px;
}

.card-content .value {
  font-size: 1.1rem;
  font-weight: 600;
  margin: 0;
}

.mode-select {
    background: transparent;
    border: none;
    color: var(--text-main);
    font-size: 1.1rem;
    font-weight: 600;
    outline: none;
    cursor: pointer;
    padding: 0;
    margin: 0;
    width: 100%;
}

.mode-select option {
    background: var(--surface);
    color: var(--text-main);
}

.truncate {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  max-width: 180px;
}

.traffic-section {
  padding: 24px;
  border-radius: 16px;
  background: var(--surface);
  border: 1px solid var(--glass-border);
  min-height: 200px;
}

.section-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 20px;
}

.section-title {
  font-size: 1rem;
  margin: 0;
}

.traffic-indicators {
  display: flex;
  gap: 16px;
  font-size: 0.85rem;
  font-family: monospace;
}

.indicator {
  display: flex;
  align-items: center;
  gap: 6px;
}

.indicator.up .dot { width: 8px; height: 8px; border-radius: 50%; background: #3b82f6; }
.indicator.down .dot { width: 8px; height: 8px; border-radius: 50%; background: #10b981; }

.chart-placeholder {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  height: 120px;
  color: var(--text-muted);
}

.hint {
  font-size: 0.75rem;
  margin-top: auto;
}
</style>