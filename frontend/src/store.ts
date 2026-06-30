// 文件路径: frontend/src/store.ts
import { reactive } from 'vue';
import { EventsOn } from '../wailsjs/runtime/runtime';
import * as API from '../wailsjs/go/main/App';
import { runtimeassets } from '../wailsjs/go/models';

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

let initialOutboundIP: OutboundIPResult | null = null;
try {
  initialOutboundIP = cachedOutboundIP
    ? normalizeOutboundIP(JSON.parse(cachedOutboundIP))
    : null;
} catch {
  initialOutboundIP = null;
}

// 存储全局倒计时 ID，不放在 reactive 中防止不必要的响应式开销
const delayTimers: Record<string, number> = {};

// intent 持久化：卡片状态只看 intent，不看后端 runtime
function readControlIntent(): { systemProxy?: boolean; tun?: boolean } | null {
  try {
    const raw = localStorage.getItem('goclashz_control_intent');
    return raw ? JSON.parse(raw) : null;
  } catch { return null; }
}

function persistControlIntent() {
  localStorage.setItem('goclashz_control_intent', JSON.stringify({
    systemProxy: globalState.systemProxy,
    tun: globalState.tun,
  }));
}

export function setSystemProxyIntent(value: boolean) {
  globalState.systemProxy = value;
  persistControlIntent();
}

export function setTunIntent(value: boolean) {
  globalState.tun = value;
  persistControlIntent();
}

const cachedIntent = readControlIntent();

// 定义全局响应式状态
export const globalState = reactive({
  isRunning: false,
  mode: 'rule',
  theme: cachedTheme,
  hideLogs: cachedHideLogs,
  // UI intent：卡片唯一数据源
  systemProxy: cachedIntent?.systemProxy ?? false,
  tun: cachedIntent?.tun ?? false,
  // backend actual：后端实际状态，用于诊断/IP 检测
  actualSystemProxy: false,
  actualTun: false,
  // pending：只控制重复点击，不影响亮灭
  systemProxyPending: false,
  tunPending: false,
  // 首次 hydrate 标记
  controlStateHydrated: false,
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

  // 全局模态框状态
  modal: {
    show: false,
    type: 'alert' as 'alert' | 'confirm',
    title: '',
    message: '',
    detail: '',
    isDanger: false,
    onConfirm: null as Function | null,
    onCancel: null as Function | null,
  }
});



// 👇 新增清洗规则：打破数据格式强粘合，防止大小写污染
const normalizeLogLevel = (level?: string) => {
  const v = String(level || '').toLowerCase().trim();
  if (v === 'warning') return 'warn';
  if (v === 'trace') return 'debug';
  if (v === 'fatal' || v === 'panic') return 'error';
  if (['debug', 'info', 'warn', 'error'].includes(v)) return v;
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

  const newTheme = rawData.theme ?? rawData.Theme;
  if (newTheme !== undefined) {
    globalState.theme = newTheme;
    localStorage.setItem('goclashz_theme', newTheme);
  }

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

  // 首次 hydrate：用后端 desired 初始化 intent，之后不再由后端覆盖
  if (!globalState.controlStateHydrated) {
    const cached = readControlIntent();
    const desiredProxy = rawData.desiredSystemProxy ?? rawData.DesiredSystemProxy;
    const desiredTun = rawData.desiredTun ?? rawData.DesiredTun;

    if (desiredProxy !== undefined) {
      globalState.systemProxy = !!desiredProxy;
    } else if (!cached && newProxy !== undefined) {
      globalState.systemProxy = !!newProxy;
    }

    if (desiredTun !== undefined) {
      globalState.tun = !!desiredTun;
    } else if (!cached && newTun !== undefined) {
      globalState.tun = !!newTun;
    }

    globalState.controlStateHydrated = true;
    persistControlIntent();
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

// 全局 Alert 提示框 (替代原生 alert)
export function showAlert(message: string, title: string = '提示', isDanger: boolean = false): Promise<void> {
  return new Promise((resolve) => {
    globalState.modal.type = 'alert';
    globalState.modal.title = title;
    globalState.modal.message = message;
    globalState.modal.detail = ''; // Clear detail to prevent state leakage
    globalState.modal.isDanger = isDanger;
    globalState.modal.onConfirm = () => resolve();
    globalState.modal.onCancel = () => resolve(); // Alert 模式下点击遮罩层取消也视为 resolve
    globalState.modal.show = true;
  });
}

// 全局 Confirm 确认框 (替代原生 confirm)
export function showConfirm(message: string, title: string = '操作确认', isDanger: boolean = false): Promise<boolean> {
  return new Promise((resolve) => {
    globalState.modal.type = 'confirm';
    globalState.modal.title = title;
    globalState.modal.message = message;
    globalState.modal.detail = ''; // Clear detail to prevent state leakage
    globalState.modal.isDanger = isDanger;
    globalState.modal.onConfirm = () => resolve(true);
    globalState.modal.onCancel = () => resolve(false);
    globalState.modal.show = true;
  });
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
    const result = await (API as any).GetOutboundIPForRoute(
      !!options?.force,
      options?.expectedRoute || ''
    );

    // 已有更新请求，丢弃旧结果
    if (seq !== ipDetectSeq) return;

    // 路由切换中：保留旧 IP，不覆盖
    if (result?.message?.includes('路由切换中')) {
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

let storeInited = false;

export async function initStore() {
  if (storeInited) return;
  storeInited = true;
  // 1. 初始化时进行一次真理同步，获取后端当前所有真实状态
  try {
    const initialState = await API.GetAppState();
    updateStateFromBackend(initialState);
  } catch (err) {
    console.error("初始化应用状态失败:", err);
  }

  // 2. 保持事件监听，响应来自 Go (托盘或后台) 的实时更新
  EventsOn("app-state-sync", (newState: any) => {
    updateStateFromBackend(newState);
  });

  EventsOn("component-update-tasks-sync", (tasksList: UpdateTaskState[]) => {
    const newTasks: Record<string, UpdateTaskState> = {};
    if (tasksList) {
      tasksList.forEach(t => { newTasks[t.key] = t; });
    }
    globalState.componentUpdate.tasks = newTasks;
  });

  EventsOn("core-update-check-start", () => { globalState.componentUpdate.checkingCoreUpdate = true; });
  EventsOn("core-update-check-error", (err: string) => {
    globalState.componentUpdate.checkingCoreUpdate = false;
  });
  EventsOn("core-update-none", (data: any) => {
    globalState.componentUpdate.checkingCoreUpdate = false;
    globalState.componentUpdate.pendingCoreUpdate = false;
    globalState.componentUpdate.coreUpdateInfo = {
      local: data?.local || '',
      remote: data?.remote || '',
      releaseUrl: ''
    };
  });
  EventsOn("core-update-available", (data: any) => {
    globalState.componentUpdate.checkingCoreUpdate = false;
    globalState.componentUpdate.pendingCoreUpdate = true;
    globalState.componentUpdate.coreUpdateInfo = {
      local: data.local || '',
      remote: data.remote || '',
      releaseUrl: data.releaseUrl || ''
    };
  });

  EventsOn("core-update-start", () => { globalState.componentUpdate.updatingCore = true; });
  EventsOn("core-update-success", () => {
    globalState.componentUpdate.updatingCore = false;
    globalState.componentUpdate.pendingCoreUpdate = false;
  });
  EventsOn("core-update-error", () => {
    globalState.componentUpdate.updatingCore = false;
    globalState.componentUpdate.pendingCoreUpdate = false;
  });
  EventsOn("core-update-cancelled", () => {
    globalState.componentUpdate.updatingCore = false;
  });

  EventsOn("app-update-progress", (progress: any) => {
    globalState.appUpdateProgress = progress;
  });
  EventsOn("app-update-downloaded", (payload: any) => {
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
  EventsOn("app-update-error", () => {
    globalState.appUpdateProgress = null;
  });

  // 3. 初始化刷新出站 IP，并注册各种事件触发刷新
  refreshOutboundIP();
  
  EventsOn("core-restarted", () => scheduleOutboundIPRefresh('core-restarted', { force: true, delay: 1500, reason: 'core-restarted' }));
  ['geoip', 'geosite', 'mmdb', 'asn'].forEach(key => {
    EventsOn(`geo-update-${key}-success`, () => scheduleOutboundIPRefresh(`geo-${key}`, { force: true, delay: 1500, reason: `geo-${key}` }));
  });
  EventsOn("core-update-success", () => scheduleOutboundIPRefresh('core-update', { force: true, delay: 1500, reason: 'core-update' }));
  EventsOn("driver-install-success", () => scheduleOutboundIPRefresh('driver-install', { force: true, delay: 1500, reason: 'driver-install' }));

  // 👇 提取出一个清空延迟的通用函数
  const clearAllDelays = () => {
    globalState.proxyDelays = {};
    Object.values(delayTimers).forEach(clearTimeout);
    for (const key in delayTimers) delete delayTimers[key];
  };

  // 🛡️ 核心修复：不再监听通用的 core-restarted（因为开关系统代理也会重启内核）
  // 而是监听显式的清理指令，由后端决定何时真正需要清除历史缓存
  EventsOn("delay-cache-clear", clearAllDelays);

  // 👇 监听测速结果，全局入库（防止切页丢失数据）
  EventsOn("proxy-delay-update", (data: any) => {
    if (!data || !data.name) return;

    // 🚀 核心修复：如果是 busy 状态通知，说明还在批量测速中，不要覆盖已有结果
    if (data.status === 'busy') return;

    const status = data.status || 'success';
    const message = data.message || '';
    const delay = data.delay ?? null;

    // 🛡️ 核心优化：如果本次测速失败（非 success），且本地已有成功的延迟，则不覆盖数值，仅更新消息以供 UI 提示
    const existing = globalState.proxyDelays[data.name];
    if (status !== 'success' && existing && existing.delay && existing.delay > 0) {
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
