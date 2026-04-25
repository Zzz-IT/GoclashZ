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
        
        <div v-if="!loading && !hasFilteredRules" class="loading-state">
          {{ searchQuery ? '没有找到匹配的规则' : '暂无规则，点击上方按钮添加' }}
        </div>
      </div>

      <div class="pagination-bar" v-if="hasFilteredRules">
        <span class="page-info">共 {{ totalFilteredCount }} 条</span>
        
        <div class="pagination-controls">
          <button 
            class="page-btn" 
            @click="currentPage--" 
            :disabled="currentPage <= 1"
          >
            &lt; 上一页
          </button>
          
          <span class="page-status">
            {{ currentPage }} / {{ totalPages }}
          </span>
          
          <button 
            class="page-btn" 
            @click="currentPage++" 
            :disabled="currentPage >= totalPages"
          >
            下一页 &gt;
          </button>
        </div>

        <div class="tip-text">新规则自动置于首位</div>
      </div>
    </template>

    <Transition name="pop">
      <div v-if="showAddModal" class="modal-overlay" @click.self="showAddModal = false">
        <div class="custom-modal-card" @click.stop>
          <div class="modal-header">
            <h3>新增分流规则</h3>
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
import { ref, shallowRef, onMounted, onUnmounted, computed, watch } from 'vue';
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
const pageSize = ref(42); 

// 新增：组件卸载清理
onUnmounted(() => {
  if (searchTimer) clearTimeout(searchTimer);
});

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
  if (newId) {
    searchQuery.value = '';
    debouncedQuery.value = '';
    currentPage.value = 1;
    loadRules();
  } else {
    userRules.value = [];
  }
}, { immediate: true });

// 🚀 新增：静态缓存全小写规则，生命周期内只在 userRules 变动时执行 1 次！
// 彻底杜绝搜索过程中的瞬时大量小写字符串分配，消除 GC 抖动
const lowerCaseRulesCache = computed(() => {
  return userRules.value.map(r => r.toLowerCase());
});

// 🚀 核心优化：只过滤索引，不生成临时对象 (O(1)级开销)
const filteredIndices = computed(() => {
  const query = debouncedQuery.value;
  const indices: number[] = [];
  const rules = userRules.value;
  const lowerCache = lowerCaseRulesCache.value; // 借用预计算的小写缓存
  
  for (let i = 0; i < rules.length; i++) {
    // 零瞬时内存分配：直接在已存在的缓存小写字符串上比对
    if (!query || lowerCache[i].includes(query)) {
      indices.push(i);
    }
  }
  return indices;
});

// 新增：状态标志供模板使用
const totalFilteredCount = computed(() => filteredIndices.value.length);
const hasFilteredRules = computed(() => totalFilteredCount.value > 0);
const totalPages = computed(() => Math.ceil(totalFilteredCount.value / pageSize.value) || 1);

// 越界保护：当删除数据导致总页数缩减时，自动修正当前页码
watch(totalPages, (newTotal) => {
  if (currentPage.value > newTotal) {
    currentPage.value = newTotal;
  }
});

// 🚀 核心优化：仅将当前页的数据实例化为 Object，极致节省内存
const paginatedRules = computed(() => {
  const start = (currentPage.value - 1) * pageSize.value;
  const end = start + pageSize.value;
  return filteredIndices.value.slice(start, end).map(index => {
    const text = userRules.value[index];
    const parts = text.split(',');
    
    const ruleType = parts[0]?.trim().toUpperCase() || 'UNKNOWN';
    let payloadStr = '';
    let policyStr = '';

    // 🛡️ 修复：MATCH 规则没有 Payload，第二段直接就是 Policy
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
    // 1. 直接将原始字符串扔给后端，由 Go 进行绝对校验和清洗
    const newList = [ruleStr, ...userRules.value];
    await API.SaveCustomRules(globalState.activeConfigId, newList);
    
    // 2. 只有后端没有抛出错误（校验通过并落盘成功），才更新前端视图
    userRules.value = newList;
    newRuleStr.value = '';
    showAddModal.value = false;
    
    searchQuery.value = '';
    currentPage.value = 1;
    await showAlert("规则已添加并保存", "提示");
  } catch (e) {
    // 3. 拦截后端的 fmt.Errorf 并直接展示给用户
    await showAlert(String(e), '格式校验失败');
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
      currentPage.value = 1;
      searchQuery.value = '';
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

.add-rule-btn { 
  background: var(--accent); 
  color: var(--accent-fg); 
  border: none; 
  height: 36px;
  padding: 0 16px; 
  border-radius: 8px; 
  font-weight: 600; 
  cursor: pointer; 
  transition: 0.2s; 
  flex-shrink: 0; 
}
.add-rule-btn:hover { filter: brightness(0.9); }

.sync-btn {
  background: var(--accent);
  color: var(--accent-fg);
  height: 36px;
  padding: 0 16px;
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
  padding-right: 8px; /* 🚀 统一间距：与代理节点页保持一致 */
  padding-bottom: 20px; 
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
  padding: 0 18px;
  height: 36px;
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
