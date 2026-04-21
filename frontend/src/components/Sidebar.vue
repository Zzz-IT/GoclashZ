<template>
  <aside class="sidebar">
    <nav class="nav-list">
      <div v-for="item in menu" :key="item.id"
           v-show="item.id !== 'logs' || !globalState.hideLogs"
           :class="['nav-item', { active: activeId === item.id }]"
           @click="$emit('update:activeId', item.id)">
        <span class="icon" v-html="item.icon"></span>
        <span class="nav-label">{{ item.label }}</span>
      </div>
    </nav>

    <div class="sidebar-footer">
      <div class="side-traffic">
        <div class="t-item">
          <span class="icon-box">↑</span>
          <span class="t-label">上传</span>
          <span class="t-val">{{ traffic.up }}</span>
        </div>
        <div class="t-item">
          <span class="icon-box">↓</span>
          <span class="t-label">下载</span>
          <span class="t-val">{{ traffic.down }}</span>
        </div>
      </div>

      <div class="theme-switch-row" @click="toggleTheme">
        <span class="icon-box" v-html="globalState.theme === 'dark' ? icons.moon : icons.sun"></span>
        <span class="label">{{ globalState.theme === 'dark' ? '黑色模式' : '白色模式' }}</span>
      </div>

      <div class="status-indicator">
        <div class="icon-box">
          <div :class="['dot', { online: globalState.isRunning }]"></div>
        </div>
        <span :class="['status-text', { online: globalState.isRunning }]">{{ globalState.isRunning ? '内核已启动' : '服务未运行' }}</span>
      </div>
    </div>
  </aside>
</template>

<script setup lang="ts">
import { globalState } from '../store';
import * as API from '../../wailsjs/go/main/App';

// 定义 Props 接收外部数据
defineProps<{
  activeId: string;
  traffic: { up: string, down: string };
  menu: Array<{ id: string, label: string, icon: string }>;
  icons: Record<string, string>;
}>();

// 定义 Emit 通知父组件切换标签
const emit = defineEmits(['update:activeId']);

const toggleTheme = () => {
  const newTheme = globalState.theme === 'dark' ? 'light' : 'dark';
  // 🚀 核心修复：乐观 UI 更新
  // 在向后端发送请求前，先修改本地状态，确保 UI 响应瞬间完成，不被后端执行阻塞
  globalState.theme = newTheme;
  API.SaveThemePreference(newTheme === 'dark');
};
</script>

<style scoped>
/* 从 App.vue 迁移并优化的样式 */
.sidebar { width: 220px; display: flex; flex-direction: column; padding: 12px; }
.nav-list { flex: 1; }
.nav-item { 
  display: flex; align-items: center; gap: 12px; padding: 10px 14px; 
  margin-bottom: 4px; border-radius: 8px; cursor: pointer; 
  color: var(--text-main); transition: all 0.2s ease; 
}
.nav-item:hover { background: var(--surface); }
.nav-item.active { background: var(--surface-hover); font-weight: 600; }

.icon { width: 16px; height: 16px; display: flex; align-items: center; }
.nav-label { font-size: 0.85rem; letter-spacing: 0.02em; }

.sidebar-footer { padding: 16px 20px; display: flex; flex-direction: column; gap: 12px; margin-top: auto; }

.icon-box { 
  width: 16px; height: 16px; display: flex; align-items: center; 
  justify-content: center; flex-shrink: 0; font-size: 12px; 
  font-weight: bold; color: var(--text-main); 
}
.icon-box :deep(svg) { width: 14px; height: 14px; }

.side-traffic { display: flex; flex-direction: column; gap: 8px; }
.t-item, .theme-switch-row, .status-indicator { display: flex; align-items: center; gap: 12px; height: 20px; }
.t-label, .theme-switch-row .label { font-size: 0.8rem; color: var(--text-main); white-space: nowrap; }
.status-text { font-size: 0.8rem; color: var(--text-sub); transition: 0.3s; }
.status-text.online { color: var(--text-main); font-weight: 600; }
.t-val { margin-left: auto; font-family: var(--font-mono); font-size: 0.75rem; color: var(--text-main); opacity: 0.9; }

.theme-switch-row { cursor: pointer; transition: opacity 0.2s; }
.theme-switch-row:hover { opacity: 0.7; }

.dot { width: 6px; height: 6px; border-radius: 50%; background: var(--text-muted); transition: 0.3s; }
.dot.online {
  background: var(--text-main);
  box-shadow: 0 0 8px var(--text-main);
  animation: breathe 2s ease-in-out infinite;
}

@keyframes breathe {
  0%, 100% { opacity: 0.6; box-shadow: 0 0 4px var(--text-main); }
  50% { opacity: 1; box-shadow: 0 0 12px var(--text-main); }
}
</style>