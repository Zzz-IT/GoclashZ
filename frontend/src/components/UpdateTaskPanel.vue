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
import { computed, watch } from 'vue';
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

const clearFinishedTasks = async () => {
  try {
    await App.ClearFinishedUpdateTasks();
  } catch (e) {
    reportError(e, 'updateTasks.clearFinished');
  }
};

watch(tasks, (newTasks) => {
  const hasFinished = newTasks.some(t => t.status === 'success');
  if (hasFinished) {
    setTimeout(() => {
      clearFinishedTasks();
    }, 3000);
  }
}, { deep: true });
</script>

<style scoped>
.update-task-panel {
  /* 作为页面内容的第一个块，随内容一起滚动 —— 不再固定在滚动区上方
     永久占住阅读高度、也不再有"浮在页面上"的观感。 */
  margin-bottom: 12px;
  background: var(--surface-panel);
  border-radius: 12px;
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
