<template>
  <div class="settings-page">
    <div class="sub-header section-header page-sticky-mask">
      <button class="back-btn" @click="$emit('navigate', 'main')">
        <svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" stroke-width="2"><line x1="19" y1="12" x2="5" y2="12"></line><polyline points="12 19 5 12 12 5"></polyline></svg>
      </button>
      <h3>{{ t('settings.tun.title') }}</h3>
      <button class="action-btn accent-btn mini-btn-reset" @click="$emit('reset', 'tun')">
        <span class="btn-icon" v-html="ICONS.refresh"></span> {{ t('settings.tun.resetBtn') }}
      </button>
    </div>

    <div class="glass-card setting-group scrollable">
      <div class="setting-item">
        <div class="info"><h4>{{ t('settings.tun.enableTun') }}</h4></div>
        <label class="modern-switch"><input type="checkbox" :checked="globalState.tun" @change="handleTunToggle"><span class="slider"></span></label>
      </div>
      <div class="divider"></div>
      <div class="setting-item">
        <div class="info"><h4>{{ t('settings.tun.driverInstall') }}</h4><p class="status-msg">{{ t('settings.tun.statusLabel') }}<span :class="tunStatus.hasWintun ? 'green-text' : 'red-text'">{{ tunStatus.hasWintun ? t('settings.tun.driverReady') : (tunStatus.wintunError || t('settings.tun.driverUnavailable')) }}</span></p></div>
        <button class="action-btn" @click="installDriver(true)" :disabled="isInstalling || tunStatus.hasWintun">{{ isInstalling ? t('settings.tun.processing') : (tunStatus.hasWintun ? t('settings.tun.driverInstalled') : t('settings.tun.driverInstallBtn')) }}</button>
      </div>
      <div class="divider"></div>
      <div class="setting-item"><div class="info"><h4>{{ t('settings.tun.stack') }}</h4></div><ModernSelect v-model="tunConfig.stack" :options="stackOptions" @change="saveTun" :disabled="!tunStatus.hasWintun || loading" /></div>
      <div class="divider"></div>
      <div class="setting-item"><div class="info"><h4>{{ t('settings.tun.device') }}</h4></div><input type="text" class="modern-input" v-model="tunConfig.device" :placeholder="t('settings.tun.devicePlaceholder')" @blur="saveTun" :disabled="!tunStatus.hasWintun || loading" /></div>
      <div class="divider"></div>
      <div class="setting-item"><div class="info"><h4>{{ t('settings.tun.autoRoute') }}</h4></div><label class="modern-switch"><input type="checkbox" v-model="tunConfig.autoRoute" @change="saveTun" :disabled="!tunStatus.hasWintun || loading"><span class="slider"></span></label></div>
      <div class="divider"></div>
      <div class="setting-item"><div class="info"><h4>{{ t('settings.tun.autoDetect') }}</h4></div><label class="modern-switch"><input type="checkbox" v-model="tunConfig.autoDetect" @change="saveTun" :disabled="!tunStatus.hasWintun || loading"><span class="slider"></span></label></div>
      <div class="divider"></div>
      <div class="setting-item"><div class="info"><h4>{{ t('settings.tun.dnsHijack') }}</h4></div><input type="text" class="modern-input" :value="tunConfig.dnsHijack.join(', ')" @blur="updateTunDnsHijack" :placeholder="t('settings.tun.dnsHijackPlaceholder')" :disabled="!tunStatus.hasWintun || loading" /></div>
      <div class="divider"></div>
      <div class="setting-item"><div class="info"><h4>{{ t('settings.tun.strictRoute') }}</h4></div><label class="modern-switch"><input type="checkbox" v-model="tunConfig.strictRoute" @change="saveTun" :disabled="!tunStatus.hasWintun || loading"><span class="slider"></span></label></div>
      <div class="divider"></div>
      <div class="setting-item"><div class="info"><h4>{{ t('settings.tun.mtu') }}</h4></div><ModernNumberInput v-model="tunConfig.mtu" :min="576" :max="1500" @change="saveTun" :disabled="!tunStatus.hasWintun || loading" /></div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue';
import * as API from '../../wailsjs/go/main/App';
import { EventsOn } from '../../wailsjs/runtime/runtime';
import { globalState, settleTunIntent, setTunIntent, showAlert, showConfirm } from '../store';
import { ICONS } from '../utils/icons';
import ModernNumberInput from './ModernNumberInput.vue';
import ModernSelect from './ModernSelect.vue';
import { t } from '../locales';

const props = defineProps<{ resetKey: number }>();

defineEmits<{ navigate: [view: 'main']; reset: [module: 'tun'] }>();

const stackOptions = [
  { label: 'gVisor', value: 'gvisor' }, { label: 'Mixed', value: 'mixed' },
  { label: 'System', value: 'system' }, { label: 'LWIP', value: 'lwip' }
];
const loading = ref(true);
const isInstalling = computed(() => globalState.componentUpdate.tasks['driver-install']?.status === 'running');
const tunStatus = ref<Record<string, any>>({ hasWintun: false, isAdmin: false, wintunError: '' });
const tunConfig = ref({ stack: 'gvisor', device: '', autoRoute: true, autoDetect: true, dnsHijack: ['any:53'], strictRoute: true, mtu: 1500 });
const unsubs: (() => void)[] = [];

const refreshRuntimeAssets = async () => {
  const status = await (API as any).GetRuntimeAssetStatus();
  const wintun = status?.assets?.wintun;
  tunStatus.value = { ...tunStatus.value, hasWintun: !!wintun?.ready, wintunError: wintun?.ready ? '' : (wintun?.error || wintun?.hint || 'wintun 不可用'), wintun };
};

const loadData = async () => {
  loading.value = true;
  try {
    const [_, status, config] = await Promise.all([refreshRuntimeAssets(), API.CheckTunEnv(), API.GetTunConfig()]);
    tunStatus.value = status;
    if (config) tunConfig.value = config;
  } catch (e) { console.error('加载配置失败', e); } finally { loading.value = false; }
};

const handleTunToggle = async (e: Event) => {
  const target = e.target as HTMLInputElement;
  const newState = target.checked;
  if (newState && !tunStatus.value.hasWintun) {
    e.preventDefault();
    target.checked = false;
    await showAlert(t('settings.tun.missingDriverMsg'), t('settings.tun.missingDriverTitle'));
    return;
  }
  setTunIntent(newState);
  try { await API.ToggleTunMode(newState); } catch (err) {
    target.checked = !newState;
    await showAlert(t('settings.tun.tunError', { error: String(err) }), t('common.error'));
  } finally { settleTunIntent(); }
};

const installDriver = async (force: boolean = true) => {
  if (isInstalling.value) return;
  const ok = await showConfirm(t('settings.tun.reinstallConfirmMsg'), t('settings.tun.reinstallConfirmTitle'), false);
  if (!ok) return;
  (API as any).InstallTunDriverAsync(force).catch(() => {});
};
const saveTun = async () => { if (loading.value) return; try { await API.SaveTunConfig(tunConfig.value); } catch (e) { console.error('保存失败', e); } };
const updateTunDnsHijack = (e: Event) => { if (loading.value) return; tunConfig.value.dnsHijack = (e.target as HTMLInputElement).value.split(',').map(s => s.trim()).filter(s => s); saveTun(); };

onMounted(() => {
  void loadData();
  unsubs.push(EventsOn('driver-install-success', () => { void refreshRuntimeAssets(); }));
});
watch(() => props.resetKey, () => { void loadData(); });
onUnmounted(() => { unsubs.forEach(unsub => unsub && unsub()); });
</script>

<style scoped>
.settings-page { display: flex; flex-direction: column; flex: 1; min-height: 100%; overflow: visible; }
.setting-group { padding: 20px 24px; margin-bottom: 12px; }.setting-group.scrollable { padding-bottom: 20px; overflow: visible; max-height: none; }
h3 { margin: 0 0 8px 0; color: var(--text-main); font-size: 1.25rem; padding-bottom: 4px; } h4 { margin: 0 0 6px 0; color: var(--text-main); font-size: 1rem; } p { margin: 0; font-size: 0.85rem; color: var(--text-sub); max-width: 100%; line-height: 1.5; }
.info { flex: 1; padding-right: 24px; min-width: 0; }.setting-item { display: flex; justify-content: space-between; align-items: center; padding: 14px 0; }.divider { height: 1px; background: var(--glass-border); opacity: 0.5; margin: 0; }
.modern-input { background: var(--surface-hover); border: none; color: var(--text-main); padding: 10px 14px; border-radius: 8px; outline: none; font-size: 0.9rem; text-align: right; }.modern-input:disabled { opacity: 0.5; cursor: not-allowed; }
.modern-switch { position: relative; display: inline-block; width: 44px; height: 24px; }.modern-switch input { opacity: 0; width: 0; height: 0; }.slider { position: absolute; cursor: pointer; inset: 0; background-color: var(--surface-hover); transition: .3s; border-radius: 24px; box-shadow: inset 0 1px 3px rgba(0,0,0,0.1); }.slider:before { position: absolute; content: ""; height: 18px; width: 18px; left: 3px; bottom: 3px; background-color: white; transition: .3s; border-radius: 50%; box-shadow: 0 1px 3px rgba(0,0,0,0.3); } input:disabled + .slider { opacity: 0.5; cursor: not-allowed; } input:checked + .slider { background-color: var(--accent); } input:checked + .slider:before { transform: translateX(20px); background-color: var(--accent-fg); }
.sub-header { display: flex; align-items: center; gap: 16px; margin-bottom: 12px; padding: 0 0 12px 0; background: transparent; }.sub-header.page-sticky-mask { --sticky-mask-bleed: 4px; }.sub-header h3 { margin: 0; border: none; padding: 0; }.section-header { justify-content: space-between; }.sub-header.section-header h3 { flex: 1; margin-left: 12px; }.back-btn { background: var(--surface); border: none; color: var(--text-main); width: 36px; height: 36px; border-radius: 50%; display: flex; align-items: center; justify-content: center; cursor: pointer; transition: 0.2s; }.back-btn:hover { background: var(--surface-hover); }.mini-btn-reset { height: 36px !important; padding: 0 14px !important; font-size: 0.85rem !important; border-radius: 8px !important; }.mini-btn-reset :deep(.btn-icon) svg { width: 16px; height: 16px; }.status-msg { margin-top: 4px; font-weight: 500; }.green-text { color: var(--text-main); font-weight: 600; }.red-text { color: var(--text-muted); }
</style>
