<template>
  <div class="rules-view">
    <div class="rules-header">
      <div class="search-bar">
        <svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2"><circle cx="11" cy="11" r="8"/><line x1="21" y1="21" x2="16.65" y2="16.65"/></svg>
        <input v-model="searchQuery" placeholder="搜索规则、目标或策略..." />
      </div>
      <button v-if="isEditable" class="add-rule-btn" @click="showAddModal = true">+ 添加规则</button>
    </div>

    <div class="rules-grid">
      <div v-for="(rule, index) in filteredRules" :key="index" class="rule-card">
        <div class="rule-main">
          <div class="rule-type" :class="getTypeClass(rule.type)">{{ rule.type }}</div>
          <div class="rule-payload truncate" :title="rule.payload">{{ rule.payload }}</div>
        </div>
        <div class="rule-footer">
          <div class="rule-policy">{{ rule.policy }}</div>
          <button v-if="isEditable" class="delete-btn" @click="handleDelete(rule.originalIndex)" title="删除规则">
            <svg viewBox="0 0 24 24" width="14" height="14" fill="none" stroke="currentColor" stroke-width="2"><polyline points="3 6 5 6 21 6"></polyline><path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"></path></svg>
          </button>
        </div>
      </div>
    </div>

    <div v-if="showAddModal" class="modal-mask" @click.self="showAddModal = false">
      <div class="modal-box glass-panel">
        <h3>新增分流规则</h3>
        <p class="hint">格式: 类型,目标,策略 (例如: DOMAIN-SUFFIX,google.com,Proxy)</p>
        <input v-model="newRuleStr" class="modal-input" placeholder="DOMAIN,example.com,DIRECT" @keyup.enter="handleAdd" />
        <div class="modal-actions">
          <button class="cancel-btn" @click="showAddModal = false">取消</button>
          <button class="confirm-btn" @click="handleAdd" :disabled="!newRuleStr">确定添加</button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, computed } from 'vue';
import * as API from '../../wailsjs/go/main/App';

const rules = ref<string[]>([]);
const isEditable = ref(false);
const searchQuery = ref('');
const showAddModal = ref(false);
const newRuleStr = ref('');

const loadRules = async () => {
  try {
    const res: any = await API.GetRules();
    rules.value = res.rules || [];
    isEditable.value = res.isEditable;
  } catch (e) {
    console.error("加载规则失败", e);
  }
};

const parsedRules = computed(() => {
  return rules.value.map((r, index) => {
    const parts = r.split(',');
    return {
      type: parts[0]?.trim() || 'UNKNOWN',
      payload: parts[1]?.trim() || '',
      policy: parts[2]?.trim() || '',
      originalIndex: index
    };
  });
});

const filteredRules = computed(() => {
  if (!searchQuery.value) return parsedRules.value;
  const q = searchQuery.value.toLowerCase();
  return parsedRules.value.filter(r => 
    r.payload?.toLowerCase().includes(q) || 
    r.type?.toLowerCase().includes(q) ||
    r.policy?.toLowerCase().includes(q)
  );
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
    await loadRules();
  } catch (e) {
    alert("添加失败: " + e);
  }
};

const handleDelete = async (idx: number) => {
  if (confirm("确定删除该规则吗？")) {
    try {
      await API.DeleteRule(idx);
      await loadRules();
    } catch (e) {
      alert("删除失败: " + e);
    }
  }
};

onMounted(() => {
  loadRules();
});
</script>

<style scoped>
.rules-view { display: flex; flex-direction: column; height: 100%; }

.rules-header { display: flex; gap: 16px; align-items: center; margin-bottom: 20px; }
.search-bar { 
  flex: 1; display: flex; align-items: center; gap: 10px;
  background: var(--surface); border: 1px solid var(--glass-border);
  padding: 10px 16px; border-radius: 12px; color: var(--text-sub);
}
.search-bar input { flex: 1; background: transparent; border: none; color: var(--text-main); outline: none; }

.add-rule-btn {
  padding: 10px 20px; background: var(--accent); color: var(--accent-fg);
  border: none; border-radius: 12px; font-weight: 600; cursor: pointer; transition: 0.2s;
}
.add-rule-btn:hover { transform: translateY(-1px); box-shadow: 0 4px 12px rgba(79, 70, 229, 0.3); }

.rules-grid {
  display: grid; grid-template-columns: repeat(auto-fill, minmax(260px, 1fr));
  gap: 16px; overflow-y: auto; padding-right: 10px; padding-bottom: 20px;
}

.rule-card {
  background: var(--surface); border: 1px solid var(--glass-border);
  border-radius: 12px; padding: 14px 16px; display: flex; flex-direction: column; gap: 12px;
  transition: transform 0.2s, background 0.2s;
}
.rule-card:hover { transform: translateY(-2px); background: var(--surface-hover); }

.rule-main { display: flex; flex-direction: column; gap: 6px; }
.rule-type { font-size: 0.7rem; font-weight: 700; padding: 2px 8px; border-radius: 4px; width: fit-content; }
.rule-payload { font-size: 0.85rem; color: var(--text-main); font-weight: 500; font-family: var(--font-mono); }

.tag-blue { background: rgba(59, 130, 246, 0.1); color: #3b82f6; }
.tag-green { background: rgba(16, 185, 129, 0.1); color: #10b981; }
.tag-orange { background: rgba(245, 158, 11, 0.1); color: #f59e0b; }
.tag-gray { background: var(--surface-hover); color: var(--text-sub); }

.rule-footer {
  display: flex; justify-content: space-between; align-items: center;
  border-top: 1px solid var(--glass-border); padding-top: 10px;
}
.rule-policy { font-size: 0.8rem; color: var(--accent); font-weight: 600; }
.delete-btn { background: none; border: none; color: var(--text-muted); cursor: pointer; padding: 4px; border-radius: 6px; transition: 0.2s; }
.delete-btn:hover { color: #ef4444; background: rgba(239, 68, 68, 0.1); }

/* Modal 样式重构：纯色遮罩 + 实色卡片 */
.modal-mask { 
  position: fixed; 
  inset: 0; 
  background: rgba(0,0,0,0.4); /* 纯粹的半透明黑色背景，不加 blur */
  display: flex; 
  align-items: center; 
  justify-content: center; 
  z-index: 100; 
}

.modal-box { 
  width: 420px; 
  padding: 24px; 
  border-radius: 12px; 
  background: var(--surface); /* 使用主题的表面实色 */
  border: 1px solid var(--border-color, #e5e7eb); /* 使用普通边框代替玻璃边框 */
  box-shadow: 0 10px 30px rgba(0,0,0,0.15); /* 加深阴影，使实色卡片浮出层级更加分明 */
}

.modal-box h3 { margin-top: 0; color: var(--text-main); }
.hint { font-size: 0.75rem; color: var(--text-sub); margin-bottom: 16px; }

.modal-input { 
  width: 100%; 
  padding: 12px; 
  margin-bottom: 20px; 
  border-radius: 8px; 
  border: 1px solid var(--border-color, #e5e7eb); 
  background: var(--surface-hover); 
  color: var(--text-main); 
  outline: none; 
}
.modal-input:focus {
  border-color: var(--accent); /* 输入框聚焦时的主题色高亮 */
}

.modal-actions { display: flex; justify-content: flex-end; gap: 12px; }
.cancel-btn, .confirm-btn { padding: 8px 16px; border-radius: 8px; border: none; cursor: pointer; font-weight: 500; transition: 0.2s;}
.cancel-btn { background: transparent; color: var(--text-sub); }
.cancel-btn:hover { background: var(--surface-hover); color: var(--text-main); }
.confirm-btn { background: var(--accent); color: var(--accent-fg); }
.confirm-btn:hover:not(:disabled) { opacity: 0.9; transform: translateY(-1px); }
.confirm-btn:disabled { opacity: 0.5; cursor: not-allowed; }
</style>
