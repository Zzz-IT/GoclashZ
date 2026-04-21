// 文件路径: frontend/src/store.ts
import { reactive } from 'vue';
import { EventsOn } from '../wailsjs/runtime/runtime';

// 定义全局响应式状态
export const globalState = reactive({
  isRunning: false,
  mode: 'rule',
  theme: 'light',
  hideLogs: false,
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
    Object.assign(globalState, newState);
  });
}
