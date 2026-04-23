<template>
  <div class="subs-view" @click="activeMenu = null">
    <div class="page-header">
      <div class="header-text">
        <h2 class="main-title">本地配置库</h2>
        <span class="sub-text">
          {{ isSortingMode ? '排序模式：点击卡片左侧箭头上下移动' : '点击卡片应用该配置' }}
        </span>
      </div>
      
      <div class="header-actions">
        <button 
          class="action-btn" 
          :class="isSortingMode ? 'sorting-active' : 'accent-btn'" 
          @click.stop="toggleSortMode"
        >
          <span class="btn-icon" v-html="ICONS.sort"></span> 
          {{ isSortingMode ? '完成' : '排序' }}
        </button>
        <button class="action-btn accent-btn" @click.stop="activeModal = 'import'">
          <span class="btn-icon" v-html="ICONS.plus"></span> 导入配置
        </button>
        <button class="primary-btn accent-btn" @click="handleUpdateAll" :disabled="isUpdating">
          <span class="btn-icon" v-html="ICONS.refresh" :class="{ 'spin': isUpdating }"></span>
          {{ isUpdating ? '更新中...' : '更新全部' }}
        </button>
      </div>
    </div>

    <!-- 列表过渡动画 -->
    <TransitionGroup name="list" tag="div" class="subs-list">
      <div v-if="localConfigs.length === 0" class="empty-state" key="empty">
        暂无本地配置文件。
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
              <span>正在使用</span>
            </div>
          </div>
        </div>

        <div v-if="!isSortingMode && config.total > 0" class="traffic-container">
          <div class="traffic-bar">
            <div class="traffic-fill" :style="{ width: Math.min(100, ((config.upload + config.download) / config.total) * 100) + '%' }"></div>
          </div>
          <div class="traffic-text">
            <span>已用 {{ formatBytes(config.upload + config.download) }}</span>
            <span>总计 {{ formatBytes(config.total) }}</span>
          </div>
          <div v-if="config.expire > 0" class="expire-text">
            到期时间: {{ formatDate(config.expire) }}
          </div>
        </div>

        <div class="sub-footer" v-show="!isSortingMode">
          <span class="sub-hint">点击应用此配置</span>
          <div class="sub-actions">
            <button class="icon-btn" @click.stop="toggleMenu(config.id)" v-html="ICONS.more"></button>
            <div v-if="activeMenu === config.id" class="menu-click-overlay" @click.stop="activeMenu = null"></div>
            <Transition name="dropdown">
              <div v-if="activeMenu === config.id" class="dropdown-menu card-panel">
                <button v-if="config.type === 'remote'" class="menu-item" @click.stop="handleUpdateSingle(config)">更新订阅</button>
                <div v-if="config.type === 'remote'" class="menu-divider"></div>
                <button class="menu-item" @click.stop="openRenameModal(config)">重命名</button>
                <button class="menu-item" @click.stop="handleEditFile(config.id)">记事本编辑</button>
                <button class="menu-item danger" @click.stop="openDeleteModal(config)">彻底删除</button>
              </div>
            </Transition>
          </div>
        </div>
      </div>
    </TransitionGroup>

    <!-- 统一模态框系统 (回归上上次样式的简洁结构) -->
    <Transition name="pop">
      <div v-if="activeModal" class="modal-overlay" @click="closeAllModals">
        <!-- 导入主入口弹窗 -->
        <div v-if="activeModal === 'import'" class="custom-modal-card" @click.stop>
          <div class="modal-header">
            <h3>导入配置文件</h3>
          </div>
          <div class="modal-body">
            <p class="global-modal-msg">通过订阅链接导入：</p>
            <div style="display: flex; gap: 8px;">
              <input v-model="newSubUrl" placeholder="https://..." class="modal-input" style="flex: 1;" :disabled="isImporting" />
              <button class="primary-btn mini-btn" style="height: 44px;" @click="handleDownload" :disabled="!newSubUrl.trim() || isImporting">导入</button>
            </div>
            <div class="divider-text">或者</div>
            <button class="action-btn w-full-btn" @click="handlePickFile" :disabled="isImporting">
              <span class="btn-icon" v-html="ICONS.folder"></span> 浏览本地 YAML 文件
            </button>
          </div>
        </div>

        <!-- 导入确认/命名弹窗 -->
        <div v-if="activeModal === 'import_confirm'" class="custom-modal-card" @click.stop>
          <div class="modal-header">
            <h3>{{ isImportingRemote ? '导入链接订阅' : '导入本地配置' }}</h3>
          </div>
          <div class="modal-body">
            <p class="text-xs text-gray-500 mb-2 truncate" style="margin-bottom: 8px; opacity: 0.6; width: 100%; display: block;">
              {{ isImportingRemote ? '链接:' : '文件:' }} {{ pendingImportPath }}
            </p>
            <p class="global-modal-msg">设置配置显示名称：</p>
            <input 
              v-model="configNameInput" 
              placeholder="请输入名称" 
              class="modal-input"
              @keyup.enter="confirmImport"
              :disabled="isImporting"
            />
            <div class="modal-footer">
              <button class="action-btn flex-1" @click="activeModal = 'import'" :disabled="isImporting">返回</button>
              <button class="primary-btn accent-btn flex-1" @click="confirmImport" :disabled="!configNameInput || isImporting">
                <span v-if="isImporting" class="btn-icon spin" v-html="ICONS.refresh"></span>
                {{ isImporting ? (isImportingRemote ? '下载中...' : '导入中...') : '确定' }}
              </button>
            </div>
          </div>
        </div>

        <!-- 重命名弹窗 -->
        <div v-if="activeModal === 'rename'" class="custom-modal-card" @click.stop>
          <div class="modal-header">
            <h3>重命名配置文件</h3>
          </div>
          <div class="modal-body">
            <p class="global-modal-msg">请输入新的配置显示名称：</p>
            <input v-model="renameValue" class="modal-input" placeholder="例如: 我的订阅" @keyup.enter="confirmRename" :disabled="isRenaming" />
            <div class="modal-footer">
              <button class="action-btn flex-1" @click="closeAllModals" :disabled="isRenaming">取消</button>
              <button class="primary-btn accent-btn flex-1" @click="confirmRename" :disabled="!renameValue || isRenaming">确定</button>
            </div>
          </div>
        </div>

        <!-- 删除确认弹窗 -->
        <div v-if="activeModal === 'delete'" class="custom-modal-card" @click.stop>
          <div class="modal-header">
            <h3 class="danger-text">彻底删除</h3>
          </div>
          <div class="modal-body">
            <p v-if="isCurrentConfig(targetId)" class="modal-hint-text">
              <strong>警告：</strong> "{{ targetName }}" 正在运行中。删除将强制停止所有代理服务。
            </p>
            <p v-else class="modal-hint-text">确定要彻底删除 <strong>{{ targetName }}</strong> 吗？此操作不可撤销。</p>
            <div class="modal-footer">
              <button class="action-btn flex-1" @click="closeAllModals">取消</button>
              <button class="primary-btn accent-btn red-text-btn flex-1" @click="confirmDelete" :disabled="isDeleting">
                {{ isDeleting ? '删除中...' : '确定删除' }}
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
import { ICONS } from '../utils/icons';
import { showAlert, globalState } from '../store';
import { clash } from '../../wailsjs/go/models';

const activeModal = ref<'import' | 'import_confirm' | 'rename' | 'delete' | null>(null);
const targetId = ref('');
const targetName = ref('');
const renameValue = ref('');
const isUpdating = ref(false);
const isImporting = ref(false);
const isRenaming = ref(false);
const isDeleting = ref(false);
const selecting = ref<string | null>(null);

const localConfigs = ref<clash.SubIndexItem[]>([]);
const activeMenu = ref<string | null>(null);

// --- 导入相关状态 ---
const newSubUrl = ref('');
const pendingImportPath = ref('');
const configNameInput = ref('');
const isImportingRemote = ref(false);

const formatBytes = (bytes: number) => {
  if (!bytes || bytes === 0) return '0 B';
  const k = 1024;
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
};

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

const handleSelectConfig = async (config: clash.SubIndexItem) => {
  if (isCurrentConfig(config.id) || selecting.value) return;
  selecting.value = config.id;
  try {
    await API.StartClash(config.id);
    globalState.activeConfigId = config.id;
    globalState.activeConfigName = config.name;
    globalState.activeConfigType = config.type;
  } catch (error) {
    console.error("切换失败:", error);
  } finally {
    selecting.value = null;
  }
};

const handleUpdateAll = async () => {
  const remoteItems = localConfigs.value.filter(c => c.type === 'remote');
  if (remoteItems.length === 0) {
    await showAlert("不存在链接订阅", "提示");
    return;
  }

  isUpdating.value = true;
  try {
    await API.UpdateAllSubs();
    await fetchConfigs();
    await showAlert("全部订阅更新完成！\n\n自定义规则已保留。如需应用机场的最新规则，请前往「规则管理」页面手动同步。", "更新成功");
  } catch (e) {
    await showAlert(`更新失败: ${e}`, "更新结果");
  } finally {
    isUpdating.value = false;
  }
};

const handleUpdateSingle = async (config: clash.SubIndexItem) => {
  if (config.type !== 'remote') return;
  activeMenu.value = null;
  isUpdating.value = true;
  try {
    await API.UpdateSingleSub(config.id);
    await fetchConfigs();
    await showAlert("节点更新成功！\n\n自定义规则已保留。如需应用机场的最新规则，请前往「规则管理」页面手动同步。", "更新完毕");
  } catch (e) {
    await showAlert(`更新失败: ${e}`, "发生错误");
  } finally {
    isUpdating.value = false;
  }
};

const handleDownload = () => {
  if (!newSubUrl.value.trim()) return;
  pendingImportPath.value = newSubUrl.value.trim();
  configNameInput.value = "新订阅";
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
    const finalName = configNameInput.value.trim() || (isImportingRemote.value ? "未命名订阅" : "未命名文件");
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
    await showAlert("导入失败: " + e, "错误");
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
    await showAlert("重命名失败: " + e, "错误");
  } finally {
    isRenaming.value = false;
  }
};

const handleEditFile = async (id: string) => {
  activeMenu.value = null;
  await API.OpenConfigFile(id);
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
    if (isCurrentConfig(targetId.value)) {
      await API.StopProxy();
      globalState.activeConfigId = '';
      globalState.activeConfigName = '';
      globalState.activeConfigType = '';
    }
    await API.DeleteConfig(targetId.value);
    await fetchConfigs();
    closeAllModals();
  } catch (e) {
    console.error("删除失败:", e);
    await showAlert("删除失败: " + e, "错误");
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

onMounted(() => {
  fetchConfigs();
  (window as any).runtime.EventsOn("config-changed", (newId: string) => {
    if (newId) {
      globalState.activeConfigId = newId;
      fetchConfigs();
    }
  });
});

onUnmounted(() => {
  (window as any).runtime.EventsOff("config-changed");
});
</script>

<style scoped>
.subs-view { display: flex; flex-direction: column; height: 100%; color: var(--text-main); }
.page-header { display: flex; justify-content: space-between; align-items: flex-end; margin-bottom: 32px; }
.main-title { font-size: 1.6rem; font-weight: 600; margin-bottom: 4px; }
.sub-text { font-size: 0.85rem; color: var(--text-sub); }
.truncate { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.header-actions { display: flex; gap: 12px; align-items: center; }

.subs-list { flex: 1; overflow-y: auto; padding-right: 0; position: relative; }

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

.header-actions .action-btn, 
.header-actions .primary-btn {
  height: 40px;
  border: none !important;
  box-shadow: none;
  display: flex;
  align-items: center;
  justify-content: center;
  transition: background-color 0.2s ease, color 0.2s ease;
}

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
  transform: scale(0.98);
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
.menu-divider { height: 1px; margin: 4px 0; background: var(--surface-hover); }
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

.btn-icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  margin-right: 4px;
}

.btn-icon :deep(svg) {
  width: 14px;
  height: 14px;
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