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
        <template v-if="list.length === 1">{{ list[0].field }} · {{ list[0].reason }}</template>
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
        <span class="deviation-item__field">{{ d.field }}</span>
        <span class="deviation-item__reason">{{ d.reason }}</span>
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
  // 每次刷新（打开页面 / 保存成功）都重新可见，关闭只在当次生效
  dismissed.value = false;
  if (list.value.length !== 1) open.value = false;
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
