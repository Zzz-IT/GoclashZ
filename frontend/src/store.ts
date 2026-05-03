// 文件路径: frontend/src/store.ts
import { reactive } from 'vue';
import { EventsOn } from '../wailsjs/runtime/runtime';

// 1. 同步读取本地缓存（发生在 Vue 渲染前，绝对 0 延迟）
const cachedHideLogs = localStorage.getItem('goclashz_hideLogs') === 'true';
const cachedTheme = localStorage.getItem('goclashz_theme') || 'light';
const cachedActiveConfigId = localStorage.getItem('goclashz_activeConfigId') || ''; // 👈 新增缓存预热

// 存储全局倒计时 ID，不放在 reactive 中防止不必要的响应式开销
const delayTimers: Record<string, number> = {};

// 定义全局响应式状态
export const globalState = reactive({
  isRunning: false,
  mode: 'rule',
  theme: cachedTheme,       // 👈 换成缓存初始化
  hideLogs: cachedHideLogs, // 👈 换成缓存初始化
  // 👇 新增这三个字段
  systemProxy: false, 
  tun: false,
  version: '',
  appVersion: '', // 👈 新增
  tunStatus: { hasWintun: false, isAdmin: false },
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
  proxyDelays: {} as Record<string, { delay: number | null }>,

  // 全局模态框状态
  modal: {
    show: false,
    type: 'alert' as 'alert' | 'confirm',
    title: '',
    message: '',
    isDanger: false,
    onConfirm: null as Function | null,
    onCancel: null as Function | null,
  }
});

// 👇 新增清洗规则：打破数据格式强粘合，防止大小写污染
export function updateStateFromBackend(rawData: any) {
  if (!rawData) return;
  
  if (rawData.isRunning !== undefined) globalState.isRunning = rawData.isRunning;
  else if (rawData.IsRunning !== undefined) globalState.isRunning = rawData.IsRunning;

  // 🚀 核心修复：增加对 mode (路由模式) 的实时接收，打通从托盘到 UI 的数据流
  if (rawData.mode !== undefined) globalState.mode = rawData.mode;
  else if (rawData.Mode !== undefined) globalState.mode = rawData.Mode;

  const newTheme = rawData.theme ?? rawData.Theme;
  if (newTheme !== undefined) {
    globalState.theme = newTheme;
    localStorage.setItem('goclashz_theme', newTheme); // 存入缓存
  }

  const newHideLogs = rawData.hideLogs ?? rawData.HideLogs;
  if (newHideLogs !== undefined) {
    globalState.hideLogs = newHideLogs;
    localStorage.setItem('goclashz_hideLogs', String(newHideLogs)); // 存入缓存
  }

  // 👇 新增这三个字段的清洗逻辑
  if (rawData.systemProxy !== undefined) globalState.systemProxy = rawData.systemProxy;
  else if (rawData.SystemProxy !== undefined) globalState.systemProxy = rawData.SystemProxy;

  if (rawData.tun !== undefined) globalState.tun = rawData.tun;
  else if (rawData.Tun !== undefined) globalState.tun = rawData.Tun;

  if (rawData.version !== undefined) globalState.version = rawData.version;
  else if (rawData.Version !== undefined) globalState.version = rawData.Version;

  if (rawData.appVersion !== undefined) globalState.appVersion = rawData.appVersion;
  else if (rawData.AppVersion !== undefined) globalState.appVersion = rawData.AppVersion;

  if (rawData.activeConfig !== undefined) {
      globalState.activeConfigId = rawData.activeConfig;
      localStorage.setItem('goclashz_activeConfigId', rawData.activeConfig); // 存入缓存
  } else if (rawData.ActiveConfig !== undefined) {
      globalState.activeConfigId = rawData.ActiveConfig;
      localStorage.setItem('goclashz_activeConfigId', rawData.ActiveConfig); // 存入缓存
  }

  if (rawData.activeConfigName !== undefined) globalState.activeConfigName = rawData.activeConfigName;
  else if (rawData.ActiveConfigName !== undefined) globalState.activeConfigName = rawData.ActiveConfigName;

  if (rawData.activeConfigType !== undefined) globalState.activeConfigType = rawData.activeConfigType;
  else if (rawData.ActiveConfigType !== undefined) globalState.activeConfigType = rawData.ActiveConfigType;

  if (rawData.delayRetention !== undefined) globalState.delayRetention = rawData.delayRetention;
  else if (rawData.DelayRetention !== undefined) globalState.delayRetention = rawData.DelayRetention;

  if (rawData.delayRetentionTime !== undefined) globalState.delayRetentionTime = rawData.delayRetentionTime;
  else if (rawData.DelayRetentionTime !== undefined) globalState.delayRetentionTime = rawData.DelayRetentionTime;

  // 👇 新增：应用更新状态清洗
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
 * @param retentionTime 保留时间 (s) 或 'long'
 */
export function updateProxyDelay(name: string, delay: number | null, retentionTime: DelayRetentionTime = 'long') {
  // 1. 更新数值
  globalState.proxyDelays[name] = { delay };

  // 2. 清理之前的计时器
  if (delayTimers[name]) {
    clearTimeout(delayTimers[name]);
    delete delayTimers[name];
  }

  // 3. 如果不是长时间保留且有值，开启全局定时清理
  if (delay !== null && retentionTime !== 'long') {
    const seconds = parseInt(retentionTime);
    // 🚀 核心修复：防止传入非数字字符串（如 'success'）导致 setTimeout(NaN) 立即触发清理
    if (isNaN(seconds) || seconds <= 0) return;

    delayTimers[name] = window.setTimeout(() => {
      if (globalState.proxyDelays[name]) {
        globalState.proxyDelays[name].delay = null;
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
    globalState.modal.isDanger = isDanger;
    globalState.modal.onConfirm = () => resolve(true);
    globalState.modal.onCancel = () => resolve(false);
    globalState.modal.show = true;
  });
}

let storeInited = false;

export async function initStore() {
  if (storeInited) return;
  storeInited = true;
  // 1. 初始化时进行一次真理同步，获取后端当前所有真实状态
  try {
    const { GetAppState } = await import('../wailsjs/go/main/App');
    const initialState = await GetAppState();
    updateStateFromBackend(initialState);
  } catch (err) {
    console.error("初始化应用状态失败:", err);
  }

  // 2. 保持事件监听，响应来自 Go (托盘或后台) 的实时更新
  EventsOn("app-state-sync", (newState: any) => {
    updateStateFromBackend(newState); 
  });

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

    updateProxyDelay(data.name, data.delay, globalState.delayRetention ? globalState.delayRetentionTime : 'long');
  });
}
