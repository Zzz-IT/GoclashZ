<template>
  <div class="rules-view">
    <div class="rules-header">
      <div class="search-bar">
        <span v-html="ICONS.search"></span>
        <input v-model="searchQuery" @input="onSearch" placeholder="搜索规则、目标或策略..." />
      </div>
      <button v-if="isEditable" class="add-rule-btn" @click="showAddModal = true">+ 添加规则</button>
    </div>

    <!-- 核心魔法：虚拟滚动容器 -->
    <div v-bind="containerProps" class="virtual-scroll-container">
      <div v-bind="wrapperProps" class="virtual-scroll-wrapper">
        <div v-for="item in list" :key="item.data.originalIndex" class="rule-card-list">
          <div class="rule-main">
            <div class="rule-type" :class="getTypeClass(item.data.type)">{{ item.data.type }}</div>
            <div class="rule-payload truncate" :title="item.data.payload">{{ item.data.payload }}</div>
          </div>
          <div class="rule-footer">
            <div class="rule-policy">{{ item.data.policy }}</div>
            <button v-if="isEditable" class="delete-btn" @click="handleDeleteRequest(item.data.originalIndex)" title="删除规则">
              <span v-html="ICONS.trash"></span>
            </button>
          </div>
        </div>
      </div>
      
      <div v-if="loading" class="loading-state">加载中...</div>
      <div v-if="!loading && rules.length === 0" class="empty-state">未找到匹配规则</div>
    </div>

    <Transition name="pop">
      <div v-if="showAddModal" class="modal-overlay" @click.self="showAddModal = false">
        <div class="custom-modal-card">
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
import { useVirtualList } from '@vueuse/core';
import * as API from '../../wailsjs/go/main/App';
import { showAlert, showConfirm } from '../store';
import { ICONS } from '../utils/icons';

// 接收带有原始 Index 的对象切片
const rules = ref<{index: number, text: string}[]>([]);
const isEditable = ref(false);
const searchQuery = ref('');
const showAddModal = ref(false);
const newRuleStr = ref('');
const loading = ref(false);

const loadAllRules = async () => {
  if (loading.value) return;
  loading.value = true;
  
  try {
    // 调用一次性获取所有过滤结果的 API
    const res: any = await (API as any).GetAllRules(searchQuery.value);
    rules.value = res.items || [];
    isEditable.value = res.isEditable;
  } catch (e) {
    console.error("加载规则失败", e);
  } finally {
    loading.value = false;
  }
};

// 搜索防抖
let searchTimeout: any = null;
const onSearch = () => {
  clearTimeout(searchTimeout);
  searchTimeout = setTimeout(() => {
    loadAllRules();
  }, 300);
};

const parsedRules = computed(() => {
  return rules.value.map((r) => {
    const parts = r.text.split(',');
    return {
      type: parts[0]?.trim() || 'UNKNOWN',
      payload: parts[1]?.trim() || '',
      policy: parts[2]?.trim() || '',
      originalIndex: r.index
    };
  });
});

// 激活虚拟滚动
const { list, containerProps, wrapperProps } = useVirtualList(
  parsedRules, 
  {
    itemHeight: 64, // 卡片高度 52px + 间距等
    overscan: 10,
  }
);

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
.rules-view { display: flex; flex-direction: column; height: 100%; overflow: hidden; }

.rules-header { display: flex; gap: 16px; align-items: center; margin-bottom: 20px; flex-shrink: 0; }
.search-bar { 
  flex: 1; display: flex; align-items: center; gap: 10px;
  background: var(--surface); border: none;
  padding: 10px 16px; border-radius: 12px; color: var(--text-sub);
}
.search-bar input { flex: 1; background: transparent; border: none; color: var(--text-main); outline: none; }

.add-rule-btn {
  padding: 10px 20px; background: var(--accent); color: var(--accent-fg);
  border: none; border-radius: 12px; font-weight: 600; cursor: pointer; transition: 0.2s;
}
.add-rule-btn:hover { opacity: 0.8; }

/* 虚拟滚动容器 */
.virtual-scroll-container {
  flex: 1;
  overflow-y: auto;
  width: 100%;
  padding-right: 8px;
}

.virtual-scroll-wrapper {
  display: flex;
  flex-direction: column;
  gap: 10px;
  padding-bottom: 20px;
}

/* 单列长条状卡片 */
.rule-card-list {
  height: 54px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  background: var(--surface);
  border-radius: 10px;
  padding: 0 16px;
  transition: background 0.2s;
}
.rule-card-list:hover { background: var(--surface-hover); }

.rule-main {
  display: flex;
  align-items: center;
  gap: 16px;
  flex: 1;
  min-width: 0;
}

.rule-type { font-size: 0.7rem; font-weight: 700; padding: 2px 8px; border-radius: 4px; flex-shrink: 0; }
.rule-payload { font-size: 0.85rem; color: var(--text-main); font-weight: 500; font-family: var(--font-mono); }

.tag-blue { background: rgba(128, 128, 128, 0.15); color: var(--text-main); }
.tag-green { background: rgba(128, 128, 128, 0.15); color: var(--text-sub); }
.tag-orange { background: rgba(128, 128, 128, 0.15); color: var(--text-sub); }
.tag-gray { background: rgba(128, 128, 128, 0.15); color: var(--text-muted); }

.rule-footer {
  display: flex;
  align-items: center;
  gap: 16px;
}
.rule-policy { font-size: 0.8rem; color: var(--text-main); font-weight: 600; }
.delete-btn { background: none; border: none; color: var(--text-muted); cursor: pointer; padding: 4px; border-radius: 6px; transition: 0.2s; display: flex; align-items: center; }
.delete-btn:hover { color: #ff4d4f; background: rgba(255, 77, 79, 0.1); }

.loading-state, .empty-state {
  text-align: center;
  padding: 40px;
  color: var(--text-muted);
  font-size: 0.9rem;
}

.hint { font-size: 0.75rem; color: var(--text-sub); margin-bottom: 0; line-height: 1.6; }

.modal-input { 
  width: 100%; padding: 12px; border-radius: 8px; 
  border: none;
  background: var(--surface-hover); 
  color: var(--text-main); outline: none; 
}
</style>
