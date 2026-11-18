<template>
  <div class="toast-stack" aria-live="polite">
    <TransitionGroup name="toast">
      <div
        v-for="item in items"
        :key="item.id"
        class="toast"
        :class="'toast--' + item.kind"
        @mouseenter="holdToast(item)"
        @mouseleave="releaseToast(item)"
      >
        <span class="toast__bar"></span>
        <div class="toast__body">
          <div v-if="item.title" class="toast__title">{{ item.title }}</div>
          <div class="toast__text">{{ item.message }}</div>
        </div>
        <button class="toast__close" @click="closeToast(item)">×</button>
      </div>
    </TransitionGroup>
  </div>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue';
import { globalState, dismissToast } from '../store';
import type { ToastItem } from '../store';

// H-22：toast 的倒计时在 store.showToast 里（本 Wave 不改 store.ts），组件侧无法直接暂停定时器，
// 于是这里维护一份展示镜像：条目到点被 store 移除时，若鼠标正停在上面就先留着，移开再消失
// —— 等价于「hover 期间不倒计时」，长文案能看完。顺带按 kind+title+message 去重，
// 避免连续两次同样的失败并排堆三条一模一样的 toast（showAlert 早就做了去重，两套策略据此对齐）。
const items = ref<ToastItem[]>([]);
// 鼠标正停留的 toast id：被 store 移除后继续冻结展示，直到鼠标离开
const hovered = new Set<number>();

const sameContent = (a: ToastItem, b: ToastItem) =>
  a.kind === b.kind && a.title === b.title && a.message === b.message;

const syncFromStore = () => {
  const store = globalState.toasts;
  const next: ToastItem[] = [];

  // 1) 已在展示的条目：仍在 store 里的原位保留（内容取新对象），已被移除但正被 hover 的冻结保留
  for (const shown of items.value) {
    const current = store.find((s) => s.id === shown.id);
    if (current) next.push(current);
    else if (hovered.has(shown.id)) next.push(shown);
  }

  // 2) 新到条目按 store 顺序追加到末尾
  for (const added of store) {
    if (!next.some((n) => n.id === added.id)) next.push(added);
  }

  // 3) 同文案去重：只保留最早出现的那一条占位（其余仍会各自到点从 store 消失）
  const deduped: ToastItem[] = [];
  for (const item of next) {
    if (!deduped.some((kept) => sameContent(kept, item))) deduped.push(item);
  }

  items.value = deduped;
};

// toast 内容不可变，只跟随 id 序列变化重算即可
watch(() => globalState.toasts.map((item) => item.id).join(','), syncFromStore, { immediate: true });

const holdToast = (item: ToastItem) => {
  hovered.add(item.id);
};

const releaseToast = (item: ToastItem) => {
  hovered.delete(item.id);
  // 冻结期间可能已经到点：移开鼠标后再同步一次，让它立即消失
  syncFromStore();
};

const closeToast = (item: ToastItem) => {
  hovered.delete(item.id);
  dismissToast(item.id);
  items.value = items.value.filter((shown) => shown.id !== item.id);
};
</script>
