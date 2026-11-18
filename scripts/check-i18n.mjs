#!/usr/bin/env node
// i18n 一致性校验（TypeScript AST + 轻量词法扫描，非纯正则）：
//  - error：代码 t('字面量') / tr('字面量') 用到但基准语言未定义 / 某语言缺了基准语言有的 key
//  - info ：动态 t(key) 调用（map[key]、e.titleKey 等）无法静态校验，列出但不失败
//  - warn ：叶子文案定义了但代码未直接引用（死键），只报告不阻断。
// 运行：node scripts/check-i18n.mjs；已接入 frontend build。
//
// 死键判定（Wave 2 增强，修掉动态拼接盲区，见 docs/audit-frontend/fix-i18n.md）：
//  1) 源码里任何 'a.b.c' 形式的点分 token 只要与某个叶子 key 全等，就算已用 ——
//     覆盖 manifest.ts 的 titleKey: 'settings.dns.enableDns'、utils/theme.ts 的 nameKey、
//     utils/errors.ts 的错误码常量表、以及 utils 里 tr('common.xxx') 的间接调用。
//  2) 前缀 + 拼接视为整族已用：'deviation.severity.' + code、`diag.note.${code}`、
//     localized('deviation.field', field) —— 这些前缀命中后其下所有 key 都不再算死键。
//  3) 只把「值本身是字符串」的叶子算死键候选；命名空间对象（settings、subs…）不参与。

import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { createRequire } from 'node:module';

// 脚本在 scripts/ 下，而 typescript 装在 frontend/node_modules —— 用 frontend/package.json 建 require 解析
const frontendDir = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..', 'frontend');
const require = createRequire(path.join(frontendDir, 'package.json'));
const ts = require('typescript');

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const root = path.resolve(__dirname, '..');
const srcDir = path.join(root, 'frontend/src');
const localesDir = path.join(srcDir, 'locales');
const LOCALES = ['zh-CN', 'zh-TW', 'zh-HK', 'en-US', 'ja-JP', 'ru-RU', 'fr-FR', 'de-DE'];
const BASE = 'zh-CN';

// .vue 模板段的点分 token（至少两段；可选尾部点表示「前缀 + 拼接」），不依赖引号配对
const TEMPLATE_TOKEN = /[A-Za-z0-9_$][A-Za-z0-9_$-]*(?:\.[A-Za-z0-9_$][A-Za-z0-9_$-]+)+\.?/g;

function walk(dir, out = []) {
  for (const e of fs.readdirSync(dir, { withFileTypes: true })) {
    const p = path.join(dir, e.name);
    if (e.isDirectory()) walk(p, out);
    else if (/\.(ts|vue)$/.test(e.name)) out.push(p);
  }
  return out;
}

// 用 AST 收集对象字面量的完整点分路径 key
function localeKeys(file) {
  const text = fs.readFileSync(file, 'utf8');
  const sf = ts.createSourceFile(file, text, ts.ScriptTarget.Latest, true);
  const keys = new Set(), namespaces = new Set();
  function visit(node, prefix) {
    if (ts.isObjectLiteralExpression(node)) {
      for (const p of node.properties) {
        if (!ts.isPropertyAssignment(p)) continue;
        let name = null;
        if (ts.isIdentifier(p.name) || ts.isStringLiteral(p.name)) name = p.name.text;
        if (!name) continue;
        const full = prefix ? prefix + '.' + name : name;
        keys.add(full);
        if (ts.isObjectLiteralExpression(p.initializer)) {
          namespaces.add(full);
          visit(p.initializer, full);
        }
      }
      return;
    }
    ts.forEachChild(node, n => visit(n, prefix));
  }
  visit(sf, '');
  const leaves = new Set([...keys].filter(k => !namespaces.has(k)));
  return { keys, namespaces, leaves };
}

function codeKeyRefs(file) {
  const text = fs.readFileSync(file, 'utf8');
  const sf = ts.createSourceFile(file, text, ts.ScriptTarget.Latest, true, ts.ScriptKind.TS);
  const literal = new Set(), dynamic = [];
  const WRAPPERS = new Set(['t', 'tr', 'localized']);
  function visit(n) {
    if (ts.isCallExpression(n) && ts.isIdentifier(n.expression) && WRAPPERS.has(n.expression.text)) {
      const a = n.arguments[0];
      if (a && ts.isStringLiteralLike(a)) literal.add(a.text);
      else if (a) dynamic.push(ts.SyntaxKind[a.kind]);
    }
    ts.forEachChild(n, visit);
  }
  visit(sf);
  return { literal, dynamic };
}

const locales = {}, localeNamespaces = {};
for (const loc of LOCALES) {
  const info = localeKeys(path.join(localesDir, loc + '.ts'));
  locales[loc] = info.keys;
  localeNamespaces[loc] = info.namespaces;
}
const baseLeaves = localeKeys(path.join(localesDir, BASE + '.ts')).leaves;

// 引用收集：完整 key 字面量 + 「前缀 + 拼接」的前缀族（后者为整族文案背书）
const codeLiteral = new Set();   // t()/tr()/localized() 的字面量实参，参与「key 是否定义」校验
const codeKeys = new Set();      // 代码里出现过的完整 key（含 manifest.ts / theme.ts 等数据表）
const prefixSet = new Set();     // 拼接前缀，例如 'deviation.severity.'、'diag.note.'
const dynamicRefs = [];

// 只接受「确实能匹配到既有 key」的前缀，避免把 2. / p. 这类噪声当成前缀族
function addPrefix(p) {
  if (!p || p.length < 2) return;
  for (const k of baseLeaves) if (k.startsWith(p)) { prefixSet.add(p); return; }
  for (const k of localeNamespaces[BASE]) if (k.startsWith(p)) { prefixSet.add(p); return; }
}

// 单个点分字面量：完整 key / 命名空间 / 拼接前缀 三种可能
// 注意：单段命名空间（'tray'、'settings'）不算引用 —— manifest 的关键词表里就有 'tray' 这种普通单词，
// 若按命名空间处理会把整族文案全部「洗白」。
function addLiteral(v) {
  if (baseLeaves.has(v)) codeKeys.add(v);
  else if (v.endsWith('.')) addPrefix(v);
  else if (v.includes('.') && localeNamespaces[BASE].has(v)) addPrefix(v + '.');
}

function scanTsAst(text, sf) {
  function visit(n) {
    if (ts.isStringLiteralLike(n)) addLiteral(n.text);
    else if (ts.isTemplateExpression(n)) addPrefix(n.head.text); // `diag.note.${code}`
    ts.forEachChild(n, visit);
  }
  visit(sf);
}

for (const file of walk(srcDir)) {
  if (file.startsWith(localesDir + path.sep)) continue;
  const text = fs.readFileSync(file, 'utf8');
  // .vue 的 <template> 段 TS AST 看不到，用字面量正则兜底（只取 t('...') / tr('...') 形式）
  for (const m of text.matchAll(/\b(?:t|tr)\(\s*(['"])([^'"\n]+)\1/g)) codeLiteral.add(m[2]);
  const { literal, dynamic } = codeKeyRefs(file);
  for (const k of literal) codeLiteral.add(k);
  const rel = path.relative(root, file).replace(/\\/g, '/');
  for (const kind of dynamic) dynamicRefs.push(rel + ': ' + kind);

  if (file.endsWith('.vue')) {
    // 模板段不是 TS：用点分 token 扫描（不依赖引号配对，避开 don't 之类的错位）
    for (const m of text.matchAll(/<script[^>]*>([\s\S]*?)<\/script>/g)) {
      scanTsAst(m[1], ts.createSourceFile(file, m[1], ts.ScriptTarget.Latest, true, ts.ScriptKind.TS));
    }
    const tpl = /<template>([\s\S]*)<\/template>/.exec(text);
    for (const m of (tpl ? tpl[1] : text).matchAll(TEMPLATE_TOKEN)) addLiteral(m[0]);
  } else {
    scanTsAst(text, ts.createSourceFile(file, text, ts.ScriptTarget.Latest, true, ts.ScriptKind.TS));
  }
}

// ---- 死键判定：完整 key 字面量 + 前缀族展开 ----
const usedKeys = new Set(codeKeys);
for (const p of prefixSet) for (const k of baseLeaves) if (k.startsWith(p)) usedKeys.add(k);

const errors = [];
for (const k of codeLiteral) if (!locales[BASE].has(k)) errors.push('代码使用但 ' + BASE + ' 未定义: ' + k);
for (const loc of LOCALES) for (const k of locales[BASE]) if (!locales[loc].has(k)) errors.push(loc + ' 缺少 key: ' + k);
const dead = [...baseLeaves].filter(k => !usedKeys.has(k)).sort();

console.log('[i18n] locales=' + LOCALES.length + ' codeKeys=' + codeLiteral.size + ' baseKeys=' + locales[BASE].size + ' baseLeaves=' + baseLeaves.size + ' dynamicCalls=' + dynamicRefs.length);
if (dynamicRefs.length) console.log('[i18n] 动态 key 调用（无法静态校验，需保证对应文案存在）：\n  ' + [...new Set(dynamicRefs)].slice(0, 20).join('\n  '));
if (dead.length) {
  console.warn('[i18n] 死键 ' + dead.length + ' 个（warn，不阻断；已按「前缀拼接 + 字面量」增强判定）：');
  console.warn(dead.map(k => '  ' + k).join('\n'));
}
if (errors.length) {
  console.error('[i18n] 校验失败 ' + errors.length + ' 项：');
  console.error(errors.slice(0, 40).map(e => '  ' + e).join('\n'));
  if (errors.length > 40) console.error('  ... 另 ' + (errors.length - 40) + ' 项');
  process.exit(1);
}
console.log('[i18n] OK：语言对齐 + 字面量 key 全部存在（动态 key 仅提示）');
