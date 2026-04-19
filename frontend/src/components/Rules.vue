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
          <button v-if="isEditable" class="delete-btn" @click="openDeleteModal(rule.originalIndex)" title="删除规则">
            <svg viewBox="0 0 24 24" width="14" height="14" fill="none" stroke="currentColor" stroke-width="2"><polyline points="3 6 5 6 21 6"></polyline><path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"></path></svg>
          </button>
        </div>
      </div>
    </div>

    <Transition name="pop">
      <div v-if="showAddModal || activeModal === 'delete'" class="modal-overlay" @click.self="showAddModal = false; activeModal = null">
        <!-- 新增规则弹窗 -->
        <div v-if="showAddModal" class="modal-box glass-panel">
          <h3>新增分流规则</h3>
          <p class="hint">格式: 类型,目标,策略 (例如: DOMAIN-SUFFIX,google.com,Proxy)</p>
          <input v-model="newRuleStr" class="modal-input" placeholder="DOMAIN,example.com,DIRECT" @keyup.enter="handleAdd" />
          <div class="modal-actions">
            <button class="cancel-btn flex-1" @click="showAddModal = false">取消</button>
            <button class="confirm-btn flex-1" @click="handleAdd" :disabled="!newRuleStr">确定添加</button>
          </div>
        </div>

        <!-- 删除确认弹窗 -->
        <div v-if="activeModal === 'delete'" class="modal-box glass-panel">
          <h3 class="danger-text">删除规则</h3>
          <p class="hint">确定要永久删除这条规则吗？此操作不可撤销。</p>
          <div class="modal-actions">
            <button class="cancel-btn flex-1" @click="activeModal = null">取消</button>
            <button class="confirm-btn danger-btn flex-1" @click="confirmDelete">确定删除</button>
          </div>
        </div>
      </div>
    </Transition>
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
const activeModal = ref<'add' | 'delete' | null>(null);
const pendingIdx = ref(-1);

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

const openDeleteModal = (idx: number) => {
  pendingIdx.value = idx;
  activeModal.value = 'delete';
};

const confirmDelete = async () => {
  try {
    await API.DeleteRule(pendingIdx.value);
    activeModal.value = null;
    await loadRules();
  } catch (e) {
    alert("删除失败: " + e);
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
.delete-btn:hover { color: var(--text-main); background: var(--surface-hover); }

/* 同步弹窗样式 */
.modal-overlay { 
  position: fixed; inset: 0; 
  background: rgba(0,0,0,0.4); 
  backdrop-filter: none !important; /* 彻底移除模糊 */
  display: flex; align-items: center; justify-content: center; 
  z-index: 2000; 
}

.modal-box { 
  width: 440px; padding: 24px; border-radius: 16px; 
  background: var(--glass-panel);
  border: none;
  box-shadow: 0 20px 50px rgba(0,0,0,0.3);
}

/* 统一动画类名 */
.pop-enter-active, .pop-leave-active { transition: all 0.2s cubic-bezier(0.4, 0, 0.2, 1); }
.pop-enter-from, .pop-leave-to { opacity: 0; transform: scale(0.95); }

.modal-actions { display: flex; gap: 12px; width: 100%; }
.flex-1 { flex: 1; justify-content: center; }
.danger-btn { background: #ff4d4f !important; color: #fff !important; }
.danger-text { color: #ff4d4f; }

.modal-box h3 { margin-top: 0; color: var(--text-main); }
.hint { font-size: 0.75rem; color: var(--text-sub); margin-bottom: 16px; }

.modal-input { 
  width: 100%; padding: 12px; margin-bottom: 20px; border-radius: 8px; 
  border: none;
  background: var(--surface); 
  color: var(--text-main); outline: none; 
}
.modal-input:focus {
  background: var(--surface-hover);
}

.modal-actions { display: flex; justify-content: flex-end; gap: 12px; }
.cancel-btn, .confirm-btn { padding: 8px 16px; border-radius: 8px; border: none; cursor: pointer; font-weight: 500; transition: 0.2s;}
.cancel-btn { background: transparent; color: var(--text-sub); }
.cancel-btn:hover { background: var(--surface); color: var(--text-main); }
.confirm-btn { background: var(--accent); color: var(--accent-fg); }
.confirm-btn:hover:not(:disabled) { opacity: 0.85; }
.confirm-btn:disabled { opacity: 0.5; cursor: not-allowed; }
</style>
