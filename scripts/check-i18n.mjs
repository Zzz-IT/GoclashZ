#!/usr/bin/env node
// i18n 一致性校验（基于 TypeScript AST，非正则）：
//  - error：代码 t('字面量') 用到但基准语言未定义 / 某语言缺了基准语言有的 key
//  - info ：动态 t(key) 调用（map[key]、e.titleKey 等）无法静态校验，列出但不失败
//  - warn ：定义了但代码未直接引用（死键），只报告不删除，避免误删动态 key。
// 运行：node scripts/check-i18n.mjs；已接入 frontend build。

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
  const keys = new Set();
  function visit(node, prefix) {
    if (ts.isObjectLiteralExpression(node)) {
      for (const p of node.properties) {
        if (!ts.isPropertyAssignment(p)) continue;
        let name = null;
        if (ts.isIdentifier(p.name) || ts.isStringLiteral(p.name)) name = p.name.text;
        if (!name) continue;
        const full = prefix ? prefix + '.' + name : name;
        keys.add(full);
        if (ts.isObjectLiteralExpression(p.initializer)) visit(p.initializer, full);
      }
      return;
    }
    ts.forEachChild(node, n => visit(n, prefix));
  }
  visit(sf, '');
  return keys;
}

function codeKeyRefs(file) {
  const text = fs.readFileSync(file, 'utf8');
  const sf = ts.createSourceFile(file, text, ts.ScriptTarget.Latest, true, ts.ScriptKind.TS);
  const literal = new Set(), dynamic = [];
  function visit(n) {
    if (ts.isCallExpression(n) && ts.isIdentifier(n.expression) && n.expression.text === 't') {
      const a = n.arguments[0];
      if (a && ts.isStringLiteralLike(a)) literal.add(a.text);
      else if (a) dynamic.push(ts.SyntaxKind[a.kind]);
    }
    ts.forEachChild(n, visit);
  }
  visit(sf);
  return { literal, dynamic };
}

const codeLiteral = new Set();
const dynamicRefs = [];
for (const file of walk(srcDir)) {
  if (file.startsWith(localesDir + path.sep)) continue;
  const text = fs.readFileSync(file, 'utf8');
  // .vue 的 <template> 段 TS AST 看不到，用字面量正则兜底（只取 t('...') 形式）
  for (const m of text.matchAll(/\bt\(\s*(['"])([^'"\n]+)\1/g)) codeLiteral.add(m[2]);
  const { literal, dynamic } = codeKeyRefs(file);
  for (const k of literal) codeLiteral.add(k);
  const rel = path.relative(root, file).replace(/\\/g, '/');
  for (const kind of dynamic) dynamicRefs.push(rel + ': ' + kind);
}

const keys = {};
for (const loc of LOCALES) keys[loc] = localeKeys(path.join(localesDir, loc + '.ts'));
const errors = [];
for (const k of codeLiteral) if (!keys[BASE].has(k)) errors.push('代码使用但 ' + BASE + ' 未定义: ' + k);
for (const loc of LOCALES) for (const k of keys[BASE]) if (!keys[loc].has(k)) errors.push(loc + ' 缺少 key: ' + k);
const dead = [...keys[BASE]].filter(k => !codeLiteral.has(k)).sort();

console.log('[i18n] locales=' + LOCALES.length + ' codeKeys=' + codeLiteral.size + ' baseKeys=' + keys[BASE].size + ' dynamicCalls=' + dynamicRefs.length);
if (dynamicRefs.length) console.log('[i18n] 动态 key 调用（无法静态校验，需保证对应文案存在）：\n  ' + [...new Set(dynamicRefs)].slice(0, 20).join('\n  '));
if (dead.length) {
  console.warn('[i18n] 死键 ' + dead.length + ' 个（warn，不阻断；静态不可见的动态 key 需人工确认）：');
  console.warn(dead.slice(0, 30).map(k => '  ' + k).join('\n'));
  if (dead.length > 30) console.warn('  ... 另 ' + (dead.length - 30) + ' 个（完整清单可用 AST 工具导出）');
}
if (errors.length) {
  console.error('[i18n] 校验失败 ' + errors.length + ' 项：');
  console.error(errors.slice(0, 40).map(e => '  ' + e).join('\n'));
  if (errors.length > 40) console.error('  ... 另 ' + (errors.length - 40) + ' 项');
  process.exit(1);
}
console.log('[i18n] OK：语言对齐 + 字面量 key 全部存在（动态 key 仅提示）');