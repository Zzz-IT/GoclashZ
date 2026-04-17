<template>
  <div class="proxies-view">

    <div class="action-bar glass-panel">
      <div class="sub-input-group">
        <span class="icon-wrap" v-html="ICONS.link"></span>
        <input
          v-model="subUrl"
          type="text"
          placeholder="ENTER YAML SUBSCRIPTION URL..."
          class="sub-input"
          @keyup.enter="handleUpdateSub"
        />
        <button class="minimal-btn" @click="handleUpdateSub" :disabled="isUpdating">
          {{ isUpdating ? 'SYNCING' : 'UPDATE' }}
        </button>
      </div>

      <div class="global-actions">
        <button class="primary-action-btn" @click="testAllDelays" :disabled="isTesting">
          <span class="btn-icon" v-html="ICONS.zap"></span>
          {{ isTesting ? 'TESTING' : 'LATENCY TEST' }}
        </button>
      </div>
    </div>

    <div class="scroll-content">
      <div v-for="group in localGroups" :key="group.name" class="group-box">
        <div class="group-header">
          <div class="group-info">
            <span class="micro-title">Proxy Group</span>
            <h3 class="group-title">{{ group.name }}</h3>
          </div>
          <div class="group-stats font-mono">
            {{ group.proxies?.length || 0 }} NODES
          </div>
        </div>

        <div class="node-grid">
          <div v-for="node in group.proxies" :key="node.name"
               :class="['node-item', { active: node.now === node.name }]"
               @click="selectNode(group.name, node.name)">

            <div class="node-main">
              <div class="n-name-wrapper">
                <span class="n-name" :title="node.name">{{ node.name }}</span>
              </div>
              <span class="n-type font-mono">{{ node.type }}</span>
            </div>

            <div class="n-latency-box">
              <span :class="['n-delay font-mono', getDelayColorClass(node.delay)]">
                {{ node.delay ? (node.delay > 0 ? `${node.delay}ms` : 'TIMEOUT') : '---' }}
              </span>
            </div>

          </div>
        </div>
      </div>
    </div>

  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted } from 'vue';
import * as API from '../../wailsjs/go/main/App';
import { EventsOn, EventsOff } from '../../wailsjs/runtime/runtime';

// 极简 Lucide 图标集
const ICONS = {
  zap: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polygon points="13 2 3 14 12 14 11 22 21 10 12 10 13 2"></polygon></svg>`,
  link: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M10 13a5 5 0 0 0 7.54.54l3-3a5 5 0 0 0-7.07-7.07l-1.72 1.71"></path><path d="M14 11a5 5 0 0 0-7.54-.54l-3 3a5 5 0 0 0 7.07 7.07l1.71-1.71"></path></svg>`
};

const localGroups = ref<any[]>([]);
const subUrl = ref('');
const isUpdating = ref(false);
const isTesting = ref(false);

// 核心数据加载逻辑
// 修改 loadData 逻辑，实现“工业级”精准过滤
const loadData = async () => {
  try {
    const data: any = await API.GetInitialData();
    if (data && data.groups) {
      const processedGroups: any[] = [];
      const allProxyKeys = Object.keys(data.groups);

      allProxyKeys.forEach(name => {
        const item = data.groups[name];

        // ✨ 核心过滤逻辑：
        // 1. 只显示 Selector（手动选择）、URLTest（自动测速）、Fallback（故障转移）类型的代理组
        // 2. 排除系统保留关键字：GLOBAL, DIRECT, REJECT
        const isGroupType = ['Selector', 'URLTest', 'Fallback', 'LoadBalance'].includes(item.type);
        const isSystemReserved = ['GLOBAL', 'DIRECT', 'REJECT'].includes(name);

        if (isGroupType && !isSystemReserved) {
          // 对组内的每个成员进行信息补全（从全局 map 中找其延迟和真实类型）
          const proxies = (item.all || []).map((memberName: string) => {
            const memberDetail = data.groups[memberName] || {};
            return {
              name: memberName,
              type: memberDetail.type || 'Proxy',
              now: item.now, // 记录该组成员当前是否被选中
              // 提取历史延迟中的最后一个数值
              delay: (memberDetail.history && memberDetail.history.length > 0)
                ? memberDetail.history[memberDetail.history.length - 1].delay
                : null
            };
          });

          processedGroups.push({
            name: name,
            type: item.type,
            now: item.now,
            proxies: proxies
          });
        }
      });
      localGroups.value = processedGroups;
    }
  } catch (e) {
    console.error("DATA FETCH FAILED", e);
  }
};

// 修改 selectNode，增加“非 Selector 组不可手动选择”的逻辑
const selectNode = async (groupName: string, nodeName: string) => {
  const targetGroup = localGroups.value.find(g => g.name === groupName);

  // 只有 Selector 类型的组才允许手动切换节点
  if (targetGroup && targetGroup.type !== 'Selector') {
    console.warn("Manual selection is not allowed for auto-mode groups.");
    return;
  }

  try {
    await API.SelectProxy(groupName, nodeName);
    // 乐观 UI 更新
    if (targetGroup) {
      targetGroup.now = nodeName;
      targetGroup.proxies.forEach((p: any) => p.now = nodeName);
    }
  } catch (e) {
    alert("SWITCH FAILED: " + e);
  }
};

const handleUpdateSub = async () => {
  if (!subUrl.value) return;
  isUpdating.value = true;
  try {
    await API.UpdateSub(subUrl.value);
    await loadData();
  } catch (err) {
    alert("SYNC ERROR: " + err);
  } finally {
    isUpdating.value = false;
  }
};

// 触发高并发并发测速
const testAllDelays = () => {
  isTesting.value = true;
  const allNodeNames = new Set<string>();

  localGroups.value.forEach(g => {
    g.proxies.forEach((n: any) => {
      allNodeNames.add(n.name);
      n.delay = null; // 重置视觉反馈
    });
  });

  const nodesArray = Array.from(allNodeNames);
  if (nodesArray.length > 0) {
    API.TestAllProxies(nodesArray);
  }

  // 信号结束后 8 秒自动解锁按钮
  setTimeout(() => { isTesting.value = false; }, 8000);
};

const getDelayColorClass = (delay: number | null) => {
  if (delay === null) return '';
  if (delay === 0) return 't-fail';
  if (delay < 250) return 't-fast';
  if (delay < 600) return 't-mid';
  return 't-slow';
};

onMounted(async () => {
  await loadData();

  // ⚡️ 瀑布流核心：监听单个节点测速完成的事件
  EventsOn("proxy-delay-update", (data: any) => {
    localGroups.value.forEach(g => {
      const node = g.proxies.find((n: any) => n.name === data.name);
      if (node) node.delay = data.delay;
    });
  });
});

onUnmounted(() => {
  EventsOff("proxy-delay-update");
});
</script>

<style scoped>
.proxies-view { display: flex; flex-direction: column; height: 100%; overflow: hidden; }

/* 动作栏：Zinc 风格极简 */
.action-bar {
  display: flex; justify-content: space-between; align-items: center;
  padding: 14px 20px; margin-bottom: 24px;
  background: var(--surface); border-radius: 10px;
}
.sub-input-group { display: flex; align-items: center; flex: 1; max-width: 500px; gap: 14px; }
.icon-wrap { width: 14px; height: 14px; color: var(--text-muted); }
.sub-input {
  flex: 1; background: transparent; border: none; outline: none;
  color: var(--text-main); font-size: 0.75rem; font-family: var(--font-mono);
  letter-spacing: 0.05em;
}
.sub-input::placeholder { color: var(--text-muted); opacity: 0.5; }

/* 按钮：高冷去色设计 */
.minimal-btn {
  padding: 6px 12px; background: transparent; border: 1px solid var(--glass-border);
  color: var(--text-main); font-size: 0.7rem; font-weight: 600; border-radius: 4px;
  cursor: pointer; transition: 0.2s;
}
.minimal-btn:hover:not(:disabled) { background: var(--text-main); color: var(--accent-fg); }

.primary-action-btn {
  display: flex; align-items: center; gap: 8px;
  padding: 8px 18px; border: none; background: var(--text-main);
  color: var(--accent-fg); font-size: 0.75rem; font-weight: 700;
  border-radius: 6px; cursor: pointer; transition: 0.2s;
}
.primary-action-btn:hover:not(:disabled) { opacity: 0.8; transform: translateY(-1px); }
.btn-icon { width: 12px; height: 12px; }

/* 列表容器 */
.scroll-content { flex: 1; overflow-y: auto; padding-right: 6px; }
.group-box { margin-bottom: 40px; }
.group-header {
  display: flex; justify-content: space-between; align-items: flex-end;
  margin-bottom: 16px; padding-bottom: 10px; border-bottom: 1px solid var(--glass-border);
}
.group-title { font-size: 1.1rem; font-weight: 500; color: var(--text-main); margin-top: 4px; }
.group-stats { font-size: 0.7rem; color: var(--text-muted); }

/* 节点卡片：呼吸感留白 */
.node-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(240px, 1fr)); gap: 10px; }
.node-item {
  display: flex; justify-content: space-between; align-items: center;
  padding: 14px 18px; border: 1px solid var(--glass-border); border-radius: 8px;
  cursor: pointer; background: var(--glass-panel); transition: 0.2s cubic-bezier(0.4, 0, 0.2, 1);
}
.node-item:hover { border-color: var(--text-muted); transform: translateY(-1px); }
.node-item.active { border-color: var(--text-main); border-width: 1.5px; background: var(--surface-hover); }

.node-main { display: flex; flex-direction: column; gap: 4px; overflow: hidden; flex: 1; }
.n-name { font-size: 0.85rem; font-weight: 500; color: var(--text-main); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.n-type { font-size: 0.6rem; color: var(--text-muted); letter-spacing: 0.05em; }

.n-latency-box { text-align: right; min-width: 60px; }
.n-delay { font-size: 0.75rem; font-weight: 500; }

/* 高级感配色逻辑：极简点缀 */
.t-fast { color: #10b981; }
.t-mid { color: #f59e0b; }
.t-slow { color: #ef4444; }
.t-fail { color: var(--text-muted); text-decoration: line-through; opacity: 0.5; }
</style>