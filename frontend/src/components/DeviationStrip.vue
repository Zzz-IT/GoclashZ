<!--
  生效回执条：设置保存后提示「有 N 项设置未按字面生效」。
  设计约定：不打断、不弹窗；只有 1 条时直接摊开（字段 · 原因），2 条以上才折叠；
  无偏差 / 请求失败 / 用户已关闭 / 组件已卸载时整条不渲染。
  样式类来自全局 style.css；只有 H-21 新增的 severity 徽标用本组件的 scoped 样式（不动全局 CSS）。
-->
<template>
  <template v-if="visible">
    <div class="deviation-strip">
      <span class="deviation-strip__bar"></span>
      <div class="deviation-strip__text">
        <template v-if="list.length === 1">
          <span class="deviation-item__severity" :class="severityClass(list[0])">{{ severityText(list[0]) }}</span>
          {{ fieldText(list[0]) }} · {{ reasonText(list[0]) }}
        </template>
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
      <!-- H-21：key 用后端给的稳定语义 id（d.key），不再用下标：列表整体替换时才能保住「展开的是哪一条」 -->
      <div v-for="d in list" :key="d.key" class="deviation-item">
        <span class="deviation-item__severity" :class="severityClass(d)">{{ severityText(d) }}</span>
        <span class="deviation-item__field">{{ fieldText(d) }}</span>
        <span class="deviation-item__reason">{{ reasonText(d) }}</span>
      </div>
    </div>
  </template>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue';
import { App } from '../api';
import { reportError } from '../utils/report';
import { t } from '../locales';
// H-21：用后端生成的模型而不是 any[]，severity / key 这两个字段才不会在传递中被「看不见」
import type { ConfigDeviation } from '../../bindings/goclashz/core/clash/models';

const props = defineProps<{ module: string }>();

const list = ref<ConfigDeviation[]>([]);
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
const fieldText = (d: ConfigDeviation) => localized('deviation.field', d?.field);
const reasonText = (d: ConfigDeviation) => localized('deviation.reason', d?.reason);
// H-21：后端用 severity 区分「设置被忽略（ignored）」与「设置被强制改成别的值（forced）」，
// 这两条语义完全不同的偏差以前在 UI 上长得一模一样；文案走 deviation.severity.*（缺 key 时回落原始 code）。
const severityText = (d: ConfigDeviation) => localized('deviation.severity', d?.severity);
const severityClass = (d: ConfigDeviation) =>
  d?.severity === 'forced' ? 'is-forced' : 'is-ignored';
// 上次渲染的偏差内容签名：只有内容变化才把用户关掉的回执条重新显示（只拼 code，不依赖任何语言的展示文本）
const signatureOf = (items: ConfigDeviation[]) => items.map((d) => `${codeOf(d?.field)}|${codeOf(d?.reason)}`).join(';;');
let lastSignature = '';

const visible = computed(() => !dismissed.value && list.value.length > 0);

// 拉取当前模块的配置偏差；任何异常都按「无偏差」处理，保证不打断用户
const refresh = async () => {
  if (disposed) return;
  try {
    const data = await App.GetConfigDeviations(props.module);
    if (disposed) return;
    list.value = Array.isArray(data) ? data : [];
  } catch (e) {
    if (disposed) return;
    // 失败不等于「没有偏差」：记录后保留上一次成功结果，避免漂移提示凭空消失（F-A4-21）
    reportError(e, 'deviation.load');
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

<style scoped>
/* H-21：severity 徽标。ignored = 设置写进去了但没生效（弱化）；forced = 设置被强制改值（强调）。 */
.deviation-item__severity {
  flex: 0 0 auto;
  display: inline-block;
  padding: 0 6px;
  border-radius: var(--ui-radius-full);
  font-size: 0.7rem;
  font-weight: 600;
  line-height: 1.6;
  white-space: nowrap;
}

.deviation-item__severity.is-ignored {
  background: var(--surface-hover);
  color: var(--text-sub);
}

.deviation-item__severity.is-forced {
  background: var(--ui-accent-block);
  color: var(--ui-accent-fg);
}
</style>
