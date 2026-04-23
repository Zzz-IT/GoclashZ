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

            <div style="flex: 1; min-width: 0;">
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

        <div v-if="!isSortingMode && subRecords[config] && subRecords[config].total > 0" class="traffic-container">
          <div class="traffic-bar">
            <div class="traffic-fill" :style="{ width: Math.min(100, ((subRecords[config].upload + subRecords[config].download) / subRecords[config].total) * 100) + '%' }"></div>
          </div>
          <div class="traffic-text">
            <span>已用 {{ formatBytes(subRecords[config].upload + subRecords[config].download) }}</span>
            <span>总计 {{ formatBytes(subRecords[config].total) }}</span>
          </div>
          <div v-if="subRecords[config].expire > 0" class="expire-text">
            到期时间: {{ formatDate(subRecords[config].expire) }}
          </div>
        </div>

        <div class="sub-footer" v-show="!isSortingMode">
          <span class="sub-hint">点击应用此配置</span>
          <div class="sub-actions">
            <button class="icon-btn" @click.stop="toggleMenu(config)" v-html="ICONS.more"></button>
            
            <!-- 👇 新增：全屏透明遮罩，点击页面任何地方均可关闭菜单 -->
            <div v-if="activeMenu === config" class="menu-click-overlay" @click.stop="activeMenu = null"></div>

            <Transition name="dropdown">
              <div v-if="activeMenu === config" class="dropdown-menu card-panel">
                <button v-if="hasUrlRecord(config)" class="menu-item" @click.stop="handleUpdateSingle(config)">更新订阅</button>
                <div v-if="hasUrlRecord(config)" class="menu-divider"></div>
                <button class="menu-item" @click.stop="openRenameModal(config)">重命名</button>
                <button class="menu-item" @click.stop="handleEditFile(config)">记事本编辑</button>
                <button class="menu-item danger" @click.stop="openDeleteModal(config)">彻底删除</button>
              </div>
            </Transition>
          </div>
        </div>
      </div>
    </TransitionGroup>

    <!-- 统一模态框系统 -->
    <Transition name="pop">
      <div v-if="activeModal" class="modal-overlay" @click="closeAllModals">
        <!-- 导入弹窗 -->
        <div v-if="activeModal === 'import'" class="custom-modal-card" @click.stop>
          <div class="modal-header">
            <h3>导入配置文件</h3>
          </div>
          <div class="modal-body">
            <p class="global-modal-msg">通过订阅链接下载：</p>
            <div style="display: flex; gap: 8px;">
              <input v-model="url" placeholder="https://..." class="modal-input" style="flex: 1;" />
              <button class="primary-btn mini" style="height: 44px;" @click="handleDownload" :disabled="!url || loading">下载</button>
            </div>
            <div class="divider-text">或者</div>
            <button class="action-btn w-full-btn" @click="handleImportLocal">
              <span class="btn-icon" v-html="ICONS.folder"></span> 浏览本地 YAML 文件
            </button>
          </div>
        </div>

        <!-- 重命名弹窗 -->
        <div v-if="activeModal === 'rename'" class="custom-modal-card" @click.stop>
          <div class="modal-header">
            <h3>重命名配置文件</h3>
          </div>
          <div class="modal-body">
            <p class="global-modal-msg">请输入新的文件名称：</p>
            <input v-model="renameValue" class="modal-input" placeholder="例如: MyConfig.yaml" @keyup.enter="confirmRename" />
            <div class="modal-footer">
              <button class="action-btn flex-1" @click="closeAllModals">取消</button>
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
            <div v-if="isCurrentConfig(targetFile)" class="warning-box">
              <strong>警告：</strong> "{{ targetFile }}" 正在运行中。删除将强制停止所有代理服务。
            </div>
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
import { ICONS } from '../utils/icons';
import { showAlert } from '../store';

const activeModal = ref<'import' | 'rename' | 'delete' | null>(null);
const targetFile = ref('');
const renameValue = ref('');
const url = ref('');
const loading = ref(false);
const isRenaming = ref(false);
const selecting = ref<string | null>(null);
const currentPath = ref('');
const localConfigs = ref<string[]>([]);
const activeMenu = ref<string | null>(null);
const subRecords = ref<Record<string, any>>({});

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
    
    // 🎯 核心修复：原本写的是 if (data && data.activeConfig)。
    // 当所有配置被清空，activeConfig 变成空字符串 "" 时，在 JS 里被视作 false，导致状态无法刷新！
    // 必须使用 !== undefined 来确保空字符串也能触发状态更新。
    if (data && data.activeConfig !== undefined) {
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
  // 筛选出所有拥有订阅链接的配置名称
  const targets = localConfigs.value.filter(name => hasUrlRecord(name));

  // 逻辑 1：若不存在链接订阅，则显示提示并不进行尝试
  if (targets.length === 0) {
    await showAlert("不存在链接订阅", "提示");
    return;
  }

  loading.value = true;
  const failedConfigs: string[] = [];

  // 逻辑 2：遍历更新
  for (const name of targets) {
    try {
      // 调用单体更新接口
      await (API as any).UpdateSingleSub(name);
    } catch (e) {
      // 记录失败的配置名
      failedConfigs.push(name);
    }
  }

  // 更新完成后刷新列表状态
  await fetchConfigs();
  loading.value = false;

  // 逻辑 3：根据更新结果弹出对应提示
  if (failedConfigs.length === 0) {
    // 全部成功
    await showAlert("全部更新完成", "更新成功");
  } else {
    // 存在失败项，显示具体失败的名称
    await showAlert(`${failedConfigs.join(' ')} 更新失败`, "更新结果");
  }
};

const handleUpdateSingle = async (filename: string) => {
  activeMenu.value = null;
  loading.value = true;
  try {
    await (API as any).UpdateSingleSub(filename);
    await fetchConfigs();
    // 👇 新增：成功提示
    await showAlert(`"${filename}" 更新成功`, "更新完毕");
  } catch (e) {
    console.error("更新订阅失败:", e);
    // 👇 新增：失败提示
    await showAlert(`"${filename}" 更新失败: ${e}`, "发生错误");
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
  let finalName = renameValue.value;
  if (!finalName.endsWith('.yaml') && !finalName.endsWith('.yml')) {
    finalName += '.yaml';
  }
  if (!renameValue.value || finalName === targetFile.value) {
    closeAllModals();
    return;
  }
  
  isRenaming.value = true; // 👈 修复：使用专属状态
  try {
    await API.RenameConfig(targetFile.value, finalName);
    await fetchConfigs();
    closeAllModals();
  } catch (e) {
    console.error("重命名失败:", e);
  } finally {
    isRenaming.value = false; // 👈 修复：重置专属状态
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
  border-color: var(--accent) !important; /* 👈 让边框融入背景，不改变物理尺寸 */
}
.active-card .sub-path, .active-card .sub-hint, .active-card .icon-btn { color: var(--accent-fg) !important; opacity: 0.8; }
.sub-header { display: flex; justify-content: space-between; margin-bottom: 12px; }
.sub-info { display: flex; align-items: center; gap: 16px; }
.sub-name { font-size: 0.95rem; font-weight: 600; margin: 0; }
.sub-path { font-size: 0.7rem; color: var(--text-muted); }

/* ================================== */
/* 列表过渡动画（彻底移除入场渐显，仅保留排序位移） */
/* ================================== */
.list-move {
  transition: transform 0.3s cubic-bezier(0.25, 1, 0.5, 1);
}

/* 找到你定义 .list-leave-active 的地方，添加 absolute 定位 */
.list-leave-active {
  position: absolute; /* 👈 核心修复：让离场元素脱离文档流 */
  width: calc(100% - 40px); /* 防止绝对定位后宽度坍缩，数值根据你父容器的 padding 适当调整 */
}

/* 确保你的入场离场动画基础属性存在，例如： */
.list-enter-active,
.list-leave-active {
  transition: all 0.3s cubic-bezier(0.25, 1, 0.5, 1);
}
.list-enter-from,
.list-leave-to {
  opacity: 0;
  transform: translateY(15px);
}

/* 1. 统一头部按钮的基础样式，锁定高度防止内容变化导致抖动 */
.header-actions .action-btn, 
.header-actions .primary-btn {
  height: 40px;            /* 锁定统一高度 */
  border: none !important; /* 彻底禁用边框 */
  box-shadow: none;
  display: flex;
  align-items: center;
  justify-content: center;
  transition: background-color 0.2s ease, color 0.2s ease; /* 仅平滑过渡颜色，不影响布局 */
}

/* 2. 排序模式激活状态（反色的反色） */
.sorting-active {
  /* 使用类似节点卡片悬停时的颜色 (surface-hover) 来表示“正在编辑” */
  background: var(--surface-hover) !important; 
  color: var(--text-main) !important;
  font-weight: 600;
}

/* 3. 确保图标在激活态下颜色同步 */
.sorting-active .btn-icon {
  color: var(--text-main) !important;
  opacity: 1;
}

/* 4. 优化：如果想让“完成”更有点击感，可以使用 scale 而不是位移 */
.header-actions .action-btn:active,
.header-actions .primary-btn:active {
  transform: scale(0.98); /* 缩放不会引起周围元素位移 */
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
.active-card .icon-btn:hover { background: transparent !important; } /* 👈 修复：选中状态下取消悬停背景 */

.mini { padding: 8px 16px; font-size: 0.85rem; }
.divider-text { text-align: center; margin: 10px 0; color: var(--text-muted); font-size: 0.75rem; position: relative; }
.divider-text::before, .divider-text::after { content: ""; position: absolute; top: 50%; width: 40%; height: 1px; background: var(--surface-hover); }
.divider-text::before { left: 0; } .divider-text::after { right: 0; }
.w-full-btn { width: 100%; justify-content: center; padding: 14px; font-weight: 600; border-radius: 10px; border: none; background: var(--surface-hover); color: var(--text-main); cursor: pointer; }
.warning-box { background: rgba(255, 77, 79, 0.1); padding: 12px; border-radius: 8px; color: #ff4d4f; font-size: 0.85rem; line-height: 1.4; border: 1px solid rgba(255, 77, 79, 0.2); }

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

/* ================================== */
/* 顶部按钮图标与动画样式 */
/* ================================== */
.btn-icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  margin-right: 4px; /* 图标与文字的间距 */
}

/* 强制规定 SVG 的尺寸，防止变形或消失 */
.btn-icon :deep(svg) {
  width: 14px;
  height: 14px;
}

/* 刷新按钮的旋转加载动画 */
.spin :deep(svg) {
  animation: spin 1s linear infinite;
}

@keyframes spin {
  from { transform: rotate(0deg); }
  to { transform: rotate(360deg); }
}

/* ================================== */
/* 订阅流量条样式 (实色黑白风格)       */
/* ================================== */
/* 订阅流量条样式 (实色黑白风格)       */
/* ================================== */
.traffic-container {
  margin-top: 10px; 
  display: flex;
  flex-direction: column;
  gap: 6px;
  width: 100%;
}

.traffic-bar {
  width: 100%;
  height: 6px; /* 从 4px 加粗到 6px，让进度条更明显 */
  background: rgba(128, 128, 128, 0.2); /* 使用万能的半透明灰作为轨道，取代原本容易隐形的 surface-hover */
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

/* 👇 新增：专门修复选中状态（反色）下的文字与进度条隐形问题 */
.active-card .traffic-bar {
  background: rgba(0, 0, 0, 0.2); /* 在反色背景下，使用更深的半透明轨道以凸显轮廓 */
}
.active-card .traffic-fill {
  background: var(--accent-fg); /* 填充色跟随选中卡片的文字色（通常是纯白） */
}
.active-card .traffic-text {
  color: var(--accent-fg); /* 强制文字变成反色文字 */
  opacity: 0.9;
}
.active-card .expire-text {
  color: var(--accent-fg); /* 强制过期时间文字变成反色文字 */
  opacity: 0.7;
}
</style>