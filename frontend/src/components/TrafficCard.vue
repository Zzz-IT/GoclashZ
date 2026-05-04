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
  sampleIntervalMs: 600,
  maxPoints: 64, // 降回 64，每段贝塞尔约 5px，曲线更明显

  // 小流量软区间
  lowFlowCeil: 32 * 1024,
  lowFlowMaxRatio: 0.10,

  // 视觉标尺
  minScale: 1536 * 1024,
  scaleHeadroom: 1.35,

  // 平滑
  riseAlpha: 0.20,
  fallAlpha: 0.065,

  // 标尺
  scaleRiseAlpha: 0.14,
  scaleFallAlpha: 0.014,

  // 幂次
  gamma: 1.35,

  // 图形
  baselineRatio: 1.0,
  maxAmplitude: 0.60,
};

const makeInitialSamples = () =>
  Array.from({ length: WAVE.maxPoints }, () => 0);

// 🚀 核心修复：存储的是预计算好的视觉比例 (0~1)，而非原始字节值
// 这样旧波峰不会因为 scale 变化而缩水
const uploadRatios = ref<number[]>(makeInitialSamples());
const downloadRatios = ref<number[]>(makeInitialSamples());

const latestUpload = ref(0);
const latestDownload = ref(0);

const smoothedUpload = ref(0);
const smoothedDownload = ref(0);

const uploadScale = ref(WAVE.minScale);
const downloadScale = ref(WAVE.minScale);

watch(
  () => [props.traffic.upRaw, props.traffic.downRaw],
  ([up, down]) => {
    latestUpload.value = Number(up || 0);
    latestDownload.value = Number(down || 0);
  },
  { immediate: true }
);

const smoothValue = (prev: number, next: number) => {
  const alpha = next > prev ? WAVE.riseAlpha : WAVE.fallAlpha;
  return prev + (next - prev) * alpha;
};

const updateScale = (prevScale: number, value: number) => {
  const target = Math.max(WAVE.minScale, value * WAVE.scaleHeadroom);
  const alpha = target > prevScale ? WAVE.scaleRiseAlpha : WAVE.scaleFallAlpha;
  return prevScale + (target - prevScale) * alpha;
};

const clamp01 = (v: number) => Math.max(0, Math.min(1, v));

const toVisualRatio = (value: number, scale: number) => {
  if (!Number.isFinite(value) || value <= 0) return 0;

  if (value <= WAVE.lowFlowCeil) {
    return (value / WAVE.lowFlowCeil) * WAVE.lowFlowMaxRatio;
  }

  const activeRange = Math.max(scale - WAVE.lowFlowCeil, 1);
  const activeValue = value - WAVE.lowFlowCeil;
  const linear = clamp01(activeValue / activeRange);

  return WAVE.lowFlowMaxRatio +
    Math.pow(linear, WAVE.gamma) * (1 - WAVE.lowFlowMaxRatio);
};

/**
 * 渲染前对样本做轻量高斯模糊，消除棱角
 * kernel: [0.15, 0.25, 0.20, 0.25, 0.15] (5-point weighted)
 */
const blurSamples = (samples: number[]): number[] => {
  const out = new Array(samples.length);
  out[0] = samples[0];
  out[1] = samples[0] * 0.3 + samples[1] * 0.4 + samples[2] * 0.3;
  for (let i = 2; i < samples.length - 2; i++) {
    out[i] =
      samples[i - 2] * 0.1 +
      samples[i - 1] * 0.22 +
      samples[i]     * 0.36 +
      samples[i + 1] * 0.22 +
      samples[i + 2] * 0.1;
  }
  out[samples.length - 2] =
    samples[samples.length - 3] * 0.3 +
    samples[samples.length - 2] * 0.4 +
    samples[samples.length - 1] * 0.3;
  out[samples.length - 1] = samples[samples.length - 1];
  return out;
};

const buildAreaPath = (
  ratios: number[],
  width = 320,
  height = 100
) => {
  const baseline = height * WAVE.baselineRatio;
  const usableHeight = height * WAVE.maxAmplitude;
  const step = width / Math.max(ratios.length - 1, 1);

  // 模糊后再生成路径
  const smoothed = blurSamples(ratios);

  const points = smoothed.map((ratio, index) => ({
    x: index * step,
    y: baseline - ratio * usableHeight,
  }));

  if (points.length < 2) {
    return `M0 ${baseline.toFixed(1)} L${width} ${baseline.toFixed(1)} Z`;
  }

  let d = `M0 ${baseline.toFixed(1)} L${points[0].x.toFixed(1)} ${points[0].y.toFixed(1)}`;

  for (let i = 0; i < points.length - 1; i++) {
    const p0 = points[i];
    const p1 = points[i + 1];
    const midX = (p0.x + p1.x) / 2;

    d += ` C${midX.toFixed(1)} ${p0.y.toFixed(1)}, ${midX.toFixed(1)} ${p1.y.toFixed(1)}, ${p1.x.toFixed(1)} ${p1.y.toFixed(1)}`;
  }

  d += ` L${width} ${baseline.toFixed(1)} L0 ${baseline.toFixed(1)} Z`;
  return d;
};

const uploadAreaPath = computed(() =>
  buildAreaPath(uploadRatios.value)
);

const downloadAreaPath = computed(() =>
  buildAreaPath(downloadRatios.value)
);

let sampleTimer: number | null = null;

const pushVisualSamples = () => {
  smoothedUpload.value = smoothValue(smoothedUpload.value, latestUpload.value);
  smoothedDownload.value = smoothValue(smoothedDownload.value, latestDownload.value);

  uploadScale.value = updateScale(uploadScale.value, smoothedUpload.value);
  downloadScale.value = updateScale(downloadScale.value, smoothedDownload.value);

  // 🚀 核心修复：存入预计算好的视觉比例，旧峰不再受 scale 变化影响
  const upRatio = toVisualRatio(smoothedUpload.value, uploadScale.value);
  const downRatio = toVisualRatio(smoothedDownload.value, downloadScale.value);

  uploadRatios.value = [...uploadRatios.value.slice(1), upRatio];
  downloadRatios.value = [...downloadRatios.value.slice(1), downRatio];
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
  uploadRatios.value = makeInitialSamples();
  downloadRatios.value = makeInitialSamples();

  latestUpload.value = 0;
  latestDownload.value = 0;

  smoothedUpload.value = 0;
  smoothedDownload.value = 0;

  uploadScale.value = WAVE.minScale;
  downloadScale.value = WAVE.minScale;
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
