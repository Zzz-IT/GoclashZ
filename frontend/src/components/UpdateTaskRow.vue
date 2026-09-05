<template>
  <div class="update-task-row">
    <div class="task-info">
      <span class="task-title">{{ task.title }}</span>
      <div class="task-actions">
        <span class="task-status" :class="task.status">{{ statusText }}</span>
        
        <button class="icon-btn cancel-btn" @click="handleTogglePause" :title="task.status === 'running' ? t('updateTask.pauseTask') : t('updateTask.resumeTask')" :disabled="task.status === 'success'">
          <span class="icon" v-html="task.status === 'running' ? ICONS.pause : ICONS.play"></span>
        </button>

        <button class="icon-btn retry-btn" @click="handleForceRetry" :title="t('updateTask.retryTask')" :disabled="task.status === 'running' || task.status === 'success'">
          <span class="icon" v-html="ICONS.refresh"></span>
        </button>

        <button class="icon-btn cancel-btn" @click="handleRemove" :title="t('updateTask.closeTask')">
          <span class="icon" v-html="ICONS.close"></span>
        </button>
      </div>
    </div>
    
    <div class="task-progress" v-if="['running', 'success', 'cancelled'].includes(task.status)">
      <div class="progress-bar">
        <div class="progress-fill" :style="{ width: progressPercent + '%' }" :class="task.status"></div>
      </div>
      <div class="progress-stats" v-if="task.status === 'running' || task.status === 'cancelled'">
        <span>{{ formatBytes(task.bytesDone) }} / {{ formatBytes(task.bytesTotal) }}</span>
        <span v-if="task.status === 'running'">{{ formatSpeed(task.speedBps) }}</span>
        <span v-if="task.status === 'running'">ETA: {{ formatEtaTime(task.etaSeconds) }}</span>
      </div>
    </div>
    
    <div class="task-error" v-if="task.status === 'error'">
      {{ task.error }}
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import type { UpdateTaskState } from '../store';
import { ICONS } from '../utils/icons';
import { formatBytes, formatSpeed, formatEtaTime } from '../utils/format';
import * as API from '../../wailsjs/go/main/App';
import { t } from '../locales';

const props = defineProps<{
  task: UpdateTaskState
}>();

const progressPercent = computed(() => {
  if (props.task.status === 'success') return 100;
  if (!props.task.bytesTotal) return 0;
  return Math.min(100, Math.floor((props.task.bytesDone / props.task.bytesTotal) * 100));
});

const statusText = computed(() => {
  switch (props.task.status) {
    case 'running': return t('updateTask.statusDownloading');
    case 'success': return t('updateTask.statusFinished');
    case 'error': return t('updateTask.statusFailed');
    case 'cancelled': return t('updateTask.statusPaused');
    default: return t('updateTask.statusWaiting');
  }
});


const handleTogglePause = async () => {
  if (props.task.status === 'running') {
    await (API as any).CancelUpdateTask(props.task.key);
  } else {
    // Resume
    await startTask();
  }
};

const handleRemove = async () => {
  await (API as any).RemoveUpdateTask(props.task.key);
};

const handleForceRetry = async () => {
  // 彻底删除缓存，重新下载
  await (API as any).ClearUpdateCache(props.task.key);
  await startTask();
};

const startTask = async () => {
  const key = props.task.key;
  if (key === 'core-update') {
    await (API as any).UpdateCoreComponentAsync();
  } else if (key === 'driver-install') {
    await (API as any).InstallTunDriverAsync(true);
  } else {
    await (API as any).UpdateGeoDatabaseAsync(key);
  }
};
</script>

<style scoped>
.update-task-row {
  display: flex;
  flex-direction: column;
  gap: 8px;
  padding: 12px;
  border-radius: 8px;
  background: var(--surface);
  border: 1px solid var(--surface-hover);
}

.task-info {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.task-title {
  font-weight: 600;
  color: var(--text-main);
  font-size: 0.9rem;
}

.task-status {
  font-size: 0.8rem;
  font-weight: 600;
}

.task-actions {
  display: flex;
  align-items: center;
  gap: 12px;
}

.icon-btn {
  background: transparent;
  border: none;
  padding: 4px;
  border-radius: 4px;
  cursor: pointer;
  display: flex;
  align-items: center;
  justify-content: center;
  color: var(--text-muted);
  transition: all 0.2s;
}

.icon-btn .icon {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 16px;
  height: 16px;
}

.icon-btn .icon :deep(svg) {
  width: 100%;
  height: 100%;
}

.icon-btn:disabled {
  opacity: 0.3;
  cursor: not-allowed;
}

.cancel-btn:not(:disabled):hover {
  color: var(--red-text, #ef4444);
  background: rgba(239, 68, 68, 0.1);
}

.retry-btn:not(:disabled):hover {
  color: var(--accent);
  background: rgba(var(--accent-rgb, 10, 132, 255), 0.1);
}

.task-status.running { color: var(--accent); }
.task-status.success { color: var(--text-muted); }
.task-status.error { color: var(--red-text, #ef4444); }
.task-status.cancelled { color: var(--text-muted); }

.progress-bar {
  height: 6px;
  background: var(--surface-hover);
  border-radius: 3px;
  overflow: hidden;
}

.progress-fill {
  height: 100%;
  background: var(--accent);
  transition: width 0.3s ease;
}

.progress-fill.success {
  background: var(--text-muted);
}

.progress-stats {
  display: flex;
  justify-content: space-between;
  font-size: 0.75rem;
  color: var(--text-muted);
  margin-top: 4px;
  font-variant-numeric: tabular-nums;
}

.task-error {
  font-size: 0.8rem;
  color: var(--red-text, #ef4444);
  word-break: break-word;
}
</style>
