<template>
  <transition name="panel-fade">
    <div class="update-task-panel" v-if="tasks.length > 0">
      <div class="panel-header">
        <h3>后台更新任务</h3>
      </div>
      <transition-group name="task-fade" tag="div" class="task-list">
        <UpdateTaskRow v-for="task in tasks" :key="task.key" :task="task" />
      </transition-group>
    </div>
  </transition>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { globalState } from '../store';
import UpdateTaskRow from './UpdateTaskRow.vue';
import * as API from '../../wailsjs/go/main/App';

const tasks = computed(() => {
  return Object.values(globalState.componentUpdate.tasks).sort((a, b) => {
    if (b.startedAt !== a.startedAt) {
      return b.startedAt - a.startedAt;
    }
    return a.key.localeCompare(b.key);
  });
});

const hasFinishedTasks = computed(() => {
  return tasks.value.some(t => ['success', 'error', 'cancelled'].includes(t.status));
});

const clearFinishedTasks = async () => {
  await (API as any).ClearFinishedUpdateTasks();
};

import { watch } from 'vue';

watch(tasks, (newTasks) => {
  const hasFinished = newTasks.some(t => ['success', 'error', 'cancelled'].includes(t.status));
  if (hasFinished) {
    setTimeout(() => {
      clearFinishedTasks();
    }, 3000);
  }
}, { deep: true });
</script>

<style scoped>
.update-task-panel {
  margin-top: 20px;
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
.task-fade-leave-active {
  /* No absolute positioning to prevent jumping */
}

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
