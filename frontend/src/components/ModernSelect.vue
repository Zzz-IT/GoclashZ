<template>
  <div class="modern-custom-select" :class="{ disabled, 'is-open': isOpen }" ref="selectRef">
    <div class="select-trigger" @click.stop="toggle">
      <span>{{ selectedLabel }}</span>
      <svg class="arrow" :class="{ 'arrow-up': isOpen }" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
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
          @click.stop="selectOption(opt.value)"
        >
          {{ opt.label }}
        </div>
      </div>
    </transition>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue';

const props = defineProps<{
  modelValue: string | number;
  options: { label: string; value: string | number }[];
  disabled?: boolean;
}>();

const emit = defineEmits(['update:modelValue', 'change']);

const isOpen = ref(false);
const selectRef = ref<HTMLElement | null>(null);

const selectedLabel = computed(() => {
  const match = props.options.find(o => o.value === props.modelValue);
  return match ? match.label : '请选择';
});

const toggle = () => {
  if (!props.disabled) isOpen.value = !isOpen.value;
};

const selectOption = (val: string | number) => {
  emit('update:modelValue', val);
  emit('change', val);
  isOpen.value = false;
};

// 🌟 核心重绘逻辑：自己接管“失去焦点”事件
const handleClickOutside = (event: MouseEvent) => {
  if (isOpen.value && selectRef.value && !selectRef.value.contains(event.target as Node)) {
    isOpen.value = false;
  }
};

onMounted(() => {
  // 组件挂载时，在整个窗口贴上“监听器”
  document.addEventListener('click', handleClickOutside);
});

onUnmounted(() => {
  // 组件销毁时记得撕掉监听器，防止内存泄漏
  document.removeEventListener('click', handleClickOutside);
});
</script>

<style scoped>
/* 1. 基础容器（无任何系统响应逻辑） */
.modern-custom-select {
  position: relative;
  width: 140px; 
  height: 36px;
  font-family: inherit;
  font-size: 0.85rem;
  font-weight: 500;
}

.modern-custom-select.w-full {
  width: 100%;
}

/* 2. 触发器（视觉主体） */
.select-trigger {
  display: flex;
  justify-content: space-between;
  align-items: center;
  color: var(--text-main);
  padding: 8px 12px;
  border-radius: 8px; 
  cursor: pointer;
  transition: background-color 0.2s ease;
  height: 100%;
  box-sizing: border-box;
  background-color: var(--surface-hover);
}

/* 悬停时不加高亮 (根据用户要求移除) */
/* .modern-custom-select:not(.disabled):hover .select-trigger { background-color: var(--surface-panel); } */


/* 点击展开时不加高亮 (根据用户要求移除) */
/* .modern-custom-select.is-open .select-trigger { background-color: var(--surface-panel); } */


/* 禁用状态 */
.modern-custom-select.disabled .select-trigger {
  opacity: 0.5;
  cursor: not-allowed;
  background-color: var(--surface);
}

/* 3. 动画与弹出菜单 */
.arrow {
  width: 16px;
  height: 16px;
  transition: transform 0.3s ease;
}
.arrow-up {
  transform: rotate(180deg);
}

.select-dropdown {
  position: absolute;
  top: calc(100% + 6px);
  left: 0;
  right: 0;
  background-color: var(--surface);
  border: 1px solid var(--glass-border);
  border-radius: 8px;
  box-shadow: none;
  z-index: 100;
  overflow: hidden;
  padding: 4px;
}

.select-option {
  padding: 8px 12px;
  color: var(--text-main);
  cursor: pointer;
  border-radius: 6px;
  transition: background 0.2s ease;
}

/* 悬停时不加高亮 (根据用户要求移除) */
/* .select-option:hover { background-color: var(--surface-hover); } */


.select-option.active {
  background-color: var(--surface-panel);
  color: var(--accent);
  font-weight: bold;
}

.fade-enter-active, .fade-leave-active { transition: opacity 0.2s, transform 0.2s; }
.fade-enter-from, .fade-leave-to { opacity: 0; transform: translateY(-5px); }
</style>
