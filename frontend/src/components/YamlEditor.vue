<template>
  <div class="yaml-editor-view">
    <div class="yaml-toolbar">
      <div class="editor-left">
        <button class="back-btn" @click="$emit('back')" title="返回">
          <svg viewBox="0 0 24 24" width="18" height="18" fill="none"
               stroke="currentColor" stroke-width="2">
            <line x1="19" y1="12" x2="5" y2="12"></line>
            <polyline points="12 19 5 12 12 5"></polyline>
          </svg>
        </button>

        <div class="file-meta">
          <div class="file-name">{{ configName || '配置文件' }}</div>
          <div class="file-path">{{ fileName }}</div>
        </div>
      </div>

      <div class="editor-actions">
        <button class="action-btn" @click="handleValidate" :disabled="validating">
          {{ validating ? '校验中...' : '检查语法' }}
        </button>
        <button class="action-btn" @click="handleReload" :disabled="loading">重新加载</button>
        <button class="primary-btn accent-btn mini-btn" @click="handleSave" :disabled="!isModified || saving">
          {{ saving ? '保存中...' : '保存' }}
        </button>
      </div>
    </div>

    <div class="editor-body" ref="editorContainer"></div>

    <div v-if="globalState.activeConfigType === 'remote'" class="remote-warning">
      <span>当前为远程订阅配置。建议通过【规则页】的附加规则修改。在此修改将在下次订阅更新时被覆盖。</span>
    </div>

    <div class="editor-summary-bar">
      <span>共 {{ totalLines }} 行</span>
      <span>{{ totalChars }} 字符</span>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted, shallowRef } from 'vue';
import { EditorView, basicSetup } from 'codemirror';
import { EditorState, Compartment } from '@codemirror/state';
import { yaml } from '@codemirror/lang-yaml';
import { keymap } from '@codemirror/view';
import { indentWithTab } from '@codemirror/commands';
import { HighlightStyle, syntaxHighlighting } from '@codemirror/language';
import { tags } from '@lezer/highlight';
import * as API from '../../wailsjs/go/main/App';
import { globalState } from '../store';

const props = defineProps<{
  configId: string;
  configName: string;
}>();

const emit = defineEmits<{
  back: [];
  'status-change': [payload: { text: string; modified: boolean; error: boolean }];
  'cursor-change': [payload: { line: number; col: number }];
}>();

const editorContainer = ref<HTMLElement>();
const editorView = shallowRef<EditorView>();
const themeCompartment = new Compartment();
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
const statusText = ref('已保存');
const yamlError = ref('');

const lightTheme = EditorView.theme({
  '&': {
    backgroundColor: '#FFFFFF',
    color: '#1A1A1A',
  },
  '.cm-content': {
    caretColor: '#1A1A1A',
    fontFamily: "'JetBrains Mono', 'MiSans', monospace",
    fontSize: '14px',
    lineHeight: '1.6',
  },
  '.cm-cursor': {
    borderLeftColor: '#1A1A1A',
  },
  '&.cm-focused .cm-selectionBackground, .cm-selectionBackground': {
    backgroundColor: '#D4D4D4',
  },
  '& ::selection': {
    backgroundColor: '#D4D4D4',
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

const lightHighlight = HighlightStyle.define([
  { tag: tags.keyword, color: '#D32F2F' },
  { tag: tags.atom, color: '#1976D2' },
  { tag: tags.bool, color: '#1976D2' },
  { tag: tags.comment, color: '#6A737D', fontStyle: 'italic' },
  { tag: tags.definition(tags.variableName), color: '#005CC5' },
  { tag: tags.string, color: '#032F62' },
  { tag: tags.number, color: '#005CC5' },
  { tag: tags.operator, color: '#24292E' },
  { tag: tags.tagName, color: '#22863A' },
  { tag: tags.typeName, color: '#6F42C1' },
  { tag: tags.className, color: '#6F42C1' },
  { tag: tags.labelName, color: '#E36209' },
  { tag: tags.propertyName, color: '#22863A' },
  { tag: tags.special(tags.string), color: '#032F62' },
  { tag: tags.regexp, color: '#032F62' },
  { tag: tags.escape, color: '#22863A' },
  { tag: tags.meta, color: '#6A737D' },
  { tag: tags.invalid, color: '#CB2431' },
]);

const darkTheme = EditorView.theme({
  '&': {
    backgroundColor: '#1A1A1A',
    color: '#E8E8E8',
  },
  '.cm-content': {
    caretColor: '#E8E8E8',
    fontFamily: "'JetBrains Mono', 'MiSans', monospace",
    fontSize: '14px',
    lineHeight: '1.6',
  },
  '.cm-cursor': {
    borderLeftColor: '#E8E8E8',
  },
  '&.cm-focused .cm-selectionBackground, .cm-selectionBackground': {
    backgroundColor: '#333333',
  },
  '& ::selection': {
    backgroundColor: '#333333',
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

const darkHighlight = HighlightStyle.define([
  { tag: tags.keyword, color: '#C678DD' },
  { tag: tags.atom, color: '#D19A66' },
  { tag: tags.bool, color: '#D19A66' },
  { tag: tags.comment, color: '#7F848E', fontStyle: 'italic' },
  { tag: tags.definition(tags.variableName), color: '#E06C75' },
  { tag: tags.string, color: '#98C379' },
  { tag: tags.number, color: '#D19A66' },
  { tag: tags.operator, color: '#ABB2BF' },
  { tag: tags.tagName, color: '#E06C75' },
  { tag: tags.typeName, color: '#E5C07B' },
  { tag: tags.className, color: '#E5C07B' },
  { tag: tags.labelName, color: '#E06C75' },
  { tag: tags.propertyName, color: '#61AFEF' },
  { tag: tags.special(tags.string), color: '#98C379' },
  { tag: tags.regexp, color: '#98C379' },
  { tag: tags.escape, color: '#56B6C2' },
  { tag: tags.meta, color: '#7F848E' },
  { tag: tags.invalid, color: '#F44747' },
]);

const isDark = () => document.documentElement.classList.contains('dark');

const getTheme = () => isDark() ? darkTheme : lightTheme;
const getHighlight = () => isDark() ? darkHighlight : lightHighlight;
const highlightCompartment = new Compartment();

const updateEditorStats = (view: EditorView) => {
  totalLines.value = view.state.doc.lines;
  totalChars.value = view.state.doc.length;

  const pos = view.state.selection.main.head;
  const line = view.state.doc.lineAt(pos);
  cursorLine.value = line.number;
  cursorCol.value = pos - line.from + 1;

  emit('cursor-change', { line: cursorLine.value, col: cursorCol.value });
};

const createExtensions = () => [
  basicSetup,
  yaml(),
  keymap.of([indentWithTab]),
  EditorView.lineWrapping,
  themeCompartment.of(getTheme()),
  highlightCompartment.of(syntaxHighlighting(getHighlight())),
  EditorView.updateListener.of((update) => {
    if (!editorView.value) return;

    updateEditorStats(update.view);

    if (update.docChanged) {
      isModified.value = update.state.doc.toString() !== originalContent.value;
      statusText.value = isModified.value ? '未保存' : '已保存';

      emit('status-change', {
        text: isModified.value ? '未保存' : '已保存',
        modified: isModified.value,
        error: false,
      });
    }
  }),
];

const loadConfig = async () => {
  loading.value = true;
  try {
    const result = await API.ReadConfigText(props.configId);
    fileName.value = result.name;
    originalContent.value = result.content;
    isModified.value = false;
    statusText.value = '已保存';
    yamlError.value = '';

    emit('status-change', { text: '已保存', modified: false, error: false });

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
    statusText.value = '加载失败: ' + (e.message || e);
    emit('status-change', { text: '加载失败', modified: false, error: true });
  } finally {
    loading.value = false;
  }
};

const handleSave = async () => {
  if (!isModified.value) return;
  saving.value = true;
  statusText.value = '保存中...';
  try {
    const content = editorView.value?.state.doc.toString() || '';
    await API.SaveConfigText(props.configId, content);
    originalContent.value = content;
    isModified.value = false;
    statusText.value = '已保存';
    yamlError.value = '';

    emit('status-change', { text: '已保存', modified: false, error: false });
  } catch (e: any) {
    statusText.value = '保存失败: ' + (e.message || e);
    emit('status-change', { text: '保存失败', modified: isModified.value, error: true });
  } finally {
    saving.value = false;
  }
};

const handleValidate = async () => {
  validating.value = true;
  try {
    const content = editorView.value?.state.doc.toString() || '';
    await API.ValidateConfigText(content);
    yamlError.value = '';
    statusText.value = 'YAML 语法正确';

    emit('status-change', { text: 'YAML 语法正确', modified: isModified.value, error: false });
  } catch (e: any) {
    yamlError.value = e.message || String(e);
    statusText.value = 'YAML 错误: ' + yamlError.value;

    emit('status-change', { text: 'YAML 错误', modified: isModified.value, error: true });
  } finally {
    validating.value = false;
  }
};

const handleReload = () => {
  if (isModified.value) {
    if (!confirm('当前有未保存的修改，确定要重新加载吗？')) return;
  }
  loadConfig();
};

const handleKeydown = (e: KeyboardEvent) => {
  if ((e.ctrlKey || e.metaKey) && e.key === 's') {
    e.preventDefault();
    handleSave();
  }
};

let themeObserver: MutationObserver | null = null;

onMounted(async () => {
  await loadConfig();
  document.addEventListener('keydown', handleKeydown);
  themeObserver = new MutationObserver(() => {
    if (editorView.value) {
      const currentTheme = isDark() ? darkTheme : lightTheme;
      const currentHighlight = isDark() ? darkHighlight : lightHighlight;
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
});

onUnmounted(() => {
  editorView.value?.destroy();
  document.removeEventListener('keydown', handleKeydown);
  themeObserver?.disconnect();
});
</script>

<style scoped>
.yaml-editor-view {
  display: flex;
  flex-direction: column;
  min-height: 100%;
  overflow: visible;
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
  border-radius: 8px;
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
  border-radius: 8px;
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
  background: var(--text-main);
  color: var(--accent-fg);
}

.accent-btn:hover:not(:disabled) {
  opacity: 0.85;
}

.mini-btn {
  padding: 8px 14px;
}

/* 编辑区：自然撑开，不使用内部滚动 */
.editor-body {
  flex: none;
  min-height: calc(100vh - 260px);
  overflow: visible;
  border: none;
  border-radius: 0;
  background: transparent;
}

.editor-body :deep(.cm-editor) {
  height: auto;
  min-height: calc(100vh - 260px);
  border-radius: 12px;
  background: var(--surface);
}

.editor-body :deep(.cm-scroller) {
  overflow: visible !important;
}

.editor-body :deep(.cm-content) {
  min-height: calc(100vh - 260px);
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
  background: rgba(255, 170, 0, 0.1);
  color: #d97706;
  padding: 8px 12px;
  border-radius: 8px;
  font-size: 0.8rem;
  margin-top: 8px;
  display: flex;
  align-items: center;
  border: 1px solid rgba(255, 170, 0, 0.2);
}
</style>
