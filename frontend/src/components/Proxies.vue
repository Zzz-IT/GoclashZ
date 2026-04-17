<template>
  <div class="proxies-container">
    <div v-for="group in groups" :key="group.name" class="group-section">
      <h3>{{ group.name }}</h3>
      <div class="node-grid">
        <div v-for="node in group.proxies" :key="node.name"
             :class="['node-card', 'glass-card', { active: node.now === node.name }]"
             @click="selectNode(group.name, node.name)">
          <div class="node-info">
            <span class="node-name">{{ node.name }}</span>
            <span class="node-type">{{ node.type }}</span>
          </div>
          <div class="node-latency" :class="getLatencyClass(node.history)">
            {{ getLatency(node.history) }}ms
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue';
import { GetInitialData, SelectProxy } from '../../wailsjs/go/main/App';

const groups = ref<any[]>([]);

const loadNodes = async () => {
  const data = await GetInitialData();
  groups.value = data.groups || [];
};

const selectNode = async (group: string, node: string) => {
  await SelectProxy(group, node);
  loadNodes();
};

const getLatency = (history: any[]) => history && history.length > 0 ? history[history.length-1].delay : 'N/A';
const getLatencyClass = (history: any[]) => {
  const d = getLatency(history);
  if (d === 'N/A') return '';
  return d < 200 ? 'low' : d < 500 ? 'mid' : 'high';
};

onMounted(loadNodes);
</script>

<style scoped>
.group-section h3 { margin: 20px 0 12px; color: var(--text-main); font-size: 1.1rem; }
.node-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(200px, 1fr)); gap: 12px; }
.node-card { padding: 16px; cursor: pointer; display: flex; justify-content: space-between; align-items: center; transition: 0.2s; }
.node-card:hover { border-color: var(--accent); transform: scale(1.02); }
.node-card.active { border-color: var(--accent); background: rgba(79, 70, 229, 0.1); }
.node-name { display: block; font-weight: 600; font-size: 0.95rem; margin-bottom: 4px; }
.node-type { font-size: 0.75rem; color: var(--text-sub); }
.node-latency { font-size: 0.85rem; font-weight: bold; }
.node-latency.low { color: #10b981; }
.node-latency.high { color: #ef4444; }
</style>