<template>
  <section class="traffic-card">
    <div class="traffic-head">
      <div class="title-wrap">
        <span class="title-dot"></span>
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
            <path class="wave-area up" :d="uploadAreaPath" />
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
            <path class="wave-area down" :d="downloadAreaPath" />
          </svg>
        </div>
        <span class="total-label">累计 {{ traffic.downloadTotal || '0 B' }}</span>
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, ref, watch, onMounted, onUnmounted } from 'vue';
import * as API from '../../wailsjs/go/main/App';

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

const WAVE = {
  sampleIntervalMs: 800,
  maxPoints: 64,
  deadZone: 1024, // 低于 1 KB/s 视为 0
  minVisualMax: 512 * 1024, // 最低 512 KB/s 标尺
  emaAlpha: 0.18, // 平滑系数
  peakDecay: 0.965, // 峰值缓释衰减
  peakBoost: 1.6, // 峰值留白
  maxAmplitude: 0.46, // 最大视觉高度占比
};

const makeInitialSamples = () =>
  Array.from({ length: WAVE.maxPoints }, () => 0);

const uploadSamples = ref<number[]>(makeInitialSamples());
const downloadSamples = ref<number[]>(makeInitialSamples());

const latestUpload = ref(0);
const latestDownload = ref(0);

const smoothedUpload = ref(0);
const smoothedDownload = ref(0);

const uploadPeak = ref(WAVE.minVisualMax);
const downloadPeak = ref(WAVE.minVisualMax);

watch(
  () => [props.traffic.upRaw, props.traffic.downRaw],
  ([up, down]) => {
    latestUpload.value = Number(up || 0);
    latestDownload.value = Number(down || 0);
  },
  { immediate: true }
);

const normalizeInput = (value: number) => {
  if (!Number.isFinite(value) || value < WAVE.deadZone) return 0;
  return value;
};

const smoothValue = (prev: number, next: number) => {
  return prev * (1 - WAVE.emaAlpha) + next * WAVE.emaAlpha;
};

const updatePeak = (oldPeak: number, value: number) => {
  const decayed = oldPeak * WAVE.peakDecay;
  const boosted = value * WAVE.peakBoost;
  return Math.max(WAVE.minVisualMax, decayed, boosted);
};

const compress = (value: number, scale: number) => {
  if (value <= 0) return 0;
  return Math.min(1, Math.log1p(value) / Math.log1p(scale));
};

const buildAreaPath = (
  samples: number[],
  peak: number,
  width = 320,
  height = 100
) => {
  const baseline = height;
  const topPadding = height * 0.14;
  const usableHeight = height * WAVE.maxAmplitude;
  const step = width / Math.max(samples.length - 1, 1);

  const points = samples.map((value, index) => {
    const normalized = compress(value, peak);
    return {
      x: index * step,
      y: baseline - topPadding - normalized * usableHeight,
    };
  });

  if (points.length < 2) {
    return `M0 ${baseline} L${width} ${baseline} Z`;
  }

  let d = `M0 ${baseline} L${points[0].x.toFixed(1)} ${points[0].y.toFixed(1)}`;

  for (let i = 1; i < points.length; i++) {
    const prev = points[i - 1];
    const curr = points[i];
    const midX = (prev.x + curr.x) / 2;
    const midY = (prev.y + curr.y) / 2;

    d += ` Q${prev.x.toFixed(1)} ${prev.y.toFixed(1)}, ${midX.toFixed(1)} ${midY.toFixed(1)}`;
  }

  const last = points[points.length - 1];
  d += ` T${last.x.toFixed(1)} ${last.y.toFixed(1)}`;
  d += ` L${width} ${baseline} L0 ${baseline} Z`;

  return d;
};

const uploadAreaPath = computed(() =>
  buildAreaPath(uploadSamples.value, uploadPeak.value)
);

const downloadAreaPath = computed(() =>
  buildAreaPath(downloadSamples.value, downloadPeak.value)
);

let sampleTimer: number | null = null;

const pushVisualSamples = () => {
  const up = normalizeInput(latestUpload.value);
  const down = normalizeInput(latestDownload.value);

  smoothedUpload.value = smoothValue(smoothedUpload.value, up);
  smoothedDownload.value = smoothValue(smoothedDownload.value, down);

  uploadPeak.value = updatePeak(uploadPeak.value, smoothedUpload.value);
  downloadPeak.value = updatePeak(downloadPeak.value, smoothedDownload.value);

  uploadSamples.value = [
    ...uploadSamples.value.slice(1),
    smoothedUpload.value,
  ];

  downloadSamples.value = [
    ...downloadSamples.value.slice(1),
    smoothedDownload.value,
  ];
};

onMounted(() => {
  sampleTimer = window.setInterval(pushVisualSamples, WAVE.sampleIntervalMs);
});

onUnmounted(() => {
  if (sampleTimer !== null) {
    clearInterval(sampleTimer);
    sampleTimer = null;
  }
});

const resetVisualSamples = () => {
  uploadSamples.value = makeInitialSamples();
  downloadSamples.value = makeInitialSamples();

  latestUpload.value = 0;
  latestDownload.value = 0;

  smoothedUpload.value = 0;
  smoothedDownload.value = 0;

  uploadPeak.value = WAVE.minVisualMax;
  downloadPeak.value = WAVE.minVisualMax;
};

const handleReset = async () => {
  await (API as any).ResetTrafficTotals();
  resetVisualSamples();
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

.title-dot {
  width: 14px;
  height: 14px;
  border-radius: 999px;
  background: var(--text-main);
  opacity: 0.85;
}

.reset-btn {
  border: none;
  background: var(--surface-panel);
  color: var(--text-sub);
  border-radius: 8px;
  padding: 8px 12px;
  font-size: 0.8rem;
  font-weight: 600;
  cursor: pointer;
  transition: 0.2s ease;
}

.reset-btn:hover {
  color: var(--text-main);
  background: var(--surface-hover);
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
  font-size: 0.72rem;
  font-weight: 700;
  color: var(--text-muted);
  letter-spacing: 0.08em;
}

.speed-val {
  font-family: var(--font-mono);
  font-size: 1rem;
  font-weight: 700;
  color: var(--text-main);
  font-variant-numeric: tabular-nums;
}

.wave-box {
  height: 96px;
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
}

.wave-area.up {
  fill: var(--surface-hover);
}

.wave-area.down {
  fill: var(--text-sub);
}

.total-label {
  font-size: 0.78rem;
  font-weight: 700;
  color: var(--text-sub);
  font-family: var(--font-mono);
  font-variant-numeric: tabular-nums;
}
</style>
