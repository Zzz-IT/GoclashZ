// 文件路径: frontend/src/store.ts
import { reactive } from 'vue';
import { EventsOn } from '../wailsjs/runtime/runtime';

// 1. 同步读取本地缓存（发生在 Vue 渲染前，绝对 0 延迟）
const cachedHideLogs = localStorage.getItem('goclashz_hideLogs') === 'true';
const cachedTheme = localStorage.getItem('goclashz_theme') || 'light';

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
  tunStatus: { hasWintun: false, isAdmin: false },

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
function updateStateFromBackend(rawData: any) {
  if (!rawData) return;
  
  if (rawData.isRunning !== undefined) globalState.isRunning = rawData.isRunning;
  else if (rawData.IsRunning !== undefined) globalState.isRunning = rawData.IsRunning;

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
}

// 全局 Alert 提示框 (替代原生 alert)
export function showAlert(message: string, title: string = '提示'): Promise<void> {
  return new Promise((resolve) => {
    globalState.modal.type = 'alert';
    globalState.modal.title = title;
    globalState.modal.message = message;
    globalState.modal.isDanger = false;
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

export function initStore() {
  // 保持事件监听，但不再处理主题 DOM，统一交给 App.vue 的 watch 处理
  EventsOn("app-state-sync", (newState: any) => {
    updateStateFromBackend(newState); // 👈 使用清洗逻辑
  });
}
