<template>
  <div class="rules-view ui-page-shell">
    <div v-if="!globalState.activeConfigId" class="empty-state-view">
      <div class="empty-msg">{{ t('rules.noConfigWarning') }}</div>
    </div>
    <template v-else>
      <div class="rules-header ui-page-header">
        <div v-if="isRemote" class="rules-tabs-viewport">
          <div class="rules-tabs-track" ref="tabsTrackRef">
            <button :ref="(el) => { if (ruleTab === 'subscription') ruleTabEl = el as HTMLElement | null }" class="rules-tab-btn" :class="{ active: ruleTab === 'subscription' }" @click="ruleTab = 'subscription'" :title="t('rules.tabCurrentTooltip')">{{ t('rules.tabCurrent') }}</button>
            <button :ref="(el) => { if (ruleTab === 'add') ruleTabEl = el as HTMLElement | null }" class="rules-tab-btn" :class="{ active: ruleTab === 'add' }" @click="ruleTab = 'add'">{{ t('rules.tabAdd') }}</button>
            <button :ref="(el) => { if (ruleTab === 'delete') ruleTabEl = el as HTMLElement | null }" class="rules-tab-btn" :class="{ active: ruleTab === 'delete' }" @click="ruleTab = 'delete'">{{ t('rules.tabDelete') }}</button>
            <div class="rules-tab-slider" :class="{ animated: ruleSliderReady }" v-show="ruleSliderVisible" :style="ruleSliderStyle"></div>
          </div>
        </div>

        <div v-if="isRemote" class="rules-header-spacer"></div>

        <div class="search-bar" :class="{ compact: isRemote }">
          <span v-html="ICONS.search"></span>
          <input v-model="searchQuery" :placeholder="t('rules.searchPlaceholder')" />
        </div>

        <button class="primary-btn header-action-btn" @click="openAddModal" :disabled="loading">
          <span class="btn-icon" v-html="ICONS.plus"></span> {{ t('rules.addRuleBtn') }}
        </button>
      </div>

      <div
        class="rules-page-viewport"
        ref="pageViewportRef"
        :class="[edgeFadeAxis, { 'edge-fade-on': edgeFadeOn }]"
      >
        <Transition :name="pageTransitionName">
          <div class="rules-grid ui-page-body" :key="currentPage">
          <div v-for="rule in paginatedRules" :key="rule.originalIndex" class="rule-card ui-surface-card ui-hover-surface">
            <div class="rule-main">
              <div class="rule-type tag-primary">{{ rule.type }}</div>
              <div class="rule-payload" :title="rule.payload">{{ rule.payload }}</div>
            </div>
            <div class="rule-footer">
              <div class="rule-policy">{{ rule.policy }}</div>
              <!-- P-01：传规则文本而不是渲染时的 originalIndex，确认后按文本重新定位 -->
              <button class="delete-btn" @click="handleDelete(rule.text)" :disabled="loading" :title="deleteBtnTitle">
                <span v-html="ICONS.trash"></span>
              </button>
            </div>
          </div>

          <EmptyState
            v-if="!loading && !hasFilteredRules && searchQuery"
            :icon="ICONS.search"
            :title="t('rules.noMatchingRules')"
          />
          <EmptyState
            v-else-if="!loading && !hasFilteredRules"
            :icon="ICONS.rules"
            :title="t('rules.noRules')"
          />
        </div>
        </Transition>
      </div>

      <div class="pagination-bar" v-if="hasFilteredRules">
        <span class="page-info">{{ t('rules.totalFiltered', { count: totalFilteredCount }) }}</span>
        
        <div class="pagination-controls">
          <button class="page-btn" @click="goPage(-1)" :disabled="currentPage <= 1">{{ t('rules.prevPage') }}</button>
          <span class="page-status">{{ currentPage }} / {{ totalPages }}</span>
          <button class="page-btn" @click="goPage(1)" :disabled="currentPage >= totalPages">{{ t('rules.nextPage') }}</button>
        </div>

        <div class="tip-text">{{ t('rules.newRuleHint') }}</div>
      </div>
    </template>

    <Teleport to="body">
    <Transition name="pop">
      <div v-if="showAddModal" class="modal-overlay" @click.self="showAddModal = false">
        <div class="custom-modal-card" style="overflow: visible;" @click.stop>
          <div class="modal-header">
            <h3>{{ modalTitle }}</h3>
          </div>
          <div class="modal-body rule-form-body">
            <div class="rule-form-row">
              <label>{{ t('rules.formType') }}</label>
              <ModernSelect
                v-model="selectedRuleType"
                :options="ruleTypeOptions"
                class="w-full"
              />
            </div>

            <div v-if="needPayload" class="rule-form-row">
              <label>{{ currentRuleTypeMeta?.payloadLabel || t('rules.formPayload') }}</label>
              <input
                v-model="rulePayload"
                class="modal-input compact-rule-input"
                :placeholder="currentRuleTypeMeta?.payloadHint || 'example.com'"
                @keyup.enter="handleAddFromForm"
              />
            </div>

            <div v-if="needPolicy" class="rule-form-row">
              <label>{{ t('rules.formPolicy') }}</label>
              <ModernSelect
                v-model="selectedPolicy"
                :options="rulePolicyOptions"
                class="w-full"
              />
            </div>

            <div class="rule-preview">
              <span class="preview-label">{{ t('rules.formPreview') }}</span>
              <code>{{ rulePreview }}</code>
            </div>

            <div class="modal-footer">
              <button class="action-btn flex-1" @click="showAddModal = false">{{ t('common.cancel') }}</button>
              <button class="primary-btn accent-btn flex-1" @click="handleAddFromForm" :disabled="!canSubmitRule || loading">{{ t('rules.formSubmit') }}</button>
            </div>
          </div>
        </div>
      </div>
    </Transition>
    </Teleport>
  </div>
</template>

<script setup lang="ts">
import { ref, shallowRef, onUnmounted, computed, watch, nextTick } from 'vue';
import { App } from '../api';
import { showConfirm, showToast, globalState } from '../store';
import { ICONS } from '../utils/icons';
import { alertError } from '../utils/alertError';
import { safeGetString, safeSetString } from '../utils/storage';
import { useEdgeFade } from '../utils/edgeFade';
import { reportError } from '../utils/report';
import ModernSelect from './ModernSelect.vue';
import EmptyState from './EmptyState.vue';
import { t } from '../locales';

const rulePageData = shallowRef<any>(null);

/* P-34：localStorage 在 WebView2 隐私模式 / 配额异常时会直接抛错，
   而这行在 setup 顶层 —— 抛出去就是整页白屏，所以读失败一律回退默认段。
   R-1 等价重构：读原语改走 utils/storage（内部 try/catch + 返回 null，与原来的内联实现逐行等价）。 */
const readSavedRuleTab = (): 'subscription' | 'add' | 'delete' => {
  const saved = safeGetString('goclashz_ruleTab');
  if (saved === 'subscription' || saved === 'add' || saved === 'delete') return saved;
  return 'subscription';
};

const ruleTab = ref<'subscription' | 'add' | 'delete'>(readSavedRuleTab());

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
const loading = ref(false);
// P-23 / P-01：入口互斥标记（不依赖 :disabled 的重渲染时机）
let addingRule = false;
let deletingRule = false;

const ruleFormOptions = shallowRef<any>(null);
const selectedRuleType = ref('');
const rulePayload = ref('');
const selectedPolicy = ref('');

const ruleTypeOptions = computed(() => {
  return (ruleFormOptions.value?.types || []).map((t: any) => ({
    label: t.count ? `${t.label} (${t.count})` : t.label,
    value: t.value
  }));
});

const rulePolicyOptions = computed(() => {
  return ruleFormOptions.value?.policies || [];
});

const currentRuleTypeMeta = computed(() => {
  return ruleFormOptions.value?.types?.find((x: any) => x.value === selectedRuleType.value);
});

const needPayload = computed(() => currentRuleTypeMeta.value?.needPayload !== false);
const needPolicy = computed(() => currentRuleTypeMeta.value?.needPolicy !== false);

const rulePreview = computed(() => {
  const meta = currentRuleTypeMeta.value;
  if (!meta) return '';

  const parts = [selectedRuleType.value];

  if (meta.needPayload) {
    parts.push(rulePayload.value.trim() || `<${meta.payloadLabel || '内容'}>`);
  }

  if (meta.needPolicy) {
    parts.push(selectedPolicy.value || '<策略>');
  }

  return parts.join(',');
});

const canSubmitRule = computed(() => {
  const meta = currentRuleTypeMeta.value;
  if (!meta) return false;
  if (meta.needPayload && !rulePayload.value.trim()) return false;
  if (meta.needPolicy && !selectedPolicy.value) return false;
  return true;
});

const openAddModal = async () => {
  if (!globalState.activeConfigId) return;

  loading.value = true;
  try {
    const options = await App.GetRuleFormOptions(globalState.activeConfigId);
    ruleFormOptions.value = options;

    selectedRuleType.value = options.types?.[0]?.value || 'DOMAIN-SUFFIX';
    selectedPolicy.value =
      options.policies?.find((p: any) => p.value === 'DIRECT')?.value ||
      options.policies?.[0]?.value ||
      'DIRECT';

    rulePayload.value = '';
    showAddModal.value = true;
  } catch (e) {
    // F-A4-13：title 不再塞原始错误；人话进正文，原文进 detail（R-1 收敛到 alertError，逐项等价）
    await alertError(e, 'rules.loadOptionsError');
  } finally {
    loading.value = false;
  }
};

const currentPage = ref(1);
const pageSize = ref(42); 

/* 翻页复用整页切页那套横滑，方向由操作决定。
   两页重叠对滑（不设 mode）：这才是翻页器该有的样子，
   旧页向左退、新页从右进，中途两页同屏。
   裁切处的边缘淡化也一并复用 —— 淡化挂在 .rules-page-viewport 上，
   它只包住网格区，不含页头和分页条，所以那两处不会被一起糊掉。 */
const pageDir = ref<'next' | 'prev'>('next');
const pageTransitionName = computed(() =>
  pageDir.value === 'next' ? 'pane-left' : 'pane-right'
);

const pageViewportRef = ref<HTMLElement | null>(null);
const {
  axis: edgeFadeAxis,
  on: edgeFadeOn,
  begin: edgeFadeBegin,
} = useEdgeFade(() => pageViewportRef.value);

const goPage = (delta: number) => {
  const target = currentPage.value + delta;
  if (target < 1 || target > totalPages.value) return;
  pageDir.value = delta > 0 ? 'next' : 'prev';
  currentPage.value = target;
  edgeFadeBegin('edge-fade-x');
};

const isRemote = computed(() => globalState.activeConfigType === 'remote');
const isLocal = computed(() => globalState.activeConfigType === 'local' || !globalState.activeConfigType);

const deleteBtnTitle = computed(() => {
  if (isLocal.value) return t('rules.deleteBtnLocal');
  if (ruleTab.value === 'subscription') return t('rules.deleteBtnSub');
  if (ruleTab.value === 'add') return t('rules.deleteBtnAdd');
  if (ruleTab.value === 'delete') return t('rules.deleteBtnUnblock');
  return t('rules.deleteBtnDefault');
});

const modalTitle = computed(() => {
  if (isLocal.value) return t('rules.addModalTitleLocal');
  if (ruleTab.value === 'add') return t('rules.addModalTitleAdd');
  if (ruleTab.value === 'delete') return t('rules.addModalTitleDelete');
  return t('rules.addModalTitle');
});

/* 按「配置类型 + 段」取规则数组。抽出来是为了让「渲染用的列表」和「删除前重新读取
   用于定位的列表」走同一套取值逻辑，两处一旦分叉就会按错的段算下标。 */
const rulesOf = (data: any, localConfig: boolean, section: string): string[] => {
  if (!data) return [];
  if (localConfig) return data.localRules || [];
  switch (section) {
    case 'subscription': return data.subscriptionRules || [];
    case 'add': return data.addRules || [];
    case 'delete': return data.deleteRules || [];
  }
  return [];
};

const currentRulesList = computed(() => rulesOf(rulePageData.value, isLocal.value, ruleTab.value));

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
    const data = await App.GetRulePageData(globalState.activeConfigId);
    rulePageData.value = data;
  } catch (e) {
    reportError(e, 'rules.load');
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
  // R-1 等价重构：写原语改走 utils/storage（失败静默，与原来的 try/catch 等价）
  safeSetString('goclashz_ruleTab', newVal);
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

/** 按"顶层逗号"切分一条规则，括号内的逗号不算分隔符。
    AND / OR / NOT / SUB-RULE 的逻辑表达式本身带括号和逗号，例如
      AND,((NETWORK,UDP),(DST-PORT,443)),REJECT
    直接 split(',') 会切成 ["AND","((NETWORK","UDP)","(DST-PORT","443))","REJECT"]，
    payload 只剩 "((NETWORK"、剩下的全被当成 policy —— 就是规则卡显示异常的原因。 */
function splitRuleTopLevel(text: string): string[] {
  const out: string[] = [];
  let depth = 0;
  let cur = '';
  for (const ch of text) {
    if (ch === '(') depth++;
    else if (ch === ')') depth = Math.max(0, depth - 1);
    if (ch === ',' && depth === 0) {
      out.push(cur);
      cur = '';
      continue;
    }
    cur += ch;
  }
  out.push(cur);
  return out;
}

const paginatedRules = computed(() => {
  const start = (currentPage.value - 1) * pageSize.value;
  const end = start + pageSize.value;
  return filteredIndices.value.slice(start, end).map(index => {
    const text = currentRulesList.value[index];
    const parts = splitRuleTopLevel(text);

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
      originalIndex: index,
      // P-01：删除时按这条文本重新定位（index 只在渲染那一刻有效）
      text
    };
  });
});

const handleAddFromForm = async () => {
  /* P-23：:disabled 要等一次重渲染才生效，同帧「回车 + 点击」会发两次 AddRuleFromForm，
     得到两条一模一样的规则。所以入口用同步标记互斥，loading 也在这里同步置位。 */
  if (addingRule || loading.value) return;
  if (!globalState.activeConfigId || !canSubmitRule.value) return;

  addingRule = true;
  loading.value = true;
  try {
    let targetSection = 'local';
    if (isRemote.value) {
      targetSection = ruleTab.value; // 'subscription', 'add', or 'delete'
    }

    await App.AddRuleFromForm(globalState.activeConfigId, targetSection, {
      type: selectedRuleType.value,
      payload: rulePayload.value.trim(),
      policy: selectedPolicy.value,
    });
    
    await loadRules();

    rulePayload.value = '';
    showAddModal.value = false;
    searchQuery.value = '';
    currentPage.value = 1;
    // F-A4-12：成功用 toast
    showToast(t('rules.addSuccess'), 'success');
  } catch (e) {
    // R-1 等价重构：原 toUserError + displayErrorMessage + showAlert 模板收敛到 alertError
    await alertError(e, 'rules.addError');
  } finally {
    addingRule = false;
    loading.value = false;
  }
};

const handleDelete = async (ruleText: string) => {
  // P-01：删除期间互斥，避免同帧重复进入（也顺手挡住双击）
  if (deletingRule || loading.value) return;

  let promptMsg = t('rules.deleteConfirmMsg');
  if (isRemote.value && ruleTab.value === 'subscription') {
    promptMsg = t('rules.blockRuleConfirmMsg');
  } else if (isRemote.value && ruleTab.value === 'delete') {
    promptMsg = t('rules.unblockRuleConfirmMsg');
  }

  /* 评审修复：闸门必须早于确认框。模态是队列，双击删除按钮会排两个确认框、
     确认后连续调两次 DeleteRule（同样的 P-01/P-23 口径）。这里先占用闸门，
     确认结束就释放；真正的 RPC 互斥由下面的 loading 段继续负责。 */
  deletingRule = true;
  let confirmed = false;
  try {
    confirmed = await showConfirm(promptMsg, t('rules.deleteConfirmTitle'), true);
  } finally {
    deletingRule = false;
  }
  if (!confirmed) return;

  /* P-24：确认框是异步的，这里捕获「用户按下删除时看到的那份配置与段」，
     不再在确认后重读 globalState.activeConfigId —— 否则确认期间换了激活配置，
     就会删到新配置的同序号规则。 */
  const configId = globalState.activeConfigId;
  if (!configId) return;
  const localConfig = isLocal.value;
  const targetSection = isRemote.value ? ruleTab.value : 'local';

  deletingRule = true;
  loading.value = true;
  try {
    if (configId !== globalState.activeConfigId) {
      showToast(t('rules.configSwitched'), 'info');
      return;
    }
    /* P-01：不再复用渲染时的 originalIndex —— 确认期间列表一变（页内刷新/后台写者）就会删错，
       而后端对越界索引静默返回 nil（app.go:1125），错了也看不出来。
       这里重读当前段，按「规则文本」定位；找不到就提示并同步最新列表，绝不猜一个下标去删。 */
    const fresh = await App.GetRulePageData(configId);
    if (configId !== globalState.activeConfigId) {
      rulePageData.value = fresh;
      showToast(t('rules.configSwitched'), 'info');
      return;
    }
    rulePageData.value = fresh;
    const freshIdx = rulesOf(fresh, localConfig, targetSection).indexOf(ruleText);
    if (freshIdx < 0) {
      showToast(t('rules.ruleChanged'), 'info');
      return;
    }

    await App.DeleteRule(configId, targetSection, freshIdx);
    await loadRules();
  } catch (e) {
    // R-1 等价重构：原 toUserError + displayErrorMessage + showAlert 模板收敛到 alertError
    await alertError(e, 'rules.deleteError');
  } finally {
    deletingRule = false;
    loading.value = false;
  }
};

/* P-22：这里原来还有一个 onMounted(loadRules)。上面 watch(activeConfigId, {immediate:true})
   在挂载时已经拉过一次，两处叠加会让每次进页都发两次 GetRulePageData（并发且无序号，
   loading 只反映后完成的那次），所以删掉这一处。 */

/* P-13（剩余部分）：这里原来有 onActivated(() => resetRuleSlider())。本应用没有 <KeepAlive>，
   该钩子永不触发（Rules 页面是 v-if 整块销毁重建），属死代码，删除以消除误导。
   真正的滑块归位由 onMounted 与 updateSlider 负责。 */
</script>

<style scoped>
.rules-view { 
  display: flex; 
  flex-direction: column; 
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
  border-radius: var(--ui-radius-sm);
}

.rules-tabs-track {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  background: var(--ui-surface-hover);
  padding: 6px;
  border-radius: var(--ui-radius-lg);
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
  border-radius: var(--ui-radius-sm);
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
  color: var(--ui-accent-fg);
  font-weight: 800;
}

.rules-tab-slider {
  position: absolute;
  top: 6px;
  height: 36px;
  background: var(--ui-accent-block);
  border-radius: var(--ui-radius-sm);
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


.search-bar { display: flex; align-items: center; background: var(--surface); border: 1px solid var(--ui-border-subtle); border-radius: var(--ui-radius-sm); padding: 8px 12px; flex: 1; }
.search-bar input { border: none; background: transparent; color: var(--text-main); outline: none; margin-left: 8px; width: 100%; }

/* 唯一滚动区：规则网格自己滚动，页头与分页条固定 */
.rules-grid { 
  flex: 1; 
  min-height: 0; 
  display: grid; 
  grid-template-columns: repeat(auto-fill, minmax(300px, 1fr)); 
  align-content: start; 
  gap: 16px; 
  padding-top: 4px;
  padding-bottom: 20px; 
}

/* 翻页视口：只包住网格区，作为滑动的裁切容器、也是边缘淡化的承载层。
   页头与分页条在它外面，所以不会被一起淡化。 */
.rules-page-viewport {
  position: relative;
  flex: 1;
  min-height: 0;
  overflow: hidden;
  /* 翻页比整页切页快一档：连翻多页时 0.5s 一次太拖。
     useEdgeFade 会从这个元素读时长，淡化收尾因此和位移对齐。 */
  --pane-duration: 0.34s;
}

.rules-page-viewport > .rules-grid {
  height: 100%;
  box-sizing: border-box;
}

/* 过渡期两页并存，必须脱流叠放：否则它们会平分 flex 高度，各剩半屏 */
.rules-page-viewport > .pane-left-leave-active,
.rules-page-viewport > .pane-left-enter-active,
.rules-page-viewport > .pane-right-leave-active,
.rules-page-viewport > .pane-right-enter-active {
  position: absolute;
  inset: 0;
}

/* 必须在这里重新声明一次 transform 过渡。
   .rules-grid 同时是 .ui-page-body，而 layout.css 里的
   `.ui-page-body { transition: --ui-fade-* }` 在 main.ts 中晚于 style.css 加载，
   同特异性下会把 style.css 里 .pane-*-active 的 `transition: transform` 压掉 ——
   表现就是"翻页完全不动"。这里用 .rules-page-viewport > 提高特异性拿回来。 */
.rules-page-viewport > .pane-left-enter-active,
.rules-page-viewport > .pane-left-leave-active,
.rules-page-viewport > .pane-right-enter-active,
.rules-page-viewport > .pane-right-leave-active {
  transition: transform var(--pane-duration) var(--pane-ease);
}

.rule-card { 
  border: none; 
  padding: 14px 16px; 
  display: flex; 
  flex-direction: column; 
  gap: 10px; 
  transition: background-color 0.2s; 
  height: 110px; 
  box-sizing: border-box;
  justify-content: space-between;
}
.rule-main { display: flex; flex-direction: column; gap: 8px; } 
.rule-type { font-size: 0.7rem; font-weight: 700; padding: 4px 8px; border-radius: 6px; width: fit-content; border: none; flex-shrink: 0; }
.tag-primary { background: var(--ui-accent-block); color: var(--ui-accent-fg); } 
.rule-payload { 
  font-size: 1rem; 
  color: var(--text-main); 
  font-weight: 600; 
  line-height: 1.4; 
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.rule-footer { display: flex; justify-content: space-between; align-items: center; gap: 8px; margin-top: auto; }
/* 必须截断：规则被错误解析时 policy 可能是一长串，会撑破固定高度的卡片。
   min-width: 0 让 flex 子项能被压缩，否则 text-overflow 不生效。 */
.rule-policy {
  font-size: 0.75rem;
  color: var(--text-sub);
  font-weight: 600;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.delete-btn { background: transparent; color: var(--danger); border: none; cursor: pointer; opacity: 0; transition: opacity 0.2s; padding: 4px; }
.rule-card:hover .delete-btn { opacity: 1; }

/* R-1：原 .loading-state 规则已删 —— 模板里零引用（grep 'loading-state' 仅命中此行），
   加载态实际由 EmptyState / 卡片骨架承载，该选择器永不匹配。 */

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
  border-radius: var(--ui-radius-sm);
  font-size: 0.85rem;
  font-weight: 600;
  cursor: pointer;
  transition: none;
  outline: none;
}

.page-btn:hover:not(:disabled) {
  background: var(--ui-accent-block);
  color: var(--ui-accent-fg);
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

.rule-form-body {
  display: flex;
  flex-direction: column;
  gap: 14px;
}

.rule-form-row {
  display: grid;
  grid-template-columns: 72px 1fr;
  align-items: center;
  gap: 12px;
}

.rule-form-row label {
  font-size: 0.85rem;
  color: var(--text-sub);
  font-weight: 700;
}



.compact-rule-input {
  height: 38px;
}

.rule-preview {
  padding: 10px 12px;
  border-radius: var(--ui-radius-sm);
  background: var(--surface);
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.preview-label {
  font-size: 0.72rem;
  color: var(--text-muted);
  font-weight: 700;
}

.rule-preview code {
  font-family: var(--font-mono);
  font-size: 0.78rem;
  color: var(--text-main);
  word-break: break-all;
}

/* 空态块放在规则网格里，必须占满整行，否则会被当成一个 300px 的格子 */
.rules-grid :deep(.empty-state-block) {
  grid-column: 1 / -1;
  /* 网格里没有 flex 撑开，给一个和列表页气质一致的最小高度，避免空态挤在顶部 */
  min-height: 320px;
}
</style>
