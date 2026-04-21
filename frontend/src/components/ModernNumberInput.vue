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
      @input="onInput"
      @blur="onBlur"
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
const props = defineProps<{
  modelValue: number;
  min?: number;
  max?: number;
  step?: number;
  disabled?: boolean;
}>();

const emit = defineEmits(['update:modelValue', 'change']);

const step = props.step ?? 1;

const clamp = (val: number) => {
  let v = val;
  if (props.min !== undefined && v < props.min) v = props.min;
  if (props.max !== undefined && v > props.max) v = props.max;
  return v;
};

const increment = () => {
  const next = clamp(props.modelValue + step);
  emit('update:modelValue', next);
  emit('change', next);
};

const decrement = () => {
  const next = clamp(props.modelValue - step);
  emit('update:modelValue', next);
  emit('change', next);
};

const onInput = (e: Event) => {
  const val = parseInt((e.target as HTMLInputElement).value);
  if (!isNaN(val)) {
    emit('update:modelValue', val);
  }
};

const onBlur = (e: Event) => {
  const val = parseInt((e.target as HTMLInputElement).value);
  const clamped = isNaN(val) ? (props.min ?? 0) : clamp(val);
  emit('update:modelValue', clamped);
  emit('change', clamped);
};
</script>

<style scoped>
.modern-number-stepper {
  display: flex;
  align-items: center;
  background-color: var(--surface-hover);
  border-radius: 8px;
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
  background-color: var(--text-main);
  color: var(--app-bg);
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
