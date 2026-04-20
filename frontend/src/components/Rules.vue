<template>
  <div class="rules-view">
    <div class="rules-header">
      <div class="search-bar">
        <span v-html="ICONS.search"></span>
        <input v-model="searchQuery" @input="onSearch" placeholder="搜索规则、目标或策略..." />
      </div>
      <button v-if="isEditable" class="add-rule-btn" @click="showAddModal = true">+ 添加规则</button>
    </div>

    <div class="rules-grid">
      <div v-for="rule in currentPageRules" :key="rule.originalIndex" class="rule-card">
        <div class="rule-main">
          <div class="rule-type" :class="getTypeClass(rule.type)">{{ rule.type }}</div>
          <div class="rule-payload truncate" :title="rule.payload">{{ rule.payload }}</div>
        </div>
        <div class="rule-footer">
          <div class="rule-policy">{{ rule.policy }}</div>
          <button v-if="isEditable" class="delete-btn" @click="handleDeleteRequest(rule.originalIndex)" title="删除规则">
            <span v-html="ICONS.trash"></span>
          </button>
        </div>
      </div>
      
      <div v-if="!loading && parsedRules.length === 0" class="loading-state">没有找到匹配的规则</div>
    </div>

    <div v-if="parsedRules.length > 0" class="pagination-bar">
      <span class="page-info">共 {{ parsedRules.length }} 条规则 (第 {{ currentPage }} / {{ totalPages }} 页)</span>
      <div class="page-controls">
        <button class="action-btn" :disabled="currentPage === 1" @click="changePage(currentPage - 1)">上一页</button>
        <button class="action-btn" :disabled="currentPage === totalPages" @click="changePage(currentPage + 1)">下一页</button>
      </div>
    </div>

    <Transition name="pop">
      <div v-if="showAddModal" class="modal-overlay" @click.self="showAddModal = false">
        <div class="custom-modal-card" @click.stop>
          <div class="modal-header">
            <h3>新增分流规则</h3>
          </div>
          <div class="modal-body">
            <p class="hint">格式: 类型,目标,策略 (例如: DOMAIN-SUFFIX,google.com,Proxy)</p>
            <input v-model="newRuleStr" class="modal-input" placeholder="DOMAIN,example.com,DIRECT" @keyup.enter="handleAdd" />
            <div class="modal-footer">
              <button class="action-btn flex-1" @click="showAddModal = false">取消</button>
              <button class="primary-btn accent-btn flex-1" @click="handleAdd" :disabled="!newRuleStr">确定添加</button>
            </div>
          </div>
        </div>
      </div>
    </Transition>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, computed } from 'vue';
import * as API from '../../wailsjs/go/main/App';
import { showAlert, showConfirm } from '../store';
import { ICONS } from '../utils/icons';

const rawRules = ref<{index: number, text: string}[]>([]);
const isEditable = ref(false);
const searchQuery = ref('');
const showAddModal = ref(false);
const newRuleStr = ref('');
const loading = ref(false);

// 分页状态：固定渲染100条，适配大屏幕的 Grid 布局
const currentPage = ref(1);
const pageSize = 100; 

// 保留 Go 后端的全量极速检索
const loadAllRules = async () => {
  loading.value = true;
  try {
    const res: any = await (API as any).GetAllRules(searchQuery.value);
    rawRules.value = res.items || [];
    isEditable.value = res.isEditable;
    currentPage.value = 1;
  } catch (e) {
    console.error("加载规则失败", e);
  } finally {
    loading.value = false;
  }
};

let searchTimeout: any = null;
const onSearch = () => {
  clearTimeout(searchTimeout);
  searchTimeout = setTimeout(() => {
    loadAllRules();
  }, 300);
};

const parsedRules = computed(() => {
  return rawRules.value.map((r) => {
    const parts = r.text.split(',');
    return {
      type: parts[0]?.trim() || 'UNKNOWN',
      payload: parts[1]?.trim() || '',
      policy: parts[2]?.trim() || '',
      originalIndex: r.index
    };
  });
});

// 计算总页数和当前需要渲染的 100 条卡片
const totalPages = computed(() => Math.ceil(parsedRules.value.length / pageSize) || 1);

const currentPageRules = computed(() => {
  const start = (currentPage.value - 1) * pageSize;
  return parsedRules.value.slice(start, start + pageSize);
});

const changePage = (p: number) => {
  if (p < 1 || p > totalPages.value) return;
  currentPage.value = p;
  const grid = document.querySelector('.rules-grid');
  if (grid) grid.scrollTop = 0;
};

// 极简高对比度标签
const getTypeClass = (type: string) => {
  if (type.startsWith('DOMAIN')) return 'tag-blue';
  if (type.startsWith('IP')) return 'tag-green';
  if (type === 'GEOIP' || type === 'MATCH') return 'tag-orange';
  return 'tag-gray';
};

const handleAdd = async () => {
  if (!newRuleStr.value) return;
  try {
    await API.AddRule(newRuleStr.value);
    newRuleStr.value = '';
    showAddModal.value = false;
    await loadAllRules();
  } catch (e) {
    await showAlert("添加失败: " + e, '错误');
  }
};

const handleDeleteRequest = async (idx: number) => {
  const ok = await showConfirm('确定要永久删除这条规则吗？此操作不可撤销。', '删除规则');
  if (ok) {
    try {
      await API.DeleteRule(idx);
      await loadAllRules();
    } catch (e) {
      await showAlert("删除失败: " + e, '错误');
    }
  }
};

onMounted(() => {
  loadAllRules();
});
</script>

<style scoped>
.rules-view {
  display: flex;
  flex-direction: column;
  height: 100%;
}

.rules-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 16px;
}

.search-bar {
  display: flex;
  align-items: center;
  background: var(--surface);
  border: 1px solid var(--surface-hover);
  border-radius: 8px;
  padding: 8px 12px;
  width: 350px;
}

.search-bar input {
  border: none;
  background: transparent;
  color: var(--text-main);
  outline: none;
  margin-left: 8px;
  width: 100%;
}

.add-rule-btn {
  background: var(--accent);
  color: var(--accent-fg);
  border: none;
  padding: 8px 16px;
  border-radius: 8px;
  font-weight: 600;
  cursor: pointer;
  transition: 0.2s;
}
.add-rule-btn:hover { filter: brightness(0.9); }

/* ================================== */
/* 恢复经典的 Grid 多列网格布局       */
/* ================================== */
.rules-grid {
  flex: 1; 
  min-height: 0; 
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(260px, 1fr));
  align-content: start; 
  gap: 16px;
  overflow-y: auto; 
  padding-right: 10px;
  padding-bottom: 20px;
}

/* 恢复原汁原味的立体卡片 */
.rule-card {
  background: var(--surface);
  border: 1px solid var(--surface-hover);
  border-radius: 10px;
  padding: 16px;
  display: flex;
  flex-direction: column;
  gap: 12px;
  transition: all 0.2s;
}
.rule-card:hover { border-color: var(--text-muted); }

.rule-main {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

/* 极简高能实色标签 */
.rule-type {
  font-size: 0.7rem;
  font-weight: 700;
  padding: 4px 8px;
  border-radius: 6px;
  width: fit-content;
  border: 1px solid transparent;
}
.tag-blue { background: var(--text-main); color: var(--surface); } 
.tag-green { background: transparent; color: var(--text-main); border-color: var(--text-main); } 
.tag-orange { background: transparent; color: var(--text-main); border-color: var(--text-sub); border-style: dashed; } 
.tag-gray { background: var(--surface-hover); color: var(--text-muted); } 

.rule-payload {
  font-size: 0.95rem;
  color: var(--text-main);
  font-weight: 600;
  word-break: break-all;
}

.rule-footer {
  display: flex;
  justify-content: space-between;
  align-items: center;
  border-top: 1px solid var(--surface-hover);
  padding-top: 12px;
  margin-top: auto;
}

.rule-policy {
  font-size: 0.75rem;
  color: var(--text-sub);
  font-weight: 600;
}

.delete-btn {
  background: transparent;
  color: #ff4d4f;
  border: none;
  cursor: pointer;
  opacity: 0;
  transition: opacity 0.2s;
  padding: 4px;
}
.rule-card:hover .delete-btn { opacity: 1; }

.loading-state {
  grid-column: 1 / -1;
  text-align: center;
  padding: 20px;
  color: var(--text-muted);
  font-size: 0.85rem;
}

/* ================================== */
/* 底部固定分页器                     */
/* ================================== */
.pagination-bar {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding-top: 16px;
  border-top: 1px solid var(--surface-hover);
  margin-top: auto; 
}

.page-info {
  font-size: 0.85rem;
  color: var(--text-sub);
  font-weight: 500;
}

.page-controls {
  display: flex;
  gap: 12px;
}

.page-controls .action-btn {
  padding: 6px 16px;
  border-radius: 8px;
  background: var(--surface-hover);
  color: var(--text-main);
  border: 1px solid transparent;
  font-size: 0.85rem;
  font-weight: 600;
  cursor: pointer;
  transition: 0.2s;
}

.page-controls .action-btn:hover:not(:disabled) {
  border-color: var(--text-sub);
}

.page-controls .action-btn:disabled {
  opacity: 0.3;
  cursor: not-allowed;
}

.flex-1 { flex: 1; }
</style>
