<template>
  <div class="subs-view ui-page-shell" @click="activeMenu = null">
    <div class="page-header ui-page-header">
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
    <TransitionGroup name="list" tag="div" class="subs-list ui-page-body">
      <!-- 首次加载：先显示轻量占位，避免把“还没返回”误判成“本地没有配置” -->
      <div
        v-if="configsLoading && localConfigs.length === 0"
        key="loading"
        class="subs-loading ui-surface-card"
      >
        {{ t('common.loading') }}
      </div>

      <EmptyState
        v-else-if="!configsLoading && localConfigs.length === 0"
        key="empty"
        :icon="ICONS.subs"
        :title="t('subs.emptyState')"
      />

      <div
        v-for="(config, index) in localConfigs"
        :key="config.id"
        class="sub-card clickable ui-surface-card ui-hover-surface"
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
            <button class="icon-btn" @click.stop="toggleMenu(config.id, $event)" v-html="ICONS.more"></button>
            <Teleport to="body">
            <Transition name="card-menu">
              <div v-if="activeMenu === config.id" :data-menu-id="config.id" class="dropdown-menu ui-surface-overlay" :class="{ 'is-up': menuUp }" :style="menuStyle">
                <button v-if="config.type === 'remote'" class="menu-item" @click.stop="handleUpdateSingle(config)">{{ t('subs.menuUpdate') }}</button>
                <button class="menu-item" @click.stop="openRenameModal(config)">{{ t('subs.menuRename') }}</button>
                <button class="menu-item" @click.stop="handleEditFile(config)">{{ t('subs.menuEdit') }}</button>
                <button v-if="config.type === 'remote' && config.url" class="menu-item" @click.stop="handleShareLink(config)">{{ t('subs.menuShare') }}</button>
                <button class="menu-item" @click.stop="handleExport(config)">{{ t('subs.menuExport') }}</button>
                <button class="menu-item danger" @click.stop="openDeleteModal(config)">{{ t('subs.menuDelete') }}</button>
              </div>
            </Transition>
            </Teleport>
          </div>
        </div>
      </div>
    </TransitionGroup>

    <!-- 统一模态框系统 -->
    <!-- Teleport 到 body：弹窗必须在 .view-transition-wrapper 之外，
         否则会被过渡的 mask-image 一起裁切（position:fixed 也逃不出父级 mask） -->
    <Teleport to="body">
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
    </Teleport>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted, nextTick } from 'vue';
import { App } from '../api';
import * as clash from '../../bindings/goclashz/core/clash';
import { Events } from '@wailsio/runtime';
import { showAlert, openModal, showToast, globalState } from '../store';
import { formatBytes } from '../utils/format';
import { ICONS } from '../utils/icons';
import { displayErrorMessage, toUserError } from '../utils/errors';
import { reportError } from '../utils/report';
import EmptyState from './EmptyState.vue';
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
const configsLoading = ref(true);
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
  // 列表请求返回前保持 loading，空状态只在“确认没有配置”后出现
  configsLoading.value = true;
  try {
    localConfigs.value = await App.GetLocalConfigs() || [];
    const data: any = await App.GetInitialData();
    if (data && data.activeConfig !== undefined) {
      globalState.activeConfigId = data.activeConfig;
      globalState.activeConfigName = data.activeConfigName || '';
      globalState.activeConfigType = data.activeConfigType || '';
    }
  } catch (e) {
    reportError(e, 'subs.syncState');
  } finally {
    configsLoading.value = false;
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
      await App.SelectLocalConfig(config.id);
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
  App.UpdateAllSubsAsync().catch((e: unknown) => reportError(e, 'subs.updateAll'));
};

const handleUpdateSingle = async (config: clash.SubIndexItem) => {
  if (config.type !== 'remote') return;
  activeMenu.value = null;
  isUpdating.value = true;
  try {
    await App.UpdateSingleSub(config.id);
    await fetchConfigs();
    // F-A4-12：成功用 toast（文案较长，给足停留时间并压平换行）
    showToast(t('subs.updateSingleSuccess').replace(/\s+/g, ' '), 'success', { duration: 5000 });
  } catch (e) {
    const u = toUserError(e);
    await showAlert(u.message, t('common.error'), true, u.detail);
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
    const result = await App.SelectLocalFile();
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
      await App.UpdateSub(finalName, pendingImportPath.value);
    } else {
      await App.DoLocalImport(pendingImportPath.value, finalName);
    }
    await fetchConfigs();
    closeAllModals();
    newSubUrl.value = '';
  } catch (e) {
    reportError(e, 'subs.import');
    const u = toUserError(e);
    await showAlert(t('subs.importFailed', { error: displayErrorMessage(e) }), t('common.error'), true, u.detail);
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
    await App.RenameConfig(targetId.value, renameValue.value);
    await fetchConfigs();
    closeAllModals();
  } catch (e) {
    reportError(e, 'subs.rename');
    const u = toUserError(e);
    await showAlert(t('subs.renameFailed', { error: displayErrorMessage(e) }), t('common.error'), true, u.detail);
  } finally {
    isRenaming.value = false;
  }
};

const handleEditFile = async (config: clash.SubIndexItem) => {
  activeMenu.value = null;
  emit('edit-config', config.id, config.name, config.type as 'local' | 'remote');
};

// 订阅链接脱敏：保留协议+主机与尾部 4 位，中间的 token/path 打码（F-A4-19）
const maskUrl = (url: string): string => {
  try {
    const u = new URL(url);
    const tail = url.slice(-4);
    return u.protocol + '//' + u.host + '/***' + tail;
  } catch {
    // 解析失败时也不要露出前段（可能是 token 开头），只保留尾部 4 位便于用户核对
    return url.length > 8 ? '***' + url.slice(-4) : '***';
  }
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
  // 走受管模态：排队 + 保证 settle（旧式直写会顶掉别的确认框）
  void openModal({
    type: 'alert',
    title: t('subs.shareLinkTitle'),
    message: status,
    // 剪贴板里是完整链接（用户要能用），界面上只显示脱敏后的形态
    detail: maskUrl(config.url),
  });
};

const handleExport = (config: clash.SubIndexItem) => {
  activeMenu.value = null;
  // 修复 F-A4-19：以前「取消 / 点遮罩」也会执行 doExport(id, false)，
  // 用户以为取消了却多出一个文件。现在取消 = 不导出。
  void openModal({
    type: 'confirm',
    title: t('subs.menuExport'),
    message: t('subs.exportReplaceRulesPrompt'),
    onConfirm: () => doExport(config.id, true),
  });
};

const doExport = async (id: string, useCustomRules: boolean) => {
  try {
    await App.ExportConfig(id, useCustomRules);
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
    await App.DeleteConfig(targetId.value);
    await fetchConfigs();
    closeAllModals();
  } catch (e) {
    reportError(e, 'subs.delete');
    const u = toUserError(e);
    await showAlert(t('subs.deleteFailed', { error: displayErrorMessage(e) }), t('common.error'), true, u.detail);
  } finally {
    isDeleting.value = false;
  }
};

const menuAnchor = ref<HTMLElement | null>(null);
/* 菜单和下拉一样 teleport 到 body：留在卡片里时 backdrop-filter 只采样卡片
   这一层，背后其它行的内容永远不会被糊到（表现为"只有半透明填充"）。
   离开文档流后定位改 fixed，锚点是触发它的那个 ⋮ 按钮。 */
const menuStyle = ref<Record<string, string>>({});
const menuUp = ref(false);

// 菜单在 v-for 里且同一时刻只可能有一个打开，用当前 id 取元素比维护 ref 更稳
const menuElement = (): HTMLElement | null => {
  const id = activeMenu.value;
  if (!id) return null;
  return document.querySelector<HTMLElement>(`.dropdown-menu[data-menu-id="${id}"]`);
};

const MENU_GAP = 6;
const VIEWPORT_MARGIN = 8;

const updateMenuPosition = (anchor?: HTMLElement | null) => {
  const el = anchor || menuAnchor.value;
  if (!el) return;
  const rect = el.getBoundingClientRect();
  /* 量 scrollHeight 而不是 offsetHeight：菜单有 overflow:hidden，极端情况下
     （窗口很矮、max-height 被截断）offsetHeight 会小于内容真实高度，翻转判断会失效。
     注：本菜单已改用 card-menu 动画，不再受全局 .dropdown-* 的 max-height 0→250 影响。 */
  const menuHeight = menuElement()?.scrollHeight ?? 0;
  const spaceBelow = window.innerHeight - rect.bottom - MENU_GAP - VIEWPORT_MARGIN;
  const spaceAbove = rect.top - MENU_GAP - VIEWPORT_MARGIN;
  // 只有「下方确实放不下、且上方更宽松」时才翻上去，避免在两行卡片之间来回跳
  const openUp = menuHeight > 0 && menuHeight > spaceBelow && spaceAbove > spaceBelow;
  const room = Math.max(48, openUp ? spaceAbove : spaceBelow);
  // 唯一放不下的情况才截断成内部滚动；放得下就交给入场动画的 max-height，别抢它的动画
  const clamped = menuHeight > room;
  menuUp.value = openUp;
  menuStyle.value = {
    position: 'fixed',
    left: 'auto',
    right: Math.max(VIEWPORT_MARGIN, window.innerWidth - rect.right) + 'px',
    top: openUp ? 'auto' : (rect.bottom + MENU_GAP) + 'px',
    bottom: openUp ? (window.innerHeight - rect.top + MENU_GAP) + 'px' : 'auto',
    maxHeight: clamped ? room + 'px' : '',
    overflowY: clamped ? 'auto' : '',
    zIndex: '20',
  };
};

const onMenuViewportChange = () => {
  if (!activeMenu.value) return;
  const anchor = menuAnchor.value;
  if (anchor) {
    const rect = anchor.getBoundingClientRect();
    // 锚点被滚出可视区后菜单再跟着走只会贴着窗口边悬空，直接收起
    if (rect.bottom <= 0 || rect.top >= window.innerHeight) {
      activeMenu.value = null;
      return;
    }
  }
  updateMenuPosition();
};

/* 首帧锚点样式：只依赖按钮 rect，不需要菜单高度。
   必须在 activeMenu 置位之前同步算好 —— 否则菜单会先用 .dropdown-menu 的静态定位
   （top:30px; right:0，即窗口右上角）渲染一帧，再跳到按钮下方；
   配合曾经的 transition:all，这一步就是用户看到的「飞进来」。 */
const anchorMenuStyle = (el: HTMLElement): Record<string, string> => {
  const rect = el.getBoundingClientRect();
  return {
    position: 'fixed',
    left: 'auto',
    right: Math.max(VIEWPORT_MARGIN, window.innerWidth - rect.right) + 'px',
    top: rect.bottom + MENU_GAP + 'px',
    bottom: 'auto',
    zIndex: '20',
  };
};

const toggleMenu = async (id: string, ev?: MouseEvent) => {
  if (activeMenu.value === id) {
    activeMenu.value = null;
    return;
  }
  const anchor = (ev?.currentTarget as HTMLElement) || null;
  menuAnchor.value = anchor;
  menuUp.value = false;
  // 同步给出锚定位置（首帧即正确），挂载后再按真实高度决定是否翻转/截断
  menuStyle.value = anchor ? anchorMenuStyle(anchor) : {};
  activeMenu.value = id;
  await nextTick();
  updateMenuPosition(menuAnchor.value);
};

/* 关闭菜单不再铺全屏透明遮罩：遮罩挂在卡片内部时，指针停在哪里都算停在卡片上，
   那张卡片会一直保持 :hover（表现为"展开菜单的卡片一直带着选中遮罩"）。
   改为捕获阶段监听 document —— 点菜单外部先关菜单并吞掉这次点击，交互等价于
   原来的遮罩，但不再拦住 hover。 */
const onDocumentClick = (e: MouseEvent) => {
  if (!activeMenu.value) return;
  const target = e.target as Node | null;
  if (target && menuElement()?.contains(target)) return;
  activeMenu.value = null;
  e.stopPropagation();
  e.preventDefault();
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
  // fixed 定位不再跟着卡片走，滚动/缩放时手动重算
  window.addEventListener('scroll', onMenuViewportChange, true);
  window.addEventListener('resize', onMenuViewportChange);
  document.addEventListener('click', onDocumentClick, true);

  unsubSubsUpdateStart = Events.On("subs-update-start", () => {
    isUpdating.value = true;
  });

  unsubSubsUpdateSuccess = Events.On("subs-update-success", async () => {
    isUpdating.value = false;
    await fetchConfigs();
    showToast(t('subs.updateAllSuccessMsg').replace(/\s+/g, ' '), 'success', { duration: 5000 });
  });

  unsubSubsUpdateError = Events.On("subs-update-error", async ({ data: err }: { data: string }) => {
    isUpdating.value = false;
    const u = toUserError(err);
    await showAlert(u.message, t('common.error'), true, u.detail);
  });

  unsubConfigChanged = Events.On("config-changed", ({ data: newId }: { data: string }) => {
    if (newId) {
      globalState.activeConfigId = newId;
      fetchConfigs();
    }
  });
});

onUnmounted(() => {
  window.removeEventListener('scroll', onMenuViewportChange, true);
  window.removeEventListener('resize', onMenuViewportChange);
  document.removeEventListener('click', onDocumentClick, true);
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
.main-title { font-size: 1.6rem; font-weight: 600; margin-bottom: 4px; }
.sub-text { font-size: 0.85rem; color: var(--text-sub); }
.truncate { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.header-actions { display: flex; gap: 12px; align-items: center; }
.header-action-btn {
  padding: 0 14px !important;
}

/* 唯一滚动区：列表自己滚动，页头固定 */
.subs-list { flex: 1; min-height: 0; position: relative; display: flex; flex-direction: column; }

/* 卡片基础样式：背景由 ui-surface-card 提供，hover 由 ui-hover-surface 提供 */
.sub-card { 
  position: relative; padding: 20px; 
  margin-bottom: 16px; 
  transition: 0.2s; border: 1px solid transparent; 
}
.sub-card.is-sorting { cursor: default; }

.active-card { 
  background: var(--ui-accent-block) !important; 
  color: var(--ui-accent-fg) !important; 
  border-color: var(--ui-accent) !important;
}
.active-card .sub-path, .active-card .sub-hint, .active-card .icon-btn { color: var(--ui-accent-fg) !important; opacity: 0.8; }
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
  background: var(--ui-accent-soft) !important; 
  color: var(--ui-accent) !important;
  font-weight: 600;
}

.sorting-active .btn-icon {
  color: var(--ui-accent) !important;
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
  background: var(--ui-surface-hover);
  color: var(--ui-text-primary);
  border-color: var(--ui-border-strong);
}
.active-card .arrow-btn { color: var(--ui-accent-fg); opacity: 0.7; }
.active-card .arrow-btn:hover:not(:disabled) {
  background: color-mix(in srgb, var(--ui-accent-fg) 18%, transparent);
  color: var(--ui-accent-fg);
  opacity: 1;
  border-color: var(--ui-accent-fg);
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
.icon-btn:hover { background: var(--ui-surface-hover); color: var(--ui-text-primary); }
.active-card .icon-btn:hover { background: transparent !important; }

.mini-btn { height: 44px; padding: 0 20px; font-weight: 700; border-radius: 12px; }
.divider-text { text-align: center; margin: 15px 0; color: var(--text-muted); font-size: 0.75rem; position: relative; }
.divider-text::before, .divider-text::after { content: ""; position: absolute; top: 50%; width: 35%; height: 1px; background: var(--ui-border-subtle); }
.divider-text::before { left: 0; } .divider-text::after { right: 0; }
.w-full-btn { width: 100%; justify-content: center; padding: 14px; font-weight: 600; border-radius: 10px; border: none; background: var(--surface-hover); color: var(--text-main); cursor: pointer; display: flex; align-items: center; gap: 8px; }
.modal-hint-text { padding: 4px 0 12px 0; color: var(--text-sub); font-size: 0.85rem; line-height: 1.6; }
.danger-text { color: var(--danger) !important; }

/* --- 菜单 --- */
.dropdown-menu { 
  position: absolute; right: 0; top: 30px; width: 150px; z-index: 10; overflow: hidden;
}
.menu-item { width: 100%; padding: 10px 16px; border: none; background: transparent; text-align: left; cursor: pointer; font-size: 0.85rem; color: var(--ui-text-primary); }
.menu-item:hover { background: var(--ui-surface-hover); }
.menu-item.danger { color: var(--danger) !important; font-weight: 600; }
.subs-loading { padding: 30px; text-align: center; color: var(--ui-text-muted); font-size: 0.85rem; }

/* --- 卡片菜单动画 ---
   刻意不复用全局 .dropdown-* 那套（style.css 里是给设置页展开面板用的：
   transition: all .5s + max-height 0→250 + translateY(-8px)）。
   它有两个副作用会让人看着「像飞进来」：
     1) transition: all 会把 top/right 一起动画 —— 而菜单定位是在挂载后下一帧
        才量出来的，于是菜单从页面右上角「飞」到按钮下方；
     2) max-height 0→250 对弹层毫无意义，只会让高度跟着抽搐。
   这里只动 opacity/transform，且把 transform-origin 钉在按钮那一侧，
   观感是「从按钮旁长出来」，不是从别处飞过来。 */
.card-menu-enter-active, .card-menu-leave-active {
  transition: opacity 0.16s ease-out, transform 0.16s cubic-bezier(0.22, 1, 0.36, 1);
  transform-origin: top right;
}
.card-menu-enter-from, .card-menu-leave-to {
  opacity: 0;
  transform: translateY(-4px) scale(0.96);
}
.card-menu-enter-to, .card-menu-leave-from {
  opacity: 1;
  transform: translateY(0) scale(1);
}
/* 向上展开时从底边生长，方向与弹出方向一致 */
.dropdown-menu.is-up.card-menu-enter-active,
.dropdown-menu.is-up.card-menu-leave-active {
  transform-origin: bottom right;
}
.dropdown-menu.is-up.card-menu-enter-from,
.dropdown-menu.is-up.card-menu-leave-to {
  transform: translateY(4px) scale(0.96);
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
  background: var(--ui-accent-block); 
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
