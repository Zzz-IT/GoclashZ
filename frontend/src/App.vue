<template>
  <div class="app-shell" :class="{ dark: globalState.theme === 'dark' }">
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
      <Sidebar 
        :activeId="currentTab" 
        :traffic="traffic" 
        :menu="menu" 
        :icons="ICONS"
        @update:activeId="val => currentTab = val" 
      />

      <main class="content card-panel">
        <header class="content-header">
          <h1>{{ activeMenuLabel }}</h1>
        </header>

        <div class="view-scroller">
          <Overview v-if="currentTab === 'home'" :traffic="traffic" />

          <Subscriptions v-if="currentTab === 'subs'" />

          <Proxies v-if="currentTab === 'proxies'" />

          <Rules v-if="currentTab === 'rules'" />

          <Connections v-show="currentTab === 'connections'" />

          <div v-show="currentTab === 'logs'" class="view-logs">
            <div class="terminal-box" ref="logBox">
              <div v-for="(log, i) in logLines" :key="i" :class="['log-line', log.type]">
                <span class="l-time">{{ log.time }}</span>
                <span class="l-type">[{{ log.type.toUpperCase() }}]</span>
                <span class="l-msg">{{ log.payload }}</span>
              </div>
            </div>
          </div>

          <div v-if="currentTab === 'settings'" class="view-settings">
            <Settings :initialView="targetSettingsView" />
          </div>
        </div>
      </main>
    </div>

    <!-- 全局模态框提示系统 -->
    <Transition name="pop">
      <div v-if="globalState.modal.show" class="modal-overlay" @click.self="handleModalCancel">
        <div class="custom-modal-card" @click.stop>
          <div class="modal-header">
            <h3 :class="{ 'danger-text': globalState.modal.type === 'confirm' }">
              {{ globalState.modal.title }}
            </h3>
          </div>
          
          <div class="modal-body">
            <p class="global-modal-msg">{{ globalState.modal.message }}</p>
            
            <div class="modal-footer">
              <template v-if="globalState.modal.type === 'confirm'">
                <button class="action-btn flex-1" @click="handleModalCancel">取消</button>
                <button class="primary-btn accent-btn red-text-btn flex-1" @click="handleModalConfirm">确定</button>
              </template>
              
              <template v-else>
                <button class="primary-btn accent-btn flex-1" style="width: 100%" @click="handleModalConfirm">我知道了</button>
              </template>
            </div>
          </div>
        </div>
      </div>
    </Transition>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, computed, watch, nextTick } from 'vue';
import * as API from '../wailsjs/go/main/App';
import { ICONS } from './utils/icons';
import Sidebar from './components/Sidebar.vue';
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
import { globalState } from './store';

const currentTab = ref('home');
const targetSettingsView = ref('main');

const traffic = ref({ up: '0 B/s', down: '0 B/s' });
const logLines = ref<any[]>([]);
const logBox = ref<HTMLElement | null>(null);

const menu = [
  { id: 'home', label: '控制台', icon: ICONS.home },
  { id: 'subs', label: '订阅管理', icon: ICONS.subs },
  { id: 'proxies', label: '代理节点', icon: ICONS.proxies },
  { id: 'rules', label: '配置规则', icon: ICONS.rules },
  { id: 'connections', label: '当前连接', icon: ICONS.connections },
  { id: 'logs', label: '实时日志', icon: ICONS.logs },
  { id: 'settings', label: '软件设置', icon: ICONS.settings }
];

const modes = [
  { id: 'rule', name: '规则分流' },
  { id: 'global', name: '全局模式' },
  { id: 'direct', name: '直连模式' }
];

const activeMenuLabel = computed(() => menu.find(m => m.id === currentTab.value)?.label);
const currentModeName = computed(() => modes.find(m => m.id === globalState.mode)?.name || '规则分流');

const toggleTheme = () => {
  const newTheme = globalState.theme === 'dark' ? 'light' : 'dark';
  API.SaveThemePreference(newTheme === 'dark');
};

const handleClose = async () => {
  const config = await (API.GetAppBehavior as any)();
  if (config.closeToTray) {
    WindowHide();
  } else {
    Quit();
  }
};

const handleModalConfirm = () => {
  globalState.modal.show = false;
  if (globalState.modal.onConfirm) globalState.modal.onConfirm();
};

const handleModalCancel = () => {
  globalState.modal.show = false;
  if (globalState.modal.onCancel) globalState.modal.onCancel();
};

watch(() => globalState.theme, (val) => {
  if (val === 'dark') {
    document.documentElement.classList.add('dark');
    WindowSetDarkTheme();
    WindowSetBackgroundColour(17, 17, 17, 255);
  } else {
    document.documentElement.classList.remove('dark');
    WindowSetLightTheme();
    WindowSetBackgroundColour(242, 242, 242, 255);
  }
}, { immediate: true });

onMounted(async () => {
  // 1. 初始化同步 (必须赋值给 globalState)
  const state = await (API as any).SyncState();
  if (state) {
    globalState.isRunning = state.isRunning;
  }

  // 2. 监听内核状态变更 (用于实时点亮左下角灯)
  // 修正：将事件名从 "clash-state-changed" 改为 "app-state-sync"
  // 并且因为 app-state-sync 返回的是整个 state 对象，需要结构化赋值
  (window as any).runtime.EventsOn("app-state-sync", (state: any) => {
    globalState.isRunning = state.isRunning;
    globalState.mode = state.mode;
    // 主题等其他状态也会在这里一并更新
  });

  try {
    const status = await API.CheckTunEnv();
    globalState.tunStatus = status as any;
  } catch (e) { console.error("TUN Env Check Error:", e); }

  EventsOn("traffic-data", (data: any) => { traffic.value = data; });

  const history = await (API as any).GetRecentLogs();
  if (history) logLines.value = history;

  API.StartStreamingLogs();
  
  let scrollTimer: ReturnType<typeof setTimeout> | null = null;
  
  EventsOn("log-message", (log: any) => {
    logLines.value.push(log);
    if (logLines.value.length > 1000) logLines.value.shift();
    
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
    globalState.isRunning = false; 
    // 同时也让后端同步一下，确保状态一致
    (API as any).SyncState();
  });
});

watch(currentTab, (newTab) => {
  if (newTab === 'logs') {
    nextTick(() => {
      if (logBox.value) {
        logBox.value.scrollTop = logBox.value.scrollHeight;
      }
    });
  }
});
</script>

<style scoped>
.window-controls { display: flex; align-items: center; gap: 2px; margin-left: 12px; }
.ctrl-btn { width: 28px !important; height: 28px !important; border-radius: 4px; display: flex; align-items: center; justify-content: center; }
.ctrl-btn :deep(svg) { width: 12px; height: 12px; }
.ctrl-btn:hover { background: var(--surface-hover); }
.close-btn:hover { background: var(--text-main) !important; color: var(--accent-fg) !important; }

.app-shell { display: flex; flex-direction: column; height: 100vh; color: var(--text-main); }
.drag-bar { height: 42px; display: flex; align-items: center; justify-content: space-between; padding: 0 8px 0 24px; }
.brand { font-weight: 600; font-size: 0.85rem; letter-spacing: 0.5px; }

.top-actions { display: flex; align-items: center; }

.icon-btn { background: none; border: none; cursor: pointer; color: var(--text-sub); width: 28px; height: 28px; display: flex; align-items: center; justify-content: center; transition: color 0.2s; }
.icon-btn:hover { color: var(--text-main); }
.icon-btn :deep(svg) { width: 14px; height: 14px; }

.main-layout { display: flex; flex: 1; padding: 0 16px 16px 0; gap: 16px; overflow: hidden; }

.content { flex: 1; display: flex; flex-direction: column; padding: 32px 40px; overflow: hidden; }
.content-header h1 { font-size: 1.5rem; font-weight: 600; letter-spacing: -0.02em; margin-bottom: 32px; }
.view-scroller { flex: 1; overflow-y: auto; padding-right: 12px; }

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

.log-line.info .l-type { color: var(--text-main); }
.log-line.warning .l-type { color: var(--text-sub); font-style: italic; }
.log-line.error .l-type { color: var(--text-main); font-weight: 700; }
.log-line.debug .l-type { color: var(--text-muted); }

.view-settings { height: 100%; display: flex; flex-direction: column; }
</style>
.l-time { color: var(--text-muted); margin-right: 12px; opacity: 0.8; }
.l-type { margin-right: 12px; font-weight: 600; }

.log-line.info .l-type { color: var(--text-main); }
.log-line.warning .l-type { color: var(--text-sub); font-style: italic; }
.log-line.error .l-type { color: var(--text-main); font-weight: 700; }
.log-line.debug .l-type { color: var(--text-muted); }

.view-settings { height: 100%; display: flex; flex-direction: column; }
</style>