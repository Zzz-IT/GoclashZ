<template>
  <div class="settings-page ui-page-shell">
    <div class="sub-header ui-page-header">
      <button class="back-btn" @click="$emit('navigate', 'main')">
        <span class="icon back-icon-svg" v-html="ICONS.arrowLeft"></span>
      </button>
      <h3>{{ t('settings.update.title') }}</h3>
    </div>

    <div class="ui-page-body">
      <UpdateTaskPanel />
      <div class="glass-card setting-group ui-surface-group scrollable">
        <div class="setting-item col-item" style="padding-bottom: 0; align-items: flex-start;"><h3 style="margin: 0; font-size: 1.15rem; font-weight: 600; color: var(--text-main);">{{ t('settings.update.coreAndDriver') }}</h3></div>
        <div class="divider" style="margin-top: 10px;"></div>
        <div class="setting-item" id="set-update-mihomoCore">
          <div class="info"><h4>{{ t('settings.update.mihomoCore') }} <span style="color: var(--ui-accent); margin-left: 8px; font-style: italic; font-size: 0.8rem; font-weight: normal;">{{ t('settings.update.updateNotice') }}</span></h4><p>{{ t('settings.update.currentVersion', { version: coreVersion }) }}</p></div>
          <button class="action-btn" :class="{ 'accent-btn': globalState.componentUpdate.pendingCoreUpdate }" @click="globalState.componentUpdate.pendingCoreUpdate ? executeCoreUpdate() : handleUpdateCore()" :disabled="globalState.componentUpdate.checkingCoreUpdate || globalState.componentUpdate.updatingCore">
            <template v-if="globalState.componentUpdate.checkingCoreUpdate">{{ t('settings.update.checking') }}</template><template v-else-if="globalState.componentUpdate.updatingCore">{{ t('settings.update.processing') }}</template><template v-else-if="globalState.componentUpdate.pendingCoreUpdate">{{ t('settings.update.updateTo', { version: globalState.componentUpdate.coreUpdateInfo.remote }) }}</template><template v-else>{{ t('settings.update.checkUpdate') }}</template>
          </button>
        </div>
        <div class="divider"></div>
        <div class="setting-item" id="set-update-wintunDriver"><div class="info"><h4>{{ t('settings.update.wintunDriver') }}</h4><p>{{ t('settings.update.currentVersion', { version: wintunVersion || t('settings.update.reading') }) }}</p></div><div class="btn-group"><button class="action-btn" @click="installDriver(true)" :disabled="isInstalling">{{ isInstalling ? t('settings.update.processing') : t('settings.update.reinstall') }}</button></div></div>
        <div class="divider"></div>
        <div class="setting-item" id="set-update-helperService">
          <div class="info"><h4>{{ t('settings.update.helperService') }}</h4><p>{{ t('settings.update.helperStatus', { status: '' }) }}<span :style="{ color: helperStatus.reachable ? 'var(--green-text)' : (helperStatus.installed ? 'var(--yellow-text)' : 'var(--text-muted)') }">{{ helperStatus.reachable ? t('settings.update.helperRunning') : (helperStatus.installed ? (helperStatus.running ? t('settings.update.helperFailed') : t('settings.update.helperStopped')) : t('settings.update.helperNotInstalled')) }}</span><span v-if="!helperStatus.installed" style="color: var(--text-muted); font-size: 0.75rem;">{{ t('settings.update.helperTunNotice') }}</span></p></div>
          <div class="btn-group"><button class="action-btn" @click="restartHelper" :disabled="helperLoading" v-if="helperStatus.installed">{{ helperLoading ? '...' : t('settings.update.restart') }}</button><button class="action-btn" @click="installHelper" :disabled="helperLoading">{{ helperLoading ? '...' : (helperStatus.installed ? t('settings.update.reinstall') : t('settings.update.install')) }}</button></div>
        </div>
        <div class="divider"></div>
        <div class="setting-item col-item" style="flex-direction: row; justify-content: space-between; align-items: center; padding-bottom: 0; margin-top: 10px;"><div class="info"><h3 style="margin: 0; font-size: 1.15rem; font-weight: 600; color: var(--text-main);">{{ t('settings.update.geoDbGroup') }}</h3></div><button class="action-btn primary-btn accent-btn" @click="handleUpdateAllDbs" :disabled="isUpdatingAnyDb">{{ isUpdatingAnyDb ? t('settings.update.updating') : t('settings.update.updateAllDbs') }}</button></div>
        <div class="divider" style="margin-top: 14px;"></div>
        <template v-for="(db, idx) in dbList" :key="db.key">
          <div class="setting-item" :id="'set-update-dbFile' + db.key.charAt(0).toUpperCase() + db.key.slice(1)"><div class="info" style="overflow: hidden;"><h4>{{ t('settings.update.dbFile', { name: db.title }) }}</h4><p class="link-text" style="white-space: nowrap; overflow: hidden; text-overflow: ellipsis;">{{ behavior[db.behaviorKey] || t('settings.update.noDbLink') }}</p><p v-if="dbFileInfo[db.key]?.ready" style="font-size: 0.75rem; color: var(--text-muted); margin-top: 2px;">{{ t('settings.update.dbFileSize', { size: formatBytes(dbFileInfo[db.key].size), time: formatRelativeTime(dbFileInfo[db.key].modTime) }) }}</p><p v-else-if="dbFileInfo[db.key]?.error" style="font-size: 0.75rem; color: var(--red-text); margin-top: 2px;">{{ dbFileInfo[db.key].error }}</p><p v-else style="font-size: 0.75rem; color: var(--red-text); margin-top: 2px;">{{ t('settings.update.fileNotExists') }}</p></div><div class="btn-group" style="flex-shrink: 0;"><button class="action-btn" @click="openDbEditModal(db.key, behavior[db.behaviorKey])" :disabled="isUpdatingDb(db.key)">{{ t('settings.update.editLink') }}</button><button class="action-btn" @click="handleUpdateDb(db.key)" :disabled="isUpdatingDb(db.key)">{{ isUpdatingDb(db.key) ? t('settings.update.syncing') : t('settings.update.syncUpdate') }}</button></div></div>
          <div class="divider" v-if="idx < dbList.length - 1"></div>
        </template>
      </div>
    </div>

    <Teleport to="body">
    <Transition name="pop"><div class="modal-overlay" v-if="showDbModal" @click.self="showDbModal = false"><div class="custom-modal-card" @click.stop><div class="modal-header"><h3>{{ t('settings.update.editModalTitle', { name: dbTitles[editingDb.type] || '' }) }}</h3></div><div class="modal-body"><input type="text" class="modal-input" v-model="editingDb.link" style="text-align: left;" @keyup.enter="saveDbLink" /><div class="modal-footer"><button class="action-btn flex-1" @click="showDbModal = false">{{ t('common.cancel') }}</button><button class="primary-btn accent-btn flex-1" @click="saveDbLink">{{ t('common.save') }}</button></div></div></div></div></Transition>
    </Teleport>
    <Teleport to="body">
    <Transition name="pop"><div v-if="showCoreUpdateConfirm" class="modal-overlay" @click="cancelCoreUpdateConfirm"><div class="custom-modal-card" @click.stop><div class="modal-header"><h3>{{ t('settings.update.newVersionFound') }}</h3></div><div class="modal-body"><p class="global-modal-msg" v-html="t('settings.update.newVersionMsg', { remote: globalState.componentUpdate.coreUpdateInfo.remote, local: globalState.componentUpdate.coreUpdateInfo.local })"></p><div class="modal-footer"><button class="action-btn flex-1" @click="cancelCoreUpdateConfirm">{{ t('common.cancel') }}</button><button class="primary-btn accent-btn flex-1" @click="executeCoreUpdate">{{ t('settings.update.updateNow') }}</button></div></div></div></div></Transition>
    </Teleport>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue';
import { App } from '../api';
import { Events } from '@wailsio/runtime';
import { globalState, showAlert, showConfirm, showToast } from '../store';
import { formatBytes, formatRelativeTime } from '../utils/format';
import { ICONS } from '../utils/icons';
import { displayErrorMessage, toUserError } from '../utils/errors';
import { reportError } from '../utils/report';
import UpdateTaskPanel from './UpdateTaskPanel.vue';
import { t } from '../locales';

const props = defineProps<{ behavior: Record<string, any> }>();
const emit = defineEmits<{ navigate: [view: 'main']; 'save-behavior': [] }>();
const coreVersion = ref(t('settings.update.reading'));
const wintunVersion = ref(t('settings.update.reading'));
const isInstalling = computed(() => globalState.componentUpdate.tasks['driver-install']?.status === 'running');
const helperStatus = ref<any>({ installed: false, running: false, reachable: false });
const helperLoading = ref(false);
const showCoreUpdateConfirm = ref(false);
const showDbModal = ref(false);
const editingDb = ref({ type: '', link: '' });
const dbFileInfo = ref<Record<string, any>>({});
const unsubs: (() => void)[] = [];
const dbList = [{ key: 'geoip', title: 'GeoIP', behaviorKey: 'geoIpLink' }, { key: 'geosite', title: 'GeoSite', behaviorKey: 'geoSiteLink' }, { key: 'mmdb', title: 'MMDB', behaviorKey: 'mmdbLink' }, { key: 'asn', title: 'ASN', behaviorKey: 'asnLink' }];
const dbTitles: Record<string, string> = { geoip: 'GeoIP', geosite: 'GeoSite', mmdb: 'MMDB', asn: 'ASN' };
const isUpdatingDb = (key: string) => globalState.componentUpdate.tasks[key]?.status === 'running';
const isUpdatingAnyDb = computed(() => ['geoip', 'geosite', 'mmdb', 'asn'].some(isUpdatingDb));

const refreshRuntimeAssets = async () => {
  const status = await App.GetRuntimeAssetStatus();
  globalState.assetStatus = status;
  const core = status?.assets?.core;
  const wintun = status?.assets?.wintun;
  coreVersion.value = core?.ready ? (core.version || t('settings.update.installedUnknownVersion')) : (core?.error || core?.hint || t('settings.update.notInstalled'));
  wintunVersion.value = wintun?.ready ? (wintun.version || t('settings.update.installedUnknownVersion')) : (wintun?.error || wintun?.hint || t('settings.update.notInstalled'));
  dbFileInfo.value = { geoip: status?.assets?.geoip || {}, geosite: status?.assets?.geosite || {}, mmdb: status?.assets?.mmdb || {}, asn: status?.assets?.asn || {} };
};
const refreshHelperStatus = async () => { helperLoading.value = true; try { helperStatus.value = await App.GetHelperServiceStatus(); } catch (e) { reportError(e, 'update.helperStatus'); } finally { helperLoading.value = false; } };
const refresh = async () => { try { await Promise.all([refreshRuntimeAssets(), refreshHelperStatus()]); } catch (e) { reportError(e, 'update.refresh'); } };
const handleUpdateCore = () => { if (!globalState.componentUpdate.checkingCoreUpdate && !globalState.componentUpdate.updatingCore) App.CheckCoreUpdateAsync().catch((e: unknown) => reportError(e, 'update.checkCore')); };
const cancelCoreUpdateConfirm = () => { showCoreUpdateConfirm.value = false; globalState.componentUpdate.pendingCoreUpdate = false; globalState.componentUpdate.checkingCoreUpdate = false; };
const executeCoreUpdate = () => { showCoreUpdateConfirm.value = false; if (!globalState.componentUpdate.updatingCore) App.UpdateCoreComponentAsync().catch((e: unknown) => reportError(e, 'update.startCore')); };
const installDriver = async (force = true) => { if (isInstalling.value) return; if (!(await showConfirm(t('settings.update.reinstallConfirmMsg'), t('settings.update.reinstallConfirmTitle'), false))) return; App.InstallTunDriverAsync(force).catch((e: unknown) => reportError(e, 'update.installDriver')); };
const installHelper = async () => { helperLoading.value = true; try { await App.InstallHelperService(); await refreshHelperStatus(); if (helperStatus.value.installed) { showToast(t('settings.update.helperInstallSuccess'), 'success'); } else { showToast(t('settings.update.helperInstallPending') + (helperStatus.value.error ? ' ' + displayErrorMessage(helperStatus.value.error) : ''), 'error', { duration: 5000 }); } } catch (e) { const u = toUserError(e); showAlert(u.message, t('common.error'), true, u.detail); } finally { helperLoading.value = false; } };
const restartHelper = async () => { helperLoading.value = true; try { await App.RestartHelperService(); await refreshHelperStatus(); showToast(t('settings.update.helperRestartSuccess'), 'success'); } catch (e) { const u = toUserError(e); showAlert(u.message, t('common.error'), true, u.detail); } finally { helperLoading.value = false; } };
const openDbEditModal = (type: string, link: string) => { editingDb.value = { type, link }; showDbModal.value = true; };
const saveDbLink = async () => { const db = dbList.find(item => item.key === editingDb.value.type); if (db) props.behavior[db.behaviorKey] = editingDb.value.link; showDbModal.value = false; emit('save-behavior'); };
const handleUpdateDb = async (key: string) => { try { await App.UpdateGeoDatabaseAsync(key); } catch (e) { const u = toUserError(e); void showAlert(t('settings.update.updateStartFailed', { name: key, error: u.message }), t('common.error'), true, u.detail); } };
const handleUpdateAllDbs = async () => { try { await App.UpdateAllGeoDatabasesAsync(); } catch (e) { const u = toUserError(e); void showAlert(t('settings.update.updateStartFailed', { name: t('settings.update.geoDbGroup'), error: u.message }), t('common.error'), true, u.detail); } };

onMounted(() => {
  void refresh();
  unsubs.push(Events.On('core-version-updated', ({ data: payload }: { data: any }) => { coreVersion.value = payload?.version || coreVersion.value; showToast(t('settings.update.coreUpdateSuccess', { version: coreVersion.value }), 'success'); }));
  unsubs.push(Events.On('core-update-none', () => { void showAlert(t('settings.update.coreUpdateAlreadyLatest'), t('settings.update.checkUpdate')); }));
  unsubs.push(Events.On('wintun-version-updated', ({ data: payload }: { data: any }) => { wintunVersion.value = payload?.version || wintunVersion.value; }));
  unsubs.push(Events.On('driver-install-success', () => { void refreshRuntimeAssets(); }));
  unsubs.push(Events.On('app-update-busy', () => {
    globalState.appUpdateChecking = false;
    void showAlert(t('settings.update.appUpdateBusy'), t('common.confirm'));
  }));
  ['geoip', 'geosite', 'mmdb', 'asn'].forEach(key => unsubs.push(Events.On(`geo-update-${key}-success`, () => { void refreshRuntimeAssets(); })));
});
watch(() => globalState.componentUpdate.pendingCoreUpdate, value => { if (value) showCoreUpdateConfirm.value = true; }, { immediate: true });
onUnmounted(() => unsubs.forEach(unsub => unsub()));
</script>

<style scoped>
.settings-page { flex: 1 1 auto; min-width: 0; }.setting-group{padding:20px 24px;margin-bottom:12px}.setting-group.scrollable { padding-bottom:20px; }h3{margin:0 0 8px;color:var(--text-main);font-size:1.25rem;padding-bottom:4px}h4{margin:0 0 6px;color:var(--text-main);font-size:1rem}p{margin:0;font-size:.85rem;color:var(--text-sub);line-height:1.5}.info{flex:1;padding-right:24px;min-width:0}.setting-item{display:flex;justify-content:space-between;align-items:center;padding:14px 0}.col-item{flex-direction:column;align-items:stretch;gap:10px;padding:16px 0}.divider{height:1px;background:var(--ui-border-subtle);opacity:.5}.btn-group{display:flex;gap:8px}.sub-header{display:flex;align-items:center;gap:16px;margin-bottom:12px;padding:0 0 12px}.sub-header h3{margin:0;padding:0}.back-btn{background:var(--surface);border:none;color:var(--text-main);width:36px;height:36px;border-radius:50%;display:flex;align-items:center;justify-content:center;cursor:pointer}.back-btn:hover{background:var(--ui-surface-hover)}.back-icon-svg :deep(svg){width:18px;height:18px}.link-text{font-family:monospace;font-size:.8rem;color:var(--text-muted);margin-top:4px}.red-text{color:var(--text-muted)}.modal-footer .action-btn,.modal-footer .primary-btn{height:42px;display:flex;align-items:center;justify-content:center}
</style>
