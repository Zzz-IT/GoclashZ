<template>
  <transition name="panel-fade">
    <div class="update-task-panel" v-if="tasks.length > 0">
      <div class="panel-header">
        <h3>{{ t('updateTask.panelTitle') }}</h3>
      </div>
      <transition-group name="task-fade" tag="div" class="task-list">
        <UpdateTaskRow v-for="task in tasks" :key="task.key" :task="task" />
      </transition-group>
    </div>
  </transition>
</template>

<script setup lang="ts">
import { computed, onUnmounted, watch } from 'vue';
import { globalState } from '../store';
import UpdateTaskRow from './UpdateTaskRow.vue';
import { App } from '../api';
import { reportError } from '../utils/report';
import { t } from '../locales';

const tasks = computed(() => {
  return Object.values(globalState.componentUpdate.tasks).sort((a, b) => {
    if (b.startedAt !== a.startedAt) {
      return b.startedAt - a.startedAt;
    }
    return a.key.localeCompare(b.key);
  });
});

/* G-10：自动清理只删 success。后端的 ClearFinishedUpdateTasks 会把 error / cancelled 一并删掉，
   用户还没来得及读失败原因，错误行 3 秒后就消失了。这里逐个 RemoveUpdateTask，只动成功任务。 */
const clearFinishedTasks = async () => {
  const keys = tasks.value.filter(t => t.status === 'success').map(t => t.key);
  for (const key of keys) {
    try {
      await App.RemoveUpdateTask(key);
    } catch (e) {
      reportError(e, 'updateTasks.removeFinished');
    }
  }
};

let clearTimer: ReturnType<typeof setTimeout> | undefined;
const stopClearTimer = () => {
  if (clearTimer !== undefined) {
    clearTimeout(clearTimer);
    clearTimer = undefined;
  }
};

/* G-10：tasks 是 store 快照的派生值，store 每次 component-update-tasks-sync 都会整体替换 tasks 对象，
   下载进度期间后端每个进度回调都会广播一次 —— 这个 deep watch 会高频命中。
   原来每次命中都新排一个 3s 定时器且卸载不清理：定时器越积越多，离开设置页后还在打后端。
   现在始终只保留一个（clearTimeout 后重建），语义是"最后一次同步后 3 秒"再清理。 */
watch(tasks, (newTasks) => {
  if (!newTasks.some(t => t.status === 'success')) {
    stopClearTimer();
    return;
  }
  stopClearTimer();
  clearTimer = setTimeout(() => {
    clearTimer = undefined;
    void clearFinishedTasks();
  }, 3000);
}, { deep: true });

onUnmounted(stopClearTimer);
</script>

<style scoped>
.update-task-panel {
  /* 作为页面内容的第一个块，随内容一起滚动 —— 不再固定在滚动区上方
     永久占住阅读高度、也不再有"浮在页面上"的观感。 */
  margin-bottom: 12px;
  background: var(--surface-panel);
  border-radius: var(--ui-radius-lg);
  padding: 16px;
}

.panel-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 12px;
}

.panel-header h3 {
  margin: 0;
  font-size: 1rem;
  font-weight: 600;
  color: var(--text-main);
}

.clear-btn {
  background: transparent;
  border: 1px solid var(--surface-hover);
  color: var(--text-muted);
  border-radius: 6px;
  padding: 4px 10px;
  font-size: 0.8rem;
  cursor: pointer;
  transition: 0.2s;
}

.clear-btn:hover {
  background: var(--surface-hover);
  color: var(--text-main);
}

.task-list {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.task-fade-move, /* apply transition to moving elements */
.task-fade-enter-active,
.task-fade-leave-active {
  transition: all 0.4s cubic-bezier(0.25, 0.8, 0.25, 1);
}
.task-fade-enter-from {
  opacity: 0;
  transform: translateX(-15px);
}
.task-fade-leave-to {
  opacity: 0;
  transform: translateX(30px);
}
/* Simply fade and slide out in place. Absolute positioning breaks flex gaps and width. */
/* Panel fade for when the last item is removed and the entire panel unmounts */
.panel-fade-enter-active,
.panel-fade-leave-active {
  transition: all 0.4s ease;
}
.panel-fade-enter-from,
.panel-fade-leave-to {
  opacity: 0;
  transform: translateY(-10px);
}
</style>
