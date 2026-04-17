<template>
  <div class="overview-container">
    <div class="glass-card status-card">
      <div class="status-info">
        <div class="indicator" :class="{ active: isRunning }"></div>
        <div>
          <h3>{{ isRunning ? 'GoclashZ 正在运行' : '系统代理已停止' }}</h3>
          <p class="sub-text">安全、快速、轻量级的网络代理接管</p>
        </div>
      </div>
      <button class="primary-btn" :class="{ stop: isRunning }" @click="$emit('toggle')">
        {{ isRunning ? '🛑 停止代理' : '🚀 启动代理' }}
      </button>
    </div>

    <div class="grid-container">
      <div class="glass-card module-card">
        <h4>路由模式 (Mode)</h4>
        <div class="mode-toggles">
          <button class="mode-btn active">规则</button>
          <button class="mode-btn">全局</button>
          <button class="mode-btn">直连</button>
        </div>
      </div>

      <div class="glass-card module-card">
        <h4>TUN 虚拟网卡</h4>
        <p class="sub-text">接管所有非系统代理流量 (全局真路由)</p>
        <button class="outline-btn">检查 TUN 环境</button>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
// 接收来自 App.vue 的状态和事件
defineProps<{ isRunning: boolean }>();
defineEmits(['toggle']);
</script>

<style scoped>
.overview-container { display: flex; flex-direction: column; gap: 20px; }
.status-card { display: flex; justify-content: space-between; align-items: center; padding: 24px; }
.status-info { display: flex; align-items: center; gap: 16px; }
.indicator { width: 14px; height: 14px; border-radius: 50%; background: #ef4444; box-shadow: 0 0 10px #ef4444; transition: 0.3s; }
.indicator.active { background: #10b981; box-shadow: 0 0 10px #10b981; }
h3 { margin: 0 0 4px 0; font-size: 1.2rem; color: var(--text-main); }
.sub-text { margin: 0; font-size: 0.9rem; color: var(--text-sub); }
.primary-btn { padding: 10px 24px; border-radius: 12px; border: none; background: var(--accent); color: white; font-weight: bold; cursor: pointer; transition: 0.2s; }
.primary-btn:hover { opacity: 0.9; transform: translateY(-1px); }
.primary-btn.stop { background: #ef4444; }

.grid-container { display: grid; grid-template-columns: 1fr 1fr; gap: 20px; }
.module-card { padding: 20px; display: flex; flex-direction: column; justify-content: center; }
h4 { margin: 0 0 12px 0; color: var(--text-main); }
.mode-toggles { display: flex; background: rgba(0,0,0,0.05); padding: 4px; border-radius: 10px; }
.dark .mode-toggles { background: rgba(255,255,255,0.05); }
.mode-btn { flex: 1; padding: 8px; border: none; background: transparent; border-radius: 6px; cursor: pointer; color: var(--text-sub); font-weight: bold; transition: 0.2s; }
.mode-btn.active { background: var(--glass-bg); color: var(--accent); box-shadow: 0 2px 5px rgba(0,0,0,0.1); }
.outline-btn { padding: 8px 16px; margin-top: 12px; border: 1px solid var(--accent); background: transparent; color: var(--accent); border-radius: 8px; cursor: pointer; font-weight: bold; transition: 0.2s; }
.outline-btn:hover { background: var(--accent); color: white; }
</style>