<template>
  <div class="modern-custom-select" :class="{ disabled }" :tabindex="disabled ? -1 : 0" @blur="close">
    <div class="select-trigger" @click="toggle">
      <span>{{ selectedLabel }}</span>
      <svg class="arrow" :class="{ 'arrow-up': isOpen }" viewBox="0 0 24 24" fill="none" stroke="#777" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
        <polyline points="6 9 12 15 18 9"></polyline>
      </svg>
    </div>

    <transition name="fade">
      <div class="select-dropdown" v-show="isOpen">
        <div 
          v-for="opt in options" 
          :key="opt.value" 
          class="select-option"
          :class="{ active: opt.value === modelValue }"
          @click="selectOption(opt.value)"
        >
          {{ opt.label }}
        </div>
      </div>
    </transition>
  </div>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue';

const props = defineProps<{
  modelValue: string | number;
  options: { label: string; value: string | number }[];
  disabled?: boolean;
}>();

const emit = defineEmits(['update:modelValue', 'change']);

const isOpen = ref(false);

const selectedLabel = computed(() => {
  const match = props.options.find(o => o.value === props.modelValue);
  return match ? match.label : '请选择';
});

const toggle = () => {
  if (!props.disabled) isOpen.value = !isOpen.value;
};

const close = () => {
  isOpen.value = false;
};

const selectOption = (val: string | number) => {
  emit('update:modelValue', val);
  emit('change', val);
  close();
};
</script>

<style scoped>
/* 容器体：相对定位 */
.modern-custom-select {
  position: relative;
  width: 140px; /* 你可以根据需要调整宽度 */
  font-family: inherit;
  font-size: 0.9rem;
  outline: none;
}

/* 触发器（即原来的框） */
.select-trigger {
  display: flex;
  justify-content: space-between;
  align-items: center;
  background-color: var(--surface-hover);
  color: var(--text-main);
  padding: 8px 12px;
  border-radius: 8px; /* 完美的圆角 */
  cursor: pointer;
  transition: all 0.2s ease;
  /* 彻底没有边框 */
}

.modern-custom-select:hover:not(.disabled) .select-trigger {
  background-color: var(--surface-panel);
}

.modern-custom-select:focus:not(.disabled) .select-trigger {
  background-color: var(--surface);
  box-shadow: inset 0 0 0 1px var(--text-sub); /* 用内阴影代替边框，防止抖动 */
}

/* 禁用状态 */
.modern-custom-select.disabled .select-trigger {
  opacity: 0.5;
  cursor: not-allowed;
}

/* 纯代码 SVG 箭头的翻转动画 */
.arrow {
  width: 16px;
  height: 16px;
  transition: transform 0.3s ease;
}
.arrow-up {
  transform: rotate(180deg);
}

/* 悬浮的下拉菜单 */
.select-dropdown {
  position: absolute;
  top: calc(100% + 6px);
  left: 0;
  right: 0;
  background-color: var(--surface);
  border: 1px solid var(--glass-border);
  border-radius: 8px;
  box-shadow: 0 10px 25px rgba(0,0,0,0.4);
  z-index: 100;
  overflow: hidden;
  padding: 4px;
}

/* 菜单选项 */
.select-option {
  padding: 8px 12px;
  color: var(--text-main);
  cursor: pointer;
  border-radius: 6px;
  transition: background 0.2s ease;
}

.select-option:hover {
  background-color: var(--surface-hover);
}

.select-option.active {
  background-color: var(--surface-panel);
  color: var(--accent); /* 高亮选中项 */
  font-weight: bold;
}

/* 优雅的淡入淡出动画 */
.fade-enter-active, .fade-leave-active { transition: opacity 0.2s, transform 0.2s; }
.fade-enter-from, .fade-leave-to { opacity: 0; transform: translateY(-5px); }
</style>
