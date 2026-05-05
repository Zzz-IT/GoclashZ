/**
 * 流量波形采样状态单例
 * 将波形数据提取到组件生命周期之外，确保切换页面时不会丢失历史波形。
 * TrafficCard.vue 仅读取/写入此模块的状态，不再自己维护 ref。
 */

import { reactive } from 'vue';

const WAVE = {
  sampleIntervalMs: 400,
  maxPoints: 96,

  lowFlowCeil: 96 * 1024,
  lowFlowMaxRatio: 0.22,

  midFlowCeil: 1024 * 1024,
  midFlowRatio: 0.62,

  minScale: 2 * 1024 * 1024,
  scaleHeadroom: 1.18,

  riseAlpha: 0.30,
  fallAlpha: 0.10,

  scaleRiseAlpha: 0.045,
  scaleFallAlpha: 0.006,

  lowGamma: 0.72,
  midGamma: 0.82,
  highGamma: 0.78,

  baselineRatio: 1.0,
  maxAmplitude: 0.85,

  smoothPasses: 1,
};

const makeInitialSamples = () =>
  Array.from({ length: WAVE.maxPoints }, () => 0);

const clamp01 = (v: number) => Math.max(0, Math.min(1, v));
const pow01 = (v: number, g: number) => Math.pow(clamp01(v), g);

const toVisualRatio = (value: number, scale: number) => {
  if (!Number.isFinite(value) || value <= 0) return 0;

  if (value <= WAVE.lowFlowCeil) {
    const t = value / WAVE.lowFlowCeil;
    return pow01(t, WAVE.lowGamma) * WAVE.lowFlowMaxRatio;
  }

  if (value <= WAVE.midFlowCeil) {
    const t = (value - WAVE.lowFlowCeil) / (WAVE.midFlowCeil - WAVE.lowFlowCeil);
    return WAVE.lowFlowMaxRatio +
      pow01(t, WAVE.midGamma) * (WAVE.midFlowRatio - WAVE.lowFlowMaxRatio);
  }

  const highRange = Math.max(scale - WAVE.midFlowCeil, 1);
  const t = (value - WAVE.midFlowCeil) / highRange;
  return WAVE.midFlowRatio +
    pow01(t, WAVE.highGamma) * (1 - WAVE.midFlowRatio);
};

const smoothValue = (prev: number, next: number) => {
  const alpha = next > prev ? WAVE.riseAlpha : WAVE.fallAlpha;
  return prev + (next - prev) * alpha;
};

const updateScale = (prevScale: number, value: number) => {
  const target = Math.max(WAVE.minScale, value * WAVE.scaleHeadroom);
  const alpha = target > prevScale ? WAVE.scaleRiseAlpha : WAVE.scaleFallAlpha;
  return prevScale + (target - prevScale) * alpha;
};

// --- 持久化采样状态 ---

export const waveState = reactive({
  uploadRatios: makeInitialSamples(),
  downloadRatios: makeInitialSamples(),

  latestUpload: 0,
  latestDownload: 0,

  smoothedUpload: 0,
  smoothedDownload: 0,

  uploadScale: WAVE.minScale,
  downloadScale: WAVE.minScale,
});

let sampleTimer: number | null = null;
let refCount = 0; // 多组件挂载防重入

const pushVisualSamples = () => {
  const s = waveState;

  s.smoothedUpload = smoothValue(s.smoothedUpload, s.latestUpload);
  s.smoothedDownload = smoothValue(s.smoothedDownload, s.latestDownload);

  const upRatio = toVisualRatio(s.smoothedUpload, s.uploadScale);
  const downRatio = toVisualRatio(s.smoothedDownload, s.downloadScale);

  s.uploadScale = updateScale(s.uploadScale, s.smoothedUpload);
  s.downloadScale = updateScale(s.downloadScale, s.smoothedDownload);

  s.uploadRatios = [...s.uploadRatios.slice(1), upRatio];
  s.downloadRatios = [...s.downloadRatios.slice(1), downRatio];
};

export function startWaveSampling() {
  refCount++;
  if (sampleTimer === null) {
    sampleTimer = window.setInterval(pushVisualSamples, WAVE.sampleIntervalMs);
  }
}

export function stopWaveSampling() {
  refCount--;
  if (refCount <= 0 && sampleTimer !== null) {
    clearInterval(sampleTimer);
    sampleTimer = null;
    refCount = 0;
  }
}

export function updateLatestTraffic(upRaw: number, downRaw: number) {
  waveState.latestUpload = upRaw;
  waveState.latestDownload = downRaw;
}

export function resetWaveState() {
  waveState.uploadRatios = makeInitialSamples();
  waveState.downloadRatios = makeInitialSamples();
  waveState.latestUpload = 0;
  waveState.latestDownload = 0;
  waveState.smoothedUpload = 0;
  waveState.smoothedDownload = 0;
  waveState.uploadScale = WAVE.minScale;
  waveState.downloadScale = WAVE.minScale;
}

// --- 路径构建工具函数 ---

const smoothRatios = (samples: number[], passes = WAVE.smoothPasses): number[] => {
  let current = samples.slice();
  for (let pass = 0; pass < passes; pass++) {
    const next = new Array(current.length);
    for (let i = 0; i < current.length; i++) {
      const a = current[Math.max(0, i - 2)];
      const b = current[Math.max(0, i - 1)];
      const c = current[i];
      const d = current[Math.min(current.length - 1, i + 1)];
      const e = current[Math.min(current.length - 1, i + 2)];
      next[i] = a * 0.06 + b * 0.20 + c * 0.48 + d * 0.20 + e * 0.06;
    }
    current = next;
  }
  return current;
};

export function buildMonotoneAreaPath(
  ratios: number[],
  width = 320,
  height = 100
) {
  const baseline = height * WAVE.baselineRatio;
  const usableHeight = height * WAVE.maxAmplitude;
  const step = width / Math.max(ratios.length - 1, 1);

  const smoothed = smoothRatios(ratios);

  const points = smoothed.map((ratio, index) => ({
    x: index * step,
    y: baseline - clamp01(ratio) * usableHeight,
  }));

  if (points.length < 2) {
    return `M0 ${baseline.toFixed(1)} L${width} ${baseline.toFixed(1)} Z`;
  }

  const n = points.length;
  const dx = step;

  const d: number[] = [];
  for (let i = 0; i < n - 1; i++) {
    d[i] = (points[i + 1].y - points[i].y) / dx;
  }

  const m: number[] = new Array(n);
  m[0] = d[0];
  m[n - 1] = d[n - 2];
  for (let i = 1; i < n - 1; i++) {
    if (d[i - 1] * d[i] <= 0) {
      m[i] = 0;
    } else {
      m[i] = (d[i - 1] + d[i]) / 2;
    }
  }

  for (let i = 0; i < n - 1; i++) {
    if (d[i] === 0) {
      m[i] = 0; m[i + 1] = 0;
      continue;
    }
    const a = m[i] / d[i];
    const b = m[i + 1] / d[i];
    const h = Math.hypot(a, b);
    if (h > 3) {
      const t = 3 / h;
      m[i] = t * a * d[i];
      m[i + 1] = t * b * d[i];
    }
  }

  let path = `M0 ${baseline.toFixed(1)} L${points[0].x.toFixed(1)} ${points[0].y.toFixed(1)}`;

  for (let i = 0; i < n - 1; i++) {
    const p0 = points[i];
    const p1 = points[i + 1];
    const cp1x = p0.x + dx / 3;
    const cp1y = p0.y + (m[i] * dx) / 3;
    const cp2x = p1.x - dx / 3;
    const cp2y = p1.y - (m[i + 1] * dx) / 3;

    path += ` C${cp1x.toFixed(1)} ${cp1y.toFixed(1)}, ${cp2x.toFixed(1)} ${cp2y.toFixed(1)}, ${p1.x.toFixed(1)} ${p1.y.toFixed(1)}`;
  }

  path += ` L${width} ${baseline.toFixed(1)} L0 ${baseline.toFixed(1)} Z`;
  return path;
}

export { WAVE };
