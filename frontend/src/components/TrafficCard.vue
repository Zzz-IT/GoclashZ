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
        <!-- H-20：outboundIPStale 以前只写不读 —— 切换代理/TUN 后检测失败时，旧 IP 仍被当成当前出口展示 -->
        <span v-if="outboundIPIsStale" class="ip-stale" :title="t('traffic.staleHint')">{{ t('traffic.staleShort') }}</span>
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
import { computed } from 'vue';
import { App } from '../api';
import { globalState, refreshOutboundIPRouteAware, showConfirm, showToast } from '../store';
import { displayErrorMessage } from '../utils/errors';
import { reportError } from '../utils/report';
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

/* S-08：这里原来还有一个 watch(props.traffic.upRaw) → updateLatestTraffic。
   App.vue 的 traffic-data 事件已经在写同一份 latest（同一时刻、同一份数据），
   两处都写会让"一次推流被采两次"，波形被加速一倍。
   采样已改为事件驱动，写入口只保留 App.vue 这一处，本组件只读波形。 */

const outboundIPHasValue = computed(() => {
  return !!(globalState.outboundIP?.preferred);
});

// H-20：上一次检测成功后路由又变了（或检测失败）→ store 把 outboundIPStale 置 true，
// 此时展示的 preferred 只是「上次的出口 IP」，必须让用户看出来，否则会据此误判代理已生效。
const outboundIPIsStale = computed(() => globalState.outboundIPStale && outboundIPHasValue.value);

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

  // H-20：过期状态也要在 title 里说明，hover 时能读到「不是当前出口」
  if (globalState.outboundIPStale) {
    lines.push(t('traffic.staleHint'));
  }

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

// H-03：防重入闸门放在 showConfirm 之前 —— 模态是队列，连点两次会排两个确认框，
// 用户各点一次「确定」后 ResetTrafficTotals 会执行两次（刚归零的计数被再清一遍、弹两条成功 toast）。
let resetInFlight = false;

const handleReset = async () => {
  if (resetInFlight) return;
  resetInFlight = true;
  try {
    const ok = await showConfirm(t('traffic.resetConfirmMsg'), t('traffic.resetConfirmTitle'));
    if (!ok) return;
    await App.ResetTrafficTotals();
    resetWaveState();
    // F-A4-12：成功用 toast
    showToast(t('traffic.resetSuccess'), 'success');
  } catch (e) {
    // F-A4-22：重置失败以前既无提示也不复位，用户以为已清零
    reportError(e, 'traffic.reset');
    showToast(displayErrorMessage(e), 'error');
  } finally {
    resetInFlight = false;
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
  border-radius: var(--ui-radius-sm);
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

/* H-20：「已过期」徽标：比 IP 本身弱一档，不抢「当前出站 IP」的注意力，但能看出不是实时值 */
.ip-stale {
  flex: 0 0 auto;
  padding: 0 6px;
  border-radius: var(--ui-radius-full);
  background: var(--surface-hover);
  color: var(--text-sub);
  font-size: 0.7rem;
  font-weight: 600;
  white-space: nowrap;
}

.reset-btn {
  border: none;
  background: var(--ui-accent-block);
  color: var(--ui-accent-fg);
  border-radius: var(--ui-radius-sm);
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
  border-radius: var(--ui-radius-md);
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
