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
  readCachedThemeModeLabel,
  resolveIsDark,
  themeConfigFromBehavior,
  writeThemeToBehavior,
  type ThemeConfig,
  type ThemeMode
} from './utils/theme';
import { applyGlassEffect, refreshGlassTint } from './utils/glass';
import type {
  OutboundIPResult as BoundOutboundIPResult,
  UpdateTaskState as BoundUpdateTaskState
} from '../bindings/goclashz/core/appcore/models';

// C-12：契约类型直接复用 bindings 生成的定义（此前是手抄两份，后端改字段时前端编译期发现不了）。
// 仍以同名类型 re-export，UpdateTaskRow.vue 等既有 `import type { UpdateTaskState } from '../store'` 不受影响。
export type UpdateTaskState = BoundUpdateTaskState;
export type OutboundIPResult = BoundOutboundIPResult;

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

// localStorage 在隐私模式 / 配额满时会抛异常（S-05 / C-11）：模块级读抛错会让整个 store.ts
// 求值失败（App 白屏），同步过程中的写抛错会让 updateStateFromBackend 从中途中断（后续字段全不更新）。
// 与 applyThemeToApp / setProxyCardMode 里已有的 try/catch 是同一策略，这里抽成两个小函数复用。
function readLocalStorage(key: string): string | null {
  try {
    return localStorage.getItem(key);
  } catch {
    return null;
  }
}

function writeLocalStorage(key: string, value: string): void {
  try {
    localStorage.setItem(key, value);
  } catch {
    /* 隐私模式 / 配额满：忽略，不能因为写缓存失败中断状态同步 */
  }
}

// 1. 同步读取本地缓存（发生在 Vue 渲染前，绝对 0 延迟）
const cachedHideLogs = readLocalStorage('goclashz_hideLogs') === 'true';
// H1-1（U-08）：明暗初值不再读旧键 'goclashz_theme'（它可能记着 light 而用户是 system+深色），
// 改用与 themeState.config 同源的解析结果，避免首帧 html.dark 与 .app-shell.dark 不一致。
const cachedTheme = readCachedThemeModeLabel();
const cachedActiveConfigId = readLocalStorage('goclashz_activeConfigId') || ''; // 👈 新增缓存预热
const cachedOutboundIP = readLocalStorage('goclashz_outboundIP');
export type ProxyCardMode = 'standard' | 'compact' | 'list' | 'adaptive';
const validProxyCardModes: ProxyCardMode[] = ['standard', 'compact', 'list', 'adaptive'];
const savedProxyCardMode = readLocalStorage('goclashz_proxy_card_mode') as ProxyCardMode | null;
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
  /* 旧缓存键 'goclashz_theme' 的写入口已收敛到 utils/theme.ts 的 cacheTheme（它每次写 v2 时
     同步写一份旧键镜像），这里不再重复写 —— 两份写者会让两个键在异步窗口内不一致（U-08）。 */
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
  const previousMode = themeState.config.mode;
  applyThemeToApp({ ...themeState.config, mode });
  try {
    // ⚠️ 多写者约定（S-02，与 fix-5 的 Settings.vue saveBehavior 成对）：
    // AppBehavior 是单一 JSON 对象、后端 SaveAppBehavior 是整对象覆盖，两个入口都
    // 「读整份 → 只改自己的字段 → 写整份」才能不互相丢更新。
    // 这里必须保留 GET→只改 themeMode→SAVE 的读改写，不要改成盲写本地快照；
    // 设置页那侧同步改成「GET 最新 + 只合并本页脏字段」。
    const current: any = await App.GetAppBehavior();
    if (!current) return;
    current.themeMode = themeState.config.mode;
    try {
      await App.SaveAppBehavior(current);
    } catch (first) {
      /* 无感自愈（两层）：
         1) BEHAVIOR_CONFLICT = 期间别的入口（设置页/托盘）也保存了 behavior，我们这份快照过期 ——
            重新读最新版本、只改 themeMode、再存一次；
         2) 其它瞬时失败（后端正忙 / 原子替换撞上读）直接退避重试一次。
         都失败才由外层回滚到切换前的模式（不回滚会让界面与磁盘长期不一致，
         表现为重启后主题"自己变回去"），回滚后状态自洽，无需打扰用户。 */
      await new Promise((resolve) => window.setTimeout(resolve, 150));
      const retry: any = await App.GetAppBehavior();
      if (!retry) throw first;
      retry.themeMode = themeState.config.mode;
      await App.SaveAppBehavior(retry);
    }
  } catch (err) {
    /* 后端不可用（dev 环境 / 服务异常）：以前只记日志，用户点一下没反应且界面已变色，
       下次进设置页又被后端旧值顶回去，观感是"主题随机回退"。
       这里回滚到切换前的模式并明确提示，保证界面与后端真相一致。 */
    if (previousMode !== mode) {
      applyThemeToApp({ ...themeState.config, mode: previousMode });
    }
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
  // S-03：发起 overlay 那一刻的 globalState.controlRevision。
  // 后端 revision 一旦推进得比它更晚，说明后端已经对这次命令给出结论
  // （成功，或失败回滚后的真实 desired），本地 intent 就过期了，必须丢弃。
  systemProxySince: null as number | null,
  tunSince: null as number | null,
};

function syncVisibleControls() {
  globalState.systemProxy = controlOverlay.systemProxy ?? globalState.desiredSystemProxy;
  globalState.tun = controlOverlay.tun ?? globalState.desiredTun;
}

/**
 * S-03：后端 revision 更新时丢弃过期的本地 intent overlay。
 * 没有这条保护时，UI 直写的 overlay 会把后端「已经回滚的结论」再盖一次，
 * 卡片于是显示「已关闭」而系统代理实际仍开着（取决于事件与 RPC 的到达顺序）。
 * 只丢 overlay、不碰 desired：可见状态的语义仍是「overlay 优先，否则用后端 desired」。
 */
function settleStaleControlOverlay(revision: number | undefined) {
  if (revision === undefined) return;
  if (controlOverlay.systemProxy !== null && controlOverlay.systemProxySince !== null
    && revision > controlOverlay.systemProxySince) {
    controlOverlay.systemProxy = null;
    controlOverlay.systemProxySince = null;
  }
  if (controlOverlay.tun !== null && controlOverlay.tunSince !== null
    && revision > controlOverlay.tunSince) {
    controlOverlay.tun = null;
    controlOverlay.tunSince = null;
  }
}

export function setSystemProxyIntent(value: boolean) {
  controlOverlay.systemProxy = value;
  controlOverlay.systemProxySince = globalState.controlRevision;
  syncVisibleControls();
}

export function setTunIntent(value: boolean) {
  controlOverlay.tun = value;
  controlOverlay.tunSince = globalState.controlRevision;
  syncVisibleControls();
}

export function settleSystemProxyIntent() {
  controlOverlay.systemProxy = null;
  controlOverlay.systemProxySince = null;
  syncVisibleControls();
}

export function settleTunIntent() {
  controlOverlay.tun = null;
  controlOverlay.tunSince = null;
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
// S-09：warning 必须归一到 warn —— 前端等级档位只有 debug/info/warn/error/silent，
// 原来把 'warning' 原样返回，等于让 globalState.logLevel 停在一个不存在的档位上，
// 而内核实际合法值就是 warning（core/clash/config.go），所以两边必须收敛到同一套。
// silent 是 Mihomo 的合法级别，必须保留（App.vue 侧同类函数由 fix-2 对齐）。
const normalizeLogLevel = (level?: string) => {
  const v = String(level || '').toLowerCase().trim();
  if (v === 'trace') return 'debug';
  if (v === 'fatal' || v === 'panic') return 'error';
  if (v === 'warning') return 'warn';
  if (['debug', 'info', 'warn', 'error', 'silent'].includes(v)) return v;
  return 'info';
};

export function updateStateFromBackend(rawData: any) {
  if (!rawData) return;

  const oldRunning = globalState.isRunning;

  // C-10：Go 的 AppState json tag 全是 camelCase，原来的 rawData.PascalCase 兼容分支
  // 永不命中（纯死代码，还让人误以为契约是灵活的），已全部删除，只保留 camelCase 读取。
  if (rawData.isRunning !== undefined) globalState.isRunning = rawData.isRunning;

  // mode 更新
  if (rawData.mode !== undefined) globalState.mode = rawData.mode;

  // 刻意不消费后端 theme。
  // 明暗的用户意图只存在于前端（themeState.config.mode，来自 behavior.ThemeMode），
  // 后端 AppState.Theme 只是同一次意图的派生快照；快照在异步落盘完成前会是旧值，
  // 一旦回灌就会把用户刚做的切换顶回去 —— 这正是「点暗色弹回白色 / 自己来回翻」的来源。
  // 唯一需要跟随外部变化的是 system 模式，那条路由 bindSystemTheme 单独处理。

  if (rawData.hideLogs !== undefined) {
    globalState.hideLogs = rawData.hideLogs;
    writeLocalStorage('goclashz_hideLogs', String(rawData.hideLogs));
  }

  if (rawData.logLevel !== undefined) {
    globalState.logLevel = normalizeLogLevel(rawData.logLevel || 'error');
  }

  if (rawData.appLogLevel !== undefined) {
    globalState.appLogLevel = normalizeLogLevel(rawData.appLogLevel || 'info');
  }

  // 检测代理/TUN 状态变化，触发强制 IP 检测（基于 actual，不基于 intent）
  const oldActualProxy = globalState.actualSystemProxy;
  const oldActualTun = globalState.actualTun;

  // actual 优先、旧字段（无 actualSystemProxy 的早期版本）兜底
  const newProxy = rawData.actualSystemProxy ?? rawData.systemProxy;
  const newTun = rawData.actualTun ?? rawData.tun;

  // 始终更新 actual（后端真实 runtime 状态）
  if (newProxy !== undefined) globalState.actualSystemProxy = !!newProxy;
  if (newTun !== undefined) globalState.actualTun = !!newTun;

  const revision = rawData.controlRevision;
  if (revision === undefined || revision >= globalState.controlRevision) {
    if (revision !== undefined) globalState.controlRevision = revision;
    if (rawData.desiredSystemProxy !== undefined) globalState.desiredSystemProxy = !!rawData.desiredSystemProxy;
    if (rawData.desiredTun !== undefined) globalState.desiredTun = !!rawData.desiredTun;
    if (rawData.desiredPortProxy !== undefined) globalState.desiredPortProxy = !!rawData.desiredPortProxy;
    // S-03：revision 已经比 overlay 的发起时刻更新，说明后端对本次命令已有结论，
    // 先丢弃过期 overlay 再算可见状态，否则 UI 的 intent 会盖住后端回滚后的真实 desired。
    settleStaleControlOverlay(revision);
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

  if (rawData.appVersion !== undefined) globalState.appVersion = rawData.appVersion;

  if (rawData.isAdmin !== undefined) globalState.isAdmin = rawData.isAdmin;

  if (rawData.activeConfig !== undefined) {
    globalState.activeConfigId = rawData.activeConfig;
    writeLocalStorage('goclashz_activeConfigId', rawData.activeConfig);
  }

  if (rawData.activeConfigName !== undefined) globalState.activeConfigName = rawData.activeConfigName;

  if (rawData.activeConfigType !== undefined) globalState.activeConfigType = rawData.activeConfigType;

  if (rawData.delayRetention !== undefined) globalState.delayRetention = rawData.delayRetention;

  if (rawData.delayRetentionTime !== undefined) globalState.delayRetentionTime = rawData.delayRetentionTime;

  if (rawData.updateReady !== undefined) globalState.updateReady = rawData.updateReady;

  if (rawData.newAppVersion !== undefined) globalState.newAppVersion = rawData.newAppVersion;

  if (rawData.updateDownloaded !== undefined) globalState.updateDownloaded = rawData.updateDownloaded;

  if (rawData.downloadedPath !== undefined) globalState.downloadedPath = rawData.downloadedPath;

  if (rawData.portProxyMode !== undefined) globalState.portProxyMode = !!rawData.portProxyMode;

  if (rawData.language) setLocale(rawData.language as SupportedLocale);
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
 *   3. modal 的写权限只在 store 内部（presentModal/settleModal）；S-07 已确认全仓
 *      组件侧 0 处直写，旧式直写的兼容分支因此全部删除。App.vue 模板仍只读
 *      globalState.modal 渲染，字段契约不变。
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
  // 期间已有模态上台（例如 onConfirm 回调里又弹了一个）：先不动，等下一次结算时再出队，
  // 避免把新模态顶掉导致它的 Promise 悬挂。
  // 不再判 globalState.modal.show —— activeModal 非空与 show 为真现在是同一个不变量（S-07）。
  if (activeModal) return;
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
  // S-07：结算时一并清掉按钮处理器，避免模板/旧代码拿到上一条模态的残留 handler。
  globalState.modal.onConfirm = null;
  globalState.modal.onCancel = null;
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
  if (modalQueue.length === 0) return;
  window.setTimeout(showNextModal, 0);
}

// S-07：原来这里有个 reconcileModalDisplacement()，专门兜底「受管模态被旧式直写顶掉」
// 的情况。全仓复核确认组件侧 0 处直写 globalState.modal（写点只有本文件的
// presentModal / settleModal），该分支不可达，已整体删除。

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
    // S-07：不再区分「旧式直写占用」——activeModal 非空即视为已有模态在台，入队等待。
    if (activeModal) {
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

/** 供模板按钮使用：确认当前模态 */
export function confirmModal(): void {
  if (activeModal) {
    settleModal(true);
    return;
  }
  // 无受管模态（模板只在 show 为真时渲染按钮，理论上不可达）：只做兜底复位 + 排空队列
  globalState.modal.show = false;
  drainModalQueue();
}

/** 供模板按钮/遮罩使用：取消当前模态 */
export function cancelModal(): void {
  if (activeModal) {
    settleModal(false);
    return;
  }
  // 同上：无受管模态时只兜底复位，不再执行 legacy handler（S-07）
  globalState.modal.show = false;
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

  // S-11：手动刷新前先取消已排期的 route settle 定时器。
  // 否则路由刚变化（600~1500ms 窗口）时点刷新，手动请求发出后定时器到点还会再发一次，
  // 一次点击变成两次后端往返（再叠加 pending 重放最多 3 次）。
  if (routeSettleTimer) {
    clearTimeout(routeSettleTimer);
    routeSettleTimer = null;
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
      writeLocalStorage('goclashz_outboundIP', JSON.stringify(cleaned));
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
 * 通知 UI 级别表（F-A4-18 / F-A4-14 / S-04）
 * ------------------------------------------------------------
 * 只对「后端已经给出 code」的通知生效；未列入的 code 走回落逻辑。
 * A(silent)：瞬时可自愈，只记录日志；B(toast)：需知情但不阻塞；
 * C(modal)：用户必须做选择。
 *
 * ⚠️ 收录规则（S-04 复核后的口径）：只列「后端真的会产出」的 code。
 *   原来照着方案错误码表预写了 7 个 Go 侧 0 引用的 code（ASSET_WINTUN_BUSY /
 *   ASSET_CORE_PROBE_TIMEOUT / HELPER_WAKE_TIMEOUT / TUN_ADAPTER_PENDING /
 *   ASSET_SEED_UNAVAILABLE / STARTUP_TASK_CHECK_WARNING / STARTUP_TASK_UNHEALTHY），
 *   其中 4 个还是 silent 级 —— 一旦后端按方案补上同名 code，通知就会被静默吞掉；
 *   后两个真实通道是独立事件（startup-task-check-warning / -unhealthy），已单独处理。
 *   现在每个键都标了 Go 侧出处，删键/加键请对齐 c.notify / c.setLastErrorCode 的实参。
 *
 * ⚠️ 不把「与落回同档」的 code 抄进来：落回分支里还有「长连接延迟降级为 toast」
 *   这条语义（resolveNotificationUi），显式写成 modal 反而会误伤它。
 * ========================================================== */
export type NotificationUi = 'silent' | 'toast' | 'modal';

const NOTIFICATION_UI: Record<string, NotificationUi> = {
  // A：静默（用户不需要被打断，卡片/后台流程自己会体现）
  // outbound_ip.go:233，节流 5s；IP 卡自己会显示「切换中」，且消息是内部诊断串
  ROUTE_SWITCHING: 'silent',
  // controller.go:1416；后台还有自愈阶梯在跑（可重试），不打断用户
  /* TUN_NOT_READY 是后端静默阶梯走到 L3（60s 仍未就绪）才发的，含义是"自愈失败、需要用户知情"，
     不是瞬时可自愈的那一类 —— 因此归 B 类（toast），不能静默。
     （2026-11-18 实测：TUN 适配器 46s 未就绪期间界面一直显示"启动中"，若这里再静默，
     用户端就是彻底的"卡住无反应"。） */
  TUN_NOT_READY: 'toast',
  // B：toast（需知情但不阻塞）
  // controller_long_connection.go:175 / :196 / :201（ABANDONED 是 error 级，但只是延后不是失败）
  DEFERRED_RESTART_QUEUED: 'toast',
  DEFERRED_RESTART_RESUMED: 'toast',
  DEFERRED_RESTART_ABANDONED: 'toast',
  // control_coordinator.go:457（warn）
  SYSTEM_PROXY_STATE_UNKNOWN: 'toast',
  // controller.go:2234；持久化失败告知即可，不该为一次保存失败阻塞用户
  MODE_SAVE_FAILED: 'toast',
  // C：模态（用户必须处理）
  // controller.go:425 / :429 / :444 / :446 / :450
  STARTUP_RESTORE_HELPER_REQUIRED: 'modal',
  STARTUP_RESTORE_NO_CONFIG: 'modal',
  STARTUP_RESTORE_WINTUN_MISSING: 'modal',
  STARTUP_RESTORE_FAILED: 'modal',
};

/**
 * code → 前端文案 key（C-20）：后端 notify 的 message 是中文技术串，
 * 8 语言界面里不能直出。只收录「后端文案是固定句、不含变量」的 code；
 * 含错误串/参数的仍展示后端原文（丢信息比出中文更糟）。
 */
const NOTIFICATION_TEXT: Record<string, string> = {
  SYSTEM_PROXY_STATE_UNKNOWN: 'notify.systemProxyStateUnknown',
};

/** t() 在 key 缺失时原样返回 key 本身（locales/index.ts:64-67），据此回退到调用方给的原文 */
const tr = (key: string, fallback: string, params?: Record<string, string | number>) => {
  const value = t(key, params);
  return value === key ? fallback : value;
};

/** 取通知文案：有 code→key 映射就本地化，未收录（或语言包还没补）就回落后端原文 */
function localizedNotificationText(code: string, message: string): string {
  const key = code ? NOTIFICATION_TEXT[code] : '';
  return key ? tr(key, message) : message;
}

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
/** 进行中的初始化：并发的第二次 initStore() 复用它，避免同一批订阅被注册两遍（S-06） */
let storeInitPromise: Promise<void> | null = null;
/**
 * App 级事件订阅的注销函数（S-06）。
 * 这些订阅跟随主窗口生命周期，正常**不卸载**（生产环境单页常驻，无泄漏）；
 * 收集起来是为了让「初始化中途失败可回滚重试」以及将来的 HMR / 测试边界有统一出口。
 */
const eventUnsubs: Array<() => void> = [];

export async function initStore(): Promise<void> {
  // 唯一调用点是 main.ts（早于 createApp，保证首帧前开始订阅；App.vue 里那次死调用已删）：
  // - 记住进行中的 Promise：万一将来又被重复调用，也不会在首个 await 处插队把订阅注册两遍；
  // - storeInited 等到 setupStore 全部注册完成才置真（S-06）：中途抛错不会把状态
  //   永久钉在「已初始化」，下一次调用还能重试。
  if (storeInitPromise) return storeInitPromise;
  if (storeInited) return;
  const initPromise = (async () => {
    try {
      await setupStore();
      storeInited = true;
    } catch (err) {
      // 部分订阅可能已经注册：先全部注销，保证下一次调用是干净的一次性注册
      eventUnsubs.splice(0).forEach((unsub) => {
        try { unsub(); } catch { /* 注销失败不影响重试 */ }
      });
      reportError(err, 'boot.initStore');
    } finally {
      storeInitPromise = null;
    }
  })();
  storeInitPromise = initPromise;
  return initPromise;
}

/** 注销全部事件订阅并允许重新初始化（当前无调用者：单页常驻不需要 teardown，留给将来的 HMR/测试边界） */
export function disposeStore(): void {
  eventUnsubs.splice(0).forEach((unsub) => {
    try { unsub(); } catch { /* 忽略 */ }
  });
  storeInited = false;
  storeInitPromise = null;
}

/** 真正的初始化：启动看门狗 → 首次全量同步 → 注册全部事件订阅 */
async function setupStore(): Promise<void> {
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
  eventUnsubs.push(Events.On("app-state-sync", ({ data: newState }: { data: any }) => {
    updateStateFromBackend(newState);
  }));

  // 通知路由（F-A4-14 + F-A4-18）
  // 1) 有 code 先按 code 决定 UI 级别（后端补 code 后前端只改这张表）；
  // 2) 无 code 再按 level + 文案适配层；source 不再做白名单
  //    （以前 appcore / config-switch 的失败被整段丢弃）。
  eventUnsubs.push(Events.On("app-notification", async ({ data: notification }: { data: { level?: string; code?: string; message?: string; source?: string } }) => {
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

    // B 类（toast）：resolver 已把 success/info/warn 与「长连接延迟」都归到这里。
    // C-20：有 code→文案映射的走 t()；未收录的（含动态错误串）仍展示后端原文。
    if (level === 'error') reportError(new Error(message || code), 'notify.' + (code || 'unknown'));
    const kind: 'error' | 'success' | 'info' = level === 'error' ? 'error' : level === 'success' ? 'success' : 'info';
    showToast(localizedNotificationText(code, message) || t('common.notice'), kind, { duration: 4200 });
  }));

  // F-A5-06：启动任务的自检告警/不健康状态此前前端零监听（Go 侧 controller.go:456/494
  // 一直在发，用户完全不知道）。这里补成 toast + 日志，不弹模态（A/B 类）。
  eventUnsubs.push(Events.On("startup-task-check-warning", ({ data }: { data: string }) => {
    const text = String(data || '');
    reportError(new Error(text || 'startup-task-check-warning'), 'boot.checkWarning');
    showToast(text || t('common.warning'), 'info', { duration: 5000 });
  }));

  eventUnsubs.push(Events.On("startup-task-unhealthy", ({ data }: { data: string }) => {
    const text = String(data || '');
    reportError(new Error(text || 'startup-task-unhealthy'), 'boot.unhealthy');
    showToast(text || t('common.warning'), 'error', { duration: 6000 });
  }));

  eventUnsubs.push(Events.On("component-update-tasks-sync", ({ data: tasksList }: { data: UpdateTaskState[] }) => {
    const newTasks: Record<string, UpdateTaskState> = {};
    if (tasksList) {
      tasksList.forEach(t => { newTasks[t.key] = t; });
    }
    globalState.componentUpdate.tasks = newTasks;
  }));

  eventUnsubs.push(Events.On("core-update-check-start", () => { globalState.componentUpdate.checkingCoreUpdate = true; }));
  eventUnsubs.push(Events.On("core-update-check-error", ({ data: err }: { data: string }) => {
    globalState.componentUpdate.checkingCoreUpdate = false;
    /* C-04：tasks 框架用 name+"-error" 把 data 填成完整错误文本（core/tasks/manager.go），
       以前整条丢掉。现在**取出来只记日志、不弹提示**：
       检查更新失败几乎都是瞬时网络问题，按钮转 12s 后自动复位、用户随时可再点一次 ——
       属于"无感自愈"的一类（提示策略：只报无法解决/严重问题）。 */
    const text = String(err || '');
    reportError(new Error(text || 'core-update-check-error'), 'component.checkCore');
  }));
  eventUnsubs.push(Events.On("core-update-none", ({ data }: { data: any }) => {
    globalState.componentUpdate.checkingCoreUpdate = false;
    globalState.componentUpdate.pendingCoreUpdate = false;
    globalState.componentUpdate.coreUpdateInfo = {
      local: data?.local || '',
      remote: data?.remote || '',
      releaseUrl: ''
    };
  }));
  eventUnsubs.push(Events.On("core-update-available", ({ data }: { data: any }) => {
    globalState.componentUpdate.checkingCoreUpdate = false;
    globalState.componentUpdate.pendingCoreUpdate = true;
    globalState.componentUpdate.coreUpdateInfo = {
      local: data.local || '',
      remote: data.remote || '',
      releaseUrl: data.releaseUrl || ''
    };
  }));

  eventUnsubs.push(Events.On("core-update-start", () => { globalState.componentUpdate.updatingCore = true; }));
  // S-06：core-update-success 原来在 initStore 里注册了两次（这里 + 后面的 IP 刷新处），
  // 合成为一个 handler，顺序保持「先复位组件更新状态，再排期刷出站 IP」。
  eventUnsubs.push(Events.On("core-update-success", () => {
    globalState.componentUpdate.updatingCore = false;
    globalState.componentUpdate.pendingCoreUpdate = false;
    scheduleOutboundIPRefresh('core-update', { force: true, delay: 1500, reason: 'core-update' });
  }));
  eventUnsubs.push(Events.On("core-update-error", () => {
    globalState.componentUpdate.updatingCore = false;
    globalState.componentUpdate.pendingCoreUpdate = false;
  }));
  eventUnsubs.push(Events.On("core-update-cancelled", () => {
    globalState.componentUpdate.updatingCore = false;
  }));

  eventUnsubs.push(Events.On("app-update-progress", ({ data: progress }: { data: any }) => {
    globalState.appUpdateProgress = progress;
  }));
  eventUnsubs.push(Events.On("app-update-downloaded", ({ data: payload }: { data: any }) => {
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
  }));
  eventUnsubs.push(Events.On("app-update-error", () => {
    globalState.appUpdateProgress = null;
  }));

  // 3. 初始化刷新出站 IP，并注册各种事件触发刷新
  refreshOutboundIP();
  
  eventUnsubs.push(Events.On("core-restarted", () => scheduleOutboundIPRefresh('core-restarted', { force: true, delay: 1500, reason: 'core-restarted' })));
  ['geoip', 'geosite', 'mmdb', 'asn'].forEach(key => {
    eventUnsubs.push(Events.On(`geo-update-${key}-success`, () => scheduleOutboundIPRefresh(`geo-${key}`, { force: true, delay: 1500, reason: `geo-${key}` })));
  });
  // core-update-success 已合并到上方的单一 handler（S-06），这里不再重复注册
  eventUnsubs.push(Events.On("driver-install-success", () => scheduleOutboundIPRefresh('driver-install', { force: true, delay: 1500, reason: 'driver-install' })));

  // 👇 提取出一个清空延迟的通用函数
  const clearAllDelays = () => {
    globalState.proxyDelays = {};
    Object.values(delayTimers).forEach(clearTimeout);
    for (const key in delayTimers) delete delayTimers[key];
  };

  // 🛡️ 核心修复：不再监听通用的 core-restarted（因为开关系统代理也会重启内核）
  // 而是监听显式的清理指令，由后端决定何时真正需要清除历史缓存
  eventUnsubs.push(Events.On("delay-cache-clear", () => { void clearAllDelays(); }));

  // 👇 监听测速结果，全局入库（防止切页丢失数据）
  eventUnsubs.push(Events.On("proxy-delay-update", ({ data }: { data: any }) => {
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
  }));
}

/* ============================================================
 * pending / 模态看门狗
 * ------------------------------------------------------------
 * 场景：某个 await showConfirm/showAlert 因后端无响应或组件提前卸载而永不结算，
 * 调用方的 finally 不执行，开关类 pending 会永久为 true ——
 * 之后每次点击都在入口 return，表现为「点了没反应」。
 * 这里每 5s 巡检一次：超过 60s 未结算就强制复位并告警（保留日志证据）。
 *
 * ⚠️ S-01：后端单条控制命令的预算是 25s（control_coordinator.go），
 * 所以「pending 超过 60s」在实践中只可能是卡在模态框等用户点击 ——
 * 那是合法等待，不是卡死。因此只要有模态在台上（modal.show 为真）就不计时，
 * 每 tick 把起点顺延到当下，等用户处理完再重新算 60s。
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
    // 先排空可能被卡住的模态队列（activeModal 为空时才有动作），再检查 pending
    drainModalQueue();
    (['tunPending', 'systemProxyPending'] as const).forEach((key) => {
      if (!globalState[key]) {
        delete pendingSince[key];
        return;
      }
      // S-01：有模态在等用户时不计时（用户可能正在读确认框），顺延起点而非强清
      if (globalState.modal.show) {
        pendingSince[key] = now;
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
  const title = opts.title ?? '';
  const duration = opts.duration ?? 2400;

  /* H-22（后半）：同 kind + 同标题 + 同文案在存活期内不重复插入。
     连续失败的场景里（批量测速、订阅更新）以前会堆 3 条一模一样的气泡；
     这里改为"续期"：先移除旧那条（连带它的 setTimeout 到点后空跑一次 dismiss），再插入新的，
     于是视觉上只有一条，且计时从最后一次触发重新开始。 */
  const dupIndex = globalState.toasts.findIndex(
    (t) => t.kind === kind && t.title === title && t.message === message
  );
  if (dupIndex >= 0) {
    globalState.toasts.splice(dupIndex, 1);
  }

  const id = ++toastSeq;
  const item: ToastItem = {
    id,
    kind,
    title,
    message,
    duration,
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
