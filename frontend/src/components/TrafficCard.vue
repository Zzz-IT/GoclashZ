<template>
  <section class="traffic-card ui-surface-card">
    <div class="traffic-head">
      <div class="title-wrap">
        <h3>{{ t('traffic.title') }}</h3>
      </div>

      <div
        class="outbound-ip ui-hover-surface"
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
        <div class="wave-box ui-surface-control">
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
        <div class="wave-box ui-surface-control">
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
import { computed, watch } from 'vue';
import { App } from '../api';
import { globalState, refreshOutboundIPRouteAware, showAlert, showConfirm, showToast } from '../store';
import { displayErrorMessage } from '../utils/errors';
import { reportError } from '../utils/report';
import { t } from '../locales';
import {
  waveState,
  resetWaveState,
  buildMonotoneAreaPath,
  updateLatestTraffic,
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

watch(
  () => [props.traffic?.upRaw, props.traffic?.downRaw],
  ([up, down]) => {
    updateLatestTraffic(Number(up || 0), Number(down || 0));
  },
  { immediate: true }
);

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
  try {
    await App.ResetTrafficTotals();
    resetWaveState();
    // F-A4-12：成功用 toast
    showToast(t('traffic.resetSuccess'), 'success');
  } catch (e) {
    // F-A4-22：重置失败以前既无提示也不复位，用户以为已清零
    reportError(e, 'traffic.reset');
    showToast(displayErrorMessage(e), 'error');
  }
};
</script>

<style scoped>
.traffic-card {
  padding: 24px 28px;
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
  border: none;
  background: var(--ui-accent-block);
  color: var(--ui-accent-fg);
  border-radius: 8px;
  padding: 8px 14px;
  font-size: 0.8rem;
  font-weight: 600;
  cursor: pointer;
  transition: 0.2s ease;
  justify-self: end;
  white-space: nowrap;
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

/* 图表承载区：控件凹槽 surface 由 .ui-surface-control 提供，这里只负责尺寸与形状 */
.wave-box {
  width: 100%;
  height: 128px;
  overflow: hidden;
  border-radius: 10px;
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

@media (max-width: 700px) {
  .traffic-head {
    grid-template-columns: minmax(0, 1fr) auto;
    row-gap: 8px;
  }

  .outbound-ip {
    grid-column: 1 / -1;
    grid-row: 2;
    justify-self: stretch;
    justify-content: center;
  }

  .ip-value {
    max-width: none;
    white-space: normal;
    overflow-wrap: anywhere;
    text-align: center;
  }
}

@keyframes pulse {
  0%, 100% { opacity: 0.4; }
  50% { opacity: 1; }
}
</style>
