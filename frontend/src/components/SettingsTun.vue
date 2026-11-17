<template>
  <div class="settings-page ui-page-shell" :class="{ 'no-anim': !isInitialized }">
    <div class="sub-header section-header ui-page-header">
      <button class="back-btn" @click="$emit('navigate', 'main')">
        <svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" stroke-width="2"><line x1="19" y1="12" x2="5" y2="12"></line><polyline points="12 19 5 12 12 5"></polyline></svg>
      </button>
      <h3>{{ t('settings.tun.title') }}</h3>
      <button class="action-btn accent-btn mini-btn-reset" @click="$emit('reset', 'tun')">
        <span class="btn-icon" v-html="ICONS.refresh"></span> {{ t('settings.tun.resetBtn') }}
      </button>
    </div>

    <DeviationStrip ref="deviationRef" module="tun" />

    <div class="ui-page-body">
      <div class="glass-card setting-group ui-surface-group scrollable">
        <div class="setting-item" id="set-tun-enableTun">
          <div class="info"><h4>{{ t('settings.tun.enableTun') }}</h4></div>
          <label class="modern-switch"><input type="checkbox" :checked="globalState.tun" :disabled="tunPending" @change="handleTunToggle"><span class="slider"></span></label>
        </div>
        <div class="divider"></div>
        <div class="setting-item" id="set-tun-driverInstall">
          <div class="info"><h4>{{ t('settings.tun.driverInstall') }}</h4><p class="status-msg">{{ t('settings.tun.statusLabel') }}<span :class="tunStatus.hasWintun ? 'green-text' : 'red-text'">{{ tunStatus.hasWintun ? t('settings.tun.driverReady') : (tunStatus.wintunError || t('settings.tun.driverUnavailable')) }}</span></p></div>
          <button class="action-btn" @click="installDriver(true)" :disabled="isInstalling || tunStatus.hasWintun">{{ isInstalling ? t('settings.tun.processing') : (tunStatus.hasWintun ? t('settings.tun.driverInstalled') : t('settings.tun.driverInstallBtn')) }}</button>
        </div>
        <div class="divider"></div>
        <div class="setting-item" id="set-tun-stack"><div class="info"><h4>{{ t('settings.tun.stack') }}</h4></div><ModernSelect v-model="tunConfig.stack" :options="stackOptions" @change="saveTun" :disabled="!tunStatus.hasWintun || loading" /></div>
        <Transition :name="isInitialized ? 'dropdown' : ''">
          <div v-if="tunConfig.stack === 'mips'" class="congestion-sub-items">
            <div class="divider"></div>
            <div class="setting-item" id="set-tun-congestion">
              <div class="info">
                <h4>{{ t('settings.tun.congestion') }}</h4>
                <p class="status-msg">{{ t('settings.tun.congestionMipsOnly') }}</p>
              </div>
              <ModernSelect
                v-model="tunConfig.congestionController"
                :options="congestionOptions"
                @change="saveTun"
                :disabled="!tunStatus.hasWintun || loading"
              />
            </div>
          </div>
        </Transition>
        <div class="divider"></div>
        <div class="setting-item" id="set-tun-device"><div class="info"><h4>{{ t('settings.tun.device') }}</h4></div><input type="text" class="modern-input" v-model="tunConfig.device" :placeholder="t('settings.tun.devicePlaceholder')" @blur="saveTun" :disabled="!tunStatus.hasWintun || loading" /></div>
        <div class="divider"></div>
        <div class="setting-item" id="set-tun-autoRoute"><div class="info"><h4>{{ t('settings.tun.autoRoute') }}</h4></div><label class="modern-switch"><input type="checkbox" v-model="tunConfig.autoRoute" @change="saveTun" :disabled="!tunStatus.hasWintun || loading"><span class="slider"></span></label></div>
        <div class="divider"></div>
        <div class="setting-item" id="set-tun-autoDetect"><div class="info"><h4>{{ t('settings.tun.autoDetect') }}</h4></div><label class="modern-switch"><input type="checkbox" v-model="tunConfig.autoDetect" @change="saveTun" :disabled="!tunStatus.hasWintun || loading"><span class="slider"></span></label></div>
        <div class="divider"></div>
        <div class="setting-item" id="set-tun-dnsHijack"><div class="info"><h4>{{ t('settings.tun.dnsHijack') }}</h4></div><input type="text" class="modern-input" :value="tunConfig.dnsHijack.join(', ')" @blur="updateTunDnsHijack" :placeholder="t('settings.tun.dnsHijackPlaceholder')" :disabled="!tunStatus.hasWintun || loading" /></div>
        <div class="divider"></div>
        <div class="setting-item" id="set-tun-strictRoute"><div class="info"><h4>{{ t('settings.tun.strictRoute') }}</h4></div><label class="modern-switch"><input type="checkbox" v-model="tunConfig.strictRoute" @change="saveTun" :disabled="!tunStatus.hasWintun || loading"><span class="slider"></span></label></div>
        <div class="divider"></div>
        <div class="setting-item" id="set-tun-mtu"><div class="info"><h4>{{ t('settings.tun.mtu') }}</h4></div><ModernNumberInput v-model="tunConfig.mtu" :min="576" :max="1500" @change="saveTun" :disabled="!tunStatus.hasWintun || loading" /></div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue';
import { App } from '../api';
import { Events } from '@wailsio/runtime';
import { globalState, showAlert, showConfirm, showToast } from '../store';
import { ICONS } from '../utils/icons';
import { toUserError } from '../utils/errors';
import { reportError } from '../utils/report';
import { useTunToggle } from '../composables/useTunToggle';
import DeviationStrip from './DeviationStrip.vue';
import ModernNumberInput from './ModernNumberInput.vue';
import ModernSelect from './ModernSelect.vue';
import { t } from '../locales';

const props = defineProps<{ resetKey: number }>();

defineEmits<{ navigate: [view: 'main']; reset: [module: 'tun'] }>();

const stackOptions = computed(() => {
  const options = [
    { label: t('settings.tun.stackFollow'), value: 'follow' },
  ];
  if (tunStatus.value.supportsMips) {
    options.push({ label: t('settings.tun.stackMips'), value: 'mips' });
  }
  options.push(
    { label: 'gVisor', value: 'gvisor' },
    { label: 'Mixed', value: 'mixed' },
    { label: 'System', value: 'system' },
    { label: 'LWIP', value: 'lwip' }
  );
  return options;
});

const congestionOptions = computed(() => [
  { label: `bbr3 (${t('settings.tun.congestionRecommended')})`, value: 'bbr3' },
  { label: 'bbr', value: 'bbr' },
  { label: 'cubic', value: 'cubic' },
  { label: 'reno', value: 'reno' }
]);
const loading = ref(true);
const deviationRef = ref<InstanceType<typeof DeviationStrip> | null>(null);
const isInitialized = ref(false);
const isInstalling = computed(() => globalState.componentUpdate.tasks['driver-install']?.status === 'running');
const tunStatus = ref<Record<string, any>>({ hasWintun: false, isAdmin: false, wintunError: '' });
const tunConfig = ref({ stack: 'mips', device: '', autoRoute: true, autoDetect: true, dnsHijack: ['any:53'], strictRoute: true, mtu: 1500, congestionController: 'bbr3' });
const unsubs: (() => void)[] = [];

// 把两种状态载荷（GetRuntimeAssetStatus / CheckTunEnv）归一成同一份 ui 状态，
// 避免「后一次的 map 覆盖前一次的 wintunError」（F-A4-09）。
const applyTunStatus = (status: any) => {
  const wintun = status?.wintun ?? status?.assets?.wintun;
  const ready = wintun ? !!wintun.ready : !!status?.hasWintun;
  tunStatus.value = {
    ...(status ?? {}),
    hasWintun: ready,
    wintunError: ready ? '' : (wintun?.error || wintun?.hint || t('settings.tun.driverUnavailable')),
    wintun,
  };
};

const refreshRuntimeAssets = async () => {
  try {
    applyTunStatus(await App.GetRuntimeAssetStatus());
  } catch (e) {
    reportError(e, 'tun.refreshAssets');
  }
};

const loadData = async () => {
  loading.value = true;
  isInitialized.value = false;
  // mips 不可用时回退到 mixed：必须落盘，否则每次进页都会重弹提示
  let needPersistFallback = false;
  try {
    // 只调一次 CheckTunEnv：它内部已含 GetStatus + wintun + supportsMips；
    // 原先再并发一次 GetRuntimeAssetStatus = 同页 2 次全量 61MB 哈希（F-A0-02）。
    const [status, config] = await Promise.all([App.CheckTunEnv(), App.GetTunConfig()]);
    applyTunStatus(status);
    if (config) {
      // bindings 中 TunConfig.dnsHijack 为 string[] | null，本地 ref 固定为 string[]，这里归一化兜底
      tunConfig.value = { ...config, dnsHijack: config.dnsHijack ?? [] };
      if (tunConfig.value.stack === 'mips' && !status?.supportsMips) {
        tunConfig.value.stack = 'mixed';
        needPersistFallback = true;
      }
    }
  } catch (e) {
    reportError(e, 'tun.load');
  } finally {
    loading.value = false;
    // 必须等浏览器真正绘制一帧（rAF）后再解除 no-anim。
    // 若只用 nextTick，解除发生在绘制之前，浏览器首次绘制时 transition 已恢复，
    // 会把"默认值→真实值"当成状态变化播成滑动动画，抑制形同无效。
    requestAnimationFrame(() => {
      requestAnimationFrame(() => { isInitialized.value = true; });
    });
  }
  // 关键：saveTun 在 loading=true 时会直接 return（见下方闸门），
  // 因此回退保存必须放在 finally（loading 已复位）之后执行，否则永远不落盘。
  if (needPersistFallback) {
    const saved = await saveTunCore(true);
    if (saved) void showAlert(t('settings.tun.mipsFallbackWarning'), t('common.notice'), false);
  }
};

// TUN 开关：与首页共用同一实现（pending / classifyError 分支 / helper 与驱动引导）
const { pending: tunPending, toggle: toggleTunState } = useTunToggle({
  // 装完驱动/helper 后立即刷一次资产状态，事件 driver-install-success 仍作为最终信号
  onAfterInstall: async () => { await refreshRuntimeAssets(); },
});

const handleTunToggle = async (e: Event) => {
  const input = e.target as HTMLInputElement;
  const next = input.checked;
  const ok = await toggleTunState(next);
  // 失败时回滚视觉（成功会由 updateStateFromBackend 回灌 globalState.tun）
  if (!ok) input.checked = !next;
};

const installDriver = async (force: boolean = true) => {
  if (isInstalling.value) return;
  const ok = await showConfirm(t('settings.tun.reinstallConfirmMsg'), t('settings.tun.reinstallConfirmTitle'), false);
  if (!ok) return;
  App.InstallTunDriverAsync(force).catch((e: unknown) => reportError(e, 'tun.installDriver'));
};
// 自动保存会频繁触发：同一条失败提示 5 秒内只弹一次，避免连弹
let saveFailAt = 0;
let saveFailMsg = '';
// force=true 用于「加载期自动回退」这类必须落盘的场景（绕过 loading 闸门）
const saveTunCore = async (force: boolean = false): Promise<boolean> => {
  if (loading.value && !force) return false;
  try {
    await App.SaveTunConfig(tunConfig.value);
    void deviationRef.value?.refresh();
    return true;
  } catch (e) {
    reportError(e, 'tun.save');
    const u = toUserError(e);
    const now = Date.now();
    // 自动保存是高频触发：同一条失败 5s 内只提示一次，避免 toast 刷屏
    if (u.message !== saveFailMsg || now - saveFailAt > 5000) {
      saveFailAt = now;
      saveFailMsg = u.message;
      showToast(t('settings.saveFailed', { error: u.message }), 'error');
    }
    void deviationRef.value?.refresh();
    return false;
  }
};
const saveTun = async () => { await saveTunCore(false); };
const updateTunDnsHijack = (e: Event) => { if (loading.value) return; tunConfig.value.dnsHijack = (e.target as HTMLInputElement).value.split(',').map(s => s.trim()).filter(s => s); saveTun(); };

onMounted(() => {
  void loadData();
  unsubs.push(Events.On('driver-install-success', () => { void refreshRuntimeAssets(); }));
});
watch(() => props.resetKey, () => { void loadData(); });
onUnmounted(() => { unsubs.forEach(unsub => unsub && unsub()); });
</script>

<style scoped>
.settings-page { flex: 1 1 auto; min-width: 0; }
.setting-group { padding: 20px 24px; margin-bottom: 12px; }.setting-group.scrollable { padding-bottom: 20px; }
h3 { margin: 0 0 8px 0; color: var(--text-main); font-size: 1.25rem; padding-bottom: 4px; } h4 { margin: 0 0 6px 0; color: var(--text-main); font-size: 1rem; } p { margin: 0; font-size: 0.85rem; color: var(--text-sub); max-width: 100%; line-height: 1.5; }
.info { flex: 1; padding-right: 24px; min-width: 0; }.setting-item { display: flex; justify-content: space-between; align-items: center; padding: 14px 0; }.divider { height: 1px; background: var(--ui-border-subtle); opacity: 0.5; margin: 0; }
.modern-input { background: var(--ui-surface-hover); border: none; color: var(--text-main); padding: 10px 14px; border-radius: 8px; outline: none; font-size: 0.9rem; text-align: right; }.modern-input:disabled { opacity: 0.5; cursor: not-allowed; }
.modern-switch { position: relative; display: inline-block; width: 44px; height: 24px; }.modern-switch input { opacity: 0; width: 0; height: 0; }.slider { position: absolute; cursor: pointer; inset: 0; background-color: var(--ui-surface-hover); transition: .3s; border-radius: 24px; box-shadow: inset 0 1px 3px rgba(0,0,0,0.1); }.slider:before { position: absolute; content: ""; height: 18px; width: 18px; left: 3px; bottom: 3px; background-color: white; transition: .3s; border-radius: 50%; box-shadow: 0 1px 3px rgba(0,0,0,0.3); } input:disabled + .slider { opacity: 0.5; cursor: not-allowed; } input:checked + .slider { background-color: var(--ui-accent-block); } input:checked + .slider:before { transform: translateX(20px); background-color: var(--ui-accent-fg); }
.sub-header { display: flex; align-items: center; gap: 16px; margin-bottom: 12px; padding: 0 0 12px 0; background: transparent; }.sub-header h3 { margin: 0; border: none; padding: 0; }.section-header { justify-content: space-between; }.sub-header.section-header h3 { flex: 1; margin-left: 12px; }.back-btn { background: var(--surface); border: none; color: var(--text-main); width: 36px; height: 36px; border-radius: 50%; display: flex; align-items: center; justify-content: center; cursor: pointer; transition: 0.2s; }.back-btn:hover { background: var(--ui-surface-hover); }.mini-btn-reset { height: 36px !important; padding: 0 14px !important; font-size: 0.85rem !important; border-radius: 8px !important; }.mini-btn-reset :deep(.btn-icon) svg { width: 16px; height: 16px; }.status-msg { margin-top: 4px; font-weight: 500; }.green-text { color: var(--text-main); font-weight: 600; }.red-text { color: var(--text-muted); }
/* 拥塞控制子项高度较小，覆盖公共 dropdown 过渡的 max-height */
.congestion-sub-items.dropdown-enter-active,
.congestion-sub-items.dropdown-leave-active {
  max-height: 120px;
}
</style>
