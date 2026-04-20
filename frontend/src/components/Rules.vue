<template>
  <div class="rules-view">
    <div class="rules-header">
      <div class="search-bar">
        <span v-html="ICONS.search"></span>
        <input v-model="searchQuery" @input="onSearch" placeholder="搜索规则、目标或策略..." />
      </div>
      <button v-if="isEditable" class="add-rule-btn" @click="showAddModal = true">+ 添加规则</button>
    </div>

    <div class="rules-grid" @scroll="handleScroll">
      <div v-for="rule in parsedRules" :key="rule.originalIndex" class="rule-card">
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
      
      <div v-if="loading" class="loading-state">加载中...</div>
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
import * as API from '../../wailsjs/go/main/App';
import { showAlert, showConfirm } from '../store';
import { ICONS } from '../utils/icons';

// 接收带有原始 Index 的对象切片
const rules = ref<{index: number, text: string}[]>([]);
const isEditable = ref(false);
const searchQuery = ref('');
const showAddModal = ref(false);
const newRuleStr = ref('');

// 分页与滚动控制参数
const currentPage = ref(1);
const totalRules = ref(0);
const pageSize = 50; // 每次只拉 50 条，渲染瞬间完成
const loading = ref(false);

const loadRules = async (resetPage = false) => {
  if (resetPage) {
    currentPage.value = 1;
    rules.value = [];
  }
  
  if (loading.value) return;
  loading.value = true;
  
  try {
    // 调用我们刚写的后端分页 API
    const res: any = await (API as any).GetRulesPaged(currentPage.value, pageSize, searchQuery.value);
    
    if (resetPage) {
      rules.value = res.items || [];
    } else {
      rules.value = [...rules.value, ...(res.items || [])];
    }
    
    totalRules.value = res.total;
    isEditable.value = res.isEditable;
  } catch (e) {
    console.error("加载规则失败", e);
  } finally {
    loading.value = false;
  }
};

// 搜索防抖机制 (输入 300ms 后才请求后端)
let searchTimeout: any = null;
const onSearch = () => {
  clearTimeout(searchTimeout);
  searchTimeout = setTimeout(() => {
    loadRules(true);
  }, 300);
};

// 监听列表滚动实现无限加载
const handleScroll = (e: any) => {
  const { scrollTop, clientHeight, scrollHeight } = e.target;
  // 距底部 100px 缓冲即触发加载下一页
  if (scrollTop + clientHeight >= scrollHeight - 100) {
    if (rules.value.length < totalRules.value && !loading.value) {
      currentPage.value++;
      loadRules(false);
    }
  }
};

const parsedRules = computed(() => {
  return rules.value.map((r) => {
    const parts = r.text.split(',');
    return {
      type: parts[0]?.trim() || 'UNKNOWN',
      payload: parts[1]?.trim() || '',
      policy: parts[2]?.trim() || '',
      originalIndex: r.index // 后端准确返回，直接解绑前端索引强依赖
    };
  });
});

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
    await loadRules(true); // 添加后重置回第一页以显示
  } catch (e) {
    await showAlert("添加失败: " + e, '错误');
  }
};

const handleDeleteRequest = async (idx: number) => {
  const ok = await showConfirm('确定要永久删除这条规则吗？此操作不可撤销。', '删除规则');
  if (ok) {
    try {
      await API.DeleteRule(idx);
      await loadRules(true); // 删除后刷新视图
    } catch (e) {
      await showAlert("删除失败: " + e, '错误');
    }
  }
};

onMounted(() => {
  loadRules(true);
});
</script>

<style scoped>
.rules-view { display: flex; flex-direction: column; height: 100%; }

.rules-header { display: flex; gap: 16px; align-items: center; margin-bottom: 20px; }
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

.rules-grid {
  display: grid; grid-template-columns: repeat(auto-fill, minmax(260px, 1fr));
  gap: 16px; overflow-y: auto; padding-right: 10px; padding-bottom: 20px;
}

.rule-card {
  background: var(--surface); border: none;
  border-radius: 12px; padding: 14px 16px; display: flex; flex-direction: column; gap: 12px;
  transition: transform 0.2s, background 0.2s;
}
.rule-card:hover { transform: translateY(-2px); background: var(--surface-hover); }

.rule-main { display: flex; flex-direction: column; gap: 6px; }
.rule-type { font-size: 0.7rem; font-weight: 700; padding: 2px 8px; border-radius: 4px; width: fit-content; }
.rule-payload { font-size: 0.85rem; color: var(--text-main); font-weight: 500; font-family: var(--font-mono); }

/* 标签：纯灰度，靠明度区分类型 */
.tag-blue { background: var(--surface-hover); color: var(--text-main); }
.tag-green { background: var(--surface-hover); color: var(--text-sub); }
.tag-orange { background: var(--surface-hover); color: var(--text-sub); }
.tag-gray { background: var(--surface-hover); color: var(--text-muted); }

.rule-footer {
  display: flex; justify-content: space-between; align-items: center;
  padding-top: 10px;
}
.rule-policy { font-size: 0.8rem; color: var(--text-main); font-weight: 600; }
.delete-btn { background: none; border: none; color: var(--text-muted); cursor: pointer; padding: 4px; border-radius: 6px; transition: 0.2s; }
.delete-btn:hover { color: #ff4d4f; background: rgba(255, 77, 79, 0.1); }

.hint { font-size: 0.75rem; color: var(--text-sub); margin-bottom: 0; line-height: 1.6; }

.modal-input { 
  width: 100%; padding: 12px; border-radius: 8px; 
  border: none;
  background: var(--surface-hover); 
  color: var(--text-main); outline: none; 
}

/* 加载占位提示样式 */
.loading-state {
  grid-column: 1 / -1;
  text-align: center;
  padding: 20px;
  color: var(--text-muted);
  font-size: 0.85rem;
}
</style>
