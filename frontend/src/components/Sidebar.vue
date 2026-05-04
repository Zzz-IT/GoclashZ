<template>
  <aside class="sidebar">
    <div class="sidebar-brand">GoclashZ</div>
    <nav class="nav-list">
      <TransitionGroup name="nav-slide">
        <div v-for="item in menu" :key="item.id"
             v-show="(item.id !== 'logs' || !globalState.hideLogs) && (item.id !== 'proxies' || globalState.mode !== 'direct')"
             :class="['nav-item', { active: activeId === item.id }]"
             @click="$emit('update:activeId', item.id)">
          <span class="icon" v-html="item.icon"></span>
          <span class="nav-label">{{ item.label }}</span>
        </div>
      </TransitionGroup>
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
  traffic: { 
    up: string; 
    down: string; 
    upRaw?: number; 
    downRaw?: number; 
    uploadTotal?: string; 
    downloadTotal?: string; 
    uploadTotalRaw?: number; 
    downloadTotalRaw?: number; 
  };
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
.sidebar { 
  /* 👇 使用我们在 App.vue 根节点定义的变量 (提供 220px 作为降级兜底) */
  width: var(--sidebar-width, 220px); 
  display: flex; 
  flex-direction: column; 
  padding: 8px 12px 12px 12px; 
  flex-shrink: 0; /* 强制防止在极小窗口下被右侧内容挤压变形 */
  transition: width 0.3s cubic-bezier(0.4, 0, 0.2, 1); /* 为未来做"折叠侧边栏"铺垫动画 */
}

.sidebar-brand {
  font-weight: 800;
  font-size: 1.5rem;
  letter-spacing: -0.02em;
  color: var(--text-main);
  text-align: center;
  padding: 8px 0 32px 0;
}

.nav-list { flex: 1; }
.nav-item { 
  display: flex; 
  align-items: center; 
  /* 🚀 魔法 1：再次压缩。左边距微调至 16px，更紧凑 */
  padding: 10px 12px 10px 16px;  
  gap: 12px; 
  margin-bottom: 4px; 
  border-radius: 8px; 
  cursor: pointer; 
  color: var(--text-main); 
  transition: all 0.2s ease; 
  position: relative; 
}

.nav-item:hover { 
  background: var(--surface); 
}

.nav-item.active { 
  background: var(--surface-hover); 
  font-weight: 600; 
}

/* 🚀 魔法 2：无尾箭头锚点 (Chevron Indicator) */
/* 使用精致的 45 度折角代替竖条，更具指向性与科技感 */
.nav-item.active::after {
  content: '';
  position: absolute;
  right: 14px;
  width: 5px;
  height: 5px;
  border-top: 1.5px solid var(--text-main);
  border-right: 1.5px solid var(--text-main);
  transform: rotate(45deg);
  opacity: 0.8;
}

.icon { 
  width: 16px; 
  height: 16px; 
  display: flex; 
  align-items: center; 
  flex-shrink: 0; 
}

.nav-label { 
  font-size: 0.85rem; 
  letter-spacing: 0.05em; 
}

.sidebar-footer { padding: 16px; display: flex; flex-direction: column; gap: 12px; margin-top: auto; }

.icon-box { 
  width: 16px; height: 16px; display: flex; align-items: center; 
  justify-content: center; flex-shrink: 0; font-size: 12px; 
  font-weight: bold; color: var(--text-main); 
}
.icon-box :deep(svg) { width: 14px; height: 14px; }

.side-traffic { display: flex; flex-direction: column; gap: 8px; }
.t-item, .theme-switch-row, .status-indicator { display: flex; align-items: center; gap: 10px; height: 20px; }
.t-label, .theme-switch-row .label { font-size: 0.8rem; color: var(--text-main); white-space: nowrap; flex-shrink: 0; }
.status-text { font-size: 0.8rem; color: var(--text-sub); transition: 0.3s; white-space: nowrap; }
.status-text.online { color: var(--text-main); font-weight: 600; }
.t-val { 
  margin-left: auto; 
  font-family: var(--font-mono); 
  font-size: 0.7rem; 
  color: var(--text-main); 
  opacity: 0.9; 
  white-space: nowrap; 
  flex-shrink: 0; 
  font-variant-numeric: tabular-nums;
}

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
/* ========================================== */
/* 导航菜单交错动效 (Staggered Slide)            */
/* ========================================== */

/* 1. 正在移动的元素 (v-move 是 TransitionGroup 自动提供的类名) */
.nav-slide-move {
  transition: transform 0.4s cubic-bezier(0.4, 0, 0.2, 1);
}

/* 2. 进入与离开的激活状态 */
.nav-slide-enter-active {
  /* 展开时：高度先撑开 (0.3s)，内容延迟淡入 (delay 0.1s) */
  transition: 
    max-height 0.3s cubic-bezier(0.4, 0, 0.2, 1),
    padding 0.3s cubic-bezier(0.4, 0, 0.2, 1),
    margin 0.3s cubic-bezier(0.4, 0, 0.2, 1),
    opacity 0.2s ease-out 0.1s,
    transform 0.2s cubic-bezier(0.4, 0, 0.2, 1) 0.1s;
  overflow: hidden;
}

.nav-slide-leave-active {
  /* 折叠时：内容先淡出位移 (0.2s)，高度延迟收缩 (delay 0.2s) */
  transition: 
    opacity 0.2s ease-in,
    transform 0.2s cubic-bezier(0.4, 0, 0.2, 1),
    max-height 0.3s cubic-bezier(0.4, 0, 0.2, 1) 0.2s,
    padding 0.3s cubic-bezier(0.4, 0, 0.2, 1) 0.2s,
    margin 0.3s cubic-bezier(0.4, 0, 0.2, 1) 0.2s;
  overflow: hidden;
}

/* 3. 初始/结束状态 */
.nav-slide-enter-from,
.nav-slide-leave-to {
  max-height: 0;
  opacity: 0;
  padding-top: 0;
  padding-bottom: 0;
  margin-top: 0;
  margin-bottom: 0;
  transform: translateX(-20px);
}

.nav-slide-enter-to,
.nav-slide-leave-from {
  max-height: 48px; /* 🚀 核心修复：精准匹配 nav-item 的实际高度 (44px + 4px margin) */
  opacity: 1;
  transform: translateX(0);
}
</style>