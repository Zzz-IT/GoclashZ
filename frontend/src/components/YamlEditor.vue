<template>
  <div class="yaml-editor-view ui-page-shell">
    <div class="yaml-toolbar ui-page-header">
      <div class="editor-left">
        <!-- P-07：返回要走未保存守卫，改由 handleBack 处理（原来直接 $emit('back')，改动静默丢失） -->
        <button class="back-btn" @click="handleBack" :title="t('common.back')">
          <svg viewBox="0 0 24 24" width="18" height="18" fill="none"
               stroke="currentColor" stroke-width="2">
            <line x1="19" y1="12" x2="5" y2="12"></line>
            <polyline points="12 19 5 12 12 5"></polyline>
          </svg>
        </button>

        <div class="file-meta">
          <div class="file-name">{{ configName || t('yamlEditor.defaultConfigName') }}</div>
          <div class="file-path">{{ fileName }}</div>
        </div>
      </div>

      <div class="editor-actions">
        <button class="action-btn" @click="handleValidate" :disabled="validating">
          {{ validating ? t('yamlEditor.validating') : t('yamlEditor.validateBtn') }}
        </button>
        <button class="action-btn" @click="handleReload" :disabled="loading">{{ t('yamlEditor.reloadBtn') }}</button>
        <button class="primary-btn accent-btn mini-btn" @click="handleSave" :disabled="!isModified || saving">
          {{ saving ? t('yamlEditor.saving') : t('yamlEditor.saveBtn') }}
        </button>
      </div>
    </div>

    <div class="editor-body ui-page-body" ref="editorContainer"></div>

    <div v-if="props.configType === 'remote'" class="remote-warning">
      <span>{{ t('yamlEditor.remoteWarning') }}</span>
    </div>

    <div class="editor-summary-bar">
      <span>{{ t('yamlEditor.totalLines', { count: totalLines }) }}</span>
      <span>{{ t('yamlEditor.totalChars', { count: totalChars }) }}</span>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted, shallowRef, watch } from 'vue';
import { EditorView } from 'codemirror';
import { EditorState, Compartment } from '@codemirror/state';
import { yaml } from '@codemirror/lang-yaml';
import { keymap, lineNumbers, highlightActiveLine, highlightActiveLineGutter, dropCursor } from '@codemirror/view';
import { indentWithTab, defaultKeymap, historyKeymap } from '@codemirror/commands';
import { history } from '@codemirror/commands';
import { HighlightStyle, syntaxHighlighting, indentOnInput, bracketMatching, foldGutter, foldKeymap } from '@codemirror/language';
import { closeBrackets, closeBracketsKeymap, completionKeymap } from '@codemirror/autocomplete';
import { searchKeymap } from '@codemirror/search';
import { tags } from '@lezer/highlight';
import { App } from '../api';
import { showConfirm, showAlert } from '../store';
import { t } from '../locales';

const props = defineProps<{
  configId: string;
  configName: string;
  configType: 'local' | 'remote';
}>();

const emit = defineEmits<{
  back: [];
  'status-change': [payload: { text: string; modified: boolean; error: boolean }];
  'cursor-change': [payload: { line: number; col: number }];
}>();

const editorContainer = ref<HTMLElement>();
const editorView = shallowRef<EditorView>();
const themeCompartment = new Compartment();
const highlightCompartment = new Compartment();
const loading = ref(false);
const saving = ref(false);
const validating = ref(false);
const isModified = ref(false);
const fileName = ref('');
const originalContent = ref('');
const cursorLine = ref(1);
const cursorCol = ref(1);
const totalLines = ref(1);
const totalChars = ref(0);
const yamlError = ref('');
// P-06：await loadConfig() 期间组件可能已被卸载，卸载后不得再建 EditorView / 注册全局监听
let disposed = false;

// 自定义 editor setup：不引入 basicSetup / highlightSelectionMatches / drawSelection
const editorBaseSetup = [
  lineNumbers(),
  highlightActiveLineGutter(),
  history(),
  foldGutter(),
  dropCursor(),
  indentOnInput(),
  bracketMatching(),
  closeBrackets(),
  highlightActiveLine(),
  keymap.of([
    indentWithTab,
    ...closeBracketsKeymap,
    ...defaultKeymap,
    ...searchKeymap,
    ...historyKeymap,
    ...foldKeymap,
    ...completionKeymap,
  ]),
];

const lightTheme = EditorView.theme({
  '&': {
    backgroundColor: '#FFFFFF',
    color: '#000000',
  },
  '.cm-content': {
    caretColor: '#000000',
    fontFamily: "'JetBrains Mono', 'MiSans', monospace",
    fontSize: '14px',
    lineHeight: '1.6',
  },
  '.cm-cursor': {
    borderLeftColor: '#000000',
  },
  '& ::selection': {
    backgroundColor: '#000000',
    color: '#FFFFFF',
  },
  '.cm-content ::selection': {
    backgroundColor: '#000000',
    color: '#FFFFFF',
  },
  '.cm-line ::selection': {
    backgroundColor: '#000000',
    color: '#FFFFFF',
  },
  '.cm-selectionBackground': {
    backgroundColor: '#000000 !important',
  },
  '.cm-gutters': {
    backgroundColor: '#F5F5F5',
    color: '#AAAAAA',
    borderRight: '1px solid #E5E5E5',
    borderTopLeftRadius: '12px',
    borderBottomLeftRadius: '12px',
  },
  '.cm-activeLineGutter': {
    backgroundColor: '#EEEEEE',
    color: '#1A1A1A',
  },
  '.cm-activeLine': {
    backgroundColor: '#F8F8F8',
  },
  '.cm-foldPlaceholder': {
    backgroundColor: '#E5E5E5',
    border: 'none',
    color: '#777777',
  },
});

const darkTheme = EditorView.theme({
  '&': {
    backgroundColor: '#0D1117',
    color: '#FFFFFF',
  },
  '.cm-content': {
    caretColor: '#FFFFFF',
    fontFamily: "'JetBrains Mono', 'MiSans', monospace",
    fontSize: '14px',
    lineHeight: '1.6',
  },
  '.cm-cursor': {
    borderLeftColor: '#FFFFFF',
  },
  '& ::selection': {
    backgroundColor: '#FFFFFF',
    color: '#000000',
  },
  '.cm-content ::selection': {
    backgroundColor: '#FFFFFF',
    color: '#000000',
  },
  '.cm-line ::selection': {
    backgroundColor: '#FFFFFF',
    color: '#000000',
  },
  '.cm-selectionBackground': {
    backgroundColor: '#FFFFFF !important',
  },
  '.cm-gutters': {
    backgroundColor: '#111111',
    color: '#555555',
    borderRight: '1px solid #2A2A2A',
    borderTopLeftRadius: '12px',
    borderBottomLeftRadius: '12px',
  },
  '.cm-activeLineGutter': {
    backgroundColor: '#222222',
    color: '#E8E8E8',
  },
  '.cm-activeLine': {
    backgroundColor: '#1E1E1E',
  },
  '.cm-foldPlaceholder': {
    backgroundColor: '#333333',
    border: 'none',
    color: '#888888',
  },
});

const monoHighlight = HighlightStyle.define([
  { tag: tags.keyword, color: 'var(--text-main)', fontWeight: '700' },
  { tag: tags.atom, color: 'var(--text-main)' },
  { tag: tags.bool, color: 'var(--text-main)', fontWeight: '700' },
  { tag: tags.comment, color: 'var(--text-sub)', fontStyle: 'italic' },
  { tag: tags.string, color: 'var(--text-main)' },
  { tag: tags.number, color: 'var(--text-main)' },
  { tag: tags.propertyName, color: 'var(--text-main)', fontWeight: '600' },
  { tag: tags.operator, color: 'var(--text-sub)' },
  { tag: tags.invalid, color: 'var(--text-main)', textDecoration: 'underline wavy' },
]);

const isDark = () => document.documentElement.classList.contains('dark');

const getTheme = () => isDark() ? darkTheme : lightTheme;
const getHighlight = () => monoHighlight;

const updateEditorStats = (view: EditorView) => {
  totalLines.value = view.state.doc.lines;
  totalChars.value = view.state.doc.length;

  const pos = view.state.selection.main.head;
  const line = view.state.doc.lineAt(pos);
  cursorLine.value = line.number;
  cursorCol.value = pos - line.from + 1;

  emit('cursor-change', { line: cursorLine.value, col: cursorCol.value });
};

/* 状态栏文案与「未保存 / 有错误」标记只留这一个出口（P-29）：
   以前 statusText 写了 8 次却没有读者 —— 界面显示的是 App 的 yamlEditorStatus，
   真正生效的只有这里 emit 出去的 text。error 也从 yamlError 推导（P-31），
   不再像以前那样每次键入都硬发 error:false 把校验失败的红标清掉。 */
const emitStatus = (text?: string) => {
  emit('status-change', {
    text: text ?? (isModified.value ? t('yamlEditor.unsaved') : t('yamlEditor.saved')),
    modified: isModified.value,
    error: !!yamlError.value,
  });
};

let modifiedRecheckTimer: ReturnType<typeof setTimeout> | undefined;

const cancelModifiedRecheck = () => {
  if (modifiedRecheckTimer) clearTimeout(modifiedRecheckTimer);
  modifiedRecheckTimer = undefined;
};

const createExtensions = () => [
  ...editorBaseSetup,
  yaml(),
  EditorView.lineWrapping,
  themeCompartment.of(getTheme()),
  highlightCompartment.of(syntaxHighlighting(getHighlight())),
  EditorView.updateListener.of((update) => {
    if (!editorView.value) return;

    updateEditorStats(update.view);

    if (update.docChanged) {
      /* P-32：以前每次文档变更都对全文 toString() 比较，1MB 级配置下每次键入/粘贴
         都在主线程构建整串。现在先同步置脏（未保存守卫必须立刻生效），
         把「改回原样 ⇒ 清脏」这次精确比较推迟到 180ms 空闲后做一次。 */
      isModified.value = true;
      cancelModifiedRecheck();
      modifiedRecheckTimer = setTimeout(() => {
        modifiedRecheckTimer = undefined;
        const view = editorView.value;
        if (!view) return;
        // 比较结果只可能把状态改干净（保守方向），所以不必每次都跑
        if (isModified.value && view.state.doc.toString() === originalContent.value) {
          isModified.value = false;
          emitStatus();
        }
      }, 180);
      emitStatus();
    }
  }),
];

/* 读配置的 in-flight 序号：P-06 的卸载判据之外再挡一类竞态 ——
   props.configId 变化（P-33 的 watch）或重新加载与上一次读盘重叠时，
   两个续体都看到 editorView 为空会各建一个 EditorView，先建的那个从此没人销毁。 */
let loadSeq = 0;

const loadConfig = async () => {
  const seq = ++loadSeq;
  cancelModifiedRecheck();
  loading.value = true;
  try {
    const result = await App.ReadConfigText(props.configId);
    // P-06：await 期间组件可能已卸载，此时再建 EditorView 会挂在已脱离文档的容器上泄漏
    if (disposed || seq !== loadSeq) return;
    fileName.value = result.name;
    originalContent.value = result.content;
    isModified.value = false;
    yamlError.value = '';

    emitStatus(t('yamlEditor.saved'));

    if (editorView.value) {
      editorView.value.dispatch({
        changes: {
          from: 0,
          to: editorView.value.state.doc.length,
          insert: result.content,
        },
      });
      updateEditorStats(editorView.value);
    } else if (editorContainer.value) {
      const state = EditorState.create({
        doc: result.content,
        extensions: createExtensions(),
      });
      editorView.value = new EditorView({
        state,
        parent: editorContainer.value,
      });
      updateEditorStats(editorView.value);
    }
  } catch (e: any) {
    // 过期请求的失败不再弹窗/不改状态：最新那次读盘会给出最终结果
    if (seq !== loadSeq) return;
    const errorMsg = e.message || String(e);
    // 错误态留在 yamlError 里，emitStatus 才知道要打红标；isModified 不再被谎报成 false
    // （读失败时编辑器里仍是用户的未保存内容，守卫不能松）
    yamlError.value = errorMsg;
    emitStatus(t('common.error'));
    
    // 如果读取配置失败（如严格路径校验不通过），弹出提示并返回
    showAlert(t('yamlEditor.readError', { error: errorMsg }), t('common.error'), true);
  } finally {
    // 只有最后一次读盘才收起 loading，过期续体不能提前放行按钮
    if (seq === loadSeq) loading.value = false;
  }
};

/* P-30：后端把「文件已保存，只是运行配置重载失败」也折成一个 error 返回
   （core/appcore/config_editor_controller.go）。前端以前一律当保存失败：保持 dirty、
   弹「保存失败」，用户会以为改动回滚了，于是重复保存。
   判定以**稳定错误码前缀**为准（CONFIG_SAVED_RELOAD_FAILED，见 Go 侧 ErrCodeConfigSavedReloadFailed），
   中文文案匹配只作为旧后端的兜底 —— 后端改文案不会让这里退化。 */
const isSavedButReloadFailed = (e: any): boolean => {
  if (e && (e.saved === true || e.reloadFailed === true)) return true;
  const msg = String((e && (e.message || e.error)) || e || '');
  if (msg.includes('CONFIG_SAVED_RELOAD_FAILED')) return true;
  // 兜底：旧后端没有 code 时按文案认（保留一个版本周期后删除）
  return msg.includes('保存成功') && msg.includes('重载');
};

const handleSave = async () => {
  if (!isModified.value) return;
  const content = editorView.value?.state.doc.toString() || '';
  /* P-32 把「是否改回原样」的比较延后了 180ms，这里（保存是低频动作）再精确比一次：
     否则"改回原样后立刻按 Ctrl+S"会白发一次保存 + 内核重载。 */
  if (content === originalContent.value) {
    cancelModifiedRecheck();
    isModified.value = false;
    emitStatus(t('yamlEditor.saved'));
    return;
  }
  saving.value = true;
  try {
    await App.SaveConfigText(props.configId, content);
    cancelModifiedRecheck();
    originalContent.value = content;
    isModified.value = false;
    yamlError.value = '';

    emitStatus(t('yamlEditor.saved'));
  } catch (e: any) {
    const errorMsg = e.message || String(e);
    yamlError.value = errorMsg;
    if (isSavedButReloadFailed(e)) {
      // 文件确实已落盘：既不能说「保存失败」，也不能继续挂着未保存状态
      cancelModifiedRecheck();
      originalContent.value = content;
      isModified.value = false;
      emitStatus(t('yamlEditor.saveReloadFailed', { error: errorMsg }));
      showAlert(t('yamlEditor.saveReloadFailed', { error: errorMsg }), t('common.warning'), true);
    } else {
      emitStatus(t('yamlEditor.saveError', { error: errorMsg }));
      showAlert(t('yamlEditor.saveFileError', { error: errorMsg }), t('common.error'), true);
    }
  } finally {
    saving.value = false;
  }
};

const handleValidate = async () => {
  validating.value = true;
  try {
    const content = editorView.value?.state.doc.toString() || '';
    await App.ValidateConfigText(content);
    yamlError.value = '';
    emitStatus(t('yamlEditor.syntaxValid'));
  } catch (e: any) {
    // 诊断结果写进 yamlError：之后的键入不会再把红标清掉（P-31）
    yamlError.value = e.message || String(e);
    emitStatus(t('yamlEditor.formatError'));
  } finally {
    validating.value = false;
  }
};

const handleReload = async () => {
  if (isModified.value) {
    const ok = await showConfirm(t('yamlEditor.reloadConfirmMsg'), t('common.notice'));
    if (!ok) return;
  }
  loadConfig();
};

/* P-07：返回箭头以前直接 $emit('back')，未保存的改动静默丢失（只有「重新加载」有确认）。
   App 侧切页/侧边栏的守卫由 fix-2 负责，编辑器侧先把自己这个入口拦住。 */
const handleBack = async () => {
  if (isModified.value) {
    const ok = await showConfirm(t('yamlEditor.backConfirmMsg'), t('common.notice'));
    if (!ok) return;
  }
  emit('back');
};

const handleKeydown = (e: KeyboardEvent) => {
  if ((e.ctrlKey || e.metaKey) && e.key === 's') {
    e.preventDefault();
    handleSave();
  }
};

let themeObserver: MutationObserver | null = null;

/* P-33：现在父组件用 v-if 销毁/重建，props.configId 在实例生命周期内不会变。
   但一旦将来改成 v-show 或加 KeepAlive，换配置时不会重新挂载，
   保存就会把旧内容写进新 id —— 这里显式重载（有未保存改动先确认）。 */
watch(() => props.configId, async (newId, oldId) => {
  if (!newId || newId === oldId) return;
  if (isModified.value) {
    const ok = await showConfirm(t('yamlEditor.reloadConfirmMsg'), t('common.notice'));
    if (!ok) return;
  }
  await loadConfig();
});

onMounted(async () => {
  /* P-06：这两个全局监听以前注册在 await loadConfig() 之后。卸载与 loadConfig 竞态时，
     onUnmounted 已经把「还没注册」的东西清过一遍，续体又把它们装回去 ⇒
     document 上的 keydown 与 documentElement 上的 MutationObserver 永久残留，
     并闭包住本组件实例（EditorView 也一起挂在已脱离文档的容器上）。
     现在先同步注册，onUnmounted 一定清得到。 */
  document.addEventListener('keydown', handleKeydown);
  themeObserver = new MutationObserver(() => {
    if (editorView.value) {
      const currentTheme = isDark() ? darkTheme : lightTheme;
      const currentHighlight = monoHighlight;
      editorView.value.dispatch({
        effects: [
          themeCompartment.reconfigure(currentTheme),
          highlightCompartment.reconfigure(syntaxHighlighting(currentHighlight)),
        ],
      });
    }
  });
  themeObserver.observe(document.documentElement, {
    attributes: true,
    attributeFilter: ['class'],
  });

  await loadConfig();
});

onUnmounted(() => {
  disposed = true;
  cancelModifiedRecheck();
  editorView.value?.destroy();
  editorView.value = undefined;
  document.removeEventListener('keydown', handleKeydown);
  themeObserver?.disconnect();
  themeObserver = null;
});
</script>

<style scoped>
.yaml-editor-view {
  /* PageShell：toolbar 固定，编辑器区占用剩余高度（不再用 min-height:100%） */
  flex: 1 1 auto;
  min-width: 0;
  gap: 16px;
}

.yaml-toolbar {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 16px;
  padding: 4px 0 12px 0;
  background: transparent;
}

.editor-left {
  display: flex;
  align-items: center;
  gap: 14px;
  min-width: 0;
}

.file-meta {
  min-width: 0;
}

.file-name {
  font-size: 1rem;
  font-weight: 600;
  color: var(--text-main);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.file-path {
  margin-top: 4px;
  font-size: 0.75rem;
  color: var(--text-sub);
  font-family: var(--font-mono);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.back-btn {
  background: var(--surface);
  border: none;
  color: var(--text-main);
  width: 36px;
  height: 36px;
  border-radius: 50%;
  display: flex;
  align-items: center;
  justify-content: center;
  cursor: pointer;
  transition: 0.2s;
  flex-shrink: 0;
}

.back-btn:hover {
  background: var(--surface-hover);
}

.editor-actions {
  display: flex;
  gap: 8px;
}

.action-btn {
  border: none;
  background: var(--surface);
  color: var(--text-main);
  border-radius: var(--ui-radius-sm);
  padding: 8px 14px;
  font-size: 0.8rem;
  font-weight: 600;
  cursor: pointer;
  transition: 0.2s;
  white-space: nowrap;
}

.action-btn:hover:not(:disabled) {
  background: var(--surface-hover);
}

.action-btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.primary-btn {
  border: none;
  border-radius: var(--ui-radius-sm);
  padding: 8px 14px;
  font-size: 0.8rem;
  font-weight: 600;
  cursor: pointer;
  transition: 0.2s;
  white-space: nowrap;
}

.primary-btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.accent-btn {
  background: var(--ui-accent-block);
  color: var(--ui-accent-fg);
}

.accent-btn:hover:not(:disabled) {
  opacity: 0.85;
}

.mini-btn {
  padding: 8px 14px;
}

/* 编辑区 */
.editor-body {
  /* body 槽位只负责撑满高度；真正的滚动交给 CodeMirror 内部 .cm-scroller，
     避免页面滚动与编辑器滚动互抢，也不再依赖 100vh 减法算高度 */
  display: flex;
  flex-direction: column;
  flex: 1 1 auto;
  min-height: 0;
  height: auto;
  overflow: hidden;
  padding: 0 !important;
  border: none;
  border-radius: 0;
  background: transparent;
}

.editor-body :deep(.cm-editor) {
  flex: 1 1 auto;
  min-height: 0;
  height: auto;
  border-radius: var(--ui-radius-lg);
  background: var(--surface);
}

.editor-body :deep(.cm-scroller) {
  overflow: auto;
}

.editor-body :deep(.cm-content) {
  min-height: 100%;
}

/* 底部统计栏：滚到底部后显示 */
.editor-summary-bar {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-top: auto;
  padding-top: 16px;
  font-size: 0.8rem;
  color: var(--text-sub);
  font-family: var(--font-mono);
}

.remote-warning {
  background: var(--surface-panel);
  color: var(--text-main);
  padding: 8px 12px;
  border-radius: var(--ui-radius-sm);
  font-size: 0.8rem;
  margin-top: 8px;
  display: flex;
  align-items: center;
  border: 1px dashed var(--text-muted);
}

/* 选区 span 级兜底：确保 token 颜色不干扰选中文字 */
.editor-body :deep(.cm-content span::selection) {
  background: #000 !important;
  color: #fff !important;
}

:root.dark .editor-body :deep(.cm-content span::selection) {
  background: #fff !important;
  color: #000 !important;
}
</style>
