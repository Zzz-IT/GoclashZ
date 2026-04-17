<template>
  <aside class="sidebar">
    <div class="logo">GoclashZ</div>
    <nav class="nav">
      <div v-for="item in menu" :key="item.id"
           :class="['nav-item', { active: activeId === item.id }]"
           @click="$emit('update:activeId', item.id)">
        <span class="icon">{{ item.icon }}</span>
        <span class="label">{{ item.label }}</span>
      </div>
    </nav>
  </aside>
</template>

<script setup lang="ts">
defineProps<{ activeId: string }>();
defineEmits(['update:activeId']);

const menu = [
  { id: 'home', label: '运行状态', icon: '📊' },
  { id: 'proxies', label: '节点列表', icon: '⚡' },
  { id: 'logs', label: '日志面板', icon: '📄' },
  { id: 'settings', label: '系统设置', icon: '⚙️' }
];
</script>

<style scoped>
.sidebar {
  width: 220px;
  background: var(--sidebar-bg);
  border-right: 1px solid var(--border-color);
  display: flex;
  flex-direction: column;
  padding: 24px 12px;
}
.logo {
  font-size: 1.25rem;
  font-weight: 800;
  color: var(--accent);
  margin-bottom: 32px;
  padding-left: 12px;
}
.nav-item {
  display: flex;
  align-items: center;
  padding: 12px 16px;
  border-radius: var(--radius-lg);
  cursor: pointer;
  color: var(--text-muted);
  transition: all 0.2s;
  margin-bottom: 4px;
}
.nav-item:hover { background: var(--accent-soft); color: var(--accent); }
.nav-item.active { background: var(--accent); color: white; font-weight: 600; }
.icon { margin-right: 12px; font-size: 1.2rem; }
</style>