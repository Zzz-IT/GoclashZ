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
        <div v-for="(rule, index) in filteredRules" :key="index" class="rule-card">
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

      <div class="pagination-bar" v-if="userRules.length > 0">
        <span class="page-info">当前订阅共有 {{ userRules.length }} 条规则</span>
        <div class="tip-text">新添规则将自动注入置于规则文件最前端</div>
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
import { ref, onMounted, computed, watch } from 'vue';
import * as API from '../../wailsjs/go/main/App';
import { showAlert, showConfirm, globalState } from '../store';
import { ICONS } from '../utils/icons';

const userRules = ref<string[]>([]);
const searchQuery = ref('');
const showAddModal = ref(false);
const newRuleStr = ref('');
const loading = ref(false);

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
  const query = searchQuery.value.toLowerCase();
  if (!query) return parsedRules.value;
  return parsedRules.value.filter(r => 
    r.type.toLowerCase().includes(query) || 
    r.payload.toLowerCase().includes(query) || 
    r.policy.toLowerCase().includes(query)
  );
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

.rules-grid { flex: 1; min-height: 0; display: grid; grid-template-columns: repeat(auto-fill, minmax(260px, 1fr)); align-content: start; gap: 16px; overflow-y: auto; padding-right: 10px; padding-bottom: 20px; }
.rule-card { background: var(--surface); border: 1px solid var(--surface-hover); border-radius: 10px; padding: 12px 14px; display: flex; flex-direction: column; gap: 6px; transition: background 0.2s; }
.rule-card:hover { background: var(--surface-hover); }
.rule-main { display: flex; flex-direction: column; gap: 6px; }
.rule-type { font-size: 0.7rem; font-weight: 700; padding: 4px 8px; border-radius: 6px; width: fit-content; border: none; }
.tag-primary { background: var(--text-main); color: var(--surface); } 
.rule-payload { font-size: 0.95rem; color: var(--text-main); font-weight: 600; word-break: break-all; }
.rule-footer { display: flex; justify-content: space-between; align-items: center; margin-top: auto; }
.rule-policy { font-size: 0.75rem; color: var(--text-sub); font-weight: 600; }

.delete-btn { background: transparent; color: #ff4d4f; border: none; cursor: pointer; opacity: 0; transition: opacity 0.2s; padding: 4px; }
.rule-card:hover .delete-btn { opacity: 1; }

.loading-state { grid-column: 1 / -1; text-align: center; padding: 20px; color: var(--text-muted); font-size: 0.85rem; }

.pagination-bar { display: flex; justify-content: space-between; align-items: center; padding-top: 16px; border-top: 1px solid var(--surface-hover); margin-top: auto; }
.page-info { font-size: 0.85rem; color: var(--text-sub); font-weight: 500; }
.tip-text { font-size: 0.75rem; color: var(--text-muted); font-style: italic; }

.flex-1 { flex: 1; }
</style>
