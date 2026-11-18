<template>
  <div class="modern-number-stepper" :class="{ disabled }">
    <button 
      class="stepper-btn" 
      @click="decrement" 
      :disabled="disabled || (min !== undefined && modelValue <= min)"
    >
      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round">
        <line x1="5" y1="12" x2="19" y2="12"></line>
      </svg>
    </button>

    <input
      type="number"
      class="stepper-input"
      :value="modelValue"
      @focus="onFocus"
      @input="onInput"
      @blur="onBlur"
      @keydown.enter.prevent="onSubmitKey"
      @keydown.esc.prevent="onCancelKey"
      :min="min"
      :max="max"
      :disabled="disabled"
    />

    <button 
      class="stepper-btn" 
      @click="increment" 
      :disabled="disabled || (max !== undefined && modelValue >= max)"
    >
      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round">
        <line x1="12" y1="5" x2="12" y2="19"></line>
        <line x1="5" y1="12" x2="19" y2="12"></line>
      </svg>
    </button>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue';

const props = defineProps<{
  modelValue: number;
  min?: number;
  max?: number;
  step?: number;
  disabled?: boolean;
}>();

const emit = defineEmits(['update:modelValue', 'change']);

// G-07：step 必须是 computed —— 原来的写法 `const step = props.step ?? 1` 只在 setup 求值一次，
// 父级动态改 step 会静默失效。
const step = computed(() => props.step ?? 1);

// 上一次已提交的合法值。清空/非法输入时回退到它，而不是回落到 min ?? 0：
// 0 在 Go 侧很多字段被当成"未设置"再换成别的默认值（如 AutoDelayTestInterval<=0 → 60），
// 用户会以为设置没生效。
const lastValid = ref(props.modelValue);
watch(() => props.modelValue, (v) => {
  if (typeof v === 'number' && !Number.isNaN(v)) lastValid.value = v;
});

const clamp = (val: number) => {
  let v = val;
  if (props.min !== undefined && v < props.min) v = props.min;
  if (props.max !== undefined && v > props.max) v = props.max;
  return v;
};

/** 把输入框里的原始文本提交成合法值（失焦 / Enter 共用一条路径） */
const commit = (raw: string) => {
  const val = parseInt(raw);
  // G-07：失焦时才 clamp 到 [min,max]；NaN 优先回落 min，没有 min 再回上一次合法值
  const next = Number.isNaN(val) ? (props.min ?? lastValid.value) : clamp(val);
  emit('update:modelValue', next);
  emit('change', next);
  return next;
};

const increment = () => {
  const next = clamp(props.modelValue + step.value);
  emit('update:modelValue', next);
  emit('change', next);
};

const decrement = () => {
  const next = clamp(props.modelValue - step.value);
  emit('update:modelValue', next);
  emit('change', next);
};

const onInput = (e: Event) => {
  const val = parseInt((e.target as HTMLInputElement).value);
  if (!isNaN(val)) {
    emit('update:modelValue', val);
  }
};

/** Esc 取消本次输入：跳过 blur 提交，否则会发出一次同值 change，父级白保存一遍 */
let cancelEdit = false;

/* Esc 需要一个"进入编辑之前"的值来真正回退。
   不能用 props.modelValue 当原值：onInput 是实时 emit 的，此刻它已经是新值（例如 99999），
   拿它回填等于什么都没做，越界值会留在 model 里（空操作 + 不 clamp 的 bug）。
   所以在 focus 时快照一次。 */
let valueBeforeEdit = props.modelValue;
const onFocus = () => {
  valueBeforeEdit = props.modelValue;
};

const onBlur = (e: Event) => {
  const el = e.target as HTMLInputElement;
  if (cancelEdit) {
    cancelEdit = false;
    el.value = String(props.modelValue);
    return;
  }
  // 提交后把显示规范回合法值（输入 99999 或清空后不会停在非法文本上）
  el.value = String(commit(el.value));
};

/** Enter = 失焦提交，统一走 onBlur，避免重复 emit change */
const onSubmitKey = (e: KeyboardEvent) => {
  (e.target as HTMLInputElement).blur();
};

const onCancelKey = (e: KeyboardEvent) => {
  const el = e.target as HTMLInputElement;
  // 真回退：把编辑前的值写回父级（不发 change，父级不会落盘），再 blur 跳过 onBlur 的提交路径。
  // 这样"取消"之后 model 里不会留下未 clamp 的越界值。
  cancelEdit = true;
  lastValid.value = valueBeforeEdit;
  if (props.modelValue !== valueBeforeEdit) {
    emit('update:modelValue', valueBeforeEdit);
  }
  el.value = String(valueBeforeEdit);
  el.blur();
};
</script>

<style scoped>
.modern-number-stepper {
  display: flex;
  align-items: center;
  background-color: var(--surface-hover);
  border-radius: var(--ui-radius-sm);
  overflow: hidden;
  width: 120px; /* 缩短宽度 */
  height: 36px;
  transition: all 0.2s ease;
}

/* 隐藏原生输入框的箭头（双重保险） */
.stepper-input::-webkit-outer-spin-button,
.stepper-input::-webkit-inner-spin-button {
  -webkit-appearance: none;
  margin: 0;
}

.stepper-input {
  flex: 1;
  width: 100%;
  background: transparent;
  border: none;
  outline: none;
  color: var(--text-main);
  text-align: center;
  font-family: inherit;
  font-size: 0.9rem;
  padding: 0;
}

.stepper-btn {
  width: 32px;
  height: 100%;
  display: flex;
  align-items: center;
  justify-content: center;
  background: transparent;
  border: none;
  color: var(--text-sub);
  cursor: pointer;
  transition: all 0.2s ease;
  padding: 0;
}

/* 悬停与点击的反色处理：极简体系下的硬核反馈 */
.stepper-btn:hover:not(:disabled),
.stepper-btn:active:not(:disabled) {
  background-color: var(--ui-accent-block);
  color: var(--ui-accent-fg);
  transition: all 0.1s ease;
}



.stepper-btn:disabled {
  opacity: 0.2;
  cursor: not-allowed;
}

.stepper-btn svg {
  width: 14px;
  height: 14px;
}

/* 整体悬停效果 (根据用户要求移除) */
/* .modern-number-stepper:hover:not(.disabled) { background-color: var(--surface-panel); } */


/* 聚焦状态反馈 (根据用户要求移除，不加框不换色) */
/* .modern-number-stepper:focus-within { background-color: var(--surface-panel); box-shadow: inset 0 0 0 1px var(--text-sub); } */


.disabled {
  opacity: 0.5;
  cursor: not-allowed;
}
</style>
