<template>
  <div class="app-shell" :class="{ dark: globalState.theme === 'dark' }">
    <div class="drag-bar" style="--wails-draggable:drag">
      <div class="top-actions" style="--wails-draggable:none">
        <div class="window-controls">
          <button @click="WindowMinimise" class="ctrl-btn" title="最小化" v-html="ICONS.min"></button>
          <button @click="handleToggleMaximise" class="ctrl-btn" title="最大化/还原" v-html="isMaximized ? ICONS.restore : ICONS.max"></button>
          <button @click="handleClose" class="ctrl-btn close-btn" title="关闭" v-html="ICONS.close"></button>
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
          <Transition name="page-fade" mode="out-in">
            <div :key="currentTab" class="view-transition-wrapper">
              <Overview v-if="currentTab === 'home'" :traffic="traffic" />

              <Subscriptions v-if="currentTab === 'subs'" />

              <Proxies v-if="currentTab === 'proxies'" />

              <Rules v-if="currentTab === 'rules'" />

              <Connections v-if="currentTab === 'connections'" />

              <div v-if="currentTab === 'logs'" class="view-logs">
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
          </Transition>
        </div>
      </main>
    </div>

    <!-- 全局模态框提示系统 -->
    <Transition name="pop">
      <div v-if="globalState.modal.show" class="modal-overlay" @click.self="handleModalCancel">
        <div class="custom-modal-card" @click.stop>
          <div class="modal-header">
            <h3 :class="{ 'danger-text': globalState.modal.isDanger }">
              {{ globalState.modal.title }}
            </h3>
          </div>
          
          <div class="modal-body">
            <p class="global-modal-msg">{{ globalState.modal.message }}</p>
            
            <div class="modal-footer">
              <template v-if="globalState.modal.type === 'confirm'">
                <button class="action-btn flex-1" @click="handleModalCancel">取消</button>
                <button class="primary-btn accent-btn flex-1" :class="{ 'red-text-btn': globalState.modal.isDanger }" @click="handleModalConfirm">确定</button>
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
import { globalState, initStore } from './store';

const currentTab = ref('home');
const targetSettingsView = ref('main');
const isMaximized = ref(false);

const traffic = ref({ up: '0 B/s', down: '0 B/s' });
const logLines = ref<any[]>([]);
const logBox = ref<HTMLElement | null>(null);

const menu = [
  { id: 'home', label: '控制台', icon: ICONS.home },
  { id: 'proxies', label: '代理节点', icon: ICONS.proxies },
  { id: 'connections', label: '当前连接', icon: ICONS.connections },
  { id: 'logs', label: '实时日志', icon: ICONS.logs },
  { id: 'rules', label: '配置规则', icon: ICONS.rules },
  { id: 'subs', label: '订阅管理', icon: ICONS.subs },
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

const handleToggleMaximise = async () => {
  WindowToggleMaximise();
  // 延迟检查，确保状态已同步
  setTimeout(async () => {
    isMaximized.value = await (window as any).runtime.WindowIsMaximised();
  }, 50);
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
  // 🎯 核心修复 1：正式启动集中式状态管理，将唯一的监听权交给 store.ts
  initStore();

  // ❌ 核心修复 2：彻底删除了 App.vue 原本保留的 EventsOn("app-state-sync") 块！

  try {
    // 🎯 核心修复 3：初始拉取时，也要把新增的三个字段带上
    const state = await (API as any).GetAppState(); 
    if (state) {
      globalState.isRunning = state.isRunning ?? state.IsRunning ?? false;
      globalState.mode = state.mode ?? state.Mode ?? 'rule';
      globalState.theme = state.theme ?? state.Theme ?? 'light'; 
      // 👇 补上新字段的初始化，防止刚打开软件时界面状态错误
      globalState.systemProxy = state.systemProxy ?? state.SystemProxy ?? false;
      globalState.tun = state.tun ?? state.Tun ?? false;
      globalState.version = state.version ?? state.Version ?? '';
      globalState.appVersion = state.appVersion ?? state.AppVersion ?? ''; // 👈 统一初始化
    }
  } catch (e) {
    console.error("获取初始状态失败:", e);
  }

  // 🚀 新增：显式尝试再次拉取应用版本（双重保险）
  try {
    if (!globalState.appVersion) {
      globalState.appVersion = await (API as any).GetAppVersion();
    }
  } catch (e) {}

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
    if (logLines.value.length > 500) logLines.value.shift();
    
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

  // 监听窗口大小变化，同步最大化状态
  window.addEventListener('resize', async () => {
    isMaximized.value = await (window as any).runtime.WindowIsMaximised();
  });

  // 🚀 核心：监听软件更新就绪事件 (由后端在下载完成后触发)
  EventsOn("app-update-ready", (version: string) => {
    globalState.modal = {
      show: true,
      title: "发现新版本就绪",
      message: `GoclashZ 新版本 ${version} 已在后台静默下载完毕。是否立即关闭当前程序并启动安装？`,
      type: "confirm",
      isDanger: false,
      onConfirm: async () => {
        try {
          // 彻底、优雅地自我了断并启动安装包
          await (API as any).ApplyAppUpdate();
        } catch (e) {
          globalState.modal = {
            show: true,
            title: "安装启动失败",
            message: String(e),
            type: "alert",
            isDanger: true,
            onConfirm: null,
            onCancel: null
          };
        }
      },
      onCancel: () => {
        globalState.modal.show = false;
      }
    };
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
/* ================================== */
/* 窗口控制按钮 (右上角)               */
/* ================================== */

/* 🚀 1. 锁死最外层动作区容器 */
.top-actions { 
  display: flex; 
  align-items: center; 
  flex-shrink: 0; 
}

/* 🚀 2. 锁死按钮包裹区 */
.window-controls { 
  display: flex; 
  align-items: center; 
  gap: 6px; /* 恢复间距，让独立的圆角矩形更好看 */
  margin-left: 12px; 
  flex-shrink: 0; 
  min-width: max-content; /* 强制占据所需宽度，绝不参与外部宽度挤压 */
}

/* 🚀 3. 完美还原视觉并双重锁死按钮盒子 */
.ctrl-btn { 
  background: transparent;
  border: none;
  width: 36px;       /* 恢复更协调的宽度 */
  height: 32px; 
  min-width: 36px;   /* 双重锁死：禁止窗口还原瞬间缩小宽度 */
  min-height: 32px;  /* 双重锁死：禁止窗口还原瞬间缩小高度 */
  padding: 0; 
  border-radius: 6px; /* 恢复你喜欢的圆角设计 */
  display: flex; 
  align-items: center; 
  justify-content: center; 
  color: var(--text-sub);
  cursor: pointer; 
  transition: all 0.2s; 
  flex-shrink: 0; 
}

/* 🚀 4. 彻底锁死 SVG 的内部渲染框 */
.ctrl-btn :deep(svg) { 
  width: 12px !important;      /* 恢复图标大小 */
  height: 12px !important; 
  min-width: 12px !important;  /* 终极锁死：确保矢量图在重绘帧中绝对不拉伸 */
  min-height: 12px !important;
  display: block;
  flex-shrink: 0;
}

/* 保留对 "X" 视觉膨胀感的精细微调 */
.close-btn :deep(svg) {
  width: 11.5px !important; 
  height: 11.5px !important;
  min-width: 11.5px !important;
  min-height: 11.5px !important;
}

.ctrl-btn:hover { 
  background: var(--surface-hover); 
  color: var(--text-main); 
}

/* 还原更好看的 Windows 红色 */
.close-btn:hover { 
  background: #E81123 !important; 
  color: #FFFFFF !important; 
}

.app-shell { 
  /* ❌ 删除了这里的变量，因为已经移交给了全局 style.css */
  display: flex; flex-direction: column; height: 100vh; color: var(--text-main); 
}
.drag-bar { height: 42px; display: flex; align-items: center; justify-content: flex-end; padding: 0 8px; }

.icon-btn { background: none; border: none; cursor: pointer; color: var(--text-sub); width: 28px; height: 28px; display: flex; align-items: center; justify-content: center; transition: color 0.2s; }
.icon-btn:hover { color: var(--text-main); }
.icon-btn :deep(svg) { width: 14px; height: 14px; }

.main-layout { 
  display: flex; flex: 1; 
  padding: 0 var(--layout-padding) var(--layout-padding) var(--layout-padding); 
  gap: var(--layout-gap); 
  overflow: hidden; 
}

.content { 
  flex: 1; display: flex; flex-direction: column; 
  padding: var(--content-py) 0 0 0; 
  overflow: hidden; 
}

.content-header {
  /* 绝对对称：直接使用全局变量 */
  padding: 0 var(--content-px);
}
.content-header h1 { font-size: 1.5rem; font-weight: 600; letter-spacing: -0.02em; margin-bottom: 32px; }

.view-scroller { 
  flex: 1; 
  overflow-y: auto; 
  /* 🚀 修复：右侧 padding 减去滚动条的 4px 宽度，实现绝对像素级对称 */
  padding: 0 calc(var(--content-px) - 4px) var(--content-py) var(--content-px); 
}

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

/* 页面切换动画：淡入并向上微移 8px */
.page-fade-enter-active,
.page-fade-leave-active {
  transition: opacity 0.25s ease, transform 0.25s ease;
  overflow: hidden !important; /* 核心修复：过渡期间强制隐藏溢出，防止滚动条闪现 */
}

.page-fade-enter-from {
  opacity: 0;
  transform: translateY(8px); /* 进入时从下方浮现 */
}

.page-fade-leave-to {
  opacity: 0;
  transform: translateY(-8px); /* 离开时向上方消失 */
}

.view-transition-wrapper {
  height: 100%;
  width: 100%;
}

/* 🚀 终极修复：当内部正在执行页面切换动画时，锁死父容器的滚动条 */
.view-scroller:has(.page-fade-enter-active),
.view-scroller:has(.page-fade-leave-active) {
  overflow-y: hidden !important;
}
</style>