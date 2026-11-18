<template>
  <div class="app-shell" :class="{ dark: globalState.theme === 'dark' }" :data-lang="currentLocale">
    <!-- 玻璃模式的应用内光晕层（第 3 层）：给卡片的 backdrop-filter 提供可模糊的层次 -->
    <div class="glass-aura" aria-hidden="true"></div>
    <div class="drag-bar" style="--wails-draggable:drag">
      <div class="top-actions" style="--wails-draggable:none">
        <div
          v-if="globalState.appUpdateProgress"
          class="global-update-chip"
          :class="{ downloaded: globalState.appUpdateProgress.isDownloaded }"
          @click="handleGlobalUpdateChipClick"
          :title="globalState.appUpdateProgress.isDownloaded ? t('appUpdate.clickToInstall') : t('appUpdate.viewDetails')"
        >
          <span class="chip-pulse" v-if="!globalState.appUpdateProgress.isDownloaded"></span>
          <span class="chip-icon">
            <svg v-if="!globalState.appUpdateProgress.isDownloaded" viewBox="0 0 24 24" width="13" height="13" fill="none" stroke="currentColor" stroke-width="2.2"><path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"></path><polyline points="7 10 12 15 17 10"></polyline><line x1="12" y1="15" x2="12" y2="3"></line></svg>
            <svg v-else viewBox="0 0 24 24" width="13" height="13" fill="none" stroke="currentColor" stroke-width="2.4"><polyline points="20 6 9 17 4 12"></polyline></svg>
          </span>
          <span class="chip-text">
            <template v-if="!globalState.appUpdateProgress.isDownloaded">
              {{ globalAppUpdatePercent }}% · {{ formatSpeed(globalState.appUpdateProgress.speedBps) }}
            </template>
            <template v-else>
              {{ t('appUpdate.readyToInstall') }}
            </template>
          </span>
        </div>
        <div class="window-controls">
          <button
            class="ctrl-btn"
            :class="{ hovered: ctrlHover === 'min' }"
            :title="t('window.min')"
            @mouseenter="ctrlHover = 'min'"
            @mouseleave="ctrlHover = null"
            @click="handleMinimise"
            v-html="ICONS.min"
          ></button>
          <button
            class="ctrl-btn"
            :class="{ hovered: ctrlHover === 'max' }"
            :title="t('window.max')"
            @mouseenter="ctrlHover = 'max'"
            @mouseleave="ctrlHover = null"
            @click="handleToggleMaximise"
            v-html="isMaximized ? ICONS.restore : ICONS.max"
          ></button>
          <button
            class="ctrl-btn close-btn"
            :class="{ hovered: ctrlHover === 'close' }"
            :title="t('window.close')"
            @mouseenter="ctrlHover = 'close'"
            @mouseleave="ctrlHover = null"
            @click="handleClose"
            v-html="ICONS.close"
          ></button>
        </div>
      </div>
    </div>

    <div class="main-layout">
      <Sidebar 
        :activeId="currentTab" 
        :traffic="traffic" 
        :menu="menu" 
        :icons="ICONS"
        @update:activeId="handleTabSelect" 
      />

      <main class="content card-panel">
        <header class="content-header">
          <div class="content-title-row">
            <h1>
              {{ activeMenuLabel }}
              <span id="title-extra-target"></span>
            </h1>
            <span
              v-if="currentTab === 'yaml-editor'"
              class="yaml-save-status"
              :class="{ modified: yamlEditorModified, error: yamlEditorHasError }"
            >
              {{ yamlEditorStatus }}
            </span>
            <div style="flex: 1"></div>
            <span
              v-if="currentTab === 'yaml-editor' && yamlEditorCursor"
              class="yaml-cursor-status"
            >
              {{ yamlEditorCursor }}
            </span>
          </div>
        </header>

        <div class="view-scroller" :class="[edgeFadeAxis, { 'edge-fade-on': edgeFadeOn }]">
          <Transition :name="paneTransition" @before-enter="onPaneBeforeEnter" @after-enter="onPaneSettled" @after-leave="onPaneSettled">
            <div v-if="currentTab === 'home'" key="home" class="view-transition-wrapper">
              <Overview :traffic="traffic" />
            </div>

            <div v-else-if="currentTab === 'subs'" key="subs" class="view-transition-wrapper">
              <Subscriptions @edit-config="openYamlEditor" />
            </div>

            <div v-else-if="currentTab === 'yaml-editor'" key="yaml-editor" class="view-transition-wrapper">
              <YamlEditor :config-id="editingConfigId" :config-name="editingConfigName" :config-type="editingConfigType" @back="closeYamlEditor" @status-change="handleYamlStatusChange" @cursor-change="handleYamlCursorChange" />
            </div>

            <div v-else-if="currentTab === 'proxies'" key="proxies" class="view-transition-wrapper">
              <Proxies />
            </div>

            <div v-else-if="currentTab === 'rules'" key="rules" class="view-transition-wrapper">
              <Rules />
            </div>

            <div v-else-if="currentTab === 'connections'" key="connections" class="view-transition-wrapper">
              <Connections />
            </div>

            <div v-else-if="currentTab === 'logs'" key="logs" class="view-transition-wrapper view-logs ui-page-shell">
              <div class="logs-header ui-page-header" style="display: flex; justify-content: space-between; align-items: center;">
                <div class="conn-tabs-viewport" style="flex: none;">
                  <div class="conn-tabs-track" ref="logsTabsTrackRef">
                    <button :ref="(el) => { if (logSourceFilter === 'all') logsTabEl = el as HTMLElement | null }" :class="['conn-tab-btn', { active: logSourceFilter === 'all' }]" @click="logSourceFilter = 'all'">{{ t('logs.all') }}</button>
                    <button :ref="(el) => { if (logSourceFilter === 'core') logsTabEl = el as HTMLElement | null }" :class="['conn-tab-btn', { active: logSourceFilter === 'core' }]" @click="logSourceFilter = 'core'">{{ t('logs.core') }}</button>
                    <button :ref="(el) => { if (logSourceFilter === 'app') logsTabEl = el as HTMLElement | null }" :class="['conn-tab-btn', { active: logSourceFilter === 'app' }]" @click="logSourceFilter = 'app'">{{ t('logs.app') }}</button>
                    <!-- H-07：后端 LogSource 有 std（标准库收编进来的内核输出），原来没有入口，按来源筛选时这批日志静默消失 -->
                    <button :ref="(el) => { if (logSourceFilter === 'std') logsTabEl = el as HTMLElement | null }" :class="['conn-tab-btn', { active: logSourceFilter === 'std' }]" @click="logSourceFilter = 'std'">{{ t('logs.std') }}</button>
                    <div class="conn-tab-slider" :class="{ animated: logsSliderReady }" v-show="logsSliderVisible" :style="logsSliderStyle"></div>
                  </div>
                </div>
                <div class="logs-tools">
                  <label class="log-search-box">
                    <span class="log-search-icon" v-html="ICONS.search"></span>
                    <input v-model="logQuery" type="text" :placeholder="t('common.search')" spellcheck="false" />
                  </label>
                  <!-- H-23：aria-label 原来误用 logs.all（「全部」是来源 tab 的文案），读屏会把级别下拉读成来源控件 -->
                  <select v-model="logLevelFilter" class="log-level-select" :aria-label="t('logs.levelFilter')">
                    <option value="all">{{ t('logs.all') }}</option>
                    <option value="debug">DEBUG</option>
                    <option value="info">INFO</option>
                    <option value="warn">WARN</option>
                    <option value="error">ERROR</option>
                  </select>
                  <button class="action-btn" :title="logPaused ? t('connections.resumeRefresh') : t('connections.pauseRefresh')" @click="toggleLogPause">
                    <span class="btn-icon" v-html="logPaused ? ICONS.play : ICONS.pause"></span>
                    {{ logPaused ? t('connections.resumeRefresh') : t('connections.pauseRefresh') }}
                  </button>
                  <button class="action-btn" :title="t('common.copy')" @click="handleCopyLogs">
                    {{ t('common.copy') }}
                  </button>
                  <button class="action-btn" :title="t('settings.diagnostics.logDir')" @click="handleCopyLogDir">
                    <span class="btn-icon" v-html="ICONS.folder"></span>
                  </button>
                </div>
              </div>
              <div class="terminal-box ui-page-body" ref="logBox" @scroll.passive="handleLogScroll">
                <!-- H-05：key 改用自增 seq（后端 LogEntry 没有 id），裁剪头部时不再整表错位重渲染；
                     H-06：级别列用归一后的类型，后端只产 warn，原来的 .warning 样式永不命中 -->
                <div v-for="log in filteredLogLines" :key="log.seq" :class="['log-line', normalizeLogType(log.type)]">
                  <span class="l-time" :title="log.ts || log.time">{{ logTimeLabel(log) }}</span>
                  <span class="l-type">[{{ log.type.toUpperCase() }}]</span>
                  <span v-if="log.cat" class="l-cat">[{{ log.cat }}]</span>
                  <span class="l-msg" :title="logDetailTitle(log)">{{ log.payload }}</span>
                </div>
              </div>
              <div class="logs-footer" style="display: flex; justify-content: flex-end;">
                <button class="action-btn" :title="t('logs.clear')" @click="handleClearLogs">
                  <span class="btn-icon" v-html="ICONS.trash"></span>
                  {{ t('logs.clear') }}
                </button>
              </div>
            </div>

            <div v-else-if="currentTab === 'settings'" key="settings" class="view-transition-wrapper view-settings">
              <Settings :initialView="targetSettingsView" />
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
            <div v-if="globalState.modal.detail" class="global-modal-detail">{{ globalState.modal.detail }}</div>
            
            <div class="modal-footer">
              <template v-if="globalState.modal.type === 'confirm'">
                <button class="action-btn flex-1" @click="handleModalCancel">{{ globalState.modal.cancelText || t('common.cancel') }}</button>
                <button class="primary-btn accent-btn flex-1" :class="{ 'red-text-btn': globalState.modal.isDanger }" @click="handleModalConfirm">{{ globalState.modal.confirmText || t('common.confirm') }}</button>
              </template>
              
              <template v-else>
                <button class="primary-btn accent-btn flex-1" style="width: 100%" @click="handleModalConfirm">{{ t('common.gotIt') }}</button>
              </template>
            </div>
          </div>
        </div>
      </div>
    </Transition>

    <!-- 轻提示（成功/中性反馈，不打断操作） -->
    <Toast />

    <!-- 种子内核对比更新提示卡片 -->
    <Transition name="pop">
      <div v-if="showCoreSeedModal" class="modal-overlay" @click.self="handleCoreSeedCancel">
        <div class="custom-modal-card core-seed-modal" @click.stop>
          <div class="modal-header">
            <h3>{{ t('coreSeedPrompt.title') }}</h3>
          </div>
          
          <div class="modal-body">
            <p class="global-modal-msg">{{ t('coreSeedPrompt.message') }}</p>
            <div class="core-compare-card">
              <div class="core-compare-row">
                <span class="compare-tag-label">{{ t('coreSeedPrompt.currentVersion') }}:</span>
                <span class="version-badge current-badge">{{ currentCoreVer || t('settings.update.installedUnknownVersion') }}</span>
              </div>
              <div class="core-compare-row">
                <span class="compare-tag-label">{{ t('coreSeedPrompt.seedVersion') }}:</span>
                <span class="version-badge seed-badge">{{ seedCoreVer }}</span>
              </div>
            </div>
            
            <div class="modal-footer core-seed-footer">
              <button class="action-btn" @click="handleCoreSeedNever">{{ t('coreSeedPrompt.never') }}</button>
              <button class="primary-btn accent-btn" :disabled="replacingCore" @click="handleCoreSeedConfirm">
                {{ replacingCore ? t('coreSeedPrompt.replacing') : t('common.confirm') }}
              </button>
              <button class="action-btn" @click="handleCoreSeedCancel">{{ t('common.cancel') }}</button>
            </div>
          </div>
        </div>
      </div>
    </Transition>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted, computed, watch, nextTick } from 'vue';
import { App } from './api';
import { ICONS } from './utils/icons';
import { t, currentLocale } from './locales';
import { formatSpeed } from './utils/format';
import { installHoverGuard } from './utils/hoverGuard';
import { initScrollFades } from './utils/scrollFade';
import { useEdgeFade } from './utils/edgeFade';
import { applyGlassEffect } from './utils/glass';
import { reportError } from './utils/report';
import { copyText } from './utils/clipboard';
import Sidebar from './components/Sidebar.vue';
import Overview from './components/Overview.vue';
import Proxies from './components/Proxies.vue';
import Subscriptions from './components/Subscriptions.vue';
import Connections from './components/Connections.vue';
import Rules from './components/Rules.vue';
import Settings from './components/Settings.vue';
import YamlEditor from './components/YamlEditor.vue';
import Toast from './components/Toast.vue';
import { 
  startWaveSampling, 
  stopWaveSampling, 
  updateLatestTraffic, 
  resetWaveState 
} from './trafficWaveState';
import { Application, Events, Window } from '@wailsio/runtime';
import { globalState, themeState, updateStateFromBackend, showAlert, showConfirm, showToast, syncThemeFromBehavior, bindSystemTheme, openModal, confirmModal, cancelModal } from './store';

const currentTab = ref('home');

/**
 * 页面过渡方向。轴由导航层级决定，不是由动画本身决定：
 *
 *   平级切页（侧栏之间）—— 竖向。内容上下流动，顺序取自 NAV_ORDER，
 *     切到更靠后的页内容向上走（pane-up），切回向下的走 pane-down。
 *   下钻/返回（父页 ↔ 子页）—— 横向。进入子页内容左推（pane-left），
 *     返回内容右推（pane-right）。
 *
 * 之所以分轴：平级切页之间没有空间关系，竖向只表达"顺序"；
 * 而父子页有真实层级，横向才表达"进一层/退一层"。
 */
const NAV_ORDER = ['home', 'proxies', 'connections', 'logs', 'rules', 'subs', 'settings'];

/** 子页 → 父页。放进这张表的跳转一律走横向。 */
const DRILL_PARENT: Record<string, string> = {
  'yaml-editor': 'subs',
};

const paneTransition = ref('pane-up');

function resolvePaneTransition(next: string, prev: string): string {
  if (DRILL_PARENT[next] === prev) return 'pane-left';
  if (DRILL_PARENT[prev] === next) return 'pane-right';
  const from = NAV_ORDER.indexOf(prev);
  const to = NAV_ORDER.indexOf(next);
  return from >= 0 && to >= 0 && to < from ? 'pane-down' : 'pane-up';
}

/* 裁切处淡出：方向在"切页的那一刻"定，先于动画生效；撑住与收起全部由
   useEdgeFade 的计时器驱动，不接过渡结束钩子。
   原因：连切时上一个过渡的 after-leave 会在新过渡跑到一半时才触发，
   它拿不到"我属于哪一次过渡"，会把当前的淡化带提前收掉 —— 表现为
   "带子跟不上，露出硬切"。详见 utils/edgeFade.ts。 */
const {
  axis: edgeFadeAxis,
  on: edgeFadeOn,
  begin: edgeFadeBegin,
} = useEdgeFade();

watch(currentTab, (next, prev) => {
  paneTransition.value = resolvePaneTransition(next, prev);
  /* 纵向淡上下两条边，横向淡左右两条边 */
  const horizontal =
    paneTransition.value === 'pane-left' || paneTransition.value === 'pane-right';
  edgeFadeBegin(horizontal ? 'edge-fade-x' : 'edge-fade-y');
});

const onPaneBeforeEnter = () => {
  resetViewScroller();
};

/* 过渡落定后只做滚动归位（日志页追到最新一行）。
   淡化带的收尾不在这里处理，交给 useEdgeFade 的计时器 —— 钩子会被旧过渡误触发。 */
const onPaneSettled = () => {
  resetViewScroller();
};

const targetSettingsView = ref('main');
const editingConfigId = ref('');
const editingConfigName = ref('');
const editingConfigType = ref<'local' | 'remote'>('local');
const isMaximized = ref(false);
// 窗口控制按钮的 hover 由 JS 管理：窗口隐藏/恢复时鼠标位置不可靠，
// 不再使用 CSS :hover，避免隐藏后恢复仍停在旧 hover 状态。
const ctrlHover = ref<'min' | 'max' | 'close' | null>(null);
const clearCtrlHover = () => { ctrlHover.value = null; };
const yamlEditorStatus = ref(t('yamlEditor.saved'));
const yamlEditorModified = ref(false);
const yamlEditorHasError = ref(false);
const yamlEditorCursor = ref('');

const showCoreSeedModal = ref(false);
const currentCoreVer = ref('');
const seedCoreVer = ref('');
const replacingCore = ref(false);

const globalAppUpdatePercent = computed(() => {
  const p = globalState.appUpdateProgress;
  if (!p || !p.totalBytes || p.totalBytes <= 0) return 0;
  return Math.min(100, Math.floor((p.bytesDone / p.totalBytes) * 100));
});

/* H-03 / H-11：更新动作（下载 / 安装）的在途闸门。
   确认框会 await 让出控制权，闸门若放在 openModal 之后，连点就会排成两个确认框、
   各自执行一次安装（后端真的会被启动两次）。所以闸门必须提到 openModal 之前，
   在 onConfirm 的 finally 与 onCancel 里复位；重复点击给提示而不是静默无视。 */
const appUpdateActionBusy = ref(false);

// H-11：重复点击的提示文案复用设置页已有键（settings.update.appUpdateBusy），8 语言都齐。
const showAppUpdateBusyHint = () => {
  showToast(t('settings.update.appUpdateBusy'), 'info');
};

// H-12：同一个「安装」动作，chip 路径原本对空 path 静默 return，事件路径却弹错误框 —— 统一到这里。
const openInstallPathEmptyAlert = () => {
  void openModal({
    type: 'alert',
    title: t('appUpdate.installPathEmptyTitle'),
    message: t('appUpdate.installPathEmptyMsg'),
    isDanger: true,
  });
};

const handleGlobalUpdateChipClick = () => {
  const p = globalState.appUpdateProgress;
  if (!p) return;
  if (p.isDownloaded) {
    if (appUpdateActionBusy.value) {
      showAppUpdateBusyHint();
      return;
    }
    appUpdateActionBusy.value = true;
    void openModal({
      type: 'confirm',
      title: t('appUpdate.downloadCompleteTitle'),
      message: t('appUpdate.downloadCompleteMsg', { version: p.version || '' }),
      onConfirm: async () => {
        try {
          if (!p.path) {
            openInstallPathEmptyAlert();
            return;
          }
          await App.ApplyAppUpdate(p.path);
        } catch (e: any) {
          showAlert(String(e?.message || e || t('common.error')), t('common.error'), true);
        } finally {
          appUpdateActionBusy.value = false;
        }
      },
      // 取消 / 点遮罩也要复位，否则闸门永远关着，之后每次点 chip 都只弹提示。
      onCancel: () => { appUpdateActionBusy.value = false; },
    });
  } else {
    // 先写目标子页再切页，避免侧栏/host 顺序导致设置页先建在 main 再跳 about；
    // 走 requestTabChange 是为了让「YAML 未保存」守卫同样拦得住这次跳转（C-02）。
    void requestTabChange('settings', () => { targetSettingsView.value = 'about'; });
  }
};

const traffic = ref({ 
  up: '0 B/s', 
  down: '0 B/s',
  upRaw: 0,
  downRaw: 0,
  uploadTotal: '0 B',
  downloadTotal: '0 B',
  uploadTotalRaw: 0,
  downloadTotalRaw: 0
});
const logLines = ref<any[]>([]);
const logSourceFilter = ref('all');
// F-A3-16：搜索 / 级别 / 暂停
const logQuery = ref('');
const logLevelFilter = ref('all');
const logPaused = ref(false);

/* 级别档位表（同时服务「日志行类型」与「全局级别地板」两处）。
   - silent=4：它不是"更低级别"而是"全部静音"，rank 排在 error 之上 —— 作为地板时任何条目都被过滤，
     与 store.normalizeLogLevel 保留 silent 的口径一致（S-09）。
   - 表里**不放 'warning'**：rankOf 会先过 normalizeLogType，而它已把 warning 归一成 warn（H-06），
     所以 warning 只可能是"归一化之前"的形态，放进表里就是死键。 */
const logLevels = { debug: 0, info: 1, warn: 2, error: 3, silent: 4 };

const normalizeLogType = (t?: string) => {
  const v = String(t || 'info').toLowerCase().trim();
  if (v === 'warning') return 'warn';
  if (v === 'trace') return 'debug';
  if (v === 'fatal' || v === 'panic') return 'error';
  if (['debug', 'info', 'warn', 'error', 'silent'].includes(v)) return v;
  return 'info';
};

const rankOf = (level?: string) => {
  const normalized = normalizeLogType(level);
  return logLevels[normalized as keyof typeof logLevels] ?? 1;
};

const filteredLogLines = computed(() => {
  const query = logQuery.value.trim().toLowerCase();
  const filterRank = logLevelFilter.value === 'all' ? 0 : rankOf(logLevelFilter.value);

  return logLines.value.filter(log => {
    // H-07：缺 source 的历史条目按「未知」处理，不再冒认成 core（std 同理要能单独筛出来）。
    const source = String(log.source || 'unknown').toLowerCase();
    const type = normalizeLogType(log.type);

    if (logSourceFilter.value !== 'all' && source !== logSourceFilter.value) {
      return false;
    }

    const entryRank = rankOf(type);
    /* H-08：只有在下拉里选定「具体级别」时才套全局地板。
       选「全部」却仍被全局级别压制，会让控件标签（全部）和实际行为（只有 error 及以上）自相矛盾。
       注：silent 地板 rank=4，此时选任何具体级别都过滤干净（S-09）。 */
    if (logLevelFilter.value !== 'all') {
      // 全局设置级别是「地板」，面板顶部下拉只能进一步收紧（F-A3-16）。
      const settingsFloor = source === 'core'
        ? rankOf(globalState.logLevel)
        : rankOf(globalState.appLogLevel);
      if (entryRank < Math.max(settingsFloor, filterRank)) {
        return false;
      }
    }

    if (query) {
      const haystack = (
        String(log.payload || '') + ' ' +
        String(log.cat || '') + ' ' +
        String(log.code || '') + ' ' +
        String(log.event || '')
      ).toLowerCase();
      if (!haystack.includes(query)) {
        return false;
      }
    }

    return true;
  });
});
const logBox = ref<HTMLElement | null>(null);

/* H-04：日志面板是否「跟着最新一行走」。
   以前每次落批都无条件滚到底，core 开到 info/debug 时用户根本没机会向上翻历史错误。
   现在由滚动位置决定：贴底（距底 < 24px）才跟，用户一上翻就停止拽回，翻回底部自动恢复。 */
const LOG_FOLLOW_THRESHOLD = 24;
const logFollowTail = ref(true);
const handleLogScroll = () => {
  const el = logBox.value;
  if (!el) return;
  logFollowTail.value = el.scrollHeight - el.scrollTop - el.clientHeight < LOG_FOLLOW_THRESHOLD;
};

const logsTabsTrackRef = ref<HTMLElement | null>(null);
const logsTabEl = ref<HTMLElement | null>(null);
const logsSliderStyle = ref({ left: '0px', width: '0px' });
const logsSliderReady = ref(false);
const logsSliderVisible = ref(false);

const updateLogsSlider = () => {
  const track = logsTabsTrackRef.value;
  const btn = logsTabEl.value;
  if (track && btn) {
    logsSliderStyle.value = {
      left: `${btn.offsetLeft}px`,
      width: `${btn.offsetWidth}px`,
    };
  }
};

const resetLogsSlider = () => {
  logsSliderReady.value = false;
  logsSliderVisible.value = false;
  nextTick(() => {
    updateLogsSlider();
    nextTick(() => {
      logsSliderVisible.value = true;
      logsSliderReady.value = true;
    });
  });
};

watch(logSourceFilter, () => {
  nextTick(updateLogsSlider);
});

watch(logsTabsTrackRef, (el) => {
  if (el) {
    resetLogsSlider();
  }
});

let scrollTimer: ReturnType<typeof setTimeout> | null = null;
let appUpdateCheckFailsafeTimer: ReturnType<typeof setTimeout> | null = null;

/* ---- 日志面板：批量合并 + 时间列 ---- */

// 后端每条日志一次事件，逐条 push 会让 filteredLogLines 每次重算并触发重渲染，
// core 开到 info/debug 时明显掉帧；这里合并成一批（F-A3-10）。
// H-05 / C-13：批窗口从 16ms 提到 50ms。16ms 意味着满速时每秒 60 次「2000 行全量 filter + 整表 diff」，
// 只合并了 push 次数没降低单次渲染成本；50ms（3 帧）对肉眼仍是实时，计算量降到约 1/3。
const LOG_BATCH_MS = 50;
// 与后端 AppLogs 主环（2000，含 500 条 warn/error 保留区）对齐（F-A3-11）。
const LOG_MAX_LINES = 2000;
let logBatch: any[] = [];
let logBatchTimer: ReturnType<typeof setTimeout> | null = null;
// 暂停期间累积的日志（恢复时合并，上限与显示上限一致）。
let logPausedBuffer: any[] = [];

// 时间列：优先显示后端 TS（RFC3339 毫秒、含日期与时区），旧条目回落到 HH:MM:SS。
// TS 形如 2026-11-16T19:50:44.123+08:00 → 显示 "11-16 19:50:44.123"。
const logTimeLabel = (log: any): string => {
  const ts = typeof log?.ts === 'string' ? log.ts : '';
  if (ts.length >= 19) return ts.slice(5, 23).replace('T', ' ');
  return log?.time || '';
};

// 悬停显示结构化字段（cat/event/code/corr/dur/attempt），面板本身只显示 cat 徽标。
const logDetailTitle = (log: any): string => {
  if (!log) return '';
  const parts: string[] = [];
  if (log.ts) parts.push(String(log.ts));
  if (log.event) parts.push('event=' + log.event);
  if (log.code) parts.push('code=' + log.code);
  if (log.corr) parts.push('corr=' + log.corr);
  if (log.durMs) parts.push('dur=' + log.durMs + 'ms');
  if (log.attempt) parts.push('attempt=' + log.attempt);
  if (log.caller) parts.push('at ' + log.caller);
  return parts.join('\n');
};

/* H-05：日志条目没有 id（后端 LogEntry 也没有 seq），列表却拿 index 当 key；
   头部裁剪后所有 key 平移，Vue 只能把整表当成「同位置换了内容」重 patch，滚动锚点也一起丢。
   这里在入列时补一个自增序号，模板用它做稳定 key。 */
let logSeq = 0;
const tagLogLine = (log: any) => {
  log.seq = ++logSeq;
  return log;
};

// H-09 合并去重用的内容指纹：同一行只可能分别来自「历史快照」与「实时推送」两路。
const logLineKey = (log: any): string =>
  [log?.ts ?? '', log?.type ?? '', log?.source ?? '', log?.payload ?? ''].join('\u0000');

/* H-09：GetRecentLogs 原来是整体赋值 logLines，await 期间经 log-message 送进来的活日志
   会被整片覆盖掉（后端不重发），启动瞬间日志列表会出现一段空洞。
   这里改为「历史在前 + 活日志在后」合并，并按指纹去重，序号在此一并补齐。 */
const mergeHistoryLogs = (history: any[] | null | undefined) => {
  if (!Array.isArray(history) || history.length === 0) return;
  const seen = new Set<string>();
  for (const line of logLines.value) seen.add(logLineKey(line));
  for (const line of logBatch) seen.add(logLineKey(line));
  for (const line of logPausedBuffer) seen.add(logLineKey(line));
  const prefix = history.filter((line) => !seen.has(logLineKey(line)));
  prefix.forEach(tagLogLine);
  const merged = prefix.concat(logLines.value);
  logLines.value = merged.length > LOG_MAX_LINES ? merged.slice(-LOG_MAX_LINES) : merged;
};

const flushLogBatch = () => {
  logBatchTimer = null;
  if (logBatch.length === 0) return;
  /* H-04：是否跟随必须在 push 之前判定 —— push 之后 scrollHeight 立刻变大，
     「距底距离」会先超过阈值，正在贴底阅读的用户也会被误判成「已经上翻」。 */
  const followTail = logFollowTail.value;
  logLines.value.push(...logBatch);
  logBatch = [];
  const overflow = logLines.value.length - LOG_MAX_LINES;
  if (overflow > 0) logLines.value.splice(0, overflow);
  if (!followTail) return;
  if (!scrollTimer) {
    scrollTimer = setTimeout(() => {
      logBox.value?.scrollTo({ top: logBox.value.scrollHeight });
      scrollTimer = null;
    }, 100);
  }
};

const enqueueLogLine = (log: any) => {
  // 暂停期间不打断阅读：先进缓冲，恢复时一次性合并（上限同 LOG_MAX_LINES）。
  if (logPaused.value) {
    if (logPausedBuffer.length < LOG_MAX_LINES) logPausedBuffer.push(tagLogLine(log));
    return;
  }
  logBatch.push(tagLogLine(log));
  if (logBatchTimer === null) {
    logBatchTimer = setTimeout(flushLogBatch, LOG_BATCH_MS);
  }
};

const toggleLogPause = () => {
  logPaused.value = !logPaused.value;
  if (logPaused.value || logPausedBuffer.length === 0) return;
  const pending = logPausedBuffer.splice(0, logPausedBuffer.length);
  logBatch.push(...pending);
  if (logBatchTimer === null) {
    logBatchTimer = setTimeout(flushLogBatch, LOG_BATCH_MS);
  }
};

// 复制当前筛选结果（上限 1000 行，避免把剪贴板写爆）。
const handleCopyLogs = async () => {
  const rows = filteredLogLines.value.slice(-1000);
  if (rows.length === 0) {
    showToast(t('common.empty'), 'info');
    return;
  }
  const text = rows
    .map((log: any) => {
      const cat = log?.cat ? '[' + log.cat + ']' : '';
      return logTimeLabel(log) + ' [' + String(log.type || 'info').toUpperCase() + ']' + cat + ' ' + (log.payload ?? '');
    })
    .join('\n');
  // U-18：剪贴板统一走 utils/clipboard（失败文案从"按钮名 + ✗"改为 i18n 的 copyFailed）
  await copyText(text);
};

// 「打开日志目录」需要后端新增绑定方法（app.go / controller.go 不在本批授权），
// 先提供等价的「复制日志目录路径」，用户可直接粘到资源管理器（F-A3-16 残留见批次说明）。
const handleCopyLogDir = async () => {
  try {
    const info: any = await App.GetDataDirInfo();
    const dataDir = String(info?.dataDir ?? info?.DataDir ?? '').trim();
    const dir = dataDir ? dataDir.replace(/[\\/]+$/, '') + '\\logs' : 'logs';
    await copyText(dir, t('settings.diagnostics.logDir') + ': ' + dir);
  } catch (e: any) {
    showToast(String(e?.message ?? e), 'error');
  }
};

// 复位"检查中"并清掉兜底定时器，所有终态事件统一走这里
const settleAppUpdateChecking = () => {
  if (appUpdateCheckFailsafeTimer) {
    clearTimeout(appUpdateCheckFailsafeTimer);
    appUpdateCheckFailsafeTimer = null;
  }
  globalState.appUpdateChecking = false;
};

let unsubTrafficData: (() => void) | null = null;
let unsubTrafficModeChanged: (() => void) | null = null;
let unsubLogMessage: (() => void) | null = null;
let unsubRuntimeAssetsStatus: (() => void) | null = null;
let unsubClashExited: (() => void) | null = null;
let unsubUpdateCheckStart: (() => void) | null = null;
let unsubUpdateCheckCancelled: (() => void) | null = null;
let unsubUpdateCheckEnd: (() => void) | null = null;
let unsubUpdateAvailable: (() => void) | null = null;
let unsubUpdateStart: (() => void) | null = null;
let unsubUpdateDownloaded: (() => void) | null = null;
let unsubUpdateNone: (() => void) | null = null;
let unsubUpdateError: (() => void) | null = null;
let unsubMipsFallback: (() => void) | null = null;
let unsubUpdateBusy: (() => void) | null = null;
let uninstallHoverGuard: (() => void) | null = null;
let unsubScrollFades: (() => void) | null = null;
let unsubSystemTheme: (() => void) | null = null;

const menu = computed(() => [
  { id: 'home', label: t('menu.home'), icon: ICONS.home },
  { id: 'proxies', label: t('menu.proxies'), icon: ICONS.proxies },
  { id: 'connections', label: t('menu.connections'), icon: ICONS.connections },
  { id: 'logs', label: t('menu.logs'), icon: ICONS.logs },
  { id: 'rules', label: t('menu.rules'), icon: ICONS.rules },
  { id: 'subs', label: t('menu.subs'), icon: ICONS.subs },
  { id: 'settings', label: t('menu.settings'), icon: ICONS.settings }
]);

const activeMenuLabel = computed(() => {
  if (currentTab.value === 'yaml-editor') return t('menu.yamlEditor');
  return menu.value.find(m => m.id === currentTab.value)?.label;
});

const openYamlEditor = (id: string, name: string, type: 'local' | 'remote') => {
  editingConfigId.value = id;
  editingConfigName.value = name;
  editingConfigType.value = type;
  currentTab.value = 'yaml-editor';
};

/* C-02：离开 YAML 编辑器前的未保存守卫。
   判定必须发生在「切页之前」—— 下面 watch(currentTab) 一旦看到离开就会把
   yamlEditorModified 清掉，watcher 里再问就已经晚了。
   confirm 在途时禁止再次入队：连点侧栏不能被排成两个确认框。 */
let yamlLeaveConfirming = false;

const requestTabChange = async (next: string, beforeSwitch?: () => void): Promise<boolean> => {
  if (next === currentTab.value) {
    beforeSwitch?.();
    return true;
  }
  if (currentTab.value === 'yaml-editor' && yamlEditorModified.value) {
    if (yamlLeaveConfirming) return false;
    yamlLeaveConfirming = true;
    try {
      const ok = await showConfirm(
        t('yamlEditor.leaveConfirmMsg'),
        t('yamlEditor.discardConfirmTitle'),
        true,
        { confirmText: t('yamlEditor.discardBtn'), cancelText: t('common.cancel') }
      );
      if (!ok) return false;
    } finally {
      yamlLeaveConfirming = false;
    }
  }
  beforeSwitch?.();
  currentTab.value = next;
  return true;
};

// 侧栏与更新 chip 的跳转统一走这里（Sidebar 只 emit activeId，入口在 App 侧）。
const handleTabSelect = (next: string) => {
  void requestTabChange(next);
};

// 返回箭头：YamlEditor 自己已经确认过未保存修改（P-07 的 handleBack），这里直接切页，
// 否则同一次离开会弹两个确认框。若那条子组件守卫未来被移除，这里应改成 requestTabChange('subs')。
const closeYamlEditor = () => {
  yamlEditorModified.value = false;
  currentTab.value = 'subs';
};

watch(currentTab, (newVal) => {
  if (newVal !== 'yaml-editor') {
    editingConfigId.value = '';
    editingConfigName.value = '';
    yamlEditorStatus.value = t('yamlEditor.saved');
    yamlEditorModified.value = false;
    yamlEditorHasError.value = false;
    yamlEditorCursor.value = '';
  }
  /* H-01：targetSettingsView 以前只被更新 chip 写成 'about'、从不复位，
      Settings 又是 v-if 重建并拿 initialView 初始化 view，于是点过一次 chip 之后
      「每一次」进设置页都落在「关于」。离开设置页就复位，保证 chip 只影响那一次。 */
  if (newVal !== 'settings') {
    targetSettingsView.value = 'main';
  }
});

const handleYamlStatusChange = (payload: { text: string; modified: boolean; error: boolean }) => {
  yamlEditorStatus.value = payload.text;
  yamlEditorModified.value = payload.modified;
  yamlEditorHasError.value = payload.error;
};

const handleYamlCursorChange = (payload: { line: number; col: number }) => {
  yamlEditorCursor.value = t('yamlEditor.cursorStatus', { line: payload.line, col: payload.col });
};

/* H-19：拖拽改变窗口大小时 resize 每秒可达上百次，每次都是一次 Window.IsMaximised IPC。
   这里用 rAF 把同一帧内的多次 resize 合并成一次查询。 */
let resizeRafId: number | null = null;
let maximiseResyncTimer: ReturnType<typeof setTimeout> | null = null;
// 查询序列号：只有「最后一次」发起的查询才允许写入状态，否则乱序返回的旧结果会把图标改回去。
let maxQuerySeq = 0;

const syncMaximizedState = async () => {
  const seq = ++maxQuerySeq;
  try {
    const next = await Window.IsMaximised();
    if (seq !== maxQuerySeq) return;
    isMaximized.value = !!next;
  } catch {
    // 读不到就保持旧值：下一次 resize / 复查会再问一遍，不值得打扰用户
  }
};

const handleResize = () => {
  if (resizeRafId !== null) return;
  resizeRafId = window.requestAnimationFrame(() => {
    resizeRafId = null;
    void syncMaximizedState();
  });
};

const handleMinimise = () => {
  clearCtrlHover();
  Window.Minimise();
};

const handleToggleMaximise = () => {
  clearCtrlHover();
  Window.ToggleMaximise();
  // 立即读会拿到切换前的值，50ms 后复查一次；定时器保留句柄，卸载时清掉（H-19）。
  if (maximiseResyncTimer) clearTimeout(maximiseResyncTimer);
  maximiseResyncTimer = setTimeout(() => {
    maximiseResyncTimer = null;
    void syncMaximizedState();
  }, 50);
};

const handleClose = async () => {
  clearCtrlHover();
  // C-23：这里原来没有 try/catch —— GetAppBehavior 一失败，点关闭按钮就完全没反应。
  // 读不到行为配置时按「隐藏到托盘」处理：宁可留在托盘，也不要在未确认的情况下退出。
  let closeToTray = true;
  try {
    const config = await App.GetAppBehavior();
    closeToTray = !!(config && config.closeToTray);
  } catch (e) {
    reportError(e, 'window.closeBehavior');
  }
  if (closeToTray) {
    App.HideMainWindow();
    return;
  }
  /* C-02 的最后一条漏网路径：closeToTray=false 时这里是真退出，未保存的 YAML 会随进程消失
     （隐藏到托盘则不会，编辑器实例还在）。真退出前补一次与切页守卫一致的确认。 */
  if (yamlEditorModified.value) {
    const ok = await showConfirm(
      t('yamlEditor.leaveConfirmMsg'),
      t('yamlEditor.discardConfirmTitle'),
      true,
      { confirmText: t('yamlEditor.discardBtn'), cancelText: t('common.cancel') }
    );
    if (!ok) return;
  }
  Application.Quit();
};

// 统一出口：由 store 负责结算当前模态并出队下一条（内含旧式直写的兼容分支）
const handleModalConfirm = () => {
  confirmModal();
};

const handleModalCancel = () => {
  cancelModal();
};

const handleCoreSeedNever = async () => {
  showCoreSeedModal.value = false;
  try {
    await App.DisableCoreComparisonPrompt();
  } catch (e) {
    console.error("Disable core comparison prompt failed:", e);
  }
};

const handleCoreSeedCancel = () => {
  showCoreSeedModal.value = false;
};

const handleCoreSeedConfirm = async () => {
  replacingCore.value = true;
  try {
    const newVer = await App.ReplaceCoreWithSeed();
    showCoreSeedModal.value = false;
    await showAlert(t('coreSeedPrompt.replaceSuccess', { version: newVer || seedCoreVer.value }), t('common.success'));
  } catch (e: any) {
    await showAlert(t('coreSeedPrompt.replaceFailed', { error: String(e?.message || e) }), t('common.error'), true);
  } finally {
    replacingCore.value = false;
  }
};

/* globalState.theme 的唯一写者是 applyThemeToApp（它与 themeState.isDark 同步赋值）。
   所以这个 watcher 只保留「跟随系统时把解析结果落盘」这一件事：

   · 不再重复设置 .dark class —— applyTheme 已经 toggle 过 html 上的 class，
     再设一遍会在回声期间和它失步，表现为「底色没变但描边/高光变了」。
   · 不再回灌 applyThemeToApp —— 那会在 SYSTEM 模式下把用户偏好静默改写成
     显式 light/dark，「跟随系统」最多活到下次重启。
   · 不再调 Window.SetBackgroundColour —— 窗口是 BackgroundTypeTranslucent 时
     该调用是空操作（见 app.go 的 applyWindowAppearance 注释），窗口底色由
     applyWindowAppearance 统一负责。
   · light/dark 下的 theme_setting.txt 由后端 syncThemeSettingFile 写，
     前端只在 system 模式下写「OS 解析结果」，避免双写打架。 */
watch(() => globalState.theme, (val, oldVal) => {
  const persist = oldVal !== undefined && oldVal !== val;
  if (persist && themeState.config.mode === 'system') {
    void App.SaveThemePreference(val === 'dark');
  }
}, { immediate: true });

watch(currentLocale, (lang) => {
  document.documentElement.setAttribute('lang', lang);
  document.documentElement.setAttribute('data-lang', lang);
}, { immediate: true });

onMounted(async () => {
  // S-06：initStore 的唯一调用点是 main.ts（早于 createApp，保证首帧前开始订阅）。
  // 这里原本还有一次死调用，删掉以消除「初始化到底发生在哪」的误导；store 内部仍幂等。
  // U-11：initScrollFades 现在返回释放函数（三个观察器原先没有任何 teardown 路径）
  unsubScrollFades = initScrollFades();
  uninstallHoverGuard = installHoverGuard();

  // 启动时恢复「外观」= 玻璃 + 主题（syncThemeFromBehavior 内部先玻璃后主题）。
  // 设置页改动会即时调用 applyGlassEffect / applyThemeToApp，无需回到这里。
  try {
    const behavior: any = await App.GetAppBehavior();
    syncThemeFromBehavior(behavior);
  } catch {}
  // 软件关闭期间操作系统可能切过明暗。system 模式下把当前解析结果落一次盘，
  // 保证下次启动 main.go 建窗口前读到的是正确底色。
  if (themeState.config.mode === 'system') void App.SaveThemePreference(themeState.isDark);
  // 「跟随系统」模式要实时跟随系统明暗切换（themeState.config.mode === 'system' 时才生效）
  unsubSystemTheme = bindSystemTheme();

  // 窗口隐藏/最小化期间鼠标可能已经移走，WebView 收不到 mouseleave；
  // 在失焦、恢复、页面可见性变化时统一清掉窗口控制按钮的 hover 状态。
  window.addEventListener('blur', clearCtrlHover);
  window.addEventListener('focus', clearCtrlHover);
  document.addEventListener('visibilitychange', clearCtrlHover);

  // F-A4-08：以下重调用全部改为非阻塞（订阅必须先注册，否则启动期事件整段丢失）
  void (async () => {
    try {
      if (!globalState.appVersion) {
        globalState.appVersion = await App.GetAppVersion();
      }
    } catch (e) { /* ignore */ }
  })();

  void (async () => {
    try {
      const status = await App.CheckTunEnv();
      globalState.tunStatus = status as any;
    } catch (e) { console.error("TUN Env Check Error:", e); }
  })();

  void (async () => {
    try {
      const checkResult = await App.CheckCoreAgainstSeed();
      if (checkResult && checkResult.shouldPrompt) {
        currentCoreVer.value = checkResult.currentVersion;
        seedCoreVer.value = checkResult.seedVersion;
        showCoreSeedModal.value = true;
      }
    } catch (e) {
      console.error("Check core against seed failed:", e);
    }
  })();

  unsubTrafficData = Events.On("traffic-data", ({ data }: { data: any }) => {
    traffic.value = {
      up: data?.up ?? '0 B/s',
      down: data?.down ?? '0 B/s',
      upRaw: data?.upRaw ?? 0,
      downRaw: data?.downRaw ?? 0,
      uploadTotal: data?.uploadTotal ?? '0 B',
      downloadTotal: data?.downloadTotal ?? '0 B',
      uploadTotalRaw: data?.uploadTotalRaw ?? 0,
      downloadTotalRaw: data?.downloadTotalRaw ?? 0,
    };
    updateLatestTraffic(data?.upRaw ?? 0, data?.downRaw ?? 0);
  });

  unsubTrafficModeChanged = Events.On("traffic-stat-mode-changed", () => {
    resetWaveState();
  });

  startWaveSampling();

  /* H-09：订阅必须先于开流。原来顺序是「拉历史(不 await) → 开流 → 订阅」，
     开流与订阅之间推来的日志无人接收；现在先订阅，再由流补上历史没有的实时行。 */
  unsubLogMessage = Events.On("log-message", ({ data: log }: { data: any }) => {
    enqueueLogLine(log);
  });

  App.StartStreamingLogs();

  // H-09：历史快照与等待期间已到的活日志合并去重（原来整体赋值会把活日志整片覆盖掉）。
  void (async () => {
    try {
      mergeHistoryLogs(await App.GetRecentLogs());
    } catch (e) { console.error("load recent logs failed:", e); }
  })();

  // 启动资产自检结果：后端在窗口加载前就发出了这条事件（app.go runStartupRuntimeAssetMaintenance），
  // 之前前端从未订阅，等于白算（F-A3-12）。这里接住并写入全局状态，供设置/更新页复用。
  unsubRuntimeAssetsStatus = Events.On("runtime-assets-status", ({ data }: { data: any }) => {
    if (data) globalState.assetStatus = data;
  });

  /* H-13：同一条事件也可能在订阅装好之前就发完了（缓存热路径下自检几毫秒结束）。
     订阅后补一次快照兜底 —— GetRuntimeAssetStatus 是既有绑定，此前无人调用。 */
  void (async () => {
    try {
      const status = await App.GetRuntimeAssetStatus();
      if (status && !globalState.assetStatus) globalState.assetStatus = status;
    } catch {
      // 兜底失败保持 null 即可：设置页/更新页会各自再拉一次，不必打扰用户
    }
  })();

  unsubClashExited = Events.On("clash-exited", () => {
    globalState.isRunning = false;
    App.SyncState();
  });

  unsubMipsFallback = Events.On("tun-mips-fallback", () => {
    void showAlert(t('settings.tun.mipsFallbackWarning'), t('common.notice'), false);
  });

  window.addEventListener('resize', handleResize);

  unsubUpdateCheckStart = Events.On("app-update-check-start", () => {
    globalState.appUpdateChecking = true;

    // 🛡️ 兜底：后端最长 12s 必须给出结果；若 15s 后仍无任何事件，
    // 强制复位"检查中"，杜绝按钮永远卡死。
    if (appUpdateCheckFailsafeTimer) clearTimeout(appUpdateCheckFailsafeTimer);
    appUpdateCheckFailsafeTimer = setTimeout(() => {
      if (globalState.appUpdateChecking) {
        console.warn("[app-update] 检查超过 15s 无任何后端事件，强制复位");
        globalState.appUpdateChecking = false;
      }
    }, 15000);
  });

  unsubUpdateCheckCancelled = Events.On("app-update-check-cancelled", () => {
    settleAppUpdateChecking();
  });

  // 后端每次手动检查结束（成功/失败/取消）都会发 check-end，兜底复位
  unsubUpdateCheckEnd = Events.On("app-update-check-end", () => {
    settleAppUpdateChecking();
  });

  unsubUpdateAvailable = Events.On("app-update-available", ({ data: info }: { data: any }) => {
    settleAppUpdateChecking();
    const version = info?.version ?? "";

    void openModal({
      type: 'confirm',
      title: t('appUpdate.foundTitle'),
      message: t('appUpdate.foundMsg', { version }),
      onConfirm: async () => {
        // H-11：下载入口的在途闸门 —— 两次 app-update-available 排出的两个确认框
        // 各点一次会重复下发下载请求，后端只能回 app-update-busy 而前端毫无反应。
        if (appUpdateActionBusy.value) {
          showAppUpdateBusyHint();
          return;
        }
        appUpdateActionBusy.value = true;
        try {
          await App.DownloadPendingAppUpdateAsync();
        } catch (e: any) {
          void openModal({
            type: 'alert',
            title: t('appUpdate.startDownloadFailed'),
            message: String(e?.message || e || t('common.error')),
            isDanger: true,
          });
        } finally {
          appUpdateActionBusy.value = false;
        }
      },
    });
  });

  unsubUpdateStart = Events.On("app-update-start", () => {
    /* 这里原来只有一句 console.log（调试残留）。下载开始的真实信号由 app-update-progress
       驱动 chip 进度，因此本监听只保留订阅本身——它的价值是保证事件有消费者可追踪（H-09 的配对自检）。 */
  });

  unsubUpdateDownloaded = Events.On("app-update-downloaded", ({ data: payload }: { data: any }) => {
    settleAppUpdateChecking();
    const version = payload?.version ?? "";
    const fullPath = payload?.path ?? "";

    void openModal({
      type: 'confirm',
      title: t('appUpdate.downloadCompleteTitle'),
      message: t('appUpdate.downloadCompleteMsg', { version }),
      onConfirm: async () => {
        // H-11：安装入口同样加在途闸门（与 chip 路径共用同一个标志）。
        if (appUpdateActionBusy.value) {
          showAppUpdateBusyHint();
          return;
        }
        if (!fullPath) {
          // H-12：与 chip 路径统一走 openInstallPathEmptyAlert，不再一处提示一处静默
          openInstallPathEmptyAlert();
          return;
        }
        appUpdateActionBusy.value = true;
        try {
          await App.ApplyAppUpdate(fullPath);
        } catch (e: any) {
          void openModal({
            type: 'alert',
            title: t('appUpdate.startInstallerFailed'),
            message: String(e?.message || e || t('common.error')),
            isDanger: true,
          });
        } finally {
          appUpdateActionBusy.value = false;
        }
      },
    });
  });

  unsubUpdateNone = Events.On("app-update-none", ({ data: payload }: { data: any }) => {
    settleAppUpdateChecking();
    void openModal({
      type: 'alert',
      title: t('appUpdate.latestTitle'),
      message: payload?.message || t('appUpdate.latestMsg'),
    });
  });

  unsubUpdateError = Events.On("app-update-error", ({ data: err }: { data: string }) => {
    settleAppUpdateChecking();
    const s = String(err || t('common.error'));
    const msg = s.length > 120 ? t('appUpdate.networkOrRetryMsg') : s;

    void openModal({
      type: 'alert',
      title: t('appUpdate.updateFailedTitle'),
      message: msg,
      isDanger: true,
    });
  });

  /* H-11：后端在更新任务槽被占用时明确发 app-update-busy，外壳此前完全不接。
     设置页的「更新 / 关于」子页自己也有监听（SettingsAbout / SettingsUpdate），
     所以在设置页内不重复提示，只在其它页面补一次轻提示。 */
  unsubUpdateBusy = Events.On("app-update-busy", () => {
    if (currentTab.value === 'settings') return;
    showAppUpdateBusyHint();
  });
});

const handleClearLogs = async () => {
  // 清空是破坏性操作：内存环与磁盘外唯一证据都会没，先确认（F-A3-16）。
  const confirmed = await showConfirm(t('logs.clear'), t('common.warning'), true);
  if (!confirmed) return;

  logLines.value = [];
  // 未刷出的批次 / 暂停缓冲也要丢，否则清空后下一帧又冒出来一批旧行。
  logBatch = [];
  logPausedBuffer = [];
  try {
    await App.ClearLogs();
  } catch (e: any) {
    /* H-10：原来是裸 await —— 绑定失败时既留下未处理的 Promise 拒绝，又让「前端已空、
       后端环仍在」长期不一致（重启后旧日志又冒出来）。失败必须提示，并把后端仍然
       保留的日志拉回来对齐（前端这边已经在上面清空了）。 */
    showToast(String(e?.message ?? e ?? t('common.error')), 'error');
    try {
      mergeHistoryLogs(await App.GetRecentLogs());
    } catch {
      // 拉不回来也不再抛：失败已经提示过，用户可再点一次清空
    }
  }
};

onUnmounted(() => {
  uninstallHoverGuard?.();
  uninstallHoverGuard = null;
  unsubScrollFades?.();
  unsubScrollFades = null;
  unsubSystemTheme?.();
  unsubSystemTheme = null;
  if (scrollTimer) {
    clearTimeout(scrollTimer);
    scrollTimer = null;
  }
  if (logBatchTimer) {
    clearTimeout(logBatchTimer);
    logBatchTimer = null;
  }
  logBatch = [];
  logPausedBuffer = [];
  if (appUpdateCheckFailsafeTimer) {
    clearTimeout(appUpdateCheckFailsafeTimer);
    appUpdateCheckFailsafeTimer = null;
  }

  stopWaveSampling();
  // H-19：rAF 与最大化复查定时器都要在这里收干净，否则卸载后还会打一次 IPC
  if (resizeRafId !== null) {
    cancelAnimationFrame(resizeRafId);
    resizeRafId = null;
  }
  if (maximiseResyncTimer) {
    clearTimeout(maximiseResyncTimer);
    maximiseResyncTimer = null;
  }
  window.removeEventListener('resize', handleResize);
  window.removeEventListener('blur', clearCtrlHover);
  window.removeEventListener('focus', clearCtrlHover);
  document.removeEventListener('visibilitychange', clearCtrlHover);

  unsubTrafficData?.();
  unsubTrafficModeChanged?.();
  unsubLogMessage?.();
  unsubRuntimeAssetsStatus?.();
  unsubRuntimeAssetsStatus = null;
  unsubClashExited?.();
  unsubUpdateCheckStart?.();
  unsubUpdateCheckCancelled?.();
  unsubUpdateCheckEnd?.();
  unsubUpdateAvailable?.();
  unsubUpdateStart?.();
  unsubUpdateDownloaded?.();
  unsubUpdateNone?.();
  unsubUpdateError?.();
  unsubMipsFallback?.();
  unsubUpdateBusy?.();
  unsubUpdateBusy = null;
});

/* 页面切换时的滚动归位。
   外层 .view-scroller 已经不再滚动（唯一滚动 owner 是页面的 .ui-page-body），
   而每个页面都是 v-if 销毁重建的，其 body 天然从顶部开始，
   所以这里只剩日志页需要把内容追到最新一行。 */
const resetViewScroller = () => {
  if (currentTab.value === 'logs') {
    /* H-04：切回日志页视为「跟随最新一行」。页面是 v-if 重建的，而 logFollowTail 活在 App 上，
       不复位就会出现「明明停在底部却不再自动滚动」（上一次上翻的状态被带进新页面）。 */
    logFollowTail.value = true;
    nextTick(() => {
      if (logBox.value) {
        logBox.value.scrollTop = logBox.value.scrollHeight;
      }
    });
  }
};
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
  gap: 8px; /* 更新提示泡与窗口控制按钮之间的间距 */
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

.ctrl-btn.hovered { 
  background: var(--surface-hover); 
  color: var(--text-main); 
}

/* 还原更好看的 Windows 红色 */
.close-btn.hovered { 
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
.content-header h1 { 
  font-size: 1.5rem; 
  font-weight: 600; 
  letter-spacing: -0.02em; 
  margin-bottom: 32px; 
  display: flex;
  align-items: baseline;
  gap: 12px;
}

/* 页面唯一的滚动 owner 是页面内的 .ui-page-body。
   外层 view-scroller 只负责布局与左右内缩，不再滚动：
   这样滚动条不会贴着 .content 的圆角被裁切，也不会与页面 body 双滚动互抢。 */
.view-scroller {
  position: relative;
  flex: 1;
  min-height: 0;
  overflow: hidden;
  padding: 0 var(--content-px) 0 var(--content-px);
  overscroll-behavior: contain;
  display: flex;
  flex-direction: column;
}

/* 日志页 body：唯一的滚动 owner（不再用固定 500px 高度造成底部截断） */
.terminal-box {
  background: var(--surface);
  color: var(--text-main);
  border: none;
  padding: 20px;
  border-radius: var(--ui-radius-sm);
  height: auto;
  flex: 1 1 auto;
  min-height: 0;
  overflow-y: auto;
  font-family: var(--font-mono);
  font-size: 0.75rem;
  line-height: 1.6;
}

.l-time { color: var(--text-muted); margin-right: 12px; opacity: 0.8; }
.l-type { margin-right: 12px; font-weight: 600; }
.l-cat { color: var(--text-sub); margin-right: 12px; font-size: 0.85em; opacity: 0.85; }
.logs-tools { display: flex; align-items: center; gap: 8px; flex-wrap: wrap; justify-content: flex-end; }
.log-search-box {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 4px 8px;
  border: 1px solid var(--border);
  border-radius: var(--ui-radius-sm);
  background: var(--bg-elevated, transparent);
}
.log-search-box input {
  width: 140px;
  border: none;
  outline: none;
  background: transparent;
  color: var(--text-main);
  font-size: 0.82rem;
}
.log-search-icon { display: flex; color: var(--text-muted); }
.log-level-select {
  padding: 4px 6px;
  border: 1px solid var(--border);
  border-radius: var(--ui-radius-sm);
  background: transparent;
  color: var(--text-main);
  font-size: 0.8rem;
}

.log-line.info .l-type { color: var(--text-main); }
/* H-06：后端只会产出 warn，原来的 .warning 选择器永不命中，WARN 行丢失视觉区分 */
.log-line.warn .l-type { color: var(--text-sub); font-style: italic; }
.log-line.error .l-type { color: var(--text-main); font-weight: 700; }
.log-line.debug .l-type { color: var(--text-muted); }

.view-settings { display: flex; flex-direction: column; }
.view-logs { gap: 12px; }

/* 页面切换动画见全局 style.css 的 .pane-up / .pane-down / .pane-left / .pane-right；
   这里只保留过渡期间对滚动位置的重置钩子注释。 */

/* 页面过渡容器：所有页面都是 PageShell（固定 header + 唯一 body 滚动），
   这里只负责撑满可用高度；不再用 min-height:100% 制造临时溢出与双滚动。 */
.view-transition-wrapper {
  /* 必须撑满 .view-scroller 的整个 padding box，而不是退回到内容区：
     整面平移是靠 translateX/Y(100%) 完成的，只有 100% 等于容器整宽/整高时，
     移出视口才是"刚好走干净"。如果这里再退回 var(--content-px)，
     横向滑动会在边缘漏出一条旧页。
     左右留白改由自身 padding 提供，视觉结果与之前完全一致。 */
  position: absolute;
  inset: 0;
  padding: 0 var(--content-px);
  display: flex;
  flex-direction: column;
  min-height: 0;
  overflow: visible;
}

.content-title-row {
  display: flex;
  align-items: baseline;
  gap: 12px;
}

.yaml-save-status {
  font-size: 0.8rem;
  font-weight: 600;
  color: var(--text-sub);
  font-family: var(--font-mono);
  padding: 4px 8px;
  border-radius: 4px;
}

.yaml-save-status.modified {
  color: var(--text-main);
  background: var(--surface-hover);
}

.yaml-save-status.error {
  color: var(--surface-panel);
  background: var(--text-main);
}

.yaml-cursor-status {
  font-size: 0.8rem;
  font-weight: 400;
  color: var(--text-muted);
  font-family: var(--font-mono);
  margin-left: 4px;
}

.yaml-save-status.modified {
  color: var(--text-main);
}

.yaml-save-status.error {
  color: #ff4d4f;
}

.core-seed-modal {
  max-width: 460px;
}

.core-compare-card {
  display: flex;
  flex-direction: column;
  gap: 8px;
  background: var(--surface);
  border: 1px solid var(--surface-hover);
  border-radius: var(--ui-radius-md);
  padding: 12px 16px;
}

.core-compare-row {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.compare-tag-label {
  font-size: 0.85rem;
  color: var(--text-muted);
}

.version-badge {
  font-family: var(--font-mono);
  font-size: 0.82rem;
  font-weight: 600;
  padding: 2px 8px;
  border-radius: 6px;
}

.version-badge.current-badge {
  color: var(--text-sub);
  background: var(--surface-panel);
}

.version-badge.seed-badge {
  color: var(--accent);
  background: rgba(59, 130, 246, 0.12);
}

.core-seed-footer {
  display: flex;
  justify-content: flex-end;
  align-items: center;
  gap: 10px;
  width: 100%;
}

.global-update-chip {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  background: var(--surface-hover);
  border: 1px solid var(--glass-border);
  border-radius: 20px;
  padding: 4px 10px;
  cursor: pointer;
  font-size: 0.8rem;
  font-weight: 500;
  color: var(--text-main);
  transition: all 0.25s cubic-bezier(0.4, 0, 0.2, 1);
  user-select: none;
  position: relative;
}

.global-update-chip:hover {
  background: var(--surface-panel);
  border-color: var(--accent);
  transform: translateY(-1px);
}

/* 下载完成：使用当前主题的反色（accent/accent-fg），不再用绿色 */
.global-update-chip.downloaded {
  background: var(--accent);
  border-color: var(--accent);
  color: var(--accent-fg);
  font-weight: 600;
}

.global-update-chip.downloaded:hover {
  background: var(--accent);
  border-color: var(--accent);
  filter: brightness(1.12);
}

.chip-pulse {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background-color: var(--accent);
  box-shadow: 0 0 0 0 var(--pulse-ring, rgba(26, 26, 26, 0.45));
  animation: chipPulse 1.8s infinite;
}

@keyframes chipPulse {
  0% {
    transform: scale(0.95);
    box-shadow: 0 0 0 0 var(--pulse-ring, rgba(26, 26, 26, 0.45));
  }
  70% {
    transform: scale(1);
    box-shadow: 0 0 0 6px transparent;
  }
  100% {
    transform: scale(0.95);
    box-shadow: 0 0 0 0 transparent;
  }
}

.chip-icon {
  display: flex;
  align-items: center;
}

.chip-text {
  letter-spacing: 0.01em;
}
</style>