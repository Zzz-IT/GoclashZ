<template>
  <div class="subs-view" @click="activeMenu = null">
    <div class="page-header page-sticky-mask">
      <div class="header-text">
        <h2 class="main-title">{{ t('subs.title') }}</h2>
        <span class="sub-text">
          {{ isSortingMode ? t('subs.sortModeHint') : t('subs.defaultHint') }}
        </span>
      </div>
      
      <div class="header-actions">
        <button 
          class="action-btn" 
          :class="isSortingMode ? 'sorting-active' : 'accent-btn'" 
          @click.stop="toggleSortMode"
        >
          <span class="btn-icon" v-html="ICONS.sort"></span> 
          {{ isSortingMode ? t('subs.doneBtn') : t('subs.sortBtn') }}
        </button>
        <button class="primary-btn header-action-btn" @click.stop="activeModal = 'import'">
          <span class="btn-icon" v-html="ICONS.plus"></span> {{ t('subs.importBtn') }}
        </button>
        <button class="primary-btn header-action-btn" @click="handleUpdateAll" :disabled="isUpdating">
          <span class="btn-icon" v-html="ICONS.refresh" :class="{ 'spin': isUpdating }"></span>
          {{ isUpdating ? t('subs.updating') : t('subs.updateAllBtn') }}
        </button>
      </div>
    </div>

    <!-- 列表过渡动画 -->
    <TransitionGroup name="list" tag="div" class="subs-list">
      <div v-if="localConfigs.length === 0" class="empty-state" key="empty">
        {{ t('subs.emptyState') }}
      </div>

      <div
        v-for="(config, index) in localConfigs"
        :key="config.id"
        class="sub-card clickable"
        :class="{ 'active-card': isCurrentConfig(config.id), 'is-sorting': isSortingMode }"
        @click="!isSortingMode && handleSelectConfig(config)"
      >
        <div class="sub-header">
          <div class="sub-info">
            <div v-if="isSortingMode" class="sort-controls">
              <button class="arrow-btn" :disabled="index === 0" @click.stop="moveUp(index)" v-html="ICONS.up"></button>
              <button class="arrow-btn" :disabled="index === localConfigs.length - 1" @click.stop="moveDown(index)" v-html="ICONS.down"></button>
            </div>
            <div style="flex: 1; min-width: 0;">
              <h4 class="sub-name">{{ config.name }}</h4>
              <span class="sub-path font-mono">Subscriptions/{{ config.id }}.yaml</span>
            </div>
          </div>
          <div class="sub-status">
            <div v-if="isCurrentConfig(config.id)" class="status-badge-active">
              <div class="breathe-dot"></div>
              <span>{{ t('subs.inUse') }}</span>
            </div>
          </div>
        </div>

        <div v-if="!isSortingMode && config.total > 0" class="traffic-container">
          <div class="traffic-bar">
            <div class="traffic-fill" :style="{ width: Math.min(100, ((config.upload + config.download) / config.total) * 100) + '%' }"></div>
          </div>
          <div class="traffic-text">
            <span>{{ t('subs.used') }} {{ formatBytes(config.upload + config.download) }}</span>
            <span>{{ t('subs.total') }} {{ formatBytes(config.total) }}</span>
          </div>
          <div v-if="config.expire > 0" class="expire-text">
            {{ t('subs.expireDate', { date: formatDate(config.expire) }) }}
          </div>
        </div>

        <div class="sub-footer" v-show="!isSortingMode">
          <span class="sub-hint">{{ t('subs.clickToApply') }}</span>
          <div class="sub-actions">
            <button class="icon-btn" @click.stop="toggleMenu(config.id)" v-html="ICONS.more"></button>
            <div v-if="activeMenu === config.id" class="menu-click-overlay" @click.stop="activeMenu = null"></div>
            <Transition name="dropdown">
              <div v-if="activeMenu === config.id" class="dropdown-menu card-panel">
                <button v-if="config.type === 'remote'" class="menu-item" @click.stop="handleUpdateSingle(config)">{{ t('subs.menuUpdate') }}</button>
                <button class="menu-item" @click.stop="openRenameModal(config)">{{ t('subs.menuRename') }}</button>
                <button class="menu-item" @click.stop="handleEditFile(config)">{{ t('subs.menuEdit') }}</button>
                <button v-if="config.type === 'remote' && config.url" class="menu-item" @click.stop="handleShareLink(config)">{{ t('subs.menuShare') }}</button>
                <button class="menu-item" @click.stop="handleExport(config)">{{ t('subs.menuExport') }}</button>
                <button class="menu-item danger" @click.stop="openDeleteModal(config)">{{ t('subs.menuDelete') }}</button>
              </div>
            </Transition>
          </div>
        </div>
      </div>
    </TransitionGroup>

    <!-- 统一模态框系统 -->
    <Transition name="pop">
      <div v-if="activeModal" class="modal-overlay" @click="closeAllModals">
        <!-- 导入主入口弹窗 -->
        <div v-if="activeModal === 'import'" class="custom-modal-card" @click.stop>
          <div class="modal-header">
            <h3>{{ t('subs.importModalTitle') }}</h3>
          </div>
          <div class="modal-body">
            <p class="global-modal-msg">{{ t('subs.importUrlLabel') }}</p>
            <div style="display: flex; gap: 8px;">
              <input v-model="newSubUrl" :placeholder="t('subs.urlPlaceholder')" class="modal-input" style="flex: 1;" :disabled="isImporting" />
              <button class="primary-btn mini-btn" style="height: 44px;" @click="handleDownload" :disabled="!newSubUrl.trim() || isImporting">{{ t('subs.downloadBtn') }}</button>
            </div>
            <div class="divider-text">{{ t('subs.or') }}</div>
            <button class="action-btn w-full-btn hover-accent" @click="handlePickFile" :disabled="isImporting" style="height: 44px; margin-top: 4px;">
              <span class="btn-icon" v-html="ICONS.folder" style="margin-right: 4px;"></span> {{ t('subs.browseFile') }}
            </button>
          </div>
        </div>

        <!-- 导入确认/命名弹窗 -->
        <div v-if="activeModal === 'import_confirm'" class="custom-modal-card" @click.stop>
          <div class="modal-header">
            <h3>{{ isImportingRemote ? t('subs.importRemoteTitle') : t('subs.importLocalTitle') }}</h3>
          </div>
          <div class="modal-body">
            <p class="text-xs text-gray-500 mb-2 truncate" style="margin-bottom: 8px; opacity: 0.6; width: 100%; display: block;">
              {{ isImportingRemote ? t('subs.linkPrefix') : t('subs.filePrefix') }} {{ pendingImportPath }}
            </p>
            <p class="global-modal-msg">{{ t('subs.setNameLabel') }}</p>
            <input 
              v-model="configNameInput" 
              :placeholder="t('subs.inputNamePlaceholder')" 
              class="modal-input"
              @keyup.enter="confirmImport"
              :disabled="isImporting"
            />
            <div class="modal-footer">
              <button class="action-btn flex-1" @click="activeModal = 'import'" :disabled="isImporting">{{ t('common.back') }}</button>
              <button class="primary-btn accent-btn flex-1" @click="confirmImport" :disabled="!configNameInput || isImporting">
                <span v-if="isImporting" class="btn-icon spin" v-html="ICONS.refresh"></span>
                {{ isImporting ? (isImportingRemote ? t('subs.downloading') : t('subs.importing')) : t('common.confirm') }}
              </button>
            </div>
          </div>
        </div>

        <!-- 重命名弹窗 -->
        <div v-if="activeModal === 'rename'" class="custom-modal-card" @click.stop>
          <div class="modal-header">
            <h3>{{ t('subs.renameTitle') }}</h3>
          </div>
          <div class="modal-body">
            <p class="global-modal-msg">{{ t('subs.inputNewName') }}</p>
            <input v-model="renameValue" class="modal-input" :placeholder="t('subs.renamePlaceholder')" @keyup.enter="confirmRename" :disabled="isRenaming" />
            <div class="modal-footer">
              <button class="action-btn flex-1" @click="closeAllModals" :disabled="isRenaming">{{ t('common.cancel') }}</button>
              <button class="primary-btn accent-btn flex-1" @click="confirmRename" :disabled="!renameValue || isRenaming">{{ t('common.confirm') }}</button>
            </div>
          </div>
        </div>

        <!-- 删除确认弹窗 -->
        <div v-if="activeModal === 'delete'" class="custom-modal-card" @click.stop>
          <div class="modal-header">
            <h3 class="danger-text">{{ t('subs.menuDelete') }}</h3>
          </div>
          <div class="modal-body">
            <p v-if="isCurrentConfig(targetId)" class="modal-hint-text">
              <strong>{{ t('common.warning') }}: </strong> {{ t('subs.deleteWarningRunning', { name: targetName }) }}
            </p>
            <p v-else class="modal-hint-text">{{ t('subs.deleteConfirm', { name: targetName }) }}</p>
            <div class="modal-footer">
              <button class="action-btn flex-1" @click="closeAllModals">{{ t('common.cancel') }}</button>
              <button class="primary-btn accent-btn red-text-btn flex-1" @click="confirmDelete" :disabled="isDeleting">
                {{ isDeleting ? t('subs.deleting') : t('subs.confirmDelete') }}
              </button>
            </div>
          </div>
        </div>
      </div>
    </Transition>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted } from 'vue';
import * as API from '../../wailsjs/go/main/App';
import { clash } from '../../wailsjs/go/models';
import { EventsOn, BrowserOpenURL } from '../../wailsjs/runtime/runtime';
import { showAlert, showConfirm, globalState } from '../store';
import { formatBytes } from '../utils/format';
import { ICONS } from '../utils/icons';
import { t } from '../locales';

const emit = defineEmits<{
  'edit-config': [id: string, name: string, type: 'local' | 'remote'];
}>();

const activeModal = ref<'import' | 'import_confirm' | 'rename' | 'delete' | null>(null);
const targetId = ref('');
const targetName = ref('');
const renameValue = ref('');
const isUpdating = ref(false);
const isImporting = ref(false);
const isRenaming = ref(false);
const isDeleting = ref(false);

const localConfigs = ref<clash.SubIndexItem[]>([]);
const activeMenu = ref<string | null>(null);

// --- 导入相关状态 ---
const newSubUrl = ref('');
const pendingImportPath = ref('');
const configNameInput = ref('');
const isImportingRemote = ref(false);



const formatDate = (timestamp: number) => {
  if (!timestamp) return '';
  return new Date(timestamp * 1000).toLocaleDateString();
};

const isSortingMode = ref(false);

const toggleSortMode = () => {
  isSortingMode.value = !isSortingMode.value;
  activeMenu.value = null;
};

const moveUp = async (index: number) => {
  if (index <= 0) return;
  const items = [...localConfigs.value];
  [items[index - 1], items[index]] = [items[index], items[index - 1]];
  localConfigs.value = items;
};

const moveDown = async (index: number) => {
  if (index >= localConfigs.value.length - 1) return;
  const items = [...localConfigs.value];
  [items[index + 1], items[index]] = [items[index], items[index + 1]];
  localConfigs.value = items;
};

const isCurrentConfig = (id: string) => {
  return globalState.activeConfigId === id;
};

const fetchConfigs = async () => {
  try {
    localConfigs.value = await API.GetLocalConfigs() || [];
    const data: any = await API.GetInitialData();
    if (data && data.activeConfig !== undefined) {
      globalState.activeConfigId = data.activeConfig;
      globalState.activeConfigName = data.activeConfigName || '';
      globalState.activeConfigType = data.activeConfigType || '';
    }
  } catch (e) {
    console.error("同步状态失败:", e);
  }
};

let currentSelectionNonce = 0;

const handleSelectConfig = async (config: clash.SubIndexItem) => {
  if (isCurrentConfig(config.id)) return;
  
  // 乐观 UI 更新：不阻塞连续点击，直接放行
  currentSelectionNonce++;
  const thisNonce = currentSelectionNonce;
  
  const previousConfigId = globalState.activeConfigId;
  globalState.activeConfigId = config.id;
  
  const maxAttempts = 2;
  let lastError: any = null;
  
  for (let i = 0; i < maxAttempts; i++) {
    try {
      await API.SelectLocalConfig(config.id);
      lastError = null;
      break;
    } catch (error) {
      lastError = error;
      // 如果发生重试，检查期间是否有新的点击，如果有直接中断当前重试
      if (currentSelectionNonce !== thisNonce) return;
      if (i < maxAttempts - 1) await new Promise(r => setTimeout(r, 500));
    }
  }
  
  // 仅当当前请求仍是最新的（没有被新点击覆盖）且失败时，才进行回滚和报错
  if (lastError && currentSelectionNonce === thisNonce) {
    globalState.activeConfigId = previousConfigId;
    await showAlert(t('subs.errSwitchFailed', { error: String(lastError) }), t('common.error'));
  }
};

const handleUpdateAll = async () => {
  const remoteItems = localConfigs.value.filter(c => c.type === 'remote');
  if (remoteItems.length === 0) {
    await showAlert(t('subs.noRemoteSubs'), t('common.notice'));
    return;
  }

  // 🚀 发起异步任务，具体状态由事件监听器控制
  API.UpdateAllSubsAsync().catch(() => {});
};

const handleUpdateSingle = async (config: clash.SubIndexItem) => {
  if (config.type !== 'remote') return;
  activeMenu.value = null;
  isUpdating.value = true;
  try {
    await API.UpdateSingleSub(config.id);
    await fetchConfigs();
    await showAlert(t('subs.updateSingleSuccess'), t('subs.updateDone'));
  } catch (e) {
    await showAlert(t('subs.updateError', { error: String(e) }), t('common.error'));
  } finally {
    isUpdating.value = false;
  }
};

const handleDownload = () => {
  if (!newSubUrl.value.trim()) return;
  pendingImportPath.value = newSubUrl.value.trim();
  configNameInput.value = t('subs.newSubDefaultName');
  isImportingRemote.value = true;
  activeModal.value = 'import_confirm';
};

const handlePickFile = async () => {
  try {
    const result = await API.SelectLocalFile();
    if (result && result.path) {
      pendingImportPath.value = result.path;
      configNameInput.value = result.name;
      isImportingRemote.value = false;
      activeModal.value = 'import_confirm';
    }
  } catch (e) {
    console.log("Import cancelled");
  }
};

const confirmImport = async () => {
  if (!pendingImportPath.value || !configNameInput.value) return;
  isImporting.value = true;
  try {
    const finalName = configNameInput.value.trim() || (isImportingRemote.value ? t('subs.unnamedSub') : t('subs.unnamedFile'));
    if (isImportingRemote.value) {
      await API.UpdateSub(finalName, pendingImportPath.value);
    } else {
      await API.DoLocalImport(pendingImportPath.value, finalName);
    }
    await fetchConfigs();
    closeAllModals();
    newSubUrl.value = '';
  } catch (e) {
    console.error("导入失败:", e);
    await showAlert(t('subs.importFailed', { error: String(e) }), t('common.error'));
  } finally {
    isImporting.value = false;
  }
};

const openRenameModal = (config: clash.SubIndexItem) => {
  activeMenu.value = null;
  targetId.value = config.id;
  renameValue.value = config.name;
  activeModal.value = 'rename';
};

const confirmRename = async () => {
  if (!renameValue.value) return;
  isRenaming.value = true;
  try {
    await API.RenameConfig(targetId.value, renameValue.value);
    await fetchConfigs();
    closeAllModals();
  } catch (e) {
    console.error("重命名失败:", e);
    await showAlert(t('subs.renameFailed', { error: String(e) }), t('common.error'));
  } finally {
    isRenaming.value = false;
  }
};

const handleEditFile = async (config: clash.SubIndexItem) => {
  activeMenu.value = null;
  emit('edit-config', config.id, config.name, config.type as 'local' | 'remote');
};

const handleShareLink = async (config: clash.SubIndexItem) => {
  activeMenu.value = null;
  if (!config.url) return;
  let status: string;
  try {
    await navigator.clipboard.writeText(config.url);
    status = t('subs.linkCopied');
  } catch {
    status = t('subs.copyFailed');
  }
  globalState.modal = {
    show: true,
    title: t('subs.shareLinkTitle'),
    message: status,
    detail: config.url,
    type: 'alert',
    isDanger: false,
    onConfirm: () => { globalState.modal.show = false; },
    onCancel: null,
  };
};

const handleExport = (config: clash.SubIndexItem) => {
  activeMenu.value = null;
  globalState.modal = {
    show: true,
    title: t('subs.menuExport'),
    message: t('subs.exportReplaceRulesPrompt'),
    detail: '',
    type: 'confirm',
    isDanger: false,
    onConfirm: async () => {
      globalState.modal.show = false;
      await doExport(config.id, true);
    },
    onCancel: async () => {
      globalState.modal.show = false;
      await doExport(config.id, false);
    },
  };
};

const doExport = async (id: string, useCustomRules: boolean) => {
  try {
    await API.ExportConfig(id, useCustomRules);
  } catch (e) {
    await showAlert(t('subs.exportFailed', { error: String(e) }), t('common.error'));
  }
};

const openDeleteModal = (config: clash.SubIndexItem) => {
  activeMenu.value = null;
  targetId.value = config.id;
  targetName.value = config.name;
  activeModal.value = 'delete';
};

const confirmDelete = async () => {
  isDeleting.value = true;
  try {
    await API.DeleteConfig(targetId.value);
    await fetchConfigs();
    closeAllModals();
  } catch (e) {
    console.error("删除失败:", e);
    await showAlert(t('subs.deleteFailed', { error: String(e) }), t('common.error'));
  } finally {
    isDeleting.value = false;
  }
};

const toggleMenu = (id: string) => {
  activeMenu.value = activeMenu.value === id ? null : id;
};

const closeAllModals = () => {
  activeModal.value = null;
  pendingImportPath.value = '';
  configNameInput.value = '';
};

let unsubConfigChanged: (() => void) | null = null;
let unsubSubsUpdateStart: (() => void) | null = null;
let unsubSubsUpdateSuccess: (() => void) | null = null;
let unsubSubsUpdateError: (() => void) | null = null;

onMounted(() => {
  fetchConfigs();

  unsubSubsUpdateStart = EventsOn("subs-update-start", () => {
    isUpdating.value = true;
  });

  unsubSubsUpdateSuccess = EventsOn("subs-update-success", async () => {
    isUpdating.value = false;
    await fetchConfigs();
    await showAlert(t('subs.updateAllSuccessMsg'), t('subs.updateSuccessTitle'));
  });

  unsubSubsUpdateError = EventsOn("subs-update-error", async (err: string) => {
    isUpdating.value = false;
    await showAlert(t('subs.updateFailedMsg', { error: String(err) }), t('subs.updateResultTitle'));
  });

  unsubConfigChanged = EventsOn("config-changed", (newId: string) => {
    if (newId) {
      globalState.activeConfigId = newId;
      fetchConfigs();
    }
  });
});

onUnmounted(() => {
  unsubConfigChanged?.();
  unsubSubsUpdateStart?.();
  unsubSubsUpdateSuccess?.();
  unsubSubsUpdateError?.();
});
</script>

<style scoped>
.subs-view { 
  display: flex; 
  flex-direction: column; 
  min-height: 100%; 
  overflow: visible;
  color: var(--text-main); 
}
.page-header { 
  display: flex; 
  justify-content: space-between; 
  align-items: flex-end; 
  margin-bottom: 32px; 
  padding: 4px 0 12px 0;
  background: transparent;
}
.page-header.page-sticky-mask {
  --sticky-mask-bleed: 4px;
}
.main-title { font-size: 1.6rem; font-weight: 600; margin-bottom: 4px; }
.sub-text { font-size: 0.85rem; color: var(--text-sub); }
.truncate { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.header-actions { display: flex; gap: 12px; align-items: center; }
.header-action-btn {
  padding: 0 14px !important;
}

.subs-list { flex: 1; position: relative; }

/* 卡片基础样式 */
.sub-card { 
  position: relative; padding: 20px; border-radius: 12px; 
  background: var(--surface); margin-bottom: 16px; 
  transition: 0.2s; border: 1px solid transparent; 
}
.sub-card:not(.is-sorting):hover { background: var(--surface-hover); }
.sub-card.is-sorting { cursor: default; }

.active-card { 
  background: var(--accent) !important; 
  color: var(--accent-fg) !important; 
  border-color: var(--accent) !important;
}
.active-card .sub-path, .active-card .sub-hint, .active-card .icon-btn { color: var(--accent-fg) !important; opacity: 0.8; }
.sub-header { display: flex; justify-content: space-between; margin-bottom: 12px; }
.sub-info { display: flex; align-items: center; gap: 16px; }
.sub-name { font-size: 0.95rem; font-weight: 600; margin: 0; }
.sub-path { font-size: 0.7rem; color: var(--text-muted); }

/* ================================== */
/* 列表过渡动画 */
/* ================================== */
.list-move {
  transition: transform 0.3s cubic-bezier(0.25, 1, 0.5, 1);
}

.list-leave-active {
  position: absolute;
  width: calc(100% - 40px);
}

.list-enter-active,
.list-leave-active {
  transition: all 0.3s cubic-bezier(0.25, 1, 0.5, 1);
}
.list-enter-from,
.list-leave-to {
  opacity: 0;
  transform: translateY(15px);
}

.header-actions { display: flex; gap: 12px; align-items: center; }

.sorting-active {
  background: var(--surface-hover) !important; 
  color: var(--text-main) !important;
  font-weight: 600;
}

.sorting-active .btn-icon {
  color: var(--text-main) !important;
  opacity: 1;
}

.header-actions .action-btn:active,
.header-actions .primary-btn:active {
  transform: none;
}

.sort-controls {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.arrow-btn {
  width: 24px;
  height: 20px;
  background: transparent;
  border: 1px solid transparent;
  border-radius: 4px;
  display: flex;
  align-items: center;
  justify-content: center;
  cursor: pointer;
  color: var(--text-muted);
  transition: all 0.2s;
}

.arrow-btn :deep(svg) { width: 14px; height: 14px; }
.arrow-btn:hover:not(:disabled) {
  background: var(--surface-hover);
  color: var(--text-main);
  border-color: var(--text-sub);
}
.active-card .arrow-btn { color: var(--accent-fg); opacity: 0.7; }
.active-card .arrow-btn:hover:not(:disabled) {
  background: rgba(0,0,0,0.1);
  color: var(--accent-fg);
  opacity: 1;
  border-color: var(--accent-fg);
}
.arrow-btn:disabled {
  opacity: 0.2;
  cursor: not-allowed;
}

.status-badge-active { display: flex; align-items: center; gap: 8px; font-size: 0.75rem; font-weight: 700; }
.breathe-dot {
  width: 8px; height: 8px; border-radius: 50%;
  background: currentColor; box-shadow: 0 0 8px currentColor;
  animation: breathe 2s infinite ease-in-out;
}
@keyframes breathe {
  0%, 100% { opacity: 0.4; transform: scale(0.9); }
  50% { opacity: 1; transform: scale(1.1); }
}
.sub-footer { display: flex; justify-content: space-between; align-items: center; margin-top: 10px; padding-top: 10px; }
.sub-hint { font-size: 0.75rem; color: var(--text-sub); font-style: italic; }

.icon-btn {
  width: 24px !important; height: 24px !important;
  padding: 0; display: flex; align-items: center; justify-content: center;
  background: none; border: none; cursor: pointer; color: var(--text-sub); border-radius: 4px;
}
.icon-btn :deep(svg) { width: 14px !important; height: 14px !important; }
.icon-btn:hover { background: var(--surface-hover); color: var(--text-main); }
.active-card .icon-btn:hover { background: transparent !important; }

.mini-btn { height: 44px; padding: 0 20px; font-weight: 700; border-radius: 12px; }
.divider-text { text-align: center; margin: 15px 0; color: var(--text-muted); font-size: 0.75rem; position: relative; }
.divider-text::before, .divider-text::after { content: ""; position: absolute; top: 50%; width: 35%; height: 1px; background: var(--surface-hover); }
.divider-text::before { left: 0; } .divider-text::after { right: 0; }
.w-full-btn { width: 100%; justify-content: center; padding: 14px; font-weight: 600; border-radius: 10px; border: none; background: var(--surface-hover); color: var(--text-main); cursor: pointer; display: flex; align-items: center; gap: 8px; }
.modal-hint-text { padding: 4px 0 12px 0; color: var(--text-sub); font-size: 0.85rem; line-height: 1.6; }
.danger-text { color: #ff4d4f !important; }

/* --- 菜单遮罩 --- */
.menu-click-overlay {
  position: fixed;
  top: 0; left: 0; width: 100vw; height: 100vh;
  z-index: 9;
  background: transparent;
}

.dropdown-menu { 
  position: absolute; right: 0; top: 30px; width: 150px; border-radius: 8px; z-index: 10; overflow: hidden;
  background: var(--surface); border: 1px solid var(--surface-hover); box-shadow: 0 4px 12px rgba(0,0,0,0.1);
}
.menu-item { width: 100%; padding: 10px 16px; border: none; background: transparent; text-align: left; cursor: pointer; font-size: 0.85rem; color: var(--text-main); }
.menu-item:hover { background: var(--surface-hover); }
.menu-item.danger { color: #ff4d4f !important; font-weight: 600; }
.empty-state { padding: 30px; text-align: center; color: var(--text-muted); border: none; border-radius: 10px; background: var(--surface); }

/* --- 下拉菜单动画 --- */
.dropdown-enter-active, .dropdown-leave-active {
  transition: all 0.2s cubic-bezier(0.4, 0, 0.2, 1);
}
.dropdown-enter-from, .dropdown-leave-to {
  opacity: 0;
  transform: translateY(-8px) scale(0.95);
}
.dropdown-enter-to, .dropdown-leave-from {
  opacity: 1;
  transform: translateY(0) scale(1);
}

.spin :deep(svg) {
  animation: spin 1s linear infinite;
}

@keyframes spin {
  from { transform: rotate(0deg); }
  to { transform: rotate(360deg); }
}

.traffic-container {
  margin-top: 10px; 
  display: flex;
  flex-direction: column;
  gap: 6px;
  width: 100%;
}

.traffic-bar {
  width: 100%;
  height: 6px;
  background: rgba(128, 128, 128, 0.2);
  border-radius: 6px;
  overflow: hidden;
}

.traffic-fill {
  height: 100%;
  background: var(--text-main); 
  border-radius: 6px;
  transition: width 0.4s ease;
}

.traffic-text {
  display: flex;
  justify-content: space-between;
  font-size: 0.7rem;
  color: var(--text-sub);
  font-weight: 600;
}

.expire-text {
  font-size: 0.65rem;
  color: var(--text-muted);
  text-align: right;
  margin-top: -2px;
}

.active-card .traffic-bar {
  background: rgba(0, 0, 0, 0.2);
}
.active-card .traffic-fill {
  background: var(--accent-fg);
}
.active-card .traffic-text {
  color: var(--accent-fg);
  opacity: 0.9;
}
.active-card .expire-text {
  color: var(--accent-fg);
  opacity: 0.7;
}
</style>
