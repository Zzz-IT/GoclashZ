<template>
  <div class="rules-view">
    <div class="rules-header">
      <div class="search-bar">
        <span v-html="ICONS.search"></span>
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
          <button v-if="isEditable" class="delete-btn" @click="handleDeleteRequest(rule.originalIndex)" title="删除规则">
            <span v-html="ICONS.trash"></span>
          </button>
        </div>
      </div>
    </div>

    <Transition name="pop">
      <div v-if="showAddModal" class="modal-overlay" @click.self="showAddModal = false">
        <!-- 新增规则弹窗 -->
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
    await showAlert("添加失败: " + e, '错误');
  }
};

const handleDeleteRequest = async (idx: number) => {
  const ok = await showConfirm('确定要永久删除这条规则吗？此操作不可撤销。', '删除规则');
  if (ok) {
    try {
      await API.DeleteRule(idx);
      await loadRules();
    } catch (e) {
      await showAlert("删除失败: " + e, '错误');
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
</style>
