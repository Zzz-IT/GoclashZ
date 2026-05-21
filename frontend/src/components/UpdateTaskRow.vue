<template>
  <div class="update-task-row">
    <div class="task-info">
      <span class="task-title">{{ task.title }}</span>
      <span class="task-status" :class="task.status">{{ statusText }}</span>
    </div>
    
    <div class="task-progress" v-if="['running', 'success'].includes(task.status)">
      <div class="progress-bar">
        <div class="progress-fill" :style="{ width: progressPercent + '%' }" :class="task.status"></div>
      </div>
      <div class="progress-stats" v-if="task.status === 'running'">
        <span>{{ formatBytes(task.bytesDone) }} / {{ formatBytes(task.bytesTotal) }}</span>
        <span>{{ formatSpeed(task.speedBps) }}</span>
        <span>ETA: {{ formatTime(task.etaSeconds) }}</span>
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
    case 'running': return '下载中...';
    case 'success': return '已完成';
    case 'error': return '失败';
    case 'cancelled': return '已取消';
    default: return '等待中';
  }
});

const formatBytes = (bytes: number) => {
  if (!bytes) return '0 B';
  const k = 1024;
  const sizes = ['B', 'KB', 'MB', 'GB'];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
};

const formatSpeed = (bps: number) => {
  if (!bps) return '0 B/s';
  return formatBytes(bps) + '/s';
};

const formatTime = (seconds: number) => {
  if (!seconds || seconds < 0) return '--';
  if (seconds < 60) return `${seconds}s`;
  return `${Math.floor(seconds / 60)}m ${seconds % 60}s`;
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
