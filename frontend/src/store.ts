// 文件路径: frontend/src/store.ts
import { reactive } from 'vue';
import { Events } from '@wailsio/runtime';
import { App } from './api';
import * as runtimeassets from '../bindings/goclashz/core/runtimeassets';
import { setLocale, SupportedLocale, t } from './locales';
import { displayErrorMessage, isLongConnectionDeferral, isRouteSwitchingMessage } from './utils/errors';
import { reportError } from './utils/report';
import {
  applyTheme,
  cacheTheme,
  normalizeThemeConfig,
  onSystemThemeChange,
  readCachedTheme,
  resolveIsDark,
  themeConfigFromBehavior,
  writeThemeToBehavior,
  type ThemeConfig,
  type ThemeMode
} from './utils/theme';
import { applyGlassEffect, refreshGlassTint } from './utils/glass';

export type UpdateTaskState = {
  key: string;
  title: string;
  status: string;
  stage: string;
  progress: number;
  bytesDone: number;
  bytesTotal: number;
  speedBps: number;
  etaSeconds: number;
  error: string;
  startedAt: number;
  finishedAt: number;
};

export type OutboundIPResult = {
  preferred: string;
  ipv4: string;
  ipv6: string;
  mode: string;
  source: string;
  source4: string;
  source6: string;
  message: string;
  message4: string;
  message6: string;
  complete: boolean;
};

export function normalizeOutboundIP(raw: any): OutboundIPResult {
  return {
    preferred: raw?.preferred || '',
    ipv4: raw?.ipv4 || '',
    ipv6: raw?.ipv6 || '',
    mode: raw?.mode || '',
    source: raw?.source || '',
    source4: raw?.source4 || '',
    source6: raw?.source6 || '',
    message: raw?.message || '',
    message4: raw?.message4 || '',
    message6: raw?.message6 || '',
    complete: raw?.complete ?? false,
  };
}

// 1. 同步读取本地缓存（发生在 Vue 渲染前，绝对 0 延迟）
const cachedHideLogs = localStorage.getItem('goclashz_hideLogs') === 'true';
const cachedTheme = localStorage.getItem('goclashz_theme') || 'light';
const cachedActiveConfigId = localStorage.getItem('goclashz_activeConfigId') || ''; // 👈 新增缓存预热
const cachedOutboundIP = localStorage.getItem('goclashz_outboundIP');
export type ProxyCardMode = 'standard' | 'compact' | 'list' | 'adaptive';
const validProxyCardModes: ProxyCardMode[] = ['standard', 'compact', 'list', 'adaptive'];
const savedProxyCardMode = localStorage.getItem('goclashz_proxy_card_mode') as ProxyCardMode | null;
const cachedProxyCardMode: ProxyCardMode = (savedProxyCardMode && validProxyCardModes.includes(savedProxyCardMode))
  ? savedProxyCardMode
  : 'standard';

let initialOutboundIP: OutboundIPResult | null = null;
try {
  initialOutboundIP = cachedOutboundIP
    ? normalizeOutboundIP(JSON.parse(cachedOutboundIP))
    : null;
} catch {
  initialOutboundIP = null;
}

// 👇 应用主题（App Theme）状态：预设/强调色/圆角/光晕 + 已解析的明暗。
// config 是"用户偏好"（含 system），themeState.isDark 是"当前实际生效的明暗"。
// isDark 必须从 config 解析出来，不能写死 false：否则 App.vue 的 watchTheme 第一帧
// 就会判定"不一致"，顺手把 system 模式改写成显式 light/dark，
// 结果「跟随系统」最多活到下次重启。
const initialThemeConfig = readCachedTheme() as ThemeConfig;
export const themeState = reactive({
  config: initialThemeConfig,
  isDark: resolveIsDark(initialThemeConfig.mode)
});

/**
 * 把主题配置应用到界面（唯一入口，供启动流程与设置页共用）。
 * - applyTheme / cacheTheme 由 utils/theme.ts 完成（写 data-* 与 CSS 变量）
 * - 同步 globalState.theme（侧边栏标签、窗口底色、后端 theme_setting.txt 都依赖它）
 * @param behavior 传入时顺带把主题字段写回该对象（设置页保存用）
 */
export function applyThemeToApp(config: ThemeConfig, behavior?: any): boolean {
  const normalized = normalizeThemeConfig(config);
  const applied = applyTheme(normalized);
  cacheTheme(normalized);
  if (behavior) writeThemeToBehavior(behavior, normalized);
  themeState.config = normalized;
  themeState.isDark = applied.isDark;
  const nextTheme = applied.isDark ? 'dark' : 'light';
  if (globalState.theme !== nextTheme) globalState.theme = nextTheme;
  // 旧缓存键：App.vue 根节点的初始明暗、以及部分启动路径仍读它
  try {
    localStorage.setItem('goclashz_theme', nextTheme);
  } catch {
    /* 隐私模式下 localStorage 不可写，忽略 */
  }
  // --glass-tint-bg 是算出来的字面量，依赖刚刚被改写的 --surface-panel。
  // 不重算的话，玻璃开着时整窗会停在旧主题的面板色上（切暗色像没生效）。
  refreshGlassTint();
  return applied.isDark;
}

/**
 * 切换外观模式并「同时」写回后端 behavior —— 侧栏快捷明暗开关的唯一入口。
 *
 * 为什么必须写回：进入设置页时会用 App.GetAppBehavior() 的最新值重新应用外观
 * （syncThemeFromBehavior）。若只改内存里的 globalState.theme，behavior.themeMode 仍是旧值，
 * 那么每次重新进入设置页都会把用户的切换顶回去（表现为"点一次软件设置，明暗又翻一次"）。
 */
export async function setThemeMode(mode: ThemeMode): Promise<void> {
  applyThemeToApp({ ...themeState.config, mode });
  try {
    const current: any = await App.GetAppBehavior();
    if (!current) return;
    current.themeMode = themeState.config.mode;
    await App.SaveAppBehavior(current);
  } catch (err) {
    // 后端不可用（dev 环境 / 服务异常）时保持前端已生效的状态，不阻塞交互
    reportError(err, 'theme.saveMode');
  }
}

/**
 * 从后端 AppBehavior 同步「外观」：主题 + 玻璃。
 * 统一入口，供启动流程与设置页 loadBehavior/reset 后调用；
 * 不回写 behavior，避免把默认值写库。
 */
export function syncThemeFromBehavior(behavior: any): boolean {
  applyGlassEffect(!!behavior?.glassBlurEnabled, {
    tint: behavior?.glassTintStrength,
    depth: behavior?.glassDepth
  });
  return applyThemeToApp(themeConfigFromBehavior(behavior));
}

/** 监听系统明暗切换：仅当用户选择「跟随系统」时才需要实时跟随 */
export function bindSystemTheme(onApplied?: () => void): () => void {
  return onSystemThemeChange(() => {
    if (themeState.config.mode !== 'system') return;
    applyThemeToApp(themeState.config);
    onApplied?.();
  });
}

// 存储全局倒计时 ID，不放在 reactive 中防止不必要的响应式开销
const delayTimers: Record<string, number> = {};

// The backend owns persisted desired state. This overlay exists only while a
// local command is in flight so control cards react immediately without
// becoming an independent source of truth.
const controlOverlay = {
  systemProxy: null as boolean | null,
  tun: null as boolean | null,
};

function syncVisibleControls() {
  globalState.systemProxy = controlOverlay.systemProxy ?? globalState.desiredSystemProxy;
  globalState.tun = controlOverlay.tun ?? globalState.desiredTun;
}

export function setSystemProxyIntent(value: boolean) {
  controlOverlay.systemProxy = value;
  syncVisibleControls();
}

export function setTunIntent(value: boolean) {
  controlOverlay.tun = value;
  syncVisibleControls();
}

export function settleSystemProxyIntent() {
  controlOverlay.systemProxy = null;
  syncVisibleControls();
}

export function settleTunIntent() {
  controlOverlay.tun = null;
  syncVisibleControls();
}

export function setProxyCardMode(mode: ProxyCardMode) {
  const safeMode = validProxyCardModes.includes(mode) ? mode : 'standard';
  globalState.proxyCardMode = safeMode;
  try {
    localStorage.setItem('goclashz_proxy_card_mode', safeMode);
  } catch (e) {
    reportError(e, 'prefs.proxyCardMode');
  }
}

// 定义全局响应式状态
export type ToastKind = 'info' | 'success' | 'error';

export type ToastItem = {
  id: number;
  kind: ToastKind;
  title: string;
  message: string;
  duration: number;
};

export const globalState = reactive({
  isRunning: false,
  mode: 'rule',
  theme: cachedTheme,
  hideLogs: cachedHideLogs,
  proxyCardMode: cachedProxyCardMode,
  // Visible control state is the optimistic target or backend desired state.
  systemProxy: false,
  tun: false,
  desiredSystemProxy: false,
  desiredTun: false,
  desiredPortProxy: false,
  // backend actual：后端实际状态，用于诊断/IP 检测
  actualSystemProxy: false,
  actualTun: false,
  // pending：只控制重复点击，不影响亮灭
  systemProxyPending: false,
  tunPending: false,
  controlRevision: 0,
  version: '',
  appVersion: '',
  logLevel: 'error',
  appLogLevel: 'info',
  isAdmin: false,
  tunStatus: { hasWintun: false, isAdmin: false },
  assetStatus: null as runtimeassets.RuntimeAssetStatus | null,
  delayRetention: true,
  delayRetentionTime: 'long',

  // 👇 新增：应用更新相关
  updateReady: false,
  newAppVersion: '',
  updateDownloaded: false,
  downloadedPath: '',
  appUpdateChecking: false, // 👈 新增：跟踪软件更新检查状态

  // 仅端口代理模式（由后端 behavior 驱动）
  portProxyMode: false,

  // 🚀 核心：使用缓存初始化消除渲染空窗期的闪烁
  activeConfigId: cachedActiveConfigId,
  activeConfigName: '',
  activeConfigType: '',

  // 👇 新增：全局延迟缓存池，用于实现跨页面长效保存
  proxyDelays: {} as Record<string, { delay: number | null, status: string, message: string }>,

  // 跨页面 IP 状态缓存
  outboundIP: initialOutboundIP as OutboundIPResult | null,
  outboundIPStale: false,
  ipDetecting: false,
  appUpdateProgress: null as {
    totalBytes: number;
    bytesDone: number;
    speedBps: number;
    etaSec: number;
    isDownloaded?: boolean;
    version?: string;
    path?: string;
  } | null,

  componentUpdate: {
    tasks: {} as Record<string, UpdateTaskState>,
    checkingCoreUpdate: false,
    pendingCoreUpdate: false,
    updatingCore: false,
    coreUpdateInfo: { local: '', remote: '', releaseUrl: '' },
    networkBusy: false,
  },

  // 轻提示队列（成功/中性反馈，不打断操作；同屏最多 3 条）
  toasts: [] as ToastItem[],

  // 全局模态框状态
  modal: {
    show: false,
    type: 'alert' as 'alert' | 'confirm',
    title: '',
    message: '',
    detail: '',
    isDanger: false,
    confirmText: '',
    cancelText: '',
    onConfirm: null as Function | null,
    onCancel: null as Function | null,
  }
});



// 👇 新增清洗规则：打破数据格式强粘合，防止大小写污染
const normalizeLogLevel = (level?: string) => {
  const v = String(level || '').toLowerCase().trim();
  if (v === 'trace') return 'debug';
  if (v === 'fatal' || v === 'panic') return 'error';
  if (['debug', 'info', 'warn', 'warning', 'error', 'silent'].includes(v)) return v;
  return 'info';
};

export function updateStateFromBackend(rawData: any) {
  if (!rawData) return;

  const oldRunning = globalState.isRunning;

  if (rawData.isRunning !== undefined) globalState.isRunning = rawData.isRunning;
  else if (rawData.IsRunning !== undefined) globalState.isRunning = rawData.IsRunning;

  // mode 更新
  if (rawData.mode !== undefined) globalState.mode = rawData.mode;
  else if (rawData.Mode !== undefined) globalState.mode = rawData.Mode;

  // 刻意不消费后端 theme。
  // 明暗的用户意图只存在于前端（themeState.config.mode，来自 behavior.ThemeMode），
  // 后端 AppState.Theme 只是同一次意图的派生快照；快照在异步落盘完成前会是旧值，
  // 一旦回灌就会把用户刚做的切换顶回去 —— 这正是「点暗色弹回白色 / 自己来回翻」的来源。
  // 唯一需要跟随外部变化的是 system 模式，那条路由 bindSystemTheme 单独处理。

  const newHideLogs = rawData.hideLogs ?? rawData.HideLogs;
  if (newHideLogs !== undefined) {
    globalState.hideLogs = newHideLogs;
    localStorage.setItem('goclashz_hideLogs', String(newHideLogs));
  }

  const newLogLevel = rawData.logLevel ?? rawData.LogLevel;
  if (newLogLevel !== undefined) {
    globalState.logLevel = normalizeLogLevel(newLogLevel || 'error');
  }

  const newAppLogLevel = rawData.appLogLevel ?? rawData.AppLogLevel;
  if (newAppLogLevel !== undefined) {
    globalState.appLogLevel = normalizeLogLevel(newAppLogLevel || 'info');
  }

  // 检测代理/TUN 状态变化，触发强制 IP 检测（基于 actual，不基于 intent）
  const oldActualProxy = globalState.actualSystemProxy;
  const oldActualTun = globalState.actualTun;

  const newProxy = rawData.actualSystemProxy ?? rawData.systemProxy ?? rawData.SystemProxy;
  const newTun = rawData.actualTun ?? rawData.tun ?? rawData.Tun;

  // 始终更新 actual（后端真实 runtime 状态）
  if (newProxy !== undefined) globalState.actualSystemProxy = !!newProxy;
  if (newTun !== undefined) globalState.actualTun = !!newTun;

  const revision = rawData.controlRevision ?? rawData.ControlRevision;
  if (revision === undefined || revision >= globalState.controlRevision) {
    if (revision !== undefined) globalState.controlRevision = revision;
    const desiredProxy = rawData.desiredSystemProxy ?? rawData.DesiredSystemProxy;
    const desiredTun = rawData.desiredTun ?? rawData.DesiredTun;
    const desiredPortProxy = rawData.desiredPortProxy ?? rawData.DesiredPortProxy;
    if (desiredProxy !== undefined) globalState.desiredSystemProxy = !!desiredProxy;
    if (desiredTun !== undefined) globalState.desiredTun = !!desiredTun;
    if (desiredPortProxy !== undefined) globalState.desiredPortProxy = !!desiredPortProxy;
    syncVisibleControls();
  }

  // route-aware IP 刷新：基于 actual 状态变化
  const proxyChanged = newProxy !== undefined && oldActualProxy !== !!newProxy;
  const tunChanged = newTun !== undefined && oldActualTun !== !!newTun;
  const runningChanged = oldRunning !== globalState.isRunning;

  if (proxyChanged || tunChanged || runningChanged) {
    scheduleRouteAwareIPRefresh('state-change');
  }

  if (rawData.version !== undefined) globalState.version = rawData.version;
  else if (rawData.Version !== undefined) globalState.version = rawData.Version;

  if (rawData.appVersion !== undefined) globalState.appVersion = rawData.appVersion;
  else if (rawData.AppVersion !== undefined) globalState.appVersion = rawData.AppVersion;

  if (rawData.isAdmin !== undefined) globalState.isAdmin = rawData.isAdmin;
  else if (rawData.IsAdmin !== undefined) globalState.isAdmin = rawData.IsAdmin;

  if (rawData.activeConfig !== undefined) {
    globalState.activeConfigId = rawData.activeConfig;
    localStorage.setItem('goclashz_activeConfigId', rawData.activeConfig);
  } else if (rawData.ActiveConfig !== undefined) {
    globalState.activeConfigId = rawData.ActiveConfig;
    localStorage.setItem('goclashz_activeConfigId', rawData.ActiveConfig);
  }

  if (rawData.activeConfigName !== undefined) globalState.activeConfigName = rawData.activeConfigName;
  else if (rawData.ActiveConfigName !== undefined) globalState.activeConfigName = rawData.ActiveConfigName;

  if (rawData.activeConfigType !== undefined) globalState.activeConfigType = rawData.activeConfigType;
  else if (rawData.ActiveConfigType !== undefined) globalState.activeConfigType = rawData.ActiveConfigType;

  if (rawData.delayRetention !== undefined) globalState.delayRetention = rawData.delayRetention;
  else if (rawData.DelayRetention !== undefined) globalState.delayRetention = rawData.DelayRetention;

  if (rawData.delayRetentionTime !== undefined) globalState.delayRetentionTime = rawData.delayRetentionTime;
  else if (rawData.DelayRetentionTime !== undefined) globalState.delayRetentionTime = rawData.DelayRetentionTime;

  if (rawData.updateReady !== undefined) globalState.updateReady = rawData.updateReady;
  else if (rawData.UpdateReady !== undefined) globalState.updateReady = rawData.UpdateReady;

  if (rawData.newAppVersion !== undefined) globalState.newAppVersion = rawData.newAppVersion;
  else if (rawData.NewAppVersion !== undefined) globalState.newAppVersion = rawData.NewAppVersion;

  if (rawData.updateDownloaded !== undefined) globalState.updateDownloaded = rawData.updateDownloaded;
  else if (rawData.UpdateDownloaded !== undefined) globalState.updateDownloaded = rawData.UpdateDownloaded;

  if (rawData.downloadedPath !== undefined) globalState.downloadedPath = rawData.downloadedPath;
  else if (rawData.DownloadedPath !== undefined) globalState.downloadedPath = rawData.DownloadedPath;

  const newPortProxyMode = rawData.portProxyMode ?? rawData.PortProxyMode;
  if (newPortProxyMode !== undefined) globalState.portProxyMode = !!newPortProxyMode;

  const lang = rawData.language ?? rawData.Language;
  if (lang) setLocale(lang as SupportedLocale);
}

type DelayRetentionTime = 'long' | '30' | '60' | '300' | string;

/**
 * 更新延迟并处理保留逻辑
 * @param name 节点名称
 * @param delay 延迟数值
 * @param status 状态 (success, timeout, connect-error, test-error)
 * @param message 详细错误信息
 * @param retentionTime 保留时间 (s) 或 'long'
 */
export function updateProxyDelay(
  name: string,
  delay: number | null,
  status: string = 'success',
  message: string = '',
  retentionTime: DelayRetentionTime = 'long'
) {
  // 1. 更新数值
  globalState.proxyDelays[name] = { delay, status, message };

  // 2. 清理之前的计时器
  if (delayTimers[name]) {
    clearTimeout(delayTimers[name]);
    delete delayTimers[name];
  }

  // 3. 如果不是长时间保留且有值，开启全局定时清理
  if (delay !== null && retentionTime !== 'long') {
    const seconds = parseInt(retentionTime);
    if (isNaN(seconds) || seconds <= 0) return;

    delayTimers[name] = window.setTimeout(() => {
      if (globalState.proxyDelays[name]) {
        globalState.proxyDelays[name].delay = null;
        globalState.proxyDelays[name].status = 'unknown';
        globalState.proxyDelays[name].message = '';
      }
      delete delayTimers[name];
    }, seconds * 1000);
  }
}

// 长连接保护触发的“延迟”不是失败，只是自动排队；不要弹阻塞式错误框。
// 判定逻辑已收敛到 utils/errors.ts 的语义适配层（F-A4-14），这里只做调用。

/* ============================================================
 * 全局模态框（单槽 + 队列）
 * ------------------------------------------------------------
 * 语义：
 *   1. 同一时刻只有一个模态可见，其余排队（不再互相覆盖）；
 *   2. 任何被顶掉 / 被外部关闭的模态一律以「取消」结算，保证
 *      await showAlert/showConfirm 一定会继续 —— 否则调用方的
 *      finally 不执行，tunPending/systemProxyPending 会永久为 true；
 *   3. 旧式直写（直接改 globalState.modal）仍被兼容：由
 *      reconcileModalDisplacement() 每 5s 兜底结算 + 排空队列。
 * ========================================================== */
type ModalEntry = {
  id: number;
  type: 'alert' | 'confirm';
  title: string;
  message: string;
  detail: string;
  isDanger: boolean;
  /** 可选按钮文案（空串=用通用「取消/确定」）；用于语义化确认，如提权提示 */
  confirmText?: string;
  cancelText?: string;
  resolve: (ok: boolean) => void;
  onConfirm?: () => void | Promise<void>;
  onCancel?: () => void | Promise<void>;
};

/** openModal 的入参：语义等价于旧式直写，但走队列并保证结算 */
export type ModalRequest = {
  message: string;
  type?: 'alert' | 'confirm';
  title?: string;
  detail?: string;
  isDanger?: boolean;
  confirmText?: string;
  cancelText?: string;
  /** 用户在模态上点「确定」后执行；此时模态已隐藏，不要再写 modal.show */
  onConfirm?: () => void | Promise<void>;
  /** 用户取消 / 点击遮罩关闭后执行 */
  onCancel?: () => void | Promise<void>;
};

let modalSeq = 0;
const modalQueue: ModalEntry[] = [];
let activeModal: ModalEntry | null = null;

function runModalAction(fn?: () => void | Promise<void>) {
  if (!fn) return;
  try {
    const ret = fn();
    if (ret && typeof (ret as Promise<void>).catch === 'function') {
      (ret as Promise<void>).catch((e) => console.error('[modal] 回调执行失败', e));
    }
  } catch (e) {
    console.error('[modal] 回调执行抛错', e);
  }
}

/** 管理器内部的按钮处理器：模板/快捷键通过它结算，永远指向「当前模态」 */
function managedModalConfirm() { settleModal(true); }
function managedModalCancel() { settleModal(false); }

function presentModal(entry: ModalEntry) {
  activeModal = entry;
  globalState.modal.type = entry.type;
  globalState.modal.title = entry.title;
  globalState.modal.message = entry.message;
  globalState.modal.detail = entry.detail;
  globalState.modal.isDanger = entry.isDanger;
  globalState.modal.confirmText = entry.confirmText || '';
  globalState.modal.cancelText = entry.cancelText || '';
  globalState.modal.onConfirm = managedModalConfirm;
  globalState.modal.onCancel = managedModalCancel;
  globalState.modal.show = true;
}

function showNextModal() {
  // 期间已有模态上台（例如 onConfirm 回调里又弹了一个）或旧式直写占用：
  // 先不动，等下一次结算时再出队，避免把新模态顶掉导致它的 Promise 悬挂。
  if (activeModal || globalState.modal.show) return;
  const next = modalQueue.shift();
  if (!next) return;
  presentModal(next);
}

/** 关闭当前模态并结算：alert 一律 resolve(true)，confirm 按 ok */
function settleModal(ok: boolean) {
  const entry = activeModal;
  activeModal = null;
  globalState.modal.show = false;
  globalState.modal.confirmText = '';
  globalState.modal.cancelText = '';
  if (entry) {
    try {
      entry.resolve(entry.type === 'alert' ? true : ok);
    } catch (e) {
      console.error('[modal] resolve 失败', e);
    }
    runModalAction(ok ? entry.onConfirm : entry.onCancel);
  }
  if (modalQueue.length > 0) {
    // 延后一帧再出队，避免同一 tick 内连开关导致模板读到中间态
    window.setTimeout(showNextModal, 0);
  }
}

/** 排空队列（当前没有任何模态可见时） */
function drainModalQueue() {
  if (activeModal) return;
  if (globalState.modal.show) return;
  if (modalQueue.length === 0) return;
  window.setTimeout(showNextModal, 0);
}

/**
 * 兜底：若当前模态被旧式直写覆盖或被外部把 show 置 false，
 * 以「取消」结算它，保证调用方的 await 一定返回。
 */
function reconcileModalDisplacement() {
  if (!activeModal) {
    drainModalQueue();
    return;
  }
  const stillManaged = globalState.modal.show
    && globalState.modal.onConfirm === managedModalConfirm
    && globalState.modal.onCancel === managedModalCancel;
  if (stillManaged) return;

  const entry = activeModal;
  activeModal = null;
  console.warn('[modal] 当前模态被外部覆盖/关闭，按取消结算:', entry.message);
  try {
    entry.resolve(entry.type === 'alert' ? true : false);
  } catch (e) {
    console.error('[modal] resolve 失败', e);
  }
  runModalAction(entry.onCancel);
  drainModalQueue();
}

function enqueueModal(req: ModalRequest): Promise<boolean> {
  return new Promise<boolean>((resolve) => {
    const entry: ModalEntry = {
      id: ++modalSeq,
      type: req.type ?? 'alert',
      title: req.title ?? (req.type === 'confirm' ? '操作确认' : '提示'),
      message: req.message,
      detail: req.detail ?? '',
      isDanger: !!req.isDanger,
      confirmText: req.confirmText,
      cancelText: req.cancelText,
      resolve,
      onConfirm: req.onConfirm,
      onCancel: req.onCancel,
    };
    const legacyOccupied = globalState.modal.show && globalState.modal.onConfirm !== managedModalConfirm;
    if (activeModal || legacyOccupied) {
      modalQueue.push(entry);
      return;
    }
    presentModal(entry);
  });
}

/** 等价于旧式「直写 globalState.modal」的受管版本：走队列、回调在结算后执行 */
export function openModal(req: ModalRequest): Promise<boolean> {
  return enqueueModal(req);
}

/** 供模板按钮使用：确认当前模态（无模态时兼容旧式直写） */
export function confirmModal(): void {
  // 先把「被旧式直写顶掉」的受管模态按取消结算，避免误把它当成本次确认
  reconcileModalDisplacement();
  if (activeModal) {
    settleModal(true);
    return;
  }
  const legacy = globalState.modal.onConfirm;
  globalState.modal.show = false;
  if (legacy) runModalAction(() => legacy());
  drainModalQueue();
}

/** 供模板按钮/遮罩使用：取消当前模态（无模态时兼容旧式直写） */
export function cancelModal(): void {
  reconcileModalDisplacement();
  if (activeModal) {
    settleModal(false);
    return;
  }
  const legacy = globalState.modal.onCancel;
  globalState.modal.show = false;
  if (legacy) runModalAction(() => legacy());
  drainModalQueue();
}

// 全局 Alert 提示框 (替代原生 alert)
// detail：可选的技术详情（等宽小块，默认折叠在正文下方），供"人话 + 原始错误"分层使用
// 已有模态时入队；重复的 alert 直接 resolve，避免同一条错误排队刷屏
export function showAlert(message: string, title: string = '提示', isDanger: boolean = false, detail: string = ''): Promise<void> {
  if (isLongConnectionDeferral(message) || isLongConnectionDeferral(detail)) {
    showToast(t('common.longConnectionDeferred'), 'info');
    return Promise.resolve();
  }
  const isDuplicate = (e: ModalEntry) => e.type === 'alert' && e.message === message && e.title === title;
  if ((activeModal && isDuplicate(activeModal)) || modalQueue.some(isDuplicate)) {
    return Promise.resolve();
  }
  return enqueueModal({ type: 'alert', title, message, detail, isDanger }).then(() => undefined);
}

// 全局 Confirm 确认框 (替代原生 confirm)
// 已有模态时入队；被顶掉/被外部关闭时以 false 结算，保证 await 一定继续
export function showConfirm(
  message: string,
  title: string = '操作确认',
  isDanger: boolean = false,
  labels?: { confirmText?: string; cancelText?: string }
): Promise<boolean> {
  return enqueueModal({ type: 'confirm', title, message, isDanger, confirmText: labels?.confirmText, cancelText: labels?.cancelText });
}

// IP 检测队列（latest-wins）
let ipDetectSeq = 0;
let pendingIPRefresh: { force?: boolean; clearBeforeStart?: boolean; reason?: string; silent?: boolean; expectedRoute?: string } | null = null;
const ipRefreshTimers: Record<string, number> = {};

/**
 * 统一调度 IP 检测（latest-wins）
 * 同 key 的重复调度会覆盖前一个
 */
export function scheduleOutboundIPRefresh(
  key: string,
  opts: {
    force?: boolean;
    delay?: number;
    clearBeforeStart?: boolean;
    reason?: string;
    silent?: boolean;
  } = {}
) {
  if (ipRefreshTimers[key]) {
    clearTimeout(ipRefreshTimers[key]);
  }

  if (opts.clearBeforeStart) {
    globalState.outboundIP = null;
  }

  ipRefreshTimers[key] = window.setTimeout(() => {
    delete ipRefreshTimers[key];
    refreshOutboundIP({
      force: opts.force,
      clearBeforeStart: opts.clearBeforeStart,
      reason: opts.reason,
      silent: opts.silent,
    });
  }, opts.delay ?? 800);
}

type OutboundRoute = 'direct' | 'proxy' | 'tun-route' | 'switching';

function getDesiredOutboundRoute(): OutboundRoute {
  if (globalState.tun) return 'tun-route';
  if (globalState.systemProxy) return 'proxy';
  return 'direct';
}

function getActualOutboundRoute(): OutboundRoute {
  if (globalState.actualTun) return 'tun-route';
  if (globalState.actualSystemProxy && globalState.isRunning) return 'proxy';
  return 'direct';
}

function getEffectiveOutboundRoute(): OutboundRoute {
  const desired = getDesiredOutboundRoute();
  const actual = getActualOutboundRoute();
  if (desired !== 'direct' && actual !== desired) return 'switching';
  return actual;
}

let lastEffectiveRoute: OutboundRoute | '' = '';
let routeSettleTimer: number | null = null;

function scheduleRouteAwareIPRefresh(reason: string) {
  const route = getEffectiveOutboundRoute();

  // 切换中：保留旧 IP，不测 direct，等 actual 稳定
  if (route === 'switching') {
    return;
  }

  // route 没变：不重复检测
  if (route === lastEffectiveRoute) {
    return;
  }

  lastEffectiveRoute = route;

  if (routeSettleTimer) {
    clearTimeout(routeSettleTimer);
  }

  const delay = route === 'tun-route' ? 1500 : route === 'proxy' ? 800 : 600;

  routeSettleTimer = window.setTimeout(() => {
    routeSettleTimer = null;
    globalState.outboundIPStale = false;
    refreshOutboundIP({
      force: true,
      clearBeforeStart: false,
      reason: `route-${route}:${reason}`,
      silent: false,
      expectedRoute: route,
    });
  }, delay);
}

export function refreshOutboundIPRouteAware(reason = 'manual') {
  const route = getEffectiveOutboundRoute();

  if (route === 'switching') {
    globalState.outboundIPStale = true;
    return;
  }

  return refreshOutboundIP({
    force: true,
    clearBeforeStart: false,
    reason: `route-${route}:${reason}`,
    silent: false,
    expectedRoute: route,
  });
}

/**
 * 执行 IP 检测（latest-wins，不丢弃新请求）
 */
export async function refreshOutboundIP(options?: {
  force?: boolean;
  clearBeforeStart?: boolean;
  reason?: string;
  silent?: boolean;
  expectedRoute?: string;
}) {
  if (globalState.ipDetecting) {
    pendingIPRefresh = {
      force: true,
      clearBeforeStart: options?.clearBeforeStart,
      reason: options?.reason,
      silent: options?.silent,
      expectedRoute: options?.expectedRoute,
    };
    return;
  }

  const seq = ++ipDetectSeq;

  if (options?.clearBeforeStart) {
    globalState.outboundIP = null;
  }

  if (!options?.silent) {
    globalState.ipDetecting = true;
  }

  try {
    const result = await App.GetOutboundIPForRoute(
      !!options?.force,
      options?.expectedRoute || ''
    );

    // 已有更新请求，丢弃旧结果
    if (seq !== ipDetectSeq) return;

    // 路由切换中：保留旧 IP，不覆盖（判定在 utils/errors.ts，后端补 code 后只改那里）
    if (isRouteSwitchingMessage(result?.message)) {
      if (!options?.silent && seq === ipDetectSeq) {
        globalState.ipDetecting = false;
      }
      return;
    }

    if (result && result.preferred) {
      const cleaned = normalizeOutboundIP(result);
      globalState.outboundIP = cleaned;
      globalState.outboundIPStale = false;
      localStorage.setItem('goclashz_outboundIP', JSON.stringify(cleaned));
    } else if (!options?.silent) {
      // 检测失败：保留旧成功结果，标记 stale
      if (globalState.outboundIP?.preferred) {
        globalState.outboundIPStale = true;
      } else {
        globalState.outboundIP = {
          preferred: '',
          ipv4: '',
          ipv6: '',
          mode: '',
          source: '',
          source4: '',
          source6: '',
          message: result?.message || '检测失败',
          message4: '',
          message6: '',
          complete: false,
        };
      }
    }
  } catch {
    if (seq !== ipDetectSeq) return;

    if (!options?.silent) {
      if (globalState.outboundIP?.preferred) {
        globalState.outboundIPStale = true;
      } else {
        globalState.outboundIP = {
          preferred: '',
          ipv4: '',
          ipv6: '',
          mode: '',
          source: '',
          source4: '',
          source6: '',
          message: '网络请求失败',
          message4: '',
          message6: '',
          complete: false,
        };
      }
    }
  } finally {
    if (!options?.silent && seq === ipDetectSeq) {
      globalState.ipDetecting = false;
    }

    // latest-wins：如果有新请求排队，立即执行
    if (pendingIPRefresh) {
      const next = pendingIPRefresh;
      pendingIPRefresh = null;
      setTimeout(() => refreshOutboundIP(next), 0);
    }
  }
}

/* ============================================================
 * 通知 UI 级别表（F-A4-18 / F-A4-14）
 * ------------------------------------------------------------
 * 只对「后端已经给出 code」的通知生效；未列入的 code 走回落逻辑。
 * A(silent)：瞬时可自愈，只记录日志；B(toast)：需知情但不阻塞；
 * C(modal)：用户必须做选择。code 与主方案 §5.2 的错误码表对齐。
 * ========================================================== */
export type NotificationUi = 'silent' | 'toast' | 'modal';

const NOTIFICATION_UI: Record<string, NotificationUi> = {
  // A：静默（占用/超时/未就绪/唤醒慢/适配器晚到）
  ASSET_WINTUN_BUSY: 'silent',
  ASSET_CORE_PROBE_TIMEOUT: 'silent',
  HELPER_WAKE_TIMEOUT: 'silent',
  TUN_ADAPTER_PENDING: 'silent',
  // B：toast（可自愈但有结果需告知）
  ASSET_SEED_UNAVAILABLE: 'toast',
  DEFERRED_RESTART_QUEUED: 'toast',
  DEFERRED_RESTART_RESUMED: 'toast',
  DEFERRED_RESTART_ABANDONED: 'toast',
  SYSTEM_PROXY_STATE_UNKNOWN: 'toast',
  STARTUP_TASK_CHECK_WARNING: 'toast',
  STARTUP_TASK_UNHEALTHY: 'toast',
  // C：模态（必须处理）
  STARTUP_RESTORE_HELPER_REQUIRED: 'modal',
  STARTUP_RESTORE_NO_CONFIG: 'modal',
  STARTUP_RESTORE_WINTUN_MISSING: 'modal',
  STARTUP_RESTORE_FAILED: 'modal',
};

/**
 * 纯函数：把一条 app-notification 判成 UI 级别（可单测，不依赖 View/App）。
 * 优先级：code 表 > success/info/warn 级 > error 级（长连接延迟降级为 toast）。
 */
export function resolveNotificationUi(notification: { level?: string; code?: string; message?: string } | null | undefined): NotificationUi {
  const level = String(notification?.level || 'info');
  const code = String(notification?.code || '');
  const message = String(notification?.message || '');
  const byCode = code ? NOTIFICATION_UI[code] : undefined;
  if (byCode) return byCode;
  if (level === 'success' || level === 'info' || level === 'warn') return 'toast';
  if (level === 'error') {
    // 长连接保护导致的「自动延后」不是失败，不该弹模态
    return isLongConnectionDeferral(message) ? 'toast' : 'modal';
  }
  return 'toast';
}

let storeInited = false;

export async function initStore() {
  if (storeInited) return;
  storeInited = true;
  // 启动 pending / 模态看门狗（幂等，多次调用无副作用）
  startPendingWatchdog();
  // 1. 初始化时进行一次真理同步，获取后端当前所有真实状态
  try {
    const initialState = await App.GetAppState();
    updateStateFromBackend(initialState);
  } catch (err) {
    reportError(err, 'boot.initialState');
  }

  // 2. 保持事件监听，响应来自 Go (托盘或后台) 的实时更新
  Events.On("app-state-sync", ({ data: newState }: { data: any }) => {
    updateStateFromBackend(newState);
  });

  // 通知路由（F-A4-14 + F-A4-18）
  // 1) 有 code 先按 code 决定 UI 级别（后端补 code 后前端只改这张表）；
  // 2) 无 code 再按 level + 文案适配层；source 不再做白名单
  //    （以前 appcore / config-switch 的失败被整段丢弃）。
  Events.On("app-notification", async ({ data: notification }: { data: { level?: string; code?: string; message?: string; source?: string } }) => {
    const level = String(notification?.level || 'info');
    const code = String(notification?.code || '');
    const message = String(notification?.message || '');
    if (!message && !code) return;

    const ui = resolveNotificationUi(notification);

    // A 类：瞬时可自愈，只记录，不打扰
    if (ui === 'silent') {
      reportError(new Error(message || code), 'notify.' + code);
      return;
    }

    // C 类：用户必须处理
    if (ui === 'modal') {
      if (code === 'STARTUP_RESTORE_HELPER_REQUIRED') {
        const confirmed = await showConfirm(
          t('overview.tunHelperConfirm'),
          t('overview.tunHelperTitle')
        );
        if (confirmed) {
          try {
            await App.InstallHelperService();
            await App.ToggleTunMode(true);
            const latest = await App.GetAppState().catch(() => null);
            if (latest) updateStateFromBackend(latest);
          } catch (e: any) {
            showAlert(t('overview.errInstallHelper', { error: displayErrorMessage(e) }), t('common.error'), true);
          }
        }
        return;
      }
      showAlert(message || t('common.error'), t('common.error'), true);
      return;
    }

    // B 类（toast）：resolver 已把 success/info/warn 与「长连接延迟」都归到这里
    if (level === 'error') reportError(new Error(message || code), 'notify.' + (code || 'unknown'));
    const kind: 'error' | 'success' | 'info' = level === 'error' ? 'error' : level === 'success' ? 'success' : 'info';
    showToast(message || t('common.notice'), kind, { duration: 4200 });
  });

  // F-A5-06：启动任务的自检告警/不健康状态此前前端零监听（Go 侧 controller.go:456/494
  // 一直在发，用户完全不知道）。这里补成 toast + 日志，不弹模态（A/B 类）。
  Events.On("startup-task-check-warning", ({ data }: { data: string }) => {
    const text = String(data || '');
    reportError(new Error(text || 'startup-task-check-warning'), 'boot.checkWarning');
    showToast(text || t('common.warning'), 'info', { duration: 5000 });
  });

  Events.On("startup-task-unhealthy", ({ data }: { data: string }) => {
    const text = String(data || '');
    reportError(new Error(text || 'startup-task-unhealthy'), 'boot.unhealthy');
    showToast(text || t('common.warning'), 'error', { duration: 6000 });
  });

  Events.On("component-update-tasks-sync", ({ data: tasksList }: { data: UpdateTaskState[] }) => {
    const newTasks: Record<string, UpdateTaskState> = {};
    if (tasksList) {
      tasksList.forEach(t => { newTasks[t.key] = t; });
    }
    globalState.componentUpdate.tasks = newTasks;
  });

  Events.On("core-update-check-start", () => { globalState.componentUpdate.checkingCoreUpdate = true; });
  Events.On("core-update-check-error", ({ data: err }: { data: string }) => {
    globalState.componentUpdate.checkingCoreUpdate = false;
  });
  Events.On("core-update-none", ({ data }: { data: any }) => {
    globalState.componentUpdate.checkingCoreUpdate = false;
    globalState.componentUpdate.pendingCoreUpdate = false;
    globalState.componentUpdate.coreUpdateInfo = {
      local: data?.local || '',
      remote: data?.remote || '',
      releaseUrl: ''
    };
  });
  Events.On("core-update-available", ({ data }: { data: any }) => {
    globalState.componentUpdate.checkingCoreUpdate = false;
    globalState.componentUpdate.pendingCoreUpdate = true;
    globalState.componentUpdate.coreUpdateInfo = {
      local: data.local || '',
      remote: data.remote || '',
      releaseUrl: data.releaseUrl || ''
    };
  });

  Events.On("core-update-start", () => { globalState.componentUpdate.updatingCore = true; });
  Events.On("core-update-success", () => {
    globalState.componentUpdate.updatingCore = false;
    globalState.componentUpdate.pendingCoreUpdate = false;
  });
  Events.On("core-update-error", () => {
    globalState.componentUpdate.updatingCore = false;
    globalState.componentUpdate.pendingCoreUpdate = false;
  });
  Events.On("core-update-cancelled", () => {
    globalState.componentUpdate.updatingCore = false;
  });

  Events.On("app-update-progress", ({ data: progress }: { data: any }) => {
    globalState.appUpdateProgress = progress;
  });
  Events.On("app-update-downloaded", ({ data: payload }: { data: any }) => {
    if (globalState.appUpdateProgress) {
      globalState.appUpdateProgress.isDownloaded = true;
      globalState.appUpdateProgress.bytesDone = globalState.appUpdateProgress.totalBytes;
      globalState.appUpdateProgress.version = payload?.version;
      globalState.appUpdateProgress.path = payload?.path;
    } else {
      globalState.appUpdateProgress = {
        totalBytes: 100,
        bytesDone: 100,
        speedBps: 0,
        etaSec: 0,
        isDownloaded: true,
        version: payload?.version,
        path: payload?.path
      };
    }
  });
  Events.On("app-update-error", () => {
    globalState.appUpdateProgress = null;
  });

  // 3. 初始化刷新出站 IP，并注册各种事件触发刷新
  refreshOutboundIP();
  
  Events.On("core-restarted", () => scheduleOutboundIPRefresh('core-restarted', { force: true, delay: 1500, reason: 'core-restarted' }));
  ['geoip', 'geosite', 'mmdb', 'asn'].forEach(key => {
    Events.On(`geo-update-${key}-success`, () => scheduleOutboundIPRefresh(`geo-${key}`, { force: true, delay: 1500, reason: `geo-${key}` }));
  });
  Events.On("core-update-success", () => scheduleOutboundIPRefresh('core-update', { force: true, delay: 1500, reason: 'core-update' }));
  Events.On("driver-install-success", () => scheduleOutboundIPRefresh('driver-install', { force: true, delay: 1500, reason: 'driver-install' }));

  // 👇 提取出一个清空延迟的通用函数
  const clearAllDelays = () => {
    globalState.proxyDelays = {};
    Object.values(delayTimers).forEach(clearTimeout);
    for (const key in delayTimers) delete delayTimers[key];
  };

  // 🛡️ 核心修复：不再监听通用的 core-restarted（因为开关系统代理也会重启内核）
  // 而是监听显式的清理指令，由后端决定何时真正需要清除历史缓存
  Events.On("delay-cache-clear", () => { void clearAllDelays(); });

  // 👇 监听测速结果，全局入库（防止切页丢失数据）
  Events.On("proxy-delay-update", ({ data }: { data: any }) => {
    if (!data || !data.name) return;

    // 🚀 核心修复：如果是 busy 状态通知，说明还在批量测速中，不要覆盖已有结果
    if (data.status === 'busy') return;

    const status = data.status || 'success';
    const message = data.message || '';
    const delay = data.delay ?? null;

    // 🛡️ 核心优化：如果本次测速失败（非 success），且本地已有成功的延迟，
    // 仅在后台自动测速（非 manual）时防止网络抖动清空数值；
    // 若为手动用户测速（source === 'manual'），如实反映当前超时/错误状态
    const existing = globalState.proxyDelays[data.name];
    if (data.source !== 'manual' && status !== 'success' && existing && existing.delay && existing.delay > 0) {
      existing.status = status;
      existing.message = message;
      return;
    }

    updateProxyDelay(
      data.name,
      delay,
      status,
      message,
      globalState.delayRetention ? globalState.delayRetentionTime : 'long'
    );
  });
}

/* ============================================================
 * pending / 模态看门狗
 * ------------------------------------------------------------
 * 场景：某个 await showConfirm/showAlert 因模态被外部覆盖、后端无响应
 * 或组件提前卸载而永不结算，调用方的 finally 不执行，开关类 pending
 * 会永久为 true —— 之后每次点击都在入口 return，表现为「点了没反应」。
 * 这里每 5s 巡检一次：超过 60s 未结算就强制复位并告警（保留日志证据）。
 * ========================================================== */
const PENDING_WATCHDOG_MS = 60_000;
const PENDING_WATCHDOG_TICK_MS = 5_000;
let pendingWatchdogStarted = false;
const pendingSince: Partial<Record<'tunPending' | 'systemProxyPending', number>> = {};

export function startPendingWatchdog(): void {
  if (pendingWatchdogStarted) return;
  pendingWatchdogStarted = true;
  window.setInterval(() => {
    const now = Date.now();
    // 先兜底结算可能被顶掉的模态，再检查 pending
    reconcileModalDisplacement();
    (['tunPending', 'systemProxyPending'] as const).forEach((key) => {
      if (!globalState[key]) {
        delete pendingSince[key];
        return;
      }
      const since = pendingSince[key];
      if (since === undefined) {
        pendingSince[key] = now;
        return;
      }
      if (now - since < PENDING_WATCHDOG_MS) return;
      console.warn(
        `[store] ${key} 超过 ${PENDING_WATCHDOG_MS / 1000}s 未结算，强制复位（可能存在未 settle 的模态或后端调用）`
      );
      globalState[key] = false;
      delete pendingSince[key];
      if (key === 'tunPending') settleTunIntent();
      else settleSystemProxyIntent();
    });
  }, PENDING_WATCHDOG_TICK_MS);
}

/* ============================================================
 * 轻提示（Toast）
 * 使用边界：成功 / 中性结果用 showToast；错误与破坏性操作仍用 showAlert/showConfirm。
 * 理由：成功是高频事件，用模态框会反复打断用户；错误必须被看见，必须用模态框。
 * ========================================================== */
let toastSeq = 0;

export function showToast(
  message: string,
  kind: ToastKind = 'info',
  opts: { title?: string; duration?: number } = {}
): number {
  const id = ++toastSeq;
  const item: ToastItem = {
    id,
    kind,
    title: opts.title ?? '',
    message,
    duration: opts.duration ?? 2400,
  };
  globalState.toasts.push(item);
  while (globalState.toasts.length > 3) {
    globalState.toasts.shift();
  }
  window.setTimeout(() => dismissToast(id), item.duration);
  return id;
}

export function dismissToast(id: number): void {
  const idx = globalState.toasts.findIndex((t) => t.id === id);
  if (idx >= 0) globalState.toasts.splice(idx, 1);
}
