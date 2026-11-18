<template>
  <div class="modern-custom-select" :class="{ disabled, 'is-open': isOpen }" ref="selectRef">
    <div class="select-trigger" @click.stop="toggle">
      <span>{{ selectedLabel }}</span>
      <svg class="arrow" :class="{ 'arrow-up': isOpen }" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
        <polyline points="6 9 12 15 18 9"></polyline>
      </svg>
    </div>

    <Teleport to="body">
      <transition name="fade">
        <div
          ref="dropRef"
          class="select-dropdown"
          :class="{ 'drop-up': dropUp }"
          :style="dropStyle"
          v-show="isOpen"
        >
        <div 
          v-for="opt in options" 
          :key="opt.value" 
          class="select-option"
          :class="{ active: opt.value === modelValue, 'has-description': opt.description }"
          @click.stop="selectOption(opt.value)"
        >
          <div class="option-label">{{ opt.label }}</div>
          <div v-if="opt.description" class="option-description">{{ opt.description }}</div>
        </div>
        </div>
      </transition>
    </Teleport>
  </div>
</template>

<script lang="ts">
import { ref } from 'vue';

let globalSelectCounter = 0;
// 全局/页面共享的当前激活展开的下拉框实例 ID
const activeSelectId = ref<number | null>(null);
</script>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, watch } from 'vue';

const props = defineProps<{
  modelValue: string | number;
  options: { label: string; value: string | number; description?: string }[];
  disabled?: boolean;
}>();

const emit = defineEmits(['update:modelValue', 'change']);

// 自动修正非法值
/* G-08：纠错只改显示值，绝不能再 emit('change') —— 父级普遍把 @change 当"用户改了 → 立刻落盘"，
   后端只要返回一个不在选项表里的值（如手改过的 appLogLevel="warning"），页面一打开就会
   把第一个选项静默写盘。change 只在用户真正点选时由 selectOption() 发出。 */
watch(() => props.modelValue, (newVal) => {
  const match = props.options.find(o => o.value === newVal);
  /* 评审修复：只在「值根本没设置」时补第一项。
     对「有值但不在选项表里」的真实值（典型例子：手改过 user_behavior.json 的
     appLogLevel=warning）不再静默改写 —— 改写会把这个真实值变成父级的脏字段，
     下一次任意保存就把它落盘丢掉（G-08 只解决了不 emit change，没解决这一层）。 */
  const isEmpty = newVal === '' || newVal === null || newVal === undefined;
  if (!match && isEmpty && props.options.length > 0) {
    emit('update:modelValue', props.options[0].value);
  }
}, { immediate: true });

const instanceId = ++globalSelectCounter;
const isOpen = computed(() => activeSelectId.value === instanceId);
const selectRef = ref<HTMLElement | null>(null);
const dropRef = ref<HTMLElement | null>(null);
/* 浮层整体 teleport 到 body：它必须脱离页面里的层叠上下文，
   否则 backdrop-filter 的取样范围只到卡片这一层，背后其它行的内容永远
   不会被糊到（表现为"只有半透明填充、没有模糊"）。
   离开文档流后定位改为 fixed，坐标由触发器实时算出。 */
const dropStyle = ref<Record<string, string>>({});

const updatePosition = () => {
  const el = selectRef.value;
  if (!el) return;
  const rect = el.getBoundingClientRect();
  const gap = 6;
  const root = el.closest('.modern-custom-select') as HTMLElement | null;
  const wFull = !!root && root.classList.contains('w-full');
  const style: Record<string, string> = {
    position: 'fixed',
    right: Math.max(0, window.innerWidth - rect.right) + 'px',
    left: 'auto',
    // 必须高于 .modal-overlay(9999)：弹窗里的策略选择器也要能盖住遮罩
    zIndex: '10001',
    maxHeight: dropMaxHeight.value + 'px',
  };
  // 定位基准从"父元素的 100%"变成视口，宽度必须显式写回，
  // 否则 min-width:100% 会按整屏算。
  if (wFull) {
    style.width = rect.width + 'px';
    style.maxWidth = rect.width + 'px';
  } else {
    style.minWidth = rect.width + 'px';
    style.maxWidth = '340px';
  }
  if (dropUp.value) {
    style.top = 'auto';
    style.bottom = (window.innerHeight - rect.top + gap) + 'px';
  } else {
    style.bottom = 'auto';
    style.top = (rect.bottom + gap) + 'px';
  }
  dropStyle.value = style;
};

const selectedLabel = computed(() => {
  const match = props.options.find(o => o.value === props.modelValue);
  return match ? match.label : props.options[0]?.label || '';
});

/* 下拉列表展开时的高度与方向。
   原来固定 top:100% + max-height:260px：当触发器靠近窗口底部（弹窗里的
   策略选择就是这种情况），列表会伸到窗口外面，而 absolute 元素没法滚动到
   视口之外 —— 下半截选项就"消失"了。
   这里按上下可用空间决定向上还是向下展开，并把高度夹在可用空间内。 */
const dropUp = ref(false);
const dropMaxHeight = ref(260);

const toggle = () => {
  if (props.disabled) return;
  if (activeSelectId.value === instanceId) {
    activeSelectId.value = null;
    return;
  }
  const el = selectRef.value;
  if (el) {
    const rect = el.getBoundingClientRect();
    const gap = 12;
    const spaceBelow = window.innerHeight - rect.bottom - gap;
    const spaceAbove = rect.top - gap;
    dropUp.value = spaceBelow < 180 && spaceAbove > spaceBelow;
    const available = dropUp.value ? spaceAbove : spaceBelow;
    dropMaxHeight.value = Math.max(120, Math.min(260, available));
  }
  updatePosition();
  activeSelectId.value = instanceId;
};

const selectOption = (val: string | number) => {
  emit('update:modelValue', val);
  emit('change', val);
  activeSelectId.value = null;
};

// 核心重绘逻辑：自己接管“失去焦点”事件
const handleClickOutside = (event: MouseEvent) => {
  if (activeSelectId.value !== instanceId) return;
  const target = event.target as Node;
  // 浮层已经 teleport 到 body，不再包在 selectRef 里，两处都要判
  const inside = (selectRef.value && selectRef.value.contains(target)) || (dropRef.value && dropRef.value.contains(target));
  if (!inside) activeSelectId.value = null;
};

/* fixed 定位不再跟着父元素走，滚动/缩放时必须手动重算坐标 */
const onViewportChange = () => {
  if (activeSelectId.value === instanceId) updatePosition();
};

onMounted(() => {
  // 组件挂载时，在整个窗口贴上“监听器”
  document.addEventListener('click', handleClickOutside);
  window.addEventListener('scroll', onViewportChange, true);
  window.addEventListener('resize', onViewportChange);
});

onUnmounted(() => {
  // 组件销毁时注销监听器
  document.removeEventListener('click', handleClickOutside);
  window.removeEventListener('scroll', onViewportChange, true);
  window.removeEventListener('resize', onViewportChange);
  if (activeSelectId.value === instanceId) {
    activeSelectId.value = null;
  }
});
</script>

<style scoped>
/* 1. 基础容器（无任何系统响应逻辑） */
.modern-custom-select {
  position: relative;
  min-width: 140px; 
  width: auto;
  height: 36px;
  font-family: inherit;
  font-size: 0.85rem;
  font-weight: 500;
}

.modern-custom-select.is-open {
  z-index: 50;
}

.modern-custom-select.w-full {
  width: 100%;
}

.modern-custom-select.w-full .select-trigger {
  width: 100%;
}

.modern-custom-select.w-full .select-trigger span {
  max-width: 100%;
  flex: 1;
  text-align: left;
}

/* G-09：浮层已 Teleport 到 body，不再是 .modern-custom-select 的后代，
   所以原来这里的 `.w-full .select-dropdown` 规则永远匹配不到（宽度由 updatePosition() 内联写死），已删除。
   同理下面 `.drop-up` 与 .select-dropdown 里的 z-index 也被内联样式覆盖，一并删掉。 */

/* 2. 触发器（视觉主体） */
.select-trigger {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 8px;
  color: var(--text-main);
  padding: 8px 12px;
  border-radius: var(--ui-radius-sm); 
  cursor: pointer;
  transition: background-color 0.2s ease;
  height: 100%;
  box-sizing: border-box;
  background-color: var(--surface-hover);
  min-width: 120px;
}

.select-trigger span {
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  max-width: 240px;
}

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
  flex-shrink: 0;
  transition: transform 0.3s ease;
}
.arrow-up {
  transform: rotate(180deg);
}

.select-dropdown {
  position: absolute;
  top: calc(100% + 6px);
  right: 0;
  min-width: 100%;
  width: max-content;
  max-width: 340px;
  background-color: var(--surface);
  border: 1px solid var(--glass-border);
  border-radius: var(--ui-radius-sm);
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.15);
  /* G-09：z-index 已删掉（updatePosition() 每次都写内联 10001，这里永远不生效）。
     下面这些尺寸/定位声明同样会被内联样式覆盖，保留只为"未计算前的静态形状"。 */
  max-height: 260px;
  overflow-y: auto;
  overflow-x: hidden;
  padding: 4px;
}

.select-option {
  padding: 10px 12px;
  color: var(--text-main);
  cursor: pointer;
  border-radius: 6px;
  transition: background 0.2s ease;
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.select-option:hover {
  background-color: var(--surface-hover);
}

.select-option.active {
  background-color: var(--surface-panel);
  color: var(--accent);
}

.option-label {
  font-size: 0.85rem;
  font-weight: 600;
}

/* 原来写 var(--text-dim)，但这个变量全项目从未定义过，声明无效，
   描述文字实际继承的是 --text-main —— 等于没做降级。改走 --text-sub，
   它会被玻璃模式下的 body 覆盖自动压深，跨主题都成立。 */
.option-description {
  font-size: 0.75rem;
  color: var(--text-sub);
  font-weight: normal;
  line-height: 1.3;
}

.fade-enter-active, .fade-leave-active { transition: opacity 0.2s, transform 0.2s; }
.fade-enter-from, .fade-leave-to { opacity: 0; transform: translateY(-5px); }
</style>
