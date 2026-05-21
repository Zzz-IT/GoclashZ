<template>
  <section class="traffic-card">
    <div class="traffic-head">
      <div class="title-wrap">
        <h3>网络流量</h3>
      </div>

      <div
        class="outbound-ip"
        :title="outboundIPTitle"
        @click="refreshOutboundIP({ force: true })"
      >
        <span class="ip-label">当前出站IP</span>
        <span class="ip-value" :class="{ detecting: globalState.ipDetecting }">
          {{ outboundIPText }}
        </span>
      </div>

      <button class="reset-btn" @click="handleReset">
        重置
      </button>
    </div>

    <div class="wave-row">
      <!-- 上传 -->
      <div class="wave-col">
        <div class="wave-label-row">
          <span class="speed-label">上传速度</span>
          <strong class="speed-val">{{ traffic.up }}</strong>
        </div>
        <div class="wave-box">
          <svg class="wave-svg" viewBox="0 0 320 100" preserveAspectRatio="none">
            <path class="wave-area" :d="uploadAreaPath" />
          </svg>
        </div>
        <span class="total-label">累计 {{ traffic.uploadTotal || '0 B' }}</span>
      </div>

      <!-- 下载 -->
      <div class="wave-col">
        <div class="wave-label-row">
          <span class="speed-label">下载速度</span>
          <strong class="speed-val">{{ traffic.down }}</strong>
        </div>
        <div class="wave-box">
          <svg class="wave-svg" viewBox="0 0 320 100" preserveAspectRatio="none">
            <path class="wave-area" :d="downloadAreaPath" />
          </svg>
        </div>
        <span class="total-label">累计 {{ traffic.downloadTotal || '0 B' }}</span>
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import * as API from '../../wailsjs/go/main/App';
import { globalState, refreshOutboundIP } from '../store';
import {
  waveState,
  resetWaveState,
  buildMonotoneAreaPath,
} from '../trafficWaveState';

type TrafficSnapshot = {
  up: string;
  down: string;
  upRaw?: number;
  downRaw?: number;
  uploadTotal?: string;
  downloadTotal?: string;
  uploadTotalRaw?: number;
  downloadTotalRaw?: number;
};

const props = defineProps<{
  traffic: TrafficSnapshot;
}>();

const outboundIPText = computed(() => {
  if (globalState.ipDetecting) return '检测中...';
  if (!globalState.outboundIP) return '检测中...';
  
  const ip = globalState.outboundIP.preferred;
  const status = (globalState.outboundIP as any).status;
  const stale = (globalState.outboundIP as any).stale;
  
  if (!ip) return '检测失败';
  
  if (status === 'network_busy') {
    return ip + ' (任务繁忙)';
  } else if (status === 'proxy_starting') {
    return ip + ' (代理启动中)';
  } else if (status === 'stale' || stale) {
    return '检测失败';
  }
  
  return ip;
});

const outboundIPTitle = computed(() => {
  if (!globalState.outboundIP) return '';
  const r = globalState.outboundIP;
  const stale = (r as any).stale;
  
  const lines = [];
  if (stale && r.preferred) {
    lines.push(`上次成功检测结果：${r.preferred}`);
  }
  lines.push(`模式：${r.mode === 'proxy' ? '代理检测' : '直连检测'}`);
  lines.push(`IPv6：${r.ipv6 || '不可用'}`);
  lines.push(`IPv4：${r.ipv4 || '不可用'}`);
  lines.push(`来源：${r.source || '-'}`);
  
  return lines.join('\n');
});

const uploadAreaPath = computed(() =>
  buildMonotoneAreaPath(waveState.smoothedUploadRatios)
);

const downloadAreaPath = computed(() =>
  buildMonotoneAreaPath(waveState.smoothedDownloadRatios)
);

const handleReset = async () => {
  await (API as any).ResetTrafficTotals();
  resetWaveState();
};
</script>

<style scoped>
.traffic-card {
  padding: 24px 28px;
  border-radius: 20px;
  background: var(--surface);
  border: none;
  box-shadow: 0 4px 20px rgba(0, 0, 0, 0.03);
}

.traffic-head {
  display: grid;
  grid-template-columns: 1fr auto 1fr;
  align-items: center;
  margin-bottom: 20px;
}

.title-wrap {
  display: flex;
  align-items: center;
  gap: 10px;
  justify-self: start;
}

.title-wrap h3 {
  margin: 0;
  font-size: 1rem;
  font-weight: 600;
  color: var(--text-main);
}

.outbound-ip {
  justify-self: center;
  display: flex;
  align-items: center;
  gap: 6px;
  cursor: pointer;
  padding: 4px 10px;
  border-radius: 8px;
  transition: 0.2s;
}

.outbound-ip:hover {
  background: var(--surface-hover);
}

.ip-label {
  font-size: 0.85rem;
  font-weight: 700;
  color: var(--text-main);
  opacity: 0.8;
  white-space: nowrap;
}

.ip-value {
  font-family: var(--font-mono);
  font-size: 0.95rem;
  font-weight: 800;
  color: var(--text-main);
  white-space: nowrap;
  font-variant-numeric: tabular-nums;
  max-width: 180px;
  overflow: hidden;
  text-overflow: ellipsis;
  display: inline-block;
  vertical-align: bottom;
}

.ip-value.detecting {
  color: var(--text-muted);
}

.reset-btn {
  border: none;
  background: var(--text-main);
  color: var(--accent-fg);
  border-radius: 8px;
  padding: 8px 12px;
  font-size: 0.8rem;
  font-weight: 600;
  cursor: pointer;
  transition: 0.2s ease;
  justify-self: end;
}

.reset-btn:hover {
  opacity: 0.85;
}

.wave-row {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 16px;
}

.wave-col {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.wave-label-row {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
}

.speed-label {
  font-size: 0.85rem;
  font-weight: 700;
  color: var(--text-main);
  letter-spacing: 0.05em;
}

.speed-val {
  font-family: var(--font-mono);
  font-size: 1rem;
  font-weight: 700;
  color: var(--text-main);
  font-variant-numeric: tabular-nums;
}

.wave-box {
  height: 128px;
  overflow: hidden;
  border-radius: 10px;
  background: var(--surface-panel);
}

.wave-svg {
  width: 100%;
  height: 100%;
  display: block;
}

.wave-area {
  opacity: 1;
  fill: var(--text-main);
}

.total-label {
  font-size: 0.85rem;
  font-weight: 700;
  color: var(--text-main);
  font-family: var(--font-mono);
  font-variant-numeric: tabular-nums;
}
</style>
