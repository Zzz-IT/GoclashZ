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
          class="action-btn accent-btn" 
          :class="{ 'sorting-active': isSortingMode }" 
          @click.stop="toggleSortMode"
        >
          <span class="btn-icon" v-html="ICONS.sort"></span> 
          {{ isSortingMode ? '完成' : '排序' }}
        </button>
        <button class="action-btn accent-btn" @click.stop="activeModal = 'import'">
          <span class="btn-icon" v-html="ICONS.plus"></span> 导入配置
        </button>
        <button class="primary-btn accent-btn" @click="handleUpdateAll" :disabled="loading">
          <span class="btn-icon" v-html="ICONS.refresh" :class="{ 'spin': loading }"></span>
          {{ loading ? '更新中...' : '更新全部' }}
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
        :key="config"
        class="sub-card clickable"
        :class="{ 'active-card': isCurrentConfig(config), 'is-sorting': isSortingMode }"
        @click="!isSortingMode && handleSelectConfig(config)"
      >
        <div class="sub-header">
          <div class="sub-info">
            
            <div v-if="isSortingMode" class="sort-controls">
              <button class="arrow-btn" :disabled="index === 0" @click.stop="moveUp(index)" v-html="ICONS.up"></button>
              <button class="arrow-btn" :disabled="index === localConfigs.length - 1" @click.stop="moveDown(index)" v-html="ICONS.down"></button>
            </div>

            <div>
              <h4 class="sub-name">{{ config }}</h4>
              <span class="sub-path font-mono">core/bin/{{ config }}</span>
            </div>
          </div>
          
          <div class="sub-status">
            <div v-if="isCurrentConfig(config)" class="status-badge-active">
              <div class="breathe-dot"></div>
              <span>正在使用</span>
            </div>
          </div>
        </div>

        <div class="sub-footer" v-show="!isSortingMode">
          <span class="sub-hint">点击应用此配置</span>
          <div class="sub-actions">
            <button class="icon-btn" @click.stop="toggleMenu(config)" v-html="ICONS.more"></button>
            <div v-if="activeMenu === config" class="dropdown-menu glass-panel">
              <button v-if="hasUrlRecord(config)" class="menu-item" @click.stop="handleUpdateSingle(config)">更新订阅</button>
              <div v-if="hasUrlRecord(config)" class="menu-divider"></div>
              <button class="menu-item" @click.stop="openRenameModal(config)">重命名</button>
              <button class="menu-item" @click.stop="handleEditFile(config)">记事本编辑</button>
              <button class="menu-item danger" @click.stop="openDeleteModal(config)">彻底删除</button>
            </div>
          </div>
        </div>
      </div>
    </TransitionGroup>

    <!-- 统一模态框系统 -->
    <Transition name="pop">
      <div v-if="activeModal" class="modal-overlay" @click="closeAllModals">
        <!-- 导入弹窗 -->
        <div v-if="activeModal === 'import'" class="custom-modal-card glass-panel" @click.stop>
          <div class="modal-header">
            <h3>导入配置文件</h3>
            <button class="close-x" @click="closeAllModals">&times;</button>
          </div>
          <div class="modal-body">
            <div class="section">
              <label>订阅链接</label>
              <div class="input-row">
                <input v-model="url" placeholder="https://..." class="modern-input" />
                <button class="primary-btn mini" @click="handleDownload" :disabled="!url || loading">下载</button>
              </div>
            </div>
            <div class="divider-text">或者</div>
            <div class="section">
              <label>本地导入</label>
              <button class="action-btn w-full-btn" @click="handleImportLocal">
                <span class="btn-icon" v-html="ICONS.folder"></span> 浏览本地 YAML 文件
              </button>
            </div>
          </div>
        </div>

        <!-- 重命名弹窗 -->
        <div v-if="activeModal === 'rename'" class="custom-modal-card glass-panel" @click.stop>
          <div class="modal-header">
            <h3>重命名配置文件</h3>
          </div>
          <div class="modal-body">
            <div class="section">
              <label>新名称</label>
              <input v-model="renameValue" class="modern-input full-width-input" placeholder="输入新文件名" @keyup.enter="confirmRename" />
            </div>
            <div class="modal-footer">
              <button class="action-btn flex-1" @click="closeAllModals">取消</button>
              <button class="primary-btn accent-btn flex-1" @click="confirmRename" :disabled="!renameValue || loading">确定</button>
            </div>
          </div>
        </div>

        <!-- 删除确认弹窗 -->
        <div v-if="activeModal === 'delete'" class="custom-modal-card glass-panel" @click.stop>
          <div class="modal-header">
            <h3 class="danger-text">彻底删除</h3>
          </div>
          <div class="modal-body">
            <p v-if="isCurrentConfig(targetFile)" class="warning-box">
              <strong>警告：</strong> "{{ targetFile }}" 正在运行中。删除将强制停止所有代理服务。
            </p>
            <p v-else>确定要彻底删除 <strong>{{ targetFile }}</strong> 吗？此操作不可撤销。</p>
            <div class="modal-footer">
              <button class="action-btn flex-1" @click="closeAllModals">取消</button>
              <button class="primary-btn accent-btn red-text-btn flex-1" @click="confirmDelete" :disabled="loading">
                {{ loading ? '删除中...' : '确定删除' }}
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

const ICONS = {
  refresh: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="23 4 23 10 17 10"></polyline><path d="M20.49 15a9 9 0 1 1-2.12-9.36L23 10"></path></svg>`,
  more: `<svg viewBox="0 0 24 24" fill="currentColor"><circle cx="12" cy="12" r="2"></circle><circle cx="12" cy="5" r="2"></circle><circle cx="12" cy="19" r="2"></circle></svg>`,
  plus: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><line x1="12" y1="5" x2="12" y2="19"></line><line x1="5" y1="12" x2="19" y2="12"></line></svg>`,
  folder: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M22 19a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h5l2 3h9a2 2 0 0 1 2 2z"></path></svg>`,
  sort: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M3 6h18M7 12h10M10 18h4"/></svg>`,
  up: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"><polyline points="18 15 12 9 6 15"></polyline></svg>`,
  down: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"><polyline points="6 9 12 15 18 9"></polyline></svg>`
};

const activeModal = ref<'import' | 'rename' | 'delete' | null>(null);
const targetFile = ref('');
const renameValue = ref('');
const url = ref('');
const loading = ref(false);
const selecting = ref<string | null>(null);
const currentPath = ref('');
const localConfigs = ref<string[]>([]);
const activeMenu = ref<string | null>(null);
const subRecords = ref<Record<string, any>>({});

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
  await saveOrder();
};

const moveDown = async (index: number) => {
  if (index >= localConfigs.value.length - 1) return;
  const items = [...localConfigs.value];
  [items[index + 1], items[index]] = [items[index], items[index + 1]];
  localConfigs.value = items;
  await saveOrder();
};

const saveOrder = async () => {
  try {
    await (API as any).SaveConfigsOrder(localConfigs.value);
  } catch (e) {
    console.error("保存排序失败:", e);
  }
};

const isCurrentConfig = (name: string) => {
  if (!currentPath.value) return false;
  const currentFile = currentPath.value.split(/[\\/]/).pop();
  return currentFile === name;
};

const hasUrlRecord = (name: string) => !!subRecords.value[name];

const fetchConfigs = async () => {
  try {
    const list = await (API as any).GetLocalConfigs();
    localConfigs.value = (list || []).filter((name: string) => !name.endsWith('config.yaml'));
    const data: any = await API.GetInitialData();
    if (data && data.activeConfig) {
      currentPath.value = data.activeConfig;
    }
    subRecords.value = await (API as any).GetSubRecords() || {};
  } catch (e) {
    console.error("同步状态失败:", e);
  }
};

const handleSelectConfig = async (name: string) => {
  if (isCurrentConfig(name) || selecting.value) return;
  selecting.value = name;
  try {
    await API.SelectLocalConfig(name);
    currentPath.value = name;
  } catch (error) {
    console.error("切换失败:", error);
  } finally {
    selecting.value = null;
  }
};

const handleUpdateAll = async () => {
  loading.value = true;
  try {
    await (API as any).UpdateAllSubs();
    await fetchConfigs();
  } catch (e) {
    console.error("批量更新失败:", e);
  } finally {
    loading.value = false;
  }
};

const handleUpdateSingle = async (filename: string) => {
  activeMenu.value = null;
  loading.value = true;
  try {
    await (API as any).UpdateSingleSub(filename);
    await fetchConfigs();
  } catch (e) {
    console.error("更新订阅失败:", e);
  } finally {
    loading.value = false;
  }
};

const handleDownload = async () => {
  loading.value = true;
  try {
    await API.UpdateSub(url.value);
    await fetchConfigs();
    closeAllModals();
    url.value = '';
  } catch (e) {
    console.error("下载失败:", e);
  } finally {
    loading.value = false;
  }
};

const handleImportLocal = async () => {
  closeAllModals();
  try {
    await API.ImportLocalConfig();
    await fetchConfigs();
  } catch (e) {
    console.log("Import cancelled");
  }
};

const openRenameModal = (filename: string) => {
  activeMenu.value = null;
  targetFile.value = filename;
  renameValue.value = filename;
  activeModal.value = 'rename';
};

const confirmRename = async () => {
  if (!renameValue.value || renameValue.value === targetFile.value) {
    closeAllModals();
    return;
  }
  loading.value = true;
  try {
    await API.RenameConfig(targetFile.value, renameValue.value);
    await fetchConfigs();
    closeAllModals();
  } catch (e) {
    console.error("重命名失败:", e);
  } finally {
    loading.value = false;
  }
};

const handleEditFile = async (filename: string) => {
  activeMenu.value = null;
  await API.OpenConfigFile(filename);
};

const openDeleteModal = (filename: string) => {
  activeMenu.value = null;
  targetFile.value = filename;
  activeModal.value = 'delete';
};

const confirmDelete = async () => {
  loading.value = true;
  try {
    if (isCurrentConfig(targetFile.value)) {
      await API.StopProxy();
      currentPath.value = '';
    }
    await API.DeleteConfig(targetFile.value);
    await fetchConfigs();
    if (localConfigs.value.length === 0) {
      await (API as any).ClearBaseConfig();
    }
    closeAllModals();
  } catch (e) {
    console.error("删除失败:", e);
  } finally {
    loading.value = false;
  }
};

const toggleMenu = (name: string) => {
  activeMenu.value = activeMenu.value === name ? null : name;
};

const closeAllModals = () => {
  activeModal.value = null;
};

onMounted(() => {
  fetchConfigs();
  (window as any).runtime.EventsOn("config-changed", (newName: string) => {
    if (newName && newName !== 'config.yaml') {
      currentPath.value = newName;
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
.header-actions { display: flex; gap: 12px; }
.subs-list { flex: 1; overflow-y: auto; padding-right: 8px; position: relative; }

/* 卡片基础样式 */
.sub-card { 
  position: relative; padding: 20px; border-radius: 12px; 
  background: var(--surface); margin-bottom: 16px; 
  transition: 0.2s; border: 1px solid transparent; 
}
.sub-card:not(.is-sorting):hover { background: var(--surface-hover); }
.sub-card.is-sorting { cursor: default; }

.active-card { background: var(--accent) !important; color: var(--accent-fg) !important; border: none !important; }
.active-card .sub-path, .active-card .sub-hint, .active-card .icon-btn { color: var(--accent-fg) !important; opacity: 0.8; }
.sub-header { display: flex; justify-content: space-between; margin-bottom: 12px; }
.sub-info { display: flex; align-items: center; gap: 16px; }
.sub-name { font-size: 0.95rem; font-weight: 600; margin: 0; }
.sub-path { font-size: 0.7rem; color: var(--text-muted); }

/* ================================== */
/* 列表过渡动画 */
/* ================================== */
.list-move,
.list-enter-active,
.list-leave-active {
  transition: all 0.4s cubic-bezier(0.25, 1, 0.5, 1);
}
.list-enter-from,
.list-leave-to {
  opacity: 0;
  transform: translateY(15px) scale(0.98);
}
.list-leave-active {
  position: absolute;
  right: 8px;
  left: 0;
}

/* ================================== */
/* 排序控制区 */
/* ================================== */
.sorting-active {
  background: var(--text-main) !important;
  color: var(--surface) !important;
  border-color: var(--text-main) !important;
  font-weight: 600;
}
.sorting-active .btn-icon { color: var(--surface) !important; }

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

.modal-header { display: flex; justify-content: space-between; align-items: center; margin-bottom: 24px; }
.modal-header h3 { margin: 0; font-size: 1.25rem; font-weight: 600; }
.close-x { background: none; border: none; font-size: 28px; cursor: pointer; color: var(--text-sub); }
.modal-body { display: flex; flex-direction: column; gap: 20px; }
.section label { display: block; font-size: 0.85rem; color: var(--text-sub); margin-bottom: 8px; font-weight: 600; }
.input-row { display: flex; gap: 8px; background: var(--surface-hover); padding: 4px 4px 4px 12px; border-radius: 10px; align-items: center; }
.modern-input { flex: 1; background: transparent; border: none; color: inherit; outline: none; padding: 8px 0; font-size: 0.9rem; }
.full-width-input { width: 100%; padding: 12px !important; background: var(--surface-hover) !important; border-radius: 10px; }
.mini { padding: 8px 16px; font-size: 0.85rem; }
.divider-text { text-align: center; margin: 10px 0; color: var(--text-muted); font-size: 0.75rem; position: relative; }
.divider-text::before, .divider-text::after { content: ""; position: absolute; top: 50%; width: 40%; height: 1px; background: var(--surface-hover); }
.divider-text::before { left: 0; } .divider-text::after { right: 0; }
.w-full-btn { width: 100%; justify-content: center; padding: 14px; font-weight: 600; border-radius: 10px; border: none; background: var(--surface-hover); color: var(--text-main); cursor: pointer; }
.danger-text { color: #ff4d4f; }
.warning-box { background: rgba(255, 77, 79, 0.1); padding: 12px; border-radius: 8px; color: #ff4d4f; font-size: 0.85rem; line-height: 1.4; border: 1px solid rgba(255, 77, 79, 0.2); }
.dropdown-menu { 
  position: absolute; right: 0; top: 30px; width: 150px; border-radius: 8px; z-index: 10; overflow: hidden;
  background: var(--surface); border: 1px solid var(--surface-hover); box-shadow: 0 4px 12px rgba(0,0,0,0.1);
}
.menu-item { width: 100%; padding: 10px 16px; border: none; background: transparent; text-align: left; cursor: pointer; font-size: 0.85rem; color: var(--text-main); }
.menu-item:hover { background: var(--surface-hover); }
.menu-item.danger { color: #ff4d4f !important; font-weight: 600; }
.menu-divider { height: 1px; margin: 4px 0; background: var(--surface-hover); }
.empty-state { padding: 30px; text-align: center; color: var(--text-muted); border: none; border-radius: 10px; background: var(--surface); }
</style>