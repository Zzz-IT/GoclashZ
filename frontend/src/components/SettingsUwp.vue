<template>
  <div class="settings-page">
    <div class="sub-header page-sticky-mask">
      <button class="back-btn" @click="$emit('navigate', 'main')">
        <span class="icon back-icon-svg" v-html="ICONS.arrowLeft"></span>
      </button>
      <h3>{{ t('settings.uwp.title') }}</h3>
    </div>

    <!-- 非管理员权限提示卡片 -->
    <div v-if="!globalState.isAdmin" class="glass-card setting-group">
      <div class="setting-item" id="set-uwp-adminPromptTitle">
        <div class="info">
          <h4>{{ t('settings.uwp.adminPromptTitle') }}</h4>
          <p>{{ t('settings.uwp.notAdminBanner') }}</p>
        </div>
        <button class="action-btn" @click="handleRestartAdmin">{{ t('settings.uwp.restartAsAdmin') }}</button>
      </div>
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

    <div class="uwp-list-wrapper scrollable" id="set-uwp-exemptList">
      <!-- 列表没加载成功时既没有可改的项，也不允许提交，这里给一行提示（复用全局空态样式） -->
      <div v-if="!uwpLoaded" class="empty-state-block">
        <div class="empty-state-hint">{{ t('common.empty') }}</div>
      </div>

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
      <button class="apply-btn" id="set-uwp-applyBtn" :disabled="savingUwp || !uwpLoaded" @click="saveUwpChanges">
        <span v-if="!savingUwp">{{ t('settings.uwp.applyBtn') }}</span>
        <span v-else class="loading-spinner">{{ t('settings.uwp.saving') }}</span>
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue';
import * as API from '../../wailsjs/go/main/App';
import { globalState, showAlert, showConfirm } from '../store';
import { ICONS } from '../utils/icons';
import { t } from '../locales';

const props = defineProps<{ entry: number }>();

defineEmits<{
  navigate: [view: 'main'];
}>();

const uwpApps = ref<any[]>([]);
const uwpSearch = ref('');
const savingUwp = ref(false);
// 列表是否成功加载过：false 时禁止提交，避免「空列表 = 用户取消了全部」的误判
const uwpLoaded = ref(false);
// 加载成功那一刻的 isEnabled 快照：保存时据此算出真正的 added / removed
const initialEnabled = ref<Record<string, boolean>>({});

const handleRestartAdmin = async () => {
  try {
    await (API as any).RestartAsAdmin();
  } catch (e: any) {
    await showAlert(t('common.error') + ': ' + (e?.message || e), t('common.error'), true);
  }
};

const enterUwpManager = async () => {
  try {
    const list = await (API as any).GetUwpApps();
    uwpApps.value = list;

    const snapshot: Record<string, boolean> = {};
    for (const app of list) snapshot[app.sid] = !!app.isEnabled;
    initialEnabled.value = snapshot;

    uwpLoaded.value = true;
  } catch (e) {
    // 加载失败保持 false：按钮继续禁用，避免把「没拿到列表」当成「用户取消了全部」
    uwpLoaded.value = false;
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

// 清空确认文案：该 key 目前放在 diag.* 命名空间（locales 由 Lead 维护），
// 这里同时兼容 settings.uwp.* 的位置，避免任一位置变动时弹窗直接显示 key 名。
const clearAllConfirmText = (): string => {
  const scoped = t('settings.uwp.clearAllConfirm');
  return scoped === 'settings.uwp.clearAllConfirm' ? t('diag.clearAllConfirm') : scoped;
};

const saveUwpChanges = async () => {
  // 列表没成功加载时不允许提交（按钮已 disabled，这里兜底）
  if (!uwpLoaded.value) return;

  if (!globalState.isAdmin) {
    const ok = await showConfirm(
      t('settings.uwp.applyConfirmMsg'),
      t('settings.uwp.applyConfirmTitle'),
      false
    );
    if (!ok) return;
  }

  const targetSids = uwpApps.value.filter(a => a.isEnabled).map(a => a.sid);

  // 目标为空 = 用户取消了全部勾选：走专门的「清空全部豁免」接口与确认，
  // 不能把它当普通差异提交，否则和「列表没加载出来」无法区分。
  if (targetSids.length === 0) {
    const ok = await showConfirm(clearAllConfirmText(), t('settings.uwp.applyConfirmTitle'), false);
    if (!ok) return;

    savingUwp.value = true;
    try {
      await (API as any).ClearAllUwpExemptions();
      await enterUwpManager();
      await showAlert(t('settings.uwp.saveSuccess'), t('common.confirm'));
    } catch (e: any) {
      await showAlert(t('settings.uwp.saveFailed', { error: String(e?.message || e) }), t('common.error'));
    } finally {
      savingUwp.value = false;
    }
    return;
  }

  // 差异式应用：只提交用户真正勾上 / 取消的 SID，未出现在本次枚举里的系统豁免一律不动
  const added: string[] = [];
  const removed: string[] = [];
  for (const app of uwpApps.value) {
    const wasEnabled = !!initialEnabled.value[app.sid];
    const nowEnabled = !!app.isEnabled;
    if (nowEnabled === wasEnabled) continue;
    if (nowEnabled) added.push(app.sid);
    else removed.push(app.sid);
  }

  savingUwp.value = true;
  try {
    await (API as any).ApplyUwpExemptionChanges(added, removed);
    await enterUwpManager();
    await showAlert(t('settings.uwp.saveSuccess'), t('common.confirm'));
  } catch (e: any) {
    await showAlert(t('settings.uwp.saveFailed', { error: String(e?.message || e) }), t('common.error'));
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

.setting-group { padding: 16px 20px; margin-bottom: 16px; border-radius: 12px; }
.setting-item { display: flex; justify-content: space-between; align-items: center; }
.info { flex: 1; padding-right: 16px; min-width: 0; }
h4 { margin: 0 0 4px 0; color: var(--text-main); font-size: 0.95rem; }
p { margin: 0; font-size: 0.85rem; color: var(--text-sub); line-height: 1.5; }

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
