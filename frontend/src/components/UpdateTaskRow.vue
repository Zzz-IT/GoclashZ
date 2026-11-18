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
import { showToast } from '../store';
import { ICONS } from '../utils/icons';
import { formatBytes, formatSpeed, formatEtaTime } from '../utils/format';
import { displayErrorMessage } from '../utils/errors';
import { reportError } from '../utils/report';
import { App } from '../api';
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


const startTask = async () => {
  const key = props.task.key;
  if (key === 'core-update') {
    await App.UpdateCoreComponentAsync();
  } else if (key === 'driver-install') {
    await App.InstallTunDriverAsync(true);
  } else {
    await App.UpdateGeoDatabaseAsync(key);
  }
};

/* G-11：下面 4 个更新 RPC 原来完全没有 catch。正常情况下它们不会 reject，
   但 RPC 通道异常时会抛 → 未处理的 Promise 拒绝（控制台红字、按钮无任何反馈）。
   统一走这个包装：记录日志 + toast 出原因，与同目录其它组件的 reportError 约定一致。 */
const runTaskAction = async (action: string, fn: () => Promise<unknown>) => {
  try {
    await fn();
  } catch (e) {
    reportError(e, 'updateTask.' + action);
    // 复用 updateTask.statusFailed（"失败"）作前缀：updateTask 段原本没有任何失败文案，
    // 这里不新增 i18n key，避免 check-i18n 先变红（Wave 2 若要独立文案可换成 updateTask.actionFailed）
    showToast(t('updateTask.statusFailed') + ': ' + displayErrorMessage(e), 'error');
  }
};

const handleTogglePause = () => {
  if (props.task.status === 'running') {
    return runTaskAction('cancel', () => App.CancelUpdateTask(props.task.key));
  }
  // Resume
  return runTaskAction('resume', () => startTask());
};

const handleRemove = () => runTaskAction('remove', () => App.RemoveUpdateTask(props.task.key));

const handleForceRetry = () => runTaskAction('retry', async () => {
  // 彻底删除缓存，重新下载
  await App.ClearUpdateCache(props.task.key);
  await startTask();
});
</script>

<style scoped>
.update-task-row {
  display: flex;
  flex-direction: column;
  gap: 8px;
  padding: 12px;
  border-radius: var(--ui-radius-sm);
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
