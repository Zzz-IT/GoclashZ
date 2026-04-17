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

        <button @click="toggleTheme" class="icon-btn theme-toggle" v-html="isDark ? ICONS.moon : ICONS.sun"></button>

        <div class="window-controls">
          <button @click="handleMinimise" class="icon-btn ctrl-btn" title="最小化" v-html="ICONS.min"></button>
          <button @click="handleToggleMaximise" class="icon-btn ctrl-btn" title="最大化" v-html="ICONS.max"></button>
          <button @click="handleQuit" class="icon-btn ctrl-btn close-btn" title="关闭" v-html="ICONS.close"></button>
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
            <span class="status-text">{{ isRunning ? 'Active' : 'Offline' }}</span>
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
                <div class="micro-title">Core Engine</div>
                <h2 class="cs-title">System Proxy</h2>
                <div class="cs-meta">
                  <span>Port: 7890</span> • <span>Mode: {{ currentMode.toUpperCase() }}</span>
                </div>
              </div>
              <button class="primary-btn" :class="{ stop: isRunning }" @click="toggleProxy">
                <span class="btn-icon" v-html="ICONS.power"></span>
                {{ isRunning ? 'Terminate' : 'Initialize' }}
              </button>
            </div>

            <div class="card-grid">
              <div class="info-card">
                <div class="micro-title">Routing Mode</div>
                <div class="segmented-control">
                  <button v-for="m in ['rule', 'global', 'direct']" :key="m"
                          :class="['seg-btn', { active: currentMode === m }]"
                          @click="changeMode(m)">
                    {{ m.charAt(0).toUpperCase() + m.slice(1) }}
                  </button>
                </div>
              </div>

              <div class="info-card tun-card">
                <div class="micro-title">Network Interface</div>
                <div class="tun-status">
                  <span class="tun-icon" v-html="tunStatus.hasWintun ? ICONS.check : ICONS.tool"></span>
                  <span class="tun-text">{{ tunStatus.hasWintun ? 'Wintun Driver Ready' : 'Driver Missing' }}</span>
                </div>
              </div>
            </div>
          </div>

          <Proxies v-else-if="currentTab === 'proxies'" />

          <div v-else-if="currentTab === 'logs'" class="view-logs">
            <div class="terminal-box" ref="logBox">
              <div v-for="(log, i) in logLines" :key="i" :class="['log-line', log.type]">
                <span class="l-time">{{ log.time }}</span>
                <span class="l-type">[{{ log.type }}]</span>
                <span class="l-msg">{{ log.payload }}</span>
              </div>
            </div>
          </div>

          <div v-else-if="currentTab === 'settings'" class="view-settings">
            <div class="setting-row">
              <div class="s-info">
                <h4 class="s-title">UWP Loopback Exemption</h4>
                <p class="s-desc">Bypass Windows App Store & UWP sandbox isolation.</p>
              </div>
              <button class="outline-btn" @click="fixUWP">Execute</button>
            </div>
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
import {
  EventsOn,
  WindowSetLightTheme,
  WindowSetDarkTheme,
  WindowMinimise,
  WindowToggleMaximise,
  Quit
} from '../wailsjs/runtime/runtime';

const ICONS = {
  home: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="3" y="3" width="7" height="9"/><rect x="14" y="3" width="7" height="5"/><rect x="14" y="12" width="7" height="9"/><rect x="3" y="16" width="7" height="5"/></svg>`,
  proxies: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polygon points="13 2 3 14 12 14 11 22 21 10 12 10 13 2"/></svg>`,
  logs: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="4 17 10 11 4 5"/><line x1="12" y1="19" x2="20" y2="19"/></svg>`,
  settings: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="3"/><path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1 0 2.83 2 2 0 0 1-2.83 0l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-2 2 2 2 0 0 1-2-2v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83 0 2 2 0 0 1 0-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1-2-2 2 2 0 0 1 2-2h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 0-2.83 2 2 0 0 1 2.83 0l.06.06a1.65 1.65 0 0 0 1.82.33H9a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 2-2 2 2 0 0 1 2 2v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 0 2 2 0 0 1 0 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 2 2 2 2 0 0 1-2 2h-.09a1.65 1.65 0 0 0-1.51 1z"/></svg>`,
  sun: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="5"/><line x1="12" y1="1" x2="12" y2="3"/><line x1="12" y1="21" x2="12" y2="23"/><line x1="4.22" y1="4.22" x2="5.64" y2="5.64"/><line x1="18.36" y1="18.36" x2="19.78" y2="19.78"/><line x1="1" y1="12" x2="3" y2="12"/><line x1="21" y1="12" x2="23" y2="12"/><line x1="4.22" y1="19.78" x2="5.64" y2="18.36"/><line x1="18.36" y1="5.64" x2="19.78" y2="4.22"/></svg>`,
  moon: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 12.79A9 9 0 1 1 11.21 3 7 7 0 0 0 21 12.79z"/></svg>`,
  power: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M18.36 6.64a9 9 0 1 1-12.73 0"></path><line x1="12" y1="2" x2="12" y2="12"></line></svg>`,
  check: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="20 6 9 17 4 12"></polyline></svg>`,
  tool: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M14.7 6.3a1 1 0 0 0 0 1.4l1.6 1.6a1 1 0 0 0 1.4 0l3.77-3.77a6 6 0 0 1-7.94 7.94l-6.91 6.91a2.12 2.12 0 0 1-3-3l6.91-6.91a6 6 0 0 1 7.94-7.94l-3.76 3.76z"></path></svg>`,
  min: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"><line x1="5" y1="12" x2="19" y2="12"></line></svg>`,
  max: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"><rect x="4" y="4" width="16" height="16" rx="2"></rect></svg>`,
  close: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"><line x1="18" y1="6" x2="6" y2="18"></line><line x1="6" y1="6" x2="18" y2="18"></line></svg>`
};

const isDark = ref(false);
const currentTab = ref('home');
const isRunning = ref(false);
const traffic = ref({ up: '0.0', down: '0.0' });
const currentMode = ref('rule');
const tunStatus = ref<any>({ hasWintun: false });
const logLines = ref<any[]>([]);
const logBox = ref<HTMLElement | null>(null);

const menu = [
  { id: 'home', label: '控制台', icon: ICONS.home },
  { id: 'proxies', label: '节点', icon: ICONS.proxies },
  { id: 'logs', label: '日志', icon: ICONS.logs },
  { id: 'settings', label: '设置', icon: ICONS.settings }
];

const activeMenuLabel = computed(() => menu.find(m => m.id === currentTab.value)?.label);

const handleMinimise = () => WindowMinimise();
const handleToggleMaximise = () => WindowToggleMaximise();
const handleQuit = () => Quit();

const toggleTheme = () => {
  isDark.value = !isDark.value;
  isDark.value ? WindowSetDarkTheme() : WindowSetLightTheme();
};

const toggleProxy = async () => {
  try {
    isRunning.value ? await API.StopProxy() : await API.RunProxy();
    isRunning.value = await API.GetProxyStatus();
  } catch (e) { alert(e); }
};

const changeMode = async (mode: string) => {
  await API.SetConfigMode(mode);
  currentMode.value = mode;
};

const fixUWP = async () => {
  try {
    await API.FixUWPNetwork();
    alert("Exemption Applied.");
  } catch (e) { alert(e); }
};

const loadInitialData = async () => {
  const data: any = await API.GetInitialData();
  if (data) {
    currentMode.value = data.mode;
  }
};

onMounted(async () => {
  WindowSetLightTheme();
  isRunning.value = await API.GetProxyStatus();
  loadInitialData();
  tunStatus.value = await API.CheckTunEnv();

  EventsOn("traffic-data", (data: any) => { traffic.value = data; });

  API.StartStreamingLogs();
  EventsOn("log-message", (log: any) => {
    logLines.value.push({ ...log, time: new Date().toLocaleTimeString('en-US', { hour12: false }) });
    if (logLines.value.length > 200) logLines.value.shift();
    nextTick(() => { if (logBox.value) logBox.value.scrollTop = logBox.value.scrollHeight; });
  });

  EventsOn("clash-exited", () => { isRunning.value = false; });
});
</script>

<style scoped>
/* 基础架构 */
.app-shell { display: flex; flex-direction: column; height: 100vh; color: var(--text-main); transition: 0.3s background; }
.drag-bar { height: 42px; display: flex; align-items: center; justify-content: space-between; padding: 0 4px 0 24px; }
.brand { font-weight: 600; font-size: 0.8rem; letter-spacing: 0.5px; opacity: 0.8; }

.top-actions { display: flex; align-items: center; gap: 8px; }
.traffic-mon { font-family: var(--font-mono); font-size: 0.7rem; color: var(--text-sub); display: flex; gap: 4px; margin-right: 4px; }
.t-up { color: var(--text-main); }
.t-sep { color: var(--glass-border); }
.t-down { color: var(--text-muted); }

/* 窗口控制样式优化：尺寸更小、更精致 */
.window-controls { display: flex; align-items: center; gap: 2px; }
.ctrl-btn {
  width: 28px !important;
  height: 28px !important;
  border-radius: 4px;
  display: flex;
  align-items: center;
  justify-content: center;
}
.ctrl-btn :deep(svg) {
  width: 12px;
  height: 12px;
  stroke-width: 2.5; /* 增加粗细，即使尺寸变小也清晰 */
}

.theme-toggle {
  width: 28px !important;
  height: 28px !important;
  margin-right: 4px;
}
.theme-toggle :deep(svg) {
  width: 14px;
  height: 14px;
}

.ctrl-btn:hover { background: var(--surface-hover); }
.close-btn:hover { background: #e81123 !important; color: white !important; }

.icon-btn { background: none; border: none; cursor: pointer; color: var(--text-sub); transition: all 0.2s; }
.icon-btn:hover { color: var(--text-main); }

/* 主布局 */
.main-layout { display: flex; flex: 1; padding: 0 16px 16px 0; gap: 16px; overflow: hidden; }

/* 侧边栏 */
.sidebar { width: 220px; display: flex; flex-direction: column; padding: 12px; }
.nav-list { flex: 1; }
.nav-item { display: flex; align-items: center; gap: 12px; padding: 10px 14px; margin-bottom: 4px; border-radius: 8px; cursor: pointer; color: var(--text-sub); transition: all 0.2s ease; border: 1px solid transparent; }
.nav-item:hover { background: var(--surface); color: var(--text-main); }
.nav-item.active { background: var(--surface-hover); color: var(--text-main); font-weight: 500; border-color: var(--glass-border); box-shadow: 0 1px 2px rgba(0,0,0,0.02); }
.icon { width: 16px; height: 16px; display: flex; align-items: center; }
.nav-label { font-size: 0.85rem; letter-spacing: 0.02em; }

.sidebar-footer { padding: 14px; border-top: 1px solid var(--glass-border); margin-top: auto; }
.status-indicator { display: flex; align-items: center; gap: 8px; }
.dot { width: 6px; height: 6px; border-radius: 50%; background: var(--status-offline); transition: 0.3s; }
.dot.online { background: var(--status-online); box-shadow: 0 0 8px var(--status-online); }
.status-text { font-size: 0.75rem; font-family: var(--font-mono); color: var(--text-sub); text-transform: uppercase; letter-spacing: 0.5px; }

/* 内容区 */
.content { flex: 1; display: flex; flex-direction: column; padding: 32px 40px; overflow: hidden; }
.content-header h1 { font-size: 1.5rem; font-weight: 600; letter-spacing: -0.02em; margin-bottom: 32px; }
.view-scroller { flex: 1; overflow-y: auto; padding-right: 12px; }

/* 卡片与按钮 */
.core-status-card { display: flex; justify-content: space-between; align-items: center; padding: 24px; border: 1px solid var(--glass-border); border-radius: 12px; background: var(--surface); margin-bottom: 24px; }
.cs-title { font-size: 1.25rem; font-weight: 500; margin: 4px 0 8px; }
.cs-meta { font-family: var(--font-mono); font-size: 0.75rem; color: var(--text-sub); }

.primary-btn { display: flex; align-items: center; gap: 8px; padding: 10px 20px; border-radius: 6px; border: 1px solid transparent; background: var(--accent); color: var(--accent-fg); font-size: 0.85rem; font-weight: 500; cursor: pointer; transition: all 0.2s; box-shadow: 0 2px 4px rgba(0,0,0,0.1); }
.primary-btn:hover { opacity: 0.85; transform: translateY(-1px); }
.btn-icon { width: 14px; height: 14px; }

.card-grid { display: grid; grid-template-columns: 1fr 1fr; gap: 24px; }
.info-card { padding: 20px; border: 1px solid var(--glass-border); border-radius: 12px; background: var(--surface); }
.segmented-control { display: flex; background: var(--surface-hover); padding: 4px; border-radius: 8px; margin-top: 12px; }
.seg-btn { flex: 1; padding: 6px 0; border: none; background: transparent; border-radius: 6px; font-size: 0.8rem; font-weight: 500; color: var(--text-sub); cursor: pointer; transition: 0.2s; }
.seg-btn.active { background: var(--glass-panel); color: var(--text-main); box-shadow: 0 1px 3px rgba(0,0,0,0.05); }

.tun-status { display: flex; align-items: center; gap: 8px; margin-top: 16px; }
.tun-icon { width: 16px; height: 16px; color: var(--text-main); }
.tun-text { font-size: 0.85rem; font-weight: 500; }

/* 日志终端 */
.terminal-box { background: var(--accent); color: var(--accent-fg); padding: 20px; border-radius: 8px; height: 500px; overflow-y: auto; font-family: var(--font-mono); font-size: 0.75rem; line-height: 1.6; }
.l-time { color: var(--text-muted); margin-right: 12px; opacity: 0.6; }
.l-type { margin-right: 12px; font-weight: 600; }

/* 幽灵按钮 */
.setting-row { display: flex; justify-content: space-between; align-items: center; padding: 20px; border: 1px solid var(--glass-border); border-radius: 12px; background: var(--surface); }
.s-title { font-size: 0.95rem; font-weight: 500; margin-bottom: 4px; }
.s-desc { font-size: 0.8rem; color: var(--text-sub); }
.outline-btn { padding: 8px 16px; border: 1px solid var(--glass-border); background: transparent; color: var(--text-main); font-size: 0.8rem; font-weight: 500; border-radius: 6px; cursor: pointer; transition: 0.2s; }
.outline-btn:hover { background: var(--surface-hover); }
</style>