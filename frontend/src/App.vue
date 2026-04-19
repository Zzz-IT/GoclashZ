<template>
  <div class="app-shell" :class="{ dark: isDark }">
    <div class="drag-bar" style="--wails-draggable:drag">
      <div class="brand">GoclashZ</div>

      <div class="top-actions" style="--wails-draggable:none">
        <div class="window-controls">
          <button @click="WindowMinimise" class="icon-btn ctrl-btn" title="最小化" v-html="ICONS.min"></button>
          <button @click="WindowToggleMaximise" class="icon-btn ctrl-btn" title="最大化" v-html="ICONS.max"></button>
          <button @click="handleClose" class="icon-btn ctrl-btn close-btn" title="关闭" v-html="ICONS.close"></button>
        </div>
      </div>
    </div>

    <div class="main-layout">
      <aside class="sidebar">
        <nav class="nav-list">
          <div v-for="item in menu" :key="item.id"
               v-show="item.id !== 'logs' || !hideLogs"
               :class="['nav-item', { active: currentTab === item.id }]"
               @click="currentTab = item.id">
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
            <span class="icon-box" v-html="isDark ? ICONS.moon : ICONS.sun"></span>
            <span class="label">{{ isDark ? '夜间模式' : '日间模式' }}</span>
          </div>

          <div class="status-indicator">
            <div class="icon-box">
              <div :class="['dot', { online: isRunning }]"></div>
            </div>
            <span class="status-text">{{ isRunning ? '内核已启动' : '服务未运行' }}</span>
          </div>
        </div>
      </aside>

      <main class="content glass-panel">
        <header class="content-header">
          <h1>{{ activeMenuLabel }}</h1>
        </header>

        <div class="view-scroller">
          <Overview v-if="currentTab === 'home'" :traffic="traffic" />

          <Subscriptions v-else-if="currentTab === 'subs'" />

          <Proxies v-else-if="currentTab === 'proxies'" />

          <Rules v-else-if="currentTab === 'rules'" />

          <Connections v-else-if="currentTab === 'connections'" />

          <div v-else-if="currentTab === 'logs'" class="view-logs">
            <div class="terminal-box" ref="logBox">
              <div v-for="(log, i) in logLines" :key="i" :class="['log-line', log.type]">
                <span class="l-time">{{ log.time }}</span>
                <span class="l-type">[{{ log.type.toUpperCase() }}]</span>
                <span class="l-msg">{{ log.payload }}</span>
              </div>
            </div>
          </div>

          <div v-else-if="currentTab === 'settings'" class="view-settings">
            <Settings :initialView="targetSettingsView" />
          </div>
        </div>
      </main>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, computed, watch, nextTick } from 'vue';
import * as API from '../wailsjs/go/main/App';
import Overview from './components/Overview.vue';
import Proxies from './components/Proxies.vue';
import Subscriptions from './components/Subscriptions.vue';
import Connections from './components/Connections.vue';
import Rules from './components/Rules.vue';
import Settings from './components/Settings.vue';
import {
  EventsOn,
  WindowSetLightTheme,
  WindowSetDarkTheme,
  WindowSetBackgroundColour,
  WindowMinimise,
  WindowToggleMaximise,
  WindowHide,
  Quit
} from '../wailsjs/runtime/runtime';

const ICONS = {
  home: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="3" y="3" width="7" height="9"/><rect x="14" y="3" width="7" height="5"/><rect x="14" y="12" width="7" height="9"/><rect x="3" y="16" width="7" height="5"/></svg>`,
  subs: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M10 13a5 5 0 0 0 7.54.54l3-3a5 5 0 0 0-7.07-7.07l-1.72 1.71"/><path d="M14 11a5 5 0 0 0-7.54-.54l-3 3a5 5 0 0 0 7.07 7.07l1.71-1.71"/></svg>`,
  proxies: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polygon points="13 2 3 14 12 14 11 22 21 10 12 10 13 2"/></svg>`,
  logs: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="4 17 10 11 4 5"/><line x1="12" y1="19" x2="20" y2="19"/></svg>`,
  settings: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="3"/><path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1 0 2.83 2 2 0 0 1-2.83 0l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-2 2 2 2 0 0 1-2-2v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83 0 2 2 0 0 1 0-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1-2-2 2 2 0 0 1 2-2h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 0-2.83 2 2 0 0 1 2.83 0l.06.06a1.65 1.65 0 0 0 1.82.33H9a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 2-2 2 2 0 0 1 2 2v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 0 2 2 0 0 1 0 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 2 2 2 2 0 0 1-2 2h-.09a1.65 1.65 0 0 0-1.51 1z"/></svg>`,
  sun: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="5"/><path d="M12 1v2M12 21v2M4.22 4.22l1.42 1.42M18.36 18.36l1.42 1.42M1 12h2M21 12h2M4.22 19.78l1.42-1.42M18.36 5.64l1.42-1.42"/></svg>`,
  moon: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M21 12.79A9 9 0 1 1 11.21 3 7 7 0 0 0 21 12.79z"/></svg>`,
  power: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M18.36 6.64a9 9 0 1 1-12.73 0M12 2v10"></path></svg>`,
  connections: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M18 13v6a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h6"></path><polyline points="15 3 21 3 21 9"></polyline><line x1="10" y1="14" x2="21" y2="3"></line></svg>`,
  rules: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><line x1="8" y1="6" x2="21" y2="6"/><line x1="8" y1="12" x2="21" y2="12"/><line x1="8" y1="18" x2="21" y2="18"/><line x1="3" y1="6" x2="3.01" y2="6"/><line x1="3" y1="12" x2="3.01" y2="12"/><line x1="3" y1="18" x2="3.01" y2="18"/></svg>`,
  min: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5"><line x1="5" y1="12" x2="19" y2="12"/></svg>`,
  max: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5"><rect x="4" y="4" width="16" height="16" rx="2"/></svg>`,
  close: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5"><line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/></svg>`,
};

// 1. 初始化：优先从 localStorage 读取
const isDark = ref(localStorage.getItem('goclashz-theme') === 'dark');
const currentTab = ref('home');
const targetSettingsView = ref('main'); // 用于控制 Settings 子页面

const isRunning = ref(false);
const traffic = ref({ up: '0 B/s', down: '0 B/s' });
const currentMode = ref('rule');
const tunStatus = ref<Record<string, boolean>>({ hasWintun: false, isAdmin: false });
const logLines = ref<any[]>([]);
const logBox = ref<HTMLElement | null>(null);
const hideLogs = ref(false); // 👈 新增：控制日志项显示

const menu = [
  { id: 'home', label: '控制台', icon: ICONS.home },
  { id: 'subs', label: '订阅管理', icon: ICONS.subs },
  { id: 'proxies', label: '代理节点', icon: ICONS.proxies },
  { id: 'rules', label: '配置规则', icon: ICONS.rules },
  { id: 'connections', label: '当前连接', icon: ICONS.connections },
  { id: 'logs', label: '实时日志', icon: ICONS.logs },
  { id: 'settings', label: '系统设置', icon: ICONS.settings }
];

const modes = [
  { id: 'rule', name: '规则分流' },
  { id: 'global', name: '全局模式' },
  { id: 'direct', name: '直连模式' }
];

const activeMenuLabel = computed(() => menu.find(m => m.id === currentTab.value)?.label);
const currentModeName = computed(() => modes.find(m => m.id === currentMode.value)?.name || '规则分流');

const toggleTheme = () => {
  isDark.value = !isDark.value;
  // 2. 保存到前端缓存
  localStorage.setItem('goclashz-theme', isDark.value ? 'dark' : 'light');
  // 3. 调用后端方法持久化到磁盘
  API.SaveThemePreference(isDark.value);
};

const handleClose = async () => {
  const config = await (API.GetAppBehavior as any)();
  if (config.closeToTray) {
    WindowHide();
  } else {
    Quit();
  }
};

// 4. 监听变化，同步处理渲染和原生窗口底色
watch(isDark, (val) => {
  if (val) {
    document.documentElement.classList.add('dark');
    WindowSetDarkTheme();
    WindowSetBackgroundColour(17, 17, 17, 255); // 防止拖拽时的颜色断层 (#111111)
  } else {
    document.documentElement.classList.remove('dark');
    WindowSetLightTheme();
    WindowSetBackgroundColour(242, 242, 242, 255); // (#F2F2F2)
  }
}, { immediate: true });

onMounted(async () => {
  // 确保启动时根据初始状态设置 class
  if (isDark.value) {
    document.documentElement.classList.add('dark');
    WindowSetDarkTheme();
  } else {
    WindowSetLightTheme();
  }
  const s: any = await API.GetProxyStatus();
  isRunning.value = s.systemProxy || s.tun;
  const data: any = await API.GetInitialData();
  if (data) currentMode.value = data.mode;

  // 👉 新增：初始加载配置，判断是否隐藏日志
  const behaviorConf = await (API.GetAppBehavior as any)();
  if (behaviorConf) hideLogs.value = behaviorConf.hideLogs;

  // 👉 新增：监听后端发来的更新事件
  EventsOn("behavior-changed", (config: any) => {
    hideLogs.value = config.hideLogs;
  });

  window.addEventListener('proxy-status-sync', ((e: CustomEvent) => {
    isRunning.value = e.detail.systemProxy || e.detail.tun;
  }) as EventListener);

  try {
    const status = await API.CheckTunEnv();
    tunStatus.value = status as Record<string, boolean>;
  } catch (e) { console.error("TUN Env Check Error:", e); }

  EventsOn("traffic-data", (data: any) => { traffic.value = data; });

  API.StartStreamingLogs();
  
  let scrollTimer: ReturnType<typeof setTimeout> | null = null;
  
  EventsOn("log-message", (log: any) => {
    logLines.value.push({ ...log, time: new Date().toLocaleTimeString('zh-CN', { hour12: false }) });
    if (logLines.value.length > 200) logLines.value.shift();
    
    if (!scrollTimer) {
      scrollTimer = setTimeout(() => {
        if (logBox.value) {
          logBox.value.scrollTop = logBox.value.scrollHeight;
        }
        scrollTimer = null;
      }, 100);
    }
  });

  EventsOn("clash-exited", () => { 
    isRunning.value = false; 
    window.dispatchEvent(new CustomEvent('proxy-status-sync', { detail: { systemProxy: false, tun: false } }));
  });
});
</script>

<style scoped>
/* 窗口控制 */
.window-controls { display: flex; align-items: center; gap: 2px; margin-left: 12px; }
.ctrl-btn { width: 28px !important; height: 28px !important; border-radius: 4px; display: flex; align-items: center; justify-content: center; }
.ctrl-btn :deep(svg) { width: 12px; height: 12px; }
.ctrl-btn:hover { background: var(--surface-hover); }
.close-btn:hover { background: var(--text-main) !important; color: var(--accent-fg) !important; }

/* 基础架构 */
.app-shell { display: flex; flex-direction: column; height: 100vh; color: var(--text-main); }
.drag-bar { height: 42px; display: flex; align-items: center; justify-content: space-between; padding: 0 8px 0 24px; }
.brand { font-weight: 600; font-size: 0.85rem; letter-spacing: 0.5px; }

.top-actions { display: flex; align-items: center; }

.icon-btn { background: none; border: none; cursor: pointer; color: var(--text-sub); width: 28px; height: 28px; display: flex; align-items: center; justify-content: center; transition: color 0.2s; }
.icon-btn:hover { color: var(--text-main); }
.icon-btn :deep(svg) { width: 14px; height: 14px; }

.main-layout { display: flex; flex: 1; padding: 0 16px 16px 0; gap: 16px; overflow: hidden; }

/* 极简侧边栏 */
.sidebar { width: 220px; display: flex; flex-direction: column; padding: 12px; }
.nav-list { flex: 1; }
.nav-item { display: flex; align-items: center; gap: 12px; padding: 10px 14px; margin-bottom: 4px; border-radius: 8px; cursor: pointer; color: var(--text-sub); transition: all 0.2s ease; }
.nav-item:hover { background: var(--surface); color: var(--text-main); }
.nav-item.active { background: var(--surface-hover); color: var(--text-main); font-weight: 600; }
.icon { width: 16px; height: 16px; display: flex; align-items: center; }
.nav-label { font-size: 0.85rem; letter-spacing: 0.02em; }

/* 侧边栏底部容器 */
.sidebar-footer {
  padding: 16px 20px;
  display: flex;
  flex-direction: column;
  gap: 12px;
  border-top: none;
  margin-top: auto;
}

/* 统一的图标容器，用于水平对齐 */
.icon-box {
  width: 16px;
  height: 16px;
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
  font-size: 12px;
  font-weight: bold;
  color: var(--text-muted);
}

.icon-box :deep(svg) {
  width: 14px;
  height: 14px;
}

/* 流量行样式 */
.side-traffic {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.t-item, .theme-switch-row, .status-indicator {
  display: flex;
  align-items: center;
  gap: 12px; /* 图标与文字的间距 */
  height: 20px;
}

.t-label, .theme-switch-row .label, .status-text {
  font-size: 0.8rem;
  color: var(--text-sub);
  white-space: nowrap;
}

.t-val {
  margin-left: auto;
  font-family: var(--font-mono);
  font-size: 0.75rem;
  color: var(--text-main);
  opacity: 0.9;
}

/* 主题切换行 */
.theme-switch-row {
  cursor: pointer;
  transition: opacity 0.2s;
}

.theme-switch-row:hover {
  opacity: 0.7;
}

/* 状态指示样式 */
.status-indicator {
  display: flex;
  align-items: center;
  gap: 12px;
}

/* 状态圆点对齐补丁 */
.dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: var(--text-muted);
  transition: 0.3s;
}
.dot.online {
  background: var(--text-main);
  box-shadow: 0 0 8px var(--text-main);
  animation: breathe 2s ease-in-out infinite;
}

@keyframes breathe {
  0%, 100% { opacity: 0.6; box-shadow: 0 0 4px var(--text-main); }
  50% { opacity: 1; box-shadow: 0 0 12px var(--text-main); }
}

/* 内容区 */
.content { flex: 1; display: flex; flex-direction: column; padding: 32px 40px; overflow: hidden; }
.content-header h1 { font-size: 1.5rem; font-weight: 600; letter-spacing: -0.02em; margin-bottom: 32px; }
.view-scroller { flex: 1; overflow-y: auto; padding-right: 12px; }

/* 日志终端 */
.terminal-box { 
  background: var(--surface);
  color: var(--text-main);
  border: none;
  padding: 20px; 
  border-radius: 8px; 
  height: 500px; 
  overflow-y: auto; 
  font-family: var(--font-mono); 
  font-size: 0.75rem; 
  line-height: 1.6; 
}

.l-time { color: var(--text-muted); margin-right: 12px; opacity: 0.8; }
.l-type { margin-right: 12px; font-weight: 600; }

/* 日志级别：纯灰度，不同粗细/明度区分 */
.log-line.info .l-type { color: var(--text-main); }
.log-line.warning .l-type { color: var(--text-sub); font-style: italic; }
.log-line.error .l-type { color: var(--text-main); font-weight: 700; }
.log-line.debug .l-type { color: var(--text-muted); }

/* 设置组件容器占满 */
.view-settings { height: 100%; display: flex; flex-direction: column; }
</style>