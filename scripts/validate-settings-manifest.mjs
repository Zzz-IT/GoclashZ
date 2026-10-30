#!/usr/bin/env node
// 校验设置项清单与页面模板是否一致，防止 manifest 与 .vue 漂移。
// 规则：manifest 里每个 id，必须能在对应页面的 .vue 源码里找到（静态 id 或动态拼接前缀）。
// 失败时以非 0 退出，可直接接在 npm build 前。

import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const root = path.resolve(__dirname, '..');
const manifestPath = path.join(root, 'frontend/src/settings/manifest.ts');
const componentsDir = path.join(root, 'frontend/src/components');

const PAGE_FILES = {
  dns: 'SettingsDns.vue',
  network: 'SettingsNetwork.vue',
  tun: 'SettingsTun.vue',
  lan: 'SettingsLan.vue',
  behavior: 'SettingsBehavior.vue',
  update: 'SettingsUpdate.vue',
  about: 'SettingsAbout.vue',
  uwp: 'SettingsUwp.vue',
};

// 由 v-for 动态拼接的 id：只校验前缀出现在模板里
const DYNAMIC_PREFIX = ['set-update-dbFile'];

const manifest = fs.readFileSync(manifestPath, 'utf8');
const entries = [];
const re = /\{\s*id:\s*'([^']+)',\s*\n\s*page:\s*'([^']+)'/g;
let m;
while ((m = re.exec(manifest)) !== null) {
  entries.push({ id: m[1], page: m[2] });
}

if (entries.length === 0) {
  console.error('[settings-manifest] 未从 manifest.ts 解析到任何条目，格式可能被改动');
  process.exit(1);
}

const cache = new Map();
const loadPage = (page) => {
  if (cache.has(page)) return cache.get(page);
  const file = PAGE_FILES[page];
  const full = path.join(componentsDir, file);
  const text = fs.existsSync(full) ? fs.readFileSync(full, 'utf8') : '';
  cache.set(page, text);
  return text;
};

const missing = [];
for (const { id, page } of entries) {
  const text = loadPage(page);
  const dynamic = DYNAMIC_PREFIX.find((p) => id.startsWith(p));
  const needle = dynamic ? dynamic : id;
  if (!text.includes(needle)) missing.push({ id, page, needle });
}

if (missing.length > 0) {
  console.error('[settings-manifest] 以下 ' + missing.length + ' 个设置项在页面模板里找不到锚点：');
  for (const item of missing) {
    console.error('  - ' + item.id + ' （期望出现在 ' + PAGE_FILES[item.page] + '）');
  }
  console.error('[settings-manifest] 请在对应 setting-item 上加 id="<上面的 id>"');
  process.exit(1);
}

console.log('[settings-manifest] OK，' + entries.length + ' 个设置项锚点全部就位');
