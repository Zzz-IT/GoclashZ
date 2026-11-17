<!--
  空态块：图标 + 一句话 + 可选动作，替代原先的一行灰字。
  - icon 传 ICONS 里的 svg 字符串（直接 v-html），不传就不渲染图标节点；
  - hint 只有传了才渲染，避免空节点在 Lead 的样式里多占一行间距；
  - actionLabel 有值时才渲染按钮，点击 emit('action')，由使用方决定跳转。
  .empty-state-* / .action-btn 样式由全局 style.css 提供，本组件不新增 CSS。
-->
<template>
  <div class="empty-state-block">
    <div v-if="icon" class="empty-state-icon" v-html="icon"></div>
    <div class="empty-state-title">{{ title }}</div>
    <div v-if="hint" class="empty-state-hint">{{ hint }}</div>
    <button v-if="actionLabel" class="action-btn" @click="$emit('action')">{{ actionLabel }}</button>
  </div>
</template>

<script setup lang="ts">
defineProps<{
  icon?: string;
  title: string;
  hint?: string;
  actionLabel?: string;
}>();

defineEmits<{
  (e: 'action'): void;
}>();
</script>
