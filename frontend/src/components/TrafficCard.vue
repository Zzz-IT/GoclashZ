<template>
  <section class="traffic-card">
    <div class="traffic-head">
      <div class="title-wrap">
        <h3>网络流量</h3>
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
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 20px;
}

.title-wrap {
  display: flex;
  align-items: center;
  gap: 10px;
}

.title-wrap h3 {
  margin: 0;
  font-size: 1rem;
  font-weight: 600;
  color: var(--text-main);
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
