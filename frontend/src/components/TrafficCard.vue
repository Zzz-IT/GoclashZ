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
            <path class="wave-line up" :d="uploadPath" />
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
            <path class="wave-line down" :d="downloadPath" />
          </svg>
        </div>
        <span class="total-label">累计 {{ traffic.downloadTotal || '0 B' }}</span>
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue';
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

const maxPoints = 60;
const uploadSamples = ref<number[]>([]);
const downloadSamples = ref<number[]>([]);

watch(
  () => [props.traffic.upRaw, props.traffic.downRaw],
  ([up, down]) => {
    uploadSamples.value.push(Number(up || 0));
    downloadSamples.value.push(Number(down || 0));

    if (uploadSamples.value.length > maxPoints) uploadSamples.value.shift();
    if (downloadSamples.value.length > maxPoints) downloadSamples.value.shift();
  },
  { immediate: true }
);

/**
 * 使用 Catmull-Rom 样条插值生成平滑曲线路径
 * 将折线转为圆润的曲线
 */
const buildSmoothPath = (samples: number[], width = 320, height = 100) => {
  if (samples.length < 2) {
    const y = height * 0.9;
    return `M0 ${y} L${width} ${y}`;
  }

  const max = Math.max(...samples, 1024);
  const step = width / Math.max(samples.length - 1, 1);
  const tension = 0.3; // 控制曲线张力，越小越圆润

  // 将采样值转为坐标点
  const points = samples.map((value, i) => ({
    x: i * step,
    y: height * 0.92 - (value / max) * height * 0.82
  }));

  let d = `M${points[0].x.toFixed(1)} ${points[0].y.toFixed(1)}`;

  for (let i = 0; i < points.length - 1; i++) {
    const p0 = points[Math.max(i - 1, 0)];
    const p1 = points[i];
    const p2 = points[i + 1];
    const p3 = points[Math.min(i + 2, points.length - 1)];

    // Catmull-Rom → Cubic Bezier 控制点
    const cp1x = p1.x + (p2.x - p0.x) * tension;
    const cp1y = p1.y + (p2.y - p0.y) * tension;
    const cp2x = p2.x - (p3.x - p1.x) * tension;
    const cp2y = p2.y - (p3.y - p1.y) * tension;

    d += ` C${cp1x.toFixed(1)} ${cp1y.toFixed(1)}, ${cp2x.toFixed(1)} ${cp2y.toFixed(1)}, ${p2.x.toFixed(1)} ${p2.y.toFixed(1)}`;
  }

  return d;
};

const buildSmoothArea = (samples: number[], width = 320, height = 100) => {
  const line = buildSmoothPath(samples, width, height);
  const lastX = samples.length < 2 ? width : ((samples.length - 1) * width) / Math.max(samples.length - 1, 1);
  return `${line} L${lastX.toFixed(1)} ${height} L0 ${height} Z`;
};

const uploadPath = computed(() => buildSmoothPath(uploadSamples.value));
const downloadPath = computed(() => buildSmoothPath(downloadSamples.value));
const uploadAreaPath = computed(() => buildSmoothArea(uploadSamples.value));
const downloadAreaPath = computed(() => buildSmoothArea(downloadSamples.value));

const handleReset = async () => {
  await (API as any).ResetTrafficTotals();
  uploadSamples.value = [];
  downloadSamples.value = [];
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

/* 左右双图布局 */
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

.wave-line {
  fill: none;
  stroke-width: 2;
  stroke-linecap: round;
  stroke-linejoin: round;
}

.wave-line.up {
  stroke: var(--text-sub);
}

.wave-line.down {
  stroke: var(--text-main);
}

.wave-area {
  opacity: 0.1;
}

.wave-area.up {
  fill: var(--text-sub);
}

.wave-area.down {
  fill: var(--text-main);
}

.total-label {
  font-size: 0.78rem;
  font-weight: 700;
  color: var(--text-sub);
  font-family: var(--font-mono);
  font-variant-numeric: tabular-nums;
}
</style>
