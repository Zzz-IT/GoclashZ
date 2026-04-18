<template>
  <div class="app-shell" :class="{ dark: isDark }">
    <div class="drag-bar" style="--wails-draggable:drag">
      <div class="brand">GoclashZ</div>

      <div class="top-actions" style="--wails-draggable:none">
        <span class="traffic-mon">
          <span class="t-up">↑ {{ traffic.up }}</span>
          <span class="t-sep">/</span>
          <span class="t-down">↓ {{ traffic.down }}</span>
        </span>

        <button @click="toggleTheme" class="icon-btn theme-toggle" title="切换主题" v-html="isDark ? ICONS.moon : ICONS.sun"></button>

        <div class="window-controls">
          <button @click="WindowMinimise" class="icon-btn ctrl-btn" title="最小化" v-html="ICONS.min"></button>
          <button @click="WindowToggleMaximise" class="icon-btn ctrl-btn" title="最大化" v-html="ICONS.max"></button>
          <button @click="Quit" class="icon-btn ctrl-btn close-btn" title="关闭" v-html="ICONS.close"></button>
        </div>
      </div>
    </div>

    <div class="main-layout">
      <aside class="sidebar">
        <nav class="nav-list">
          <div v-for="item in menu" :key="item.id"
               :class="['nav-item', { active: currentTab === item.id }]"
               @click="currentTab = item.id">
            <span class="icon" v-html="item.icon"></span>
            <span class="nav-label">{{ item.label }}</span>
          </div>
        </nav>

        <div class="sidebar-footer">
          <div class="status-indicator">
            <div :class="['dot', { online: isRunning }]"></div>
            <span class="status-text">{{ isRunning ? '内核已启动' : '服务未运行' }}</span>
          </div>
        </div>
      </aside>

      <main class="content glass-panel">
        <header class="content-header">
          <h1>{{ activeMenuLabel }}</h1>
        </header>

        <div class="view-scroller">
          <div v-if="currentTab === 'home'" class="view-home">
            <div class="core-status-card">
              <div class="cs-info">
                <div class="micro-title">内核状态</div>
                <h2 class="cs-title">系统代理服务</h2>
                <div class="cs-meta">
                  <span>混合端口: 7890</span> • <span>当前模式: {{ currentModeName }}</span>
                </div>
              </div>

              <div class="cs-actions">
                <button class="secondary-btn" @click="goToTunSettings" title="配置虚拟网卡">
                  <span class="btn-icon" v-html="ICONS.network"></span>
                  虚拟网卡
                </button>

                <button class="primary-btn" :class="{ stop: isRunning }" @click="toggleProxy">
                  <span class="btn-icon" v-html="ICONS.power"></span>
                  {{ isRunning ? '断开连接' : '启动代理' }}
                </button>
              </div>
            </div>

            <div class="card-grid">
              <div class="info-card">
                <div class="micro-title">分流规则模式</div>
                <div class="segmented-control">
                  <button v-for="m in modes" :key="m.id"
                          :class="['seg-btn', { active: currentMode === m.id }]"
                          @click="changeMode(m.id)">
                    {{ m.name }}
                  </button>
                </div>
              </div>

              <div class="info-card tun-card">
                <div class="micro-title">网卡驱动检测</div>
                <div class="tun-status">
                  <span class="tun-icon" :class="tunStatus.hasWintun ? 'green-icon' : 'red-icon'" v-html="tunStatus.hasWintun ? ICONS.checkCircle : ICONS.alertCircle"></span>
                  <span class="tun-text">{{ tunStatus.hasWintun ? 'Wintun 驱动正常' : '缺失底层驱动' }}</span>
                </div>
              </div>
            </div>
          </div>

          <Subscriptions v-else-if="currentTab === 'subs'" />

          <Proxies v-else-if="currentTab === 'proxies'" />

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
import { ref, onMounted, computed, nextTick } from 'vue';
import * as API from '../wailsjs/go/main/App';
import Proxies from './components/Proxies.vue';
import Subscriptions from './components/Subscriptions.vue';
import Settings from './components/Settings.vue';
import {
  EventsOn,
  WindowSetLightTheme,
  WindowSetDarkTheme,
  WindowMinimise,
  WindowToggleMaximise,
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
  network: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="2" y="2" width="20" height="8" rx="2" ry="2"></rect><rect x="2" y="14" width="20" height="8" rx="2" ry="2"></rect><line x1="6" y1="6" x2="6.01" y2="6"></line><line x1="6" y1="18" x2="6.01" y2="18"></line></svg>`,
  checkCircle: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M22 11.08V12a10 10 0 1 1-5.93-9.14"></path><polyline points="22 4 12 14.01 9 11.01"></polyline></svg>`,
  alertCircle: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"></circle><line x1="12" y1="8" x2="12" y2="12"></line><line x1="12" y1="16" x2="12.01" y2="16"></line></svg>`,
  min: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5"><line x1="5" y1="12" x2="19" y2="12"/></svg>`,
  max: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5"><rect x="4" y="4" width="16" height="16" rx="2"/></svg>`,
  close: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5"><line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/></svg>`
};

const isDark = ref(false);
const currentTab = ref('home');
const targetSettingsView = ref('main'); // 用于控制 Settings 子页面

const isRunning = ref(false);
const traffic = ref({ up: '0.0', down: '0.0' });
const currentMode = ref('rule');
const tunStatus = ref<Record<string, boolean>>({ hasWintun: false, isAdmin: false });
const logLines = ref<any[]>([]);
const logBox = ref<HTMLElement | null>(null);

const menu = [
  { id: 'home', label: '控制台', icon: ICONS.home },
  { id: 'subs', label: '订阅管理', icon: ICONS.subs },
  { id: 'proxies', label: '代理节点', icon: ICONS.proxies },
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
  isDark.value ? WindowSetDarkTheme() : WindowSetLightTheme();
};

// 导航到 TUN 设置页
const goToTunSettings = () => {
  targetSettingsView.value = 'tun'; // 告诉 Settings 组件默认打开 tun 视图
  currentTab.value = 'settings';
};

// 如果用户点击了侧边栏的 Settings，确保它打开主页
const watchCurrentTab = (newTab: string) => {
  if (newTab === 'settings' && targetSettingsView.value !== 'tun') {
      targetSettingsView.value = 'main';
  } else if (newTab !== 'settings') {
      targetSettingsView.value = 'main'; // 离开设置页时重置状态
  }
};

const toggleProxy = async () => {
  try {
    isRunning.value ? await API.StopProxy() : await API.RunProxy();
    isRunning.value = await API.GetProxyStatus();
  } catch (e) { alert("操作失败: " + e); }
};

const changeMode = async (mode: string) => {
  await API.SetConfigMode(mode);
  currentMode.value = mode;
};

onMounted(async () => {
  WindowSetLightTheme();
  isRunning.value = await API.GetProxyStatus();
  const data: any = await API.GetInitialData();
  if (data) currentMode.value = data.mode;

  try {
    const status = await API.CheckTunEnv();
    tunStatus.value = status as Record<string, boolean>;
  } catch (e) { console.error("TUN Env Check Error:", e); }

  EventsOn("traffic-data", (data: any) => { traffic.value = data; });

  API.StartStreamingLogs();
  
  // ⚠️ 核心修复：增加滚动防抖计时器
  let scrollTimer: ReturnType<typeof setTimeout> | null = null;
  
  EventsOn("log-message", (log: any) => {
    logLines.value.push({ ...log, time: new Date().toLocaleTimeString('zh-CN', { hour12: false }) });
    if (logLines.value.length > 200) logLines.value.shift();
    
    // 聚合 DOM 渲染指令，每 100ms 最多执行一次到底部对齐
    if (!scrollTimer) {
      scrollTimer = setTimeout(() => {
        if (logBox.value) {
          logBox.value.scrollTop = logBox.value.scrollHeight;
        }
        scrollTimer = null;
      }, 100);
    }
  });

  EventsOn("clash-exited", () => { isRunning.value = false; });
});
</script>

<style scoped>
/* 窗口控制 */
.window-controls { display: flex; align-items: center; gap: 2px; margin-left: 12px; }
.ctrl-btn { width: 28px !important; height: 28px !important; border-radius: 4px; display: flex; align-items: center; justify-content: center; }
.ctrl-btn :deep(svg) { width: 12px; height: 12px; }
.ctrl-btn:hover { background: var(--surface-hover); }
.close-btn:hover { background: #e81123 !important; color: white !important; }
.theme-toggle { margin-right: 4px; border: none; background: none; cursor: pointer; color: var(--text-sub); }

/* 基础架构 */
.app-shell { display: flex; flex-direction: column; height: 100vh; color: var(--text-main); }
.drag-bar { height: 42px; display: flex; align-items: center; justify-content: space-between; padding: 0 8px 0 24px; }
.brand { font-weight: 600; font-size: 0.85rem; letter-spacing: 0.5px; }

.top-actions { display: flex; align-items: center; }
.traffic-mon { font-family: var(--font-mono); font-size: 0.75rem; color: var(--text-sub); display: flex; gap: 8px; margin-right: 12px;}

.icon-btn { background: none; border: none; cursor: pointer; color: var(--text-sub); width: 28px; height: 28px; display: flex; align-items: center; justify-content: center; transition: color 0.2s; }
.icon-btn:hover { color: var(--text-main); }
.icon-btn :deep(svg) { width: 14px; height: 14px; }

.main-layout { display: flex; flex: 1; padding: 0 16px 16px 0; gap: 16px; overflow: hidden; }

/* 极简侧边栏 */
.sidebar { width: 220px; display: flex; flex-direction: column; padding: 12px; }
.nav-list { flex: 1; }
.nav-item { display: flex; align-items: center; gap: 12px; padding: 10px 14px; margin-bottom: 4px; border-radius: 8px; cursor: pointer; color: var(--text-sub); transition: all 0.2s ease; border: 1px solid transparent; }
.nav-item:hover { background: var(--surface); color: var(--text-main); }
.nav-item.active { background: var(--surface-hover); color: var(--text-main); font-weight: 500; border-color: var(--glass-border); box-shadow: 0 1px 2px rgba(0,0,0,0.02); }
.icon { width: 16px; height: 16px; display: flex; align-items: center; }
.nav-label { font-size: 0.85rem; letter-spacing: 0.02em; }

/* 底部状态指示 */
.sidebar-footer { padding: 14px; border-top: 1px solid var(--glass-border); margin-top: auto; }
.status-indicator { display: flex; align-items: center; gap: 8px; }
.dot { width: 6px; height: 6px; border-radius: 50%; background: #94a3b8; transition: 0.3s; }
.dot.online { background: #10b981; box-shadow: 0 0 8px #10b981; }
.status-text { font-size: 0.75rem; color: var(--text-sub); }

/* 内容区 */
.content { flex: 1; display: flex; flex-direction: column; padding: 32px 40px; overflow: hidden; }
.content-header h1 { font-size: 1.5rem; font-weight: 600; letter-spacing: -0.02em; margin-bottom: 32px; }
.view-scroller { flex: 1; overflow-y: auto; padding-right: 12px; }

/* Dashboard 卡片 */
.core-status-card { display: flex; justify-content: space-between; align-items: center; padding: 24px; border: 1px solid var(--glass-border); border-radius: 12px; background: var(--surface); margin-bottom: 24px; }
.cs-title { font-size: 1.25rem; font-weight: 600; margin: 4px 0 8px; }
.cs-meta { font-family: var(--font-mono); font-size: 0.8rem; color: var(--text-sub); }

/* 控制台动作区域 */
.cs-actions { display: flex; gap: 12px; align-items: center; }

.primary-btn, .secondary-btn {
  display: flex; align-items: center; gap: 8px; padding: 10px 20px;
  border-radius: 8px; border: none; font-size: 0.85rem; font-weight: 500;
  cursor: pointer; transition: all 0.2s; box-shadow: 0 2px 4px rgba(0,0,0,0.05);
}

.primary-btn { background: var(--accent); color: var(--accent-fg); }
.primary-btn.stop { background: #ef4444; }
.primary-btn:hover { opacity: 0.85; transform: translateY(-1px); }

.secondary-btn { background: var(--surface-hover); color: var(--text-main); border: 1px solid var(--glass-border); }
.secondary-btn:hover { background: var(--glass-panel); border-color: var(--text-sub); }

.btn-icon { width: 14px; height: 14px; }

.card-grid { display: grid; grid-template-columns: 1fr 1fr; gap: 24px; }
.info-card { padding: 20px; border: 1px solid var(--glass-border); border-radius: 12px; background: var(--surface); }
.segmented-control { display: flex; background: var(--surface-hover); padding: 4px; border-radius: 8px; margin-top: 12px; }
.seg-btn { flex: 1; padding: 6px 0; border: none; background: transparent; border-radius: 6px; font-size: 0.8rem; color: var(--text-sub); cursor: pointer; transition: 0.2s; }
.seg-btn.active { background: var(--glass-panel); color: var(--text-main); font-weight: 600; box-shadow: 0 1px 3px rgba(0,0,0,0.05); }

.tun-status { display: flex; align-items: center; gap: 8px; margin-top: 16px; }
.tun-icon { display: inline-flex; align-items: center; justify-content: center; }
.tun-icon :deep(svg) { width: 18px; height: 18px; }
.green-icon { color: #10b981; }
.red-icon { color: #ef4444; }
.tun-text { font-size: 0.85rem; font-weight: 500; }

/* 日志终端 */
.terminal-box { background: var(--accent); color: var(--accent-fg); padding: 20px; border-radius: 8px; height: 500px; overflow-y: auto; font-family: var(--font-mono); font-size: 0.75rem; line-height: 1.6; }
.l-time { color: var(--text-muted); margin-right: 12px; opacity: 0.6; }
.l-type { margin-right: 12px; font-weight: 600; }

/* 设置组件容器占满 */
.view-settings { height: 100%; display: flex; flex-direction: column; }
</style>