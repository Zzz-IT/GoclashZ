<template>
  <div class="rules-view">
    <div v-if="!globalState.activeConfigId" class="empty-state-view">
      <div class="empty-msg">请先在“订阅管理”中选择并启动一个配置文件</div>
    </div>
    <template v-else>
      <div class="rules-header page-sticky-mask">
        <div v-if="isRemote" class="rules-tabs-viewport">
          <div class="rules-tabs-track" ref="tabsTrackRef">
            <button :ref="(el) => { if (ruleTab === 'subscription') ruleTabEl = el as HTMLElement | null }" class="rules-tab-btn" :class="{ active: ruleTab === 'subscription' }" @click="ruleTab = 'subscription'" title="显示当前工作配置中的规则；如果你直接编辑配置文件 rules，这里会同步变化。">当前规则</button>
            <button :ref="(el) => { if (ruleTab === 'add') ruleTabEl = el as HTMLElement | null }" class="rules-tab-btn" :class="{ active: ruleTab === 'add' }" @click="ruleTab = 'add'">附加规则</button>
            <button :ref="(el) => { if (ruleTab === 'delete') ruleTabEl = el as HTMLElement | null }" class="rules-tab-btn" :class="{ active: ruleTab === 'delete' }" @click="ruleTab = 'delete'">附加删除</button>
            <div class="rules-tab-slider" :class="{ animated: ruleSliderReady }" v-show="ruleSliderVisible" :style="ruleSliderStyle"></div>
          </div>
        </div>

        <div v-if="isRemote" class="rules-header-spacer"></div>

        <div class="search-bar" :class="{ compact: isRemote }">
          <span v-html="ICONS.search"></span>
          <input v-model="searchQuery" placeholder="搜索规则..." />
        </div>

        <button class="primary-btn header-action-btn" @click="showAddModal = true" :disabled="loading">
          <span class="btn-icon" v-html="ICONS.plus"></span> 添加规则
        </button>
      </div>

      <div class="rules-grid">
        <div v-for="rule in paginatedRules" :key="rule.originalIndex" class="rule-card">
          <div class="rule-main">
            <div class="rule-type tag-primary">{{ rule.type }}</div>
            <div class="rule-payload truncate" :title="rule.payload">{{ rule.payload }}</div>
          </div>
          <div class="rule-footer">
            <div class="rule-policy">{{ rule.policy }}</div>
            <button v-if="canDeleteRule" class="delete-btn" @click="handleDelete(rule.originalIndex)" :title="deleteBtnTitle">
              <span v-html="ICONS.trash"></span>
            </button>
          </div>
        </div>
        
        <div v-if="!loading && !hasFilteredRules" class="loading-state">
          {{ searchQuery ? '没有找到匹配的规则' : '暂无规则' }}
        </div>
      </div>

      <div class="pagination-bar" v-if="hasFilteredRules">
        <span class="page-info">共 {{ totalFilteredCount }} 条</span>
        
        <div class="pagination-controls">
          <button class="page-btn" @click="currentPage--" :disabled="currentPage <= 1">&lt; 上一页</button>
          <span class="page-status">{{ currentPage }} / {{ totalPages }}</span>
          <button class="page-btn" @click="currentPage++" :disabled="currentPage >= totalPages">下一页 &gt;</button>
        </div>

        <div class="tip-text">新规则自动置于首位</div>
      </div>
    </template>

    <Transition name="pop">
      <div v-if="showAddModal" class="modal-overlay" @click.self="showAddModal = false">
        <div class="custom-modal-card" @click.stop>
          <div class="modal-header">
            <h3>{{ modalTitle }}</h3>
          </div>
          <div class="modal-body">
            <p class="global-modal-msg">格式: 类型,目标,策略</p>
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
import { ref, shallowRef, onMounted, onUnmounted, onActivated, computed, watch, nextTick } from 'vue';
import * as API from '../../wailsjs/go/main/App';
import { showAlert, showConfirm, globalState } from '../store';
import { ICONS } from '../utils/icons';

const rulePageData = shallowRef<any>(null);
const ruleTab = ref<'subscription' | 'add' | 'delete'>(
  (localStorage.getItem('goclashz_ruleTab') as 'subscription' | 'add' | 'delete') || 'subscription'
);

const tabsTrackRef = ref<HTMLElement | null>(null);
const ruleTabEl = ref<HTMLElement | null>(null);
const ruleSliderStyle = ref({ left: '0px', width: '0px' });
const ruleSliderReady = ref(false);
const ruleSliderVisible = ref(false);

const updateRuleSlider = () => {
  const track = tabsTrackRef.value;
  const btn = ruleTabEl.value;
  if (track && btn) {
    ruleSliderStyle.value = {
      left: `${btn.offsetLeft}px`,
      width: `${btn.offsetWidth}px`,
    };
  }
};

const resetRuleSlider = () => {
  ruleSliderReady.value = false;
  ruleSliderVisible.value = false;
  nextTick(() => {
    updateRuleSlider();
    nextTick(() => {
      ruleSliderVisible.value = true;
      ruleSliderReady.value = true;
    });
  });
};

const searchQuery = ref('');
const debouncedQuery = ref('');
let searchTimer: ReturnType<typeof setTimeout>;

const showAddModal = ref(false);
const newRuleStr = ref('');
const loading = ref(false);

const currentPage = ref(1);
const pageSize = ref(42); 

const isRemote = computed(() => globalState.activeConfigType === 'remote');
const isLocal = computed(() => globalState.activeConfigType === 'local' || !globalState.activeConfigType);

const canDeleteRule = computed(() => {
  return true; // We allow deleting on all tabs (meaning is different though)
});

const deleteBtnTitle = computed(() => {
  if (isLocal.value) return '删除规则';
  if (ruleTab.value === 'subscription') return '屏蔽此规则';
  if (ruleTab.value === 'add') return '删除附加规则';
  if (ruleTab.value === 'delete') return '取消屏蔽 (恢复)';
  return '删除';
});

const modalTitle = computed(() => {
  if (isLocal.value) return '新增本地规则';
  if (ruleTab.value === 'add') return '新增附加规则';
  if (ruleTab.value === 'delete') return '新增要屏蔽的规则';
  return '新增规则';
});

const currentRulesList = computed(() => {
  if (!rulePageData.value) return [];
  if (isLocal.value) {
    return rulePageData.value.localRules || [];
  }
  switch (ruleTab.value) {
    case 'subscription': return rulePageData.value.subscriptionRules || [];
    case 'add': return rulePageData.value.addRules || [];
    case 'delete': return rulePageData.value.deleteRules || [];
  }
  return [];
});

onUnmounted(() => {
  if (searchTimer) clearTimeout(searchTimer);
});

watch(searchQuery, (newVal) => {
  clearTimeout(searchTimer);
  searchTimer = setTimeout(() => {
    debouncedQuery.value = newVal.toLowerCase().trim();
    currentPage.value = 1;
  }, 300);
});

const loadRules = async () => {
  if (!globalState.activeConfigId) return;
  loading.value = true;
  try {
    const data = await API.GetRulePageData(globalState.activeConfigId);
    rulePageData.value = data;
  } catch (e) {
    console.error("加载规则失败", e);
  } finally {
    loading.value = false;
  }
};

watch(() => globalState.activeConfigId, (newId, oldId) => {
  if (newId) {
    searchQuery.value = '';
    debouncedQuery.value = '';
    currentPage.value = 1;
    // 只有在切换到不同的配置文件时，才重置规则分类，否则保持用户记忆
    if (newId !== oldId && !oldId) {
       // First load
    }
    loadRules();
  } else {
    rulePageData.value = null;
  }
}, { immediate: true });

watch(isRemote, (newVal) => {
  if (newVal) {
    nextTick(() => {
      resetRuleSlider();
    });
  }
}, { immediate: true });

watch(ruleTab, (newVal) => {
  localStorage.setItem('goclashz_ruleTab', newVal);
  searchQuery.value = '';
  debouncedQuery.value = '';
  currentPage.value = 1;
  nextTick(updateRuleSlider);
});

const lowerCaseRulesCache = computed(() => {
  return currentRulesList.value.map((r: string) => r.toLowerCase());
});

const filteredIndices = computed(() => {
  const query = debouncedQuery.value;
  const indices: number[] = [];
  const rules = currentRulesList.value;
  const lowerCache = lowerCaseRulesCache.value;
  
  for (let i = 0; i < rules.length; i++) {
    if (!query || lowerCache[i].includes(query)) {
      indices.push(i);
    }
  }
  return indices;
});

const totalFilteredCount = computed(() => filteredIndices.value.length);
const hasFilteredRules = computed(() => totalFilteredCount.value > 0);
const totalPages = computed(() => Math.ceil(totalFilteredCount.value / pageSize.value) || 1);

watch(totalPages, (newTotal) => {
  if (currentPage.value > newTotal) {
    currentPage.value = newTotal;
  }
});

const paginatedRules = computed(() => {
  const start = (currentPage.value - 1) * pageSize.value;
  const end = start + pageSize.value;
  return filteredIndices.value.slice(start, end).map(index => {
    const text = currentRulesList.value[index];
    const parts = text.split(',');
    
    const ruleType = parts[0]?.trim().toUpperCase() || 'UNKNOWN';
    let payloadStr = '';
    let policyStr = '';

    if (ruleType === 'MATCH') {
      policyStr = parts.slice(1).join(', ').trim();
    } else {
      payloadStr = parts[1]?.trim() || '';
      policyStr = parts.slice(2).join(', ').trim();
    }

    return {
      type: ruleType,
      payload: payloadStr,
      policy: policyStr,
      originalIndex: index
    };
  });
});

const handleAdd = async () => {
  const ruleStr = newRuleStr.value.trim();
  if (!ruleStr || !globalState.activeConfigId) return;

  loading.value = true;
  try {
    let targetSection = 'local';
    if (isRemote.value) {
      targetSection = ruleTab.value; // 'subscription', 'add', or 'delete'
    }

    await API.AddRule(globalState.activeConfigId, targetSection, ruleStr);
    await loadRules();

    newRuleStr.value = '';
    showAddModal.value = false;
    searchQuery.value = '';
    currentPage.value = 1;
    await showAlert("规则已添加", "提示");
  } catch (e) {
    await showAlert(String(e), '添加失败');
  } finally {
    loading.value = false;
  }
};

const handleDelete = async (idx: number) => {
  let promptMsg = '确定要删除这条规则吗？';
  if (isRemote.value && ruleTab.value === 'subscription') {
    promptMsg = '屏蔽这条规则？（屏蔽后可到"附加删除"中恢复）';
  } else if (isRemote.value && ruleTab.value === 'delete') {
    promptMsg = '取消屏蔽这条规则吗？';
  }

  const ok = await showConfirm(promptMsg, '操作确认', true);
  if (ok && globalState.activeConfigId) {
    loading.value = true;
    try {
      let targetSection = 'local';
      if (isRemote.value) {
        targetSection = ruleTab.value;
      }
      
      await API.DeleteRule(globalState.activeConfigId, targetSection, idx);
      await loadRules();
    } catch (e) {
      await showAlert("操作失败: " + e, '错误');
    } finally {
      loading.value = false;
    }
  }
};

onMounted(() => {
  loadRules();
});

onActivated(() => {
  if (isRemote.value) {
    resetRuleSlider();
  }
});
</script>

<style scoped>
.rules-view { 
  display: flex; 
  flex-direction: column; 
  min-height: 100%; 
  overflow: visible;
}
.empty-state-view { flex: 1; display: flex; align-items: center; justify-content: center; color: var(--text-muted); font-size: 0.9rem; }

.rules-header { 
  display: flex; 
  align-items: center; 
  gap: 12px; 
  margin-bottom: 16px; 
  width: 100%; 
  padding: 4px 0 12px 0;
  background: transparent;
}

/* 选项卡样式 */
.rules-tabs-viewport {
  flex: 0 0 auto;
  min-width: 0;
  border-radius: 8px;
}

.rules-tabs-track {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  background: var(--surface-hover);
  padding: 6px;
  border-radius: 12px;
  user-select: none;
  -webkit-user-select: none;
  position: relative;
}

.rules-tab-btn {
  flex: 0 0 auto;
  display: flex;
  align-items: center;
  justify-content: center;
  height: 36px;
  padding: 0 16px;
  border: none;
  border-radius: 8px;
  background: transparent;
  color: var(--text-sub);
  font-size: 0.95rem;
  font-weight: 500;
  white-space: nowrap;
  cursor: pointer;
  position: relative;
  z-index: 1;
  transition: color 0.3s cubic-bezier(0.4, 0, 0.2, 1);
}

.rules-tab-btn:hover {
  color: var(--text-main);
}

.rules-tab-btn.active {
  color: var(--accent-fg);
  font-weight: 800;
}

.rules-tab-slider {
  position: absolute;
  top: 6px;
  height: 36px;
  background: var(--accent);
  border-radius: 8px;
  z-index: 0;
  pointer-events: none;
}

.rules-tab-slider.animated {
  transition: left 0.3s cubic-bezier(0.4, 0, 0.2, 1), width 0.3s cubic-bezier(0.4, 0, 0.2, 1);
}

.rules-header-spacer {
  flex: 1;
}

.search-bar.compact {
  flex: 0 0 280px;
  width: 280px;
}

.header-action-btn {
  padding: 0 14px !important;
}

.rules-header.page-sticky-mask {
  --sticky-mask-bleed: 2px;
}

.search-bar { display: flex; align-items: center; background: var(--surface); border: 1px solid var(--surface-hover); border-radius: 8px; padding: 8px 12px; flex: 1; }
.search-bar input { border: none; background: transparent; color: var(--text-main); outline: none; margin-left: 8px; width: 100%; }

.rules-grid { 
  flex: 1; 
  min-height: 0; 
  display: grid; 
  grid-template-columns: repeat(auto-fill, minmax(300px, 1fr)); 
  align-content: start; 
  gap: 16px; 
  overflow: visible; 
  padding-top: 4px;
  padding-bottom: 20px; 
}

.rule-card { 
  background: var(--surface); 
  border: none; 
  border-radius: 10px; 
  padding: 14px 16px; 
  display: flex; 
  flex-direction: column; 
  gap: 10px; 
  transition: background 0.2s; 
  height: 110px; 
  box-sizing: border-box;
  justify-content: space-between;
}
.rule-card:hover { background: var(--surface-hover); }
.rule-main { display: flex; flex-direction: column; gap: 8px; } 
.rule-type { font-size: 0.7rem; font-weight: 700; padding: 4px 8px; border-radius: 6px; width: fit-content; border: none; flex-shrink: 0; }
.tag-primary { background: var(--text-main); color: var(--surface); } 
.rule-payload { 
  font-size: 1rem; 
  color: var(--text-main); 
  font-weight: 600; 
  line-height: 1.4; 
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
  margin-top: auto; 
}

.page-info, .tip-text {
  flex: 1;
  font-size: 0.8rem;
  color: var(--text-sub);
  font-family: var(--font-mono);
}
.tip-text {
  text-align: right;
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
  border: none;
  color: var(--text-main);
  padding: 0 18px;
  height: 36px;
  border-radius: 8px;
  font-size: 0.85rem;
  font-weight: 600;
  cursor: pointer;
  transition: none;
  outline: none;
}

.page-btn:hover:not(:disabled) {
  background: var(--text-main);
  color: var(--surface);
}

.page-btn:active:not(:disabled) {
  transform: none;
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
