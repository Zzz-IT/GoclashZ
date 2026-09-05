<template>
  <div class="settings-page">
    <div class="sub-header page-sticky-mask">
      <button class="back-btn" @click="$emit('navigate', 'main')">
        <span class="icon back-icon-svg" v-html="ICONS.arrowLeft"></span>
      </button>
      <h3>{{ t('settings.uwp.title') }}</h3>
    </div>

    <div class="uwp-toolbar">
      <div class="uwp-search">
        <span class="search-icon" v-html="ICONS.search"></span>
        <input v-model="uwpSearch" :placeholder="t('settings.uwp.searchPlaceholder')" />
        <span v-if="uwpSearch" class="clear-icon" @click="uwpSearch = ''" v-html="ICONS.close"></span>
      </div>
      <div class="uwp-batch">
        <button class="batch-btn" @click="toggleAllUwp(true)">{{ t('settings.uwp.selectAll') }}</button>
        <button class="batch-btn" @click="toggleAllUwp(false)">{{ t('settings.uwp.invertSelect') }}</button>
      </div>
    </div>

    <div class="uwp-list-wrapper scrollable">
      <div
        v-for="app in filteredUwpApps"
        :key="app.sid"
        class="uwp-app-item"
        :class="{ 'active': app.isEnabled }"
        @click="app.isEnabled = !app.isEnabled"
      >
        <div class="app-main-content">
          <div class="app-avatar">
            {{ app.displayName?.[0]?.toUpperCase() || '?' }}
          </div>
          <div class="app-details">
            <span class="app-name">{{ app.displayName || t('settings.uwp.unnamedApp') }}</span>
            <span class="app-pkg">{{ app.packageFamilyName }}</span>
          </div>
        </div>

        <div class="app-status-wrapper">
          <div class="uwp-status-tag">
            {{ app.isEnabled ? t('settings.uwp.exempted') : t('settings.uwp.restricted') }}
          </div>
        </div>
      </div>
    </div>

    <div class="uwp-footer">
      <button class="apply-btn" :disabled="savingUwp" @click="saveUwpChanges">
        <span v-if="!savingUwp">{{ t('settings.uwp.applyBtn') }}</span>
        <span v-else class="loading-spinner">{{ t('settings.uwp.saving') }}</span>
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue';
import * as API from '../../wailsjs/go/main/App';
import { showAlert } from '../store';
import { ICONS } from '../utils/icons';
import { t } from '../locales';

const props = defineProps<{ entry: number }>();

defineEmits<{
  navigate: [view: 'main'];
}>();

const uwpApps = ref<any[]>([]);
const uwpSearch = ref('');
const savingUwp = ref(false);

const enterUwpManager = async () => {
  try {
    uwpApps.value = await (API as any).GetUwpApps();
  } catch (e) {
    showAlert(t('settings.uwp.fetchFailed', { error: String(e) }), t('common.error'));
  }
};

watch(() => props.entry, (entry) => {
  if (entry > 0) {
    void enterUwpManager();
  }
});

onMounted(() => {
  if (props.entry > 0) {
    void enterUwpManager();
  }
});

const filteredUwpApps = computed(() => {
  const q = uwpSearch.value.toLowerCase();
  return uwpApps.value.filter(app =>
    (app.displayName || '').toLowerCase().includes(q) ||
    (app.packageFamilyName || '').toLowerCase().includes(q)
  );
});

const toggleAllUwp = (val: boolean) => {
  if (val) {
    uwpApps.value.forEach(app => app.isEnabled = true);
  } else {
    uwpApps.value.forEach(app => app.isEnabled = !app.isEnabled);
  }
};

const saveUwpChanges = async () => {
  savingUwp.value = true;
  try {
    const sids = uwpApps.value.filter(a => a.isEnabled).map(a => a.sid);
    await (API as any).SaveUwpExemptions(sids);
    await showAlert(t('settings.uwp.saveSuccess'), t('common.confirm'));
  } catch (e) {
    await showAlert(t('settings.uwp.saveFailed', { error: String(e) }), t('common.error'));
  } finally {
    savingUwp.value = false;
  }
};
</script>

<style scoped>
.settings-page { display: flex; flex-direction: column; flex: 1; min-height: 100%; overflow: visible; }
h3 { margin: 0 0 8px 0; color: var(--text-main); font-size: 1.25rem; padding-bottom: 4px; }
.sub-header { display: flex; align-items: center; gap: 16px; margin-bottom: 12px; padding: 0 0 12px 0; background: transparent; }
.sub-header.page-sticky-mask { --sticky-mask-bleed: 4px; }
.sub-header h3 { margin: 0; border: none; padding: 0; }
.back-btn { background: var(--surface); border: none; color: var(--text-main); width: 36px; height: 36px; border-radius: 50%; display: flex; align-items: center; justify-content: center; cursor: pointer; transition: 0.2s; }
.back-btn:hover { background: var(--surface-hover); }
.back-btn .icon svg { width: 18px; height: 18px; display: block; }
.back-icon-svg :deep(svg) { width: 18px; height: 18px; }
.uwp-toolbar { display: flex; align-items: center; gap: 16px; margin-bottom: 20px; }
.uwp-search { flex: 1; display: flex; align-items: center; background: var(--surface); border: 1px solid var(--surface-hover); border-radius: 10px; padding: 0 12px; height: 40px; transition: all 0.2s; }
.uwp-search:focus-within { border-color: var(--accent); background: var(--surface-panel); }
.uwp-search input { flex: 1; border: none; background: transparent; color: var(--text-main); outline: none; margin-left: 8px; font-size: 0.9rem; }
.search-icon, .clear-icon { display: flex; align-items: center; color: var(--text-sub); }
.clear-icon { cursor: pointer; padding: 4px; }
.uwp-batch { display: flex; gap: 8px; }
.batch-btn { background: var(--surface-hover); color: var(--text-main); border: none; padding: 8px 16px; border-radius: 8px; font-size: 0.85rem; font-weight: 600; cursor: pointer; transition: 0.2s; }
.batch-btn:hover { background: var(--surface-panel); }
.uwp-list-wrapper { display: flex; flex-direction: column; gap: 10px; flex: 1; padding-right: 4px; }
.uwp-app-item { background: var(--surface); border-radius: 12px; padding: 12px 16px; display: flex; justify-content: space-between; align-items: center; cursor: pointer; transition: all 0.2s cubic-bezier(0.4, 0, 0.2, 1); border: 1px solid transparent; }
.uwp-app-item:hover { background: var(--surface-hover); }
.uwp-app-item.active { background: var(--accent); box-shadow: 0 4px 15px rgba(0, 0, 0, 0.1); }
.app-main-content { display: flex; align-items: center; gap: 16px; overflow: hidden; flex: 1; }
.app-avatar { width: 42px; height: 42px; background: var(--surface-panel); border-radius: 10px; display: flex; align-items: center; justify-content: center; font-weight: 800; font-size: 1.3rem; color: var(--text-sub); flex-shrink: 0; }
.uwp-app-item.active .app-avatar { background: rgba(255, 255, 255, 0.15); color: var(--accent-fg); }
.app-details { display: flex; flex-direction: column; gap: 2px; overflow: hidden; }
.app-name { font-size: 0.95rem; font-weight: 700; color: var(--text-main); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.uwp-app-item.active .app-name { color: var(--accent-fg); }
.app-pkg { font-size: 0.75rem; color: var(--text-sub); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; opacity: 0.7; }
.uwp-app-item.active .app-pkg { color: var(--accent-fg); opacity: 0.8; }
.uwp-status-tag { font-size: 0.7rem; letter-spacing: 0; font-weight: 600; padding: 3px 10px; border-radius: 4px; text-transform: uppercase; transition: all 0.2s; background: var(--surface-panel); color: var(--text-main); }
.uwp-app-item.active .uwp-status-tag { background: var(--accent-fg) !important; color: var(--accent) !important; opacity: 0.8; }
.uwp-footer { margin-top: 20px; padding-top: 10px; }
.apply-btn { width: 100%; padding: 14px; background: var(--accent); color: var(--accent-fg); border: none; border-radius: 12px; font-weight: 700; cursor: pointer; transition: all 0.2s; box-shadow: 0 4px 10px rgba(0, 0, 0, 0.1); }
.apply-btn:hover:not(:disabled) { filter: brightness(1.1); transform: translateY(-1px); }
.apply-btn:disabled { opacity: 0.5; cursor: not-allowed; }
.loading-spinner { display: flex; align-items: center; justify-content: center; gap: 8px; }
</style>
