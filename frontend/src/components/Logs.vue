<template>
  <div class="logs-wrapper glass-card">
    <div class="log-header">
      <span>实时内核日志</span>
      <button @click="logs = []">清空</button>
    </div>
    <div class="log-content" ref="logContainer">
      <div v-for="(log, i) in logs" :key="i" class="log-line">
        <span class="time">[{{ log.time }}]</span>
        <span :class="['level', log.type]">{{ log.type.toUpperCase() }}</span>
        <span class="msg">{{ log.payload }}</span>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted, nextTick } from 'vue';
import { EventsOn } from '../../wailsjs/runtime/runtime';
import { StartStreamingLogs, StopStreamingLogs } from '../../wailsjs/go/main/App';

const logs = ref<any[]>([]);
const logContainer = ref<HTMLElement | null>(null);

onMounted(() => {
  StartStreamingLogs();
  EventsOn("log-message", (msg: any) => {
    logs.value.push({ ...msg, time: new Date().toLocaleTimeString() });
    if (logs.value.length > 200) logs.value.shift();
    nextTick(() => {
      if (logContainer.value) logContainer.value.scrollTop = logContainer.value.scrollHeight;
    });
  });
});

onUnmounted(() => StopStreamingLogs());
</script>

<style scoped>
.logs-wrapper { height: 100%; display: flex; flex-direction: column; overflow: hidden; }
.log-header { padding: 12px 20px; display: flex; justify-content: space-between; align-items: center; }
.log-content { flex: 1; overflow-y: auto; padding: 15px; font-family: 'Consolas', monospace; font-size: 0.85rem; }
.log-line { margin-bottom: 4px; line-height: 1.4; display: flex; gap: 8px; }
.time { color: var(--text-sub); min-width: 80px; }
.level { font-weight: bold; min-width: 50px; }
.level.info { color: var(--text-main); }
.level.warning { color: var(--text-sub); }
.level.error { color: var(--text-main); font-weight: 800; }
.msg { color: var(--text-main); word-break: break-all; }
</style>