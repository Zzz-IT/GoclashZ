<!--
  生效回执条：设置保存后提示「有 N 项设置未按字面生效」。
  设计约定：不打断、不弹窗；只有 1 条时直接摊开（字段 · 原因），2 条以上才折叠；
  无偏差 / 请求失败 / 用户已关闭 / 组件已卸载时整条不渲染。
  样式类全部来自全局 style.css，本组件不新增 CSS。
-->
<template>
  <template v-if="visible">
    <div class="deviation-strip">
      <span class="deviation-strip__bar"></span>
      <div class="deviation-strip__text">
        <template v-if="list.length === 1">{{ fieldText(list[0]) }} · {{ reasonText(list[0]) }}</template>
        <template v-else>{{ t('diag.notApplied', { count: list.length }) }}</template>
      </div>
      <button
        v-if="list.length > 1"
        class="deviation-strip__action"
        @click="open = !open"
      >{{ open ? t('diag.hide') : t('diag.view') }}</button>
      <button class="deviation-strip__close" :title="t('diag.close')" @click="dismiss">✕</button>
    </div>
    <div v-if="list.length > 1 && open" class="deviation-list">
      <div v-for="(d, i) in list" :key="i" class="deviation-item">
        <span class="deviation-item__field">{{ fieldText(d) }}</span>
        <span class="deviation-item__reason">{{ reasonText(d) }}</span>
      </div>
    </div>
  </template>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue';
import * as API from '../../wailsjs/go/main/App';
import { t } from '../locales';

const props = defineProps<{ module: string }>();

const list = ref<any[]>([]);
const open = ref(false);
const dismissed = ref(false);
let disposed = false;
// field / reason 现在都是 { code, arg }：按 code 取 i18n；迁移期若还是字符串也能兜住
const codeOf = (value: any): string => {
  if (value && typeof value === 'object') return String(value.code || '');
  return String(value || '');
};
const localized = (prefix: string, value: any): string => {
  const code = codeOf(value);
  if (!code) return '';
  const key = prefix + '.' + code;
  const text = t(key, { arg: value?.arg || '' });
  return text === key ? code : text;
};
const fieldText = (d: any) => localized('deviation.field', d?.field);
const reasonText = (d: any) => localized('deviation.reason', d?.reason);
// 上次渲染的偏差内容签名：只有内容变化才把用户关掉的回执条重新显示（只拼 code，不依赖任何语言的展示文本）
const signatureOf = (items: any[]) => items.map((d) => `${codeOf(d?.field)}|${codeOf(d?.reason)}`).join(';;');
let lastSignature = '';

const visible = computed(() => !dismissed.value && list.value.length > 0);

// 拉取当前模块的配置偏差；任何异常都按「无偏差」处理，保证不打断用户
const refresh = async () => {
  if (disposed) return;
  try {
    const data = await API.GetConfigDeviations(props.module);
    if (disposed) return;
    list.value = Array.isArray(data) ? data : [];
  } catch {
    if (disposed) return;
    list.value = [];
  }
  // 关闭只在内容没变时保持；签名变了才重新显示，避免自动保存后反复弹回
  const sig = signatureOf(list.value);
  if (sig !== lastSignature) {
    dismissed.value = false;
    if (list.value.length !== 1) open.value = false;
  }
  lastSignature = sig;
};

const dismiss = () => {
  dismissed.value = true;
  open.value = false;
};

onMounted(() => { void refresh(); });
onUnmounted(() => { disposed = true; });

// 父页面保存成功后主动调用
defineExpose({ refresh });
</script>
