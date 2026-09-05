<template>
  <section class="traffic-card">
    <div class="traffic-head">
      <div class="title-wrap">
        <h3>{{ t('traffic.title') }}</h3>
      </div>

      <div
        class="outbound-ip"
        :title="outboundIPTitle"
        @click="refreshOutboundIPRouteAware('manual')"
      >
        <span class="ip-label">{{ t('traffic.currentOutboundIP') }}</span>
        <span
          class="ip-value"
          :class="{ detecting: !outboundIPText || outboundIPText === t('traffic.failed') }"
          :aria-label="outboundIPText"
        >
          {{ outboundIPText }}
        </span>
        <span v-if="globalState.ipDetecting && outboundIPHasValue" class="ip-refreshing">{{ t('traffic.refreshing') }}</span>
      </div>

      <button class="reset-btn" @click="handleReset">
        {{ t('traffic.resetBtn') }}
      </button>
    </div>

    <div class="wave-row">
      <!-- 上传 -->
      <div class="wave-col">
        <div class="wave-label-row">
          <span class="speed-label">{{ t('traffic.uploadSpeed') }}</span>
          <strong class="speed-val">{{ traffic.up }}</strong>
        </div>
        <div class="wave-box">
          <svg class="wave-svg" viewBox="0 0 320 100" preserveAspectRatio="none">
            <path class="wave-area" :d="uploadAreaPath" />
          </svg>
        </div>
        <span class="total-label">{{ t('traffic.totalUsed', { val: traffic.uploadTotal || '0 B' }) }}</span>
      </div>

      <!-- 下载 -->
      <div class="wave-col">
        <div class="wave-label-row">
          <span class="speed-label">{{ t('traffic.downloadSpeed') }}</span>
          <strong class="speed-val">{{ traffic.down }}</strong>
        </div>
        <div class="wave-box">
          <svg class="wave-svg" viewBox="0 0 320 100" preserveAspectRatio="none">
            <path class="wave-area" :d="downloadAreaPath" />
          </svg>
        </div>
        <span class="total-label">{{ t('traffic.totalUsed', { val: traffic.downloadTotal || '0 B' }) }}</span>
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import * as API from '../../wailsjs/go/main/App';
import { globalState, refreshOutboundIPRouteAware, showAlert, showConfirm } from '../store';
import { t } from '../locales';
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

const outboundIPHasValue = computed(() => {
  return !!(globalState.outboundIP?.preferred);
});

const outboundIPText = computed(() => {
  const r = globalState.outboundIP;

  if (!r) {
    return globalState.ipDetecting ? t('traffic.detecting') : t('traffic.undetected');
  }

  if (!r.preferred) {
    return globalState.ipDetecting ? t('traffic.detecting') : t('traffic.failed');
  }

  return r.preferred;
});

const outboundIPTitle = computed(() => {
  if (!globalState.outboundIP) return '';

  const r = globalState.outboundIP;

  const modeText =
    r.mode === 'proxy'
      ? t('traffic.modeProxy')
      : r.mode === 'tun-route'
        ? t('traffic.modeTun')
        : t('traffic.modeDirect');

  const lines = [
    `${t('traffic.modeLabel')}: ${modeText}`,
    `IPv6: ${r.ipv6 || t('traffic.unavailable')}`,
    `IPv4: ${r.ipv4 || t('traffic.unavailable')}`,
  ];

  if (r.source) {
    lines.push(`Source: ${r.source}`);
  }

  if (r.message && !r.preferred) {
    lines.push(`Status: ${r.message}`);
  }

  return lines.join('\n');
});

const uploadAreaPath = computed(() =>
  buildMonotoneAreaPath(waveState.smoothedUploadRatios)
);

const downloadAreaPath = computed(() =>
  buildMonotoneAreaPath(waveState.smoothedDownloadRatios)
);

const handleReset = async () => {
  const ok = await showConfirm(t('traffic.resetConfirmMsg'), t('traffic.resetConfirmTitle'));
  if (!ok) return;
  await (API as any).ResetTrafficTotals();
  resetWaveState();
  showAlert(t('traffic.resetSuccess'), t('common.success'));
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
  grid-template-columns: minmax(0, 1fr) auto minmax(0, 1fr);
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
  min-width: 0;
}

.outbound-ip:hover {
  background: var(--surface-hover);
}

.ip-label {
  font-size: 0.85rem;
  font-weight: 700;
  color: var(--text-main);
  white-space: nowrap;
}

.ip-value {
  font-family: var(--font-mono);
  font-size: 0.85rem;
  color: var(--text-main);
  max-width: 260px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.ip-value.detecting {
  color: var(--text-muted);
  animation: pulse 1.5s infinite;
}

.ip-refreshing {
  font-size: 0.7rem;
  color: var(--accent);
  margin-left: 2px;
  white-space: nowrap;
}

.reset-btn {
  justify-self: end;
  background: var(--surface-hover);
  border: none;
  color: var(--text-sub);
  font-size: 0.8rem;
  padding: 6px 14px;
  border-radius: 8px;
  cursor: pointer;
  transition: all 0.2s cubic-bezier(0.4, 0, 0.2, 1);
  white-space: nowrap;
}

.reset-btn:hover {
  background: var(--text-main);
  color: var(--accent-fg);
}

.wave-row {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 24px;
}

.wave-col {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.wave-label-row {
  display: flex;
  justify-content: space-between;
  align-items: baseline;
}

.speed-label {
  font-size: 0.8rem;
  color: var(--text-sub);
}

.speed-val {
  font-family: var(--font-mono);
  font-size: 1.1rem;
  font-weight: 700;
  color: var(--text-main);
}

.wave-box {
  width: 100%;
  height: 52px;
  background: var(--surface-panel);
  border-radius: 10px;
  overflow: hidden;
}

.wave-svg {
  width: 100%;
  height: 100%;
  display: block;
}

.wave-area {
  fill: var(--text-main);
  opacity: 0.08;
}

.total-label {
  font-size: 0.75rem;
  color: var(--text-sub);
  margin-top: 2px;
}

@keyframes pulse {
  0%, 100% { opacity: 0.4; }
  50% { opacity: 1; }
}
</style>
