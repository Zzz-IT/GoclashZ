/**
 * 流量波形采样状态单例
 * 将波形数据提取到组件生命周期之外，确保切换页面时不会丢失历史波形。
 * TrafficCard.vue 仅读取/写入此模块的状态，不再自己维护 ref。
 */

import { reactive } from 'vue';

const WAVE = {
  // S-08 之后采样由 traffic-data 事件驱动（后端 900ms 推流），不再需要独立采样周期常量。
  maxPoints: 96,

  lowFlowCeil: 96 * 1024,
  lowFlowMaxRatio: 0.22,

  midFlowCeil: 1024 * 1024,
  midFlowRatio: 0.62,

  minScale: 2 * 1024 * 1024,
  scaleHeadroom: 1.18,

  riseAlpha: 0.30,
  fallAlpha: 0.10,

  scaleRiseAlpha: 0.06,
  scaleFallAlpha: 0.025,

  lowGamma: 0.72,
  midGamma: 0.82,
  highGamma: 0.78,

  baselineRatio: 1.0,
  maxAmplitude: 0.85,
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
  const instant = Math.max(WAVE.minScale, value * WAVE.scaleHeadroom);
  // H-02：原来 target = max(instant, prevScale) 让缩放只升不降 —— target 恒 >= prevScale，
  // 回落分支（scaleFallAlpha）在数学上不可达，一次高速流量后波形被永久压平；
  // 改为直接朝 instant 收敛：升高走 scaleRiseAlpha，回落走 scaleFallAlpha（可自愈），并保 minScale 下限。
  const alpha = instant > prevScale ? WAVE.scaleRiseAlpha : WAVE.scaleFallAlpha;
  return Math.max(WAVE.minScale, prevScale + (instant - prevScale) * alpha);
};

// 5 点高斯核平滑（在采样循环中预计算，不在 computed 中重复分配）
const SMOOTH_KERNEL = [0.06, 0.20, 0.48, 0.20, 0.06];
const smoothSamples = (src: number[]): number[] => {
  const n = src.length;
  const out = new Array(n);
  for (let i = 0; i < n; i++) {
    const a = src[Math.max(0, i - 2)];
    const b = src[Math.max(0, i - 1)];
    const c = src[i];
    const d = src[Math.min(n - 1, i + 1)];
    const e = src[Math.min(n - 1, i + 2)];
    out[i] = a * SMOOTH_KERNEL[0] + b * SMOOTH_KERNEL[1] + c * SMOOTH_KERNEL[2] + d * SMOOTH_KERNEL[3] + e * SMOOTH_KERNEL[4];
  }
  return out;
};

// --- 持久化采样状态 ---

export const waveState = reactive({
  // 原始采样值（用于 scale 计算）
  uploadRatios: makeInitialSamples(),
  downloadRatios: makeInitialSamples(),

  // 平滑后的值（直接用于路径构建，避免 computed 中重复计算）
  smoothedUploadRatios: makeInitialSamples(),
  smoothedDownloadRatios: makeInitialSamples(),

  latestUpload: 0,
  latestDownload: 0,

  smoothedUpload: 0,
  smoothedDownload: 0,

  uploadScale: WAVE.minScale,
  downloadScale: WAVE.minScale,
});

let refCount = 0;

const pushVisualSamples = () => {
  const s = waveState;

  s.smoothedUpload = smoothValue(s.smoothedUpload, s.latestUpload);
  s.smoothedDownload = smoothValue(s.smoothedDownload, s.latestDownload);

  const upRatio = toVisualRatio(s.smoothedUpload, s.uploadScale);
  const downRatio = toVisualRatio(s.smoothedDownload, s.downloadScale);

  s.uploadScale = updateScale(s.uploadScale, s.smoothedUpload);
  s.downloadScale = updateScale(s.downloadScale, s.smoothedDownload);

  // 原地修改，避免 spread+slice 分配新数组
  s.uploadRatios.shift();
  s.uploadRatios.push(upRatio);
  s.downloadRatios.shift();
  s.downloadRatios.push(downRatio);

  // 预计算平滑路径数据，buildMonotoneAreaPath 直接读取
  s.smoothedUploadRatios = smoothSamples(s.uploadRatios);
  s.smoothedDownloadRatios = smoothSamples(s.downloadRatios);
};

/* S-08：采样改为「事件驱动」。
   旧实现是独立的 900ms setInterval，而后端 traffic-data 的推流周期也是 900ms ——
   两个等周期定时器必然异相：同一份 latest 会被采两次、或两次推流被跳过一次，
   波形上表现为平台/突跳点（而且和真实流量不同步）。
   现在改成「收到一次推流就采一次点」，一个数据点对应一个采样点，彻底消除拍频；
   没有数据时不推进（波形停在原地，比空转更好）。
   start/stop 只维护消费者计数（门是否打开），不再拥有定时器。 */
export function startWaveSampling() {
  refCount++;
}

export function stopWaveSampling() {
  // S-08：原来只在 timer 非空时才把 refCount 归零，多调一次 stop 会把它压到 -1 并永久保留；
  // 这里无条件钳位到 0，保证 start/stop 严格配对。
  refCount = Math.max(0, refCount - 1);
}

export function updateLatestTraffic(upRaw: number, downRaw: number) {
  waveState.latestUpload = upRaw;
  waveState.latestDownload = downRaw;
  if (refCount > 0) pushVisualSamples();
}

export function resetWaveState() {
  waveState.uploadRatios = makeInitialSamples();
  waveState.downloadRatios = makeInitialSamples();
  waveState.smoothedUploadRatios = makeInitialSamples();
  waveState.smoothedDownloadRatios = makeInitialSamples();
  waveState.latestUpload = 0;
  waveState.latestDownload = 0;
  waveState.smoothedUpload = 0;
  waveState.smoothedDownload = 0;
  waveState.uploadScale = WAVE.minScale;
  waveState.downloadScale = WAVE.minScale;
}

// --- 路径构建 ---

export function buildMonotoneAreaPath(
  smoothedRatios: number[],
  width = 320,
  height = 100
) {
  const baseline = height * WAVE.baselineRatio;
  const usableHeight = height * WAVE.maxAmplitude;
  const step = width / Math.max(smoothedRatios.length - 1, 1);

  const points = smoothedRatios.map((ratio, index) => ({
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
