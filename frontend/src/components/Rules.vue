<template>
  <div class="rules-view">
    <div v-if="!globalState.activeConfigId" class="empty-state-view">
      <div class="empty-msg">请先在“订阅管理”中选择并启动一个配置文件</div>
    </div>
    <template v-else>
      <div class="rules-header">
        <div class="search-bar">
          <span v-html="ICONS.search"></span>
          <input v-model="searchQuery" placeholder="搜索我的规则..." />
        </div>
        <div class="header-actions">
          <button 
            v-if="globalState.activeConfigType === 'remote'" 
            class="sync-btn" 
            @click="handleSync" 
            :disabled="loading"
            title="从机场订阅文件重新提取原始规则，将覆盖现有规则修改"
          >
            <span class="btn-icon" v-html="ICONS.refresh"></span> 同步
          </button>
          <button class="add-rule-btn" @click="showAddModal = true">+ 添加规则</button>
        </div>
      </div>

      <div class="rules-grid">
        <div v-for="rule in paginatedRules" :key="rule.originalIndex" class="rule-card">
          <div class="rule-main">
            <div class="rule-type tag-primary">{{ rule.type }}</div>
            <div class="rule-payload truncate" :title="rule.payload">{{ rule.payload }}</div>
          </div>
          <div class="rule-footer">
            <div class="rule-policy">{{ rule.policy }}</div>
            <button class="delete-btn" @click="handleDelete(rule.originalIndex)" title="删除规则">
              <span v-html="ICONS.trash"></span>
            </button>
          </div>
        </div>
        
        <div v-if="!loading && filteredRules.length === 0" class="loading-state">
          {{ searchQuery ? '没有找到匹配的规则' : '暂无规则，点击上方按钮添加' }}
        </div>
      </div>

      <div class="pagination-bar" v-if="filteredRules.length > 0">
        <span class="page-info">共 {{ filteredRules.length }} 条规则</span>
        
        <div class="pagination-controls">
          <button 
            class="page-btn" 
            @click="currentPage--" 
            :disabled="currentPage <= 1"
          >
            &lt; 上一页
          </button>
          
          <span class="page-status">第 {{ currentPage }} / {{ totalPages }} 页</span>
          
          <button 
            class="page-btn" 
            @click="currentPage++" 
            :disabled="currentPage >= totalPages"
          >
            下一页 &gt;
          </button>
        </div>

        <div class="tip-text">新添规则自动置顶</div>
      </div>
    </template>

    <Transition name="pop">
      <div v-if="showAddModal" class="modal-overlay" @click.self="showAddModal = false">
        <div class="custom-modal-card" @click.stop>
          <div class="modal-header">
            <h3>新增分流规则</h3>
          </div>
          <div class="modal-body">
            <p class="global-modal-msg">格式: 类型,目标,策略 (例如: DOMAIN,google.com,Proxy)</p>
            <input v-model="newRuleStr" class="modal-input" placeholder="DOMAIN,example.com,DIRECT" @keyup.enter="handleAdd" />
            <div class="modal-footer">
              <button class="action-btn flex-1" @click="showAddModal = false">取消</button>
              <button class="primary-btn accent-btn flex-1" @click="handleAdd" :disabled="!newRuleStr || loading">确定添加</button>
            </div>
          </div>
        </div>
      </div>
    </Transition>
  </div>
</template>

<script setup lang="ts">
import { ref, shallowRef, onMounted, computed, watch } from 'vue';
import * as API from '../../wailsjs/go/main/App';
import { showAlert, showConfirm, globalState } from '../store';
import { ICONS } from '../utils/icons';

// 🚀 性能极化：使用 shallowRef 存储几万条规则，避免 Vue 深度代理导致的内存溢出和初始化卡顿
const userRules = shallowRef<string[]>([]);
const searchQuery = ref('');
const debouncedQuery = ref(''); // 实际用于过滤的搜索词
let searchTimer: ReturnType<typeof setTimeout>;

const showAddModal = ref(false);
const newRuleStr = ref('');
const loading = ref(false);

const currentPage = ref(1);
const pageSize = ref(42); // 调整为每页 42 条规则，优化视觉排布

// 监听搜索词变化并加入防抖，防止高频触发过滤计算
watch(searchQuery, (newVal) => {
  clearTimeout(searchTimer);
  searchTimer = setTimeout(() => {
    debouncedQuery.value = newVal.toLowerCase().trim();
    currentPage.value = 1; // 搜索词确认改变后，重置页码
  }, 300);
});

const loadRules = async () => {
  if (!globalState.activeConfigId) return;
  loading.value = true;
  try {
    const rules = await API.GetCustomRules(globalState.activeConfigId);
    userRules.value = rules || [];
  } catch (e) {
    console.error("加载规则失败", e);
  } finally {
    loading.value = false;
  }
};

// 监听配置切换
watch(() => globalState.activeConfigId, (newId) => {
  if (newId) loadRules();
  else userRules.value = [];
}, { immediate: true });

const parsedRules = computed(() => {
  return userRules.value.map((text, index) => {
    const parts = text.split(',');
    return {
      type: parts[0]?.trim() || 'UNKNOWN',
      payload: parts[1]?.trim() || '',
      policy: parts[2]?.trim() || '',
      originalIndex: index
    };
  });
});

const filteredRules = computed(() => {
  if (!debouncedQuery.value) return parsedRules.value;
  return parsedRules.value.filter(r => 
    r.type.toLowerCase().includes(debouncedQuery.value) || 
    r.payload.toLowerCase().includes(debouncedQuery.value) || 
    r.policy.toLowerCase().includes(debouncedQuery.value)
  );
});

// --- 分页逻辑核心 ---

const totalPages = computed(() => {
  return Math.ceil(filteredRules.value.length / pageSize.value) || 1;
});

const paginatedRules = computed(() => {
  const start = (currentPage.value - 1) * pageSize.value;
  const end = start + pageSize.value;
  return filteredRules.value.slice(start, end);
});

// 越界保护：当删除数据导致总页数缩减时，自动修正当前页码
watch(totalPages, (newTotal) => {
  if (currentPage.value > newTotal) {
    currentPage.value = newTotal;
  }
});

const handleAdd = async () => {
  if (!newRuleStr.value || !globalState.activeConfigId) return;
  loading.value = true;
  try {
    const newList = [newRuleStr.value, ...userRules.value];
    await API.SaveCustomRules(globalState.activeConfigId, newList);
    userRules.value = newList;
    newRuleStr.value = '';
    showAddModal.value = false;
    await showAlert("规则已添加并保存", "提示");
  } catch (e) {
    await showAlert("添加失败: " + e, '错误');
  } finally {
    loading.value = false;
  }
};

const handleDelete = async (idx: number) => {
  const ok = await showConfirm('确定要永久删除这条规则吗？此操作不可撤销。', '删除规则', true);
  if (ok && globalState.activeConfigId) {
    loading.value = true;
    try {
      const newList = [...userRules.value];
      newList.splice(idx, 1);
      await API.SaveCustomRules(globalState.activeConfigId, newList);
      userRules.value = newList;
    } catch (e) {
      await showAlert("删除失败: " + e, '错误');
    } finally {
      loading.value = false;
    }
  }
};

const handleSync = async () => {
  if (!globalState.activeConfigId) return;
  const ok = await showConfirm(
    "确定要从机场订阅源重新同步规则吗？\n这将会彻底覆盖您当前对该配置的所有规则修改！",
    "同步规则警告",
    true
  );
  if (ok) {
    loading.value = true;
    try {
      await API.SyncRules(globalState.activeConfigId);
      await loadRules();
      await showAlert("规则已同步至机场最新状态", "同步成功");
    } catch (e) {
      await showAlert("同步失败: " + e, "错误");
    } finally {
      loading.value = false;
    }
  }
};

onMounted(() => {
  loadRules();
});
</script>

<style scoped>
.rules-view { display: flex; flex-direction: column; height: 100%; }
.empty-state-view { flex: 1; display: flex; align-items: center; justify-content: center; color: var(--text-muted); font-size: 0.9rem; }

.rules-header { display: flex; align-items: center; gap: 16px; margin-bottom: 16px; width: 100%; }
.header-actions { display: flex; gap: 12px; }
.search-bar { display: flex; align-items: center; background: var(--surface); border: 1px solid var(--surface-hover); border-radius: 8px; padding: 8px 12px; flex: 1; }
.search-bar input { border: none; background: transparent; color: var(--text-main); outline: none; margin-left: 8px; width: 100%; }

.add-rule-btn { background: var(--accent); color: var(--accent-fg); border: none; padding: 8px 16px; border-radius: 8px; font-weight: 600; cursor: pointer; transition: 0.2s; flex-shrink: 0; }
.add-rule-btn:hover { filter: brightness(0.9); }

.sync-btn {
  background: var(--accent);
  color: var(--accent-fg);
  border: none;
  padding: 8px 16px;
  border-radius: 8px;
  font-weight: 600;
  cursor: pointer;
  transition: 0.2s;
  display: flex;
  align-items: center;
  gap: 6px;
}
.sync-btn:hover { filter: brightness(0.9); }
.sync-btn:disabled { opacity: 0.5; cursor: not-allowed; }
.sync-btn .btn-icon :deep(svg) { width: 14px; height: 14px; }

.rules-grid { 
  flex: 1; 
  min-height: 0; 
  display: grid; 
  grid-template-columns: repeat(auto-fill, minmax(300px, 1fr)); 
  align-content: start; 
  gap: 16px; 
  overflow-y: auto; 
  padding-right: 12px; /* 🚀 恢复间距，让滚动条与卡片保持呼吸感 */
  padding-bottom: 20px; 
}

/* 🚀 滚动条样式保持简洁 */
.rules-grid::-webkit-scrollbar {
  width: 10px; 
}

.rules-grid::-webkit-scrollbar-thumb {
  background-color: var(--surface-hover);
  border: 3px solid transparent; 
  background-clip: padding-box;
  border-radius: 10px;
  transition: background 0.2s;
}

.rules-grid::-webkit-scrollbar-thumb:hover {
  background-color: var(--text-sub); /* 悬停时加深，提示已抓取 */
}

.rule-card { 
  background: var(--surface); 
  border: 1px solid var(--surface-hover); 
  border-radius: 10px; 
  padding: 14px 16px; 
  display: flex; 
  flex-direction: column; 
  gap: 10px; 
  transition: background 0.2s; 
  height: 110px; /* 🚀 增加高度，彻底解决文字“腰斩”问题 */
  box-sizing: border-box;
  justify-content: space-between;
}
.rule-card:hover { background: var(--surface-hover); }
.rule-main { display: flex; flex-direction: column; gap: 8px; } /* 🚀 增加内容间距 */
.rule-type { font-size: 0.7rem; font-weight: 700; padding: 4px 8px; border-radius: 6px; width: fit-content; border: none; flex-shrink: 0; }
.tag-primary { background: var(--text-main); color: var(--surface); } 
.rule-payload { 
  font-size: 1rem; 
  color: var(--text-main); 
  font-weight: 600; 
  line-height: 1.4; /* 🚀 优化行高 */
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.rule-footer { display: flex; justify-content: space-between; align-items: center; margin-top: auto; }
.rule-policy { font-size: 0.75rem; color: var(--text-sub); font-weight: 600; }

.delete-btn { background: transparent; color: #ff4d4f; border: none; cursor: pointer; opacity: 0; transition: opacity 0.2s; padding: 4px; }
.rule-card:hover .delete-btn { opacity: 1; }

.loading-state { grid-column: 1 / -1; text-align: center; padding: 20px; color: var(--text-muted); font-size: 0.85rem; }

.pagination-bar { 
  display: flex; 
  justify-content: space-between; 
  align-items: center; 
  padding-top: 16px; 
  border-top: 1px solid var(--surface-hover); 
  margin-top: auto; 
}

.page-info, .tip-text {
  flex: 1;
}
.tip-text {
  text-align: right;
  font-size: 0.75rem;
  color: var(--text-muted);
  font-style: italic;
}

.pagination-controls {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 16px;
  flex: 2;
}

.page-btn {
  background: var(--surface);
  border: none; /* 彻底去除轮廓线 */
  color: var(--text-main);
  padding: 8px 18px;
  border-radius: 8px;
  font-size: 0.85rem;
  font-weight: 600;
  cursor: pointer;
  transition: all 0.2s ease-in-out;
  outline: none;
}

/* 悬停与点击采用反色方案，增强交互的高级感 */
.page-btn:hover:not(:disabled) {
  background: var(--text-main);
  color: var(--surface);
}

.page-btn:active:not(:disabled) {
  transform: scale(0.96);
  opacity: 0.8;
}

.page-btn:disabled {
  opacity: 0.2;
  cursor: not-allowed;
}

.page-status {
  font-size: 0.85rem;
  font-weight: 600;
  color: var(--text-main);
  min-width: 80px;
  text-align: center;
  letter-spacing: 0.5px;
}

.flex-1 { flex: 1; }
</style>
