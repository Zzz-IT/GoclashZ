// 文件路径: frontend/src/store.ts
import { reactive } from 'vue';
import { EventsOn } from '../wailsjs/runtime/runtime';

// 定义全局响应式状态
export const globalState = reactive({
  isRunning: false,
  mode: 'rule',
  theme: 'light',
  hideLogs: false,
  tunStatus: { hasWintun: false, isAdmin: false }
});

// 在入口处调用，监听 Go 发来的状态快照
export function initStore() {
  EventsOn("app-state-sync", (newState: any) => {
    Object.assign(globalState, newState);
    
    // 处理主题原生的跟随逻辑
    if (globalState.theme === 'dark') {
      document.documentElement.classList.add('dark');
    } else {
      document.documentElement.classList.remove('dark');
    }
  });
}
