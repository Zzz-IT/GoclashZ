#!/usr/bin/env node
// 校验设置项清单与页面模板是否一致，防止 manifest 与 .vue 漂移。
// 规则：
//   1) manifest 条目数必须与解析到的 id 数一致（防止正则漏匹配造成"假绿"）；
//   2) 每个 id 必须在该页 .vue 的模板里出现 id="..."（v-for 动态拼接的按前缀匹配）；
//   3) page 必须映射到已知文件，否则直接报错（不再 path.join(undefined)）。
// 失败以非 0 退出，已接在 npm build 前。

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

// v-for 动态拼接的 id：只校验前缀出现在模板里
const DYNAMIC_PREFIX = ['set-update-dbFile'];

const manifest = fs.readFileSync(manifestPath, 'utf8');

// 只提取 id 与 page，不依赖两者的换行顺序
const entries = [...manifest.matchAll(/id:\s*'([^']+)'/g)].map((m) => m[1]);
const pages = [...manifest.matchAll(/page:\s*'([^']+)'/g)].map((m) => m[1]);

if (entries.length === 0 || entries.length !== pages.length) {
  console.error(
    '[settings-manifest] 解析条目失败：id=' + entries.length + ' page=' + pages.length + '（两者必须相等且大于 0）'
  );
  process.exit(1);
}

const fields = entries.map((id, i) => ({ id, page: pages[i] }));

const cache = new Map();
const loadPage = (page) => {
  if (cache.has(page)) return cache.get(page);
  const file = PAGE_FILES[page];
  if (!file) {
    console.error('[settings-manifest] manifest 里出现了未知页面：' + page);
    process.exit(1);
  }
  const full = path.join(componentsDir, file);
  if (!fs.existsSync(full)) {
    console.error('[settings-manifest] 找不到页面文件：' + file);
    process.exit(1);
  }
  const text = fs.readFileSync(full, 'utf8');
  cache.set(page, text);
  return text;
};

const missing = [];
for (const { id, page } of fields) {
  const text = loadPage(page);
  const dynamic = DYNAMIC_PREFIX.find((p) => id.startsWith(p));
  // 精确匹配锚点属性，避免 id 只是别处字符串的子串
  const found = dynamic
    ? text.includes("'" + dynamic + "'")
    : new RegExp('id\\s*=\\s*"' + id + '"').test(text);
  if (!found) missing.push({ id, page });
}

if (missing.length > 0) {
  console.error('[settings-manifest] 以下 ' + missing.length + ' 个设置项在页面模板里找不到锚点：');
  for (const item of missing) {
    console.error('  - ' + item.id + ' （期望出现在 ' + PAGE_FILES[item.page] + '）');
  }
  console.error('[settings-manifest] 请在对应 setting-item 上加 id="<上面的 id>"');
  process.exit(1);
}

console.log('[settings-manifest] OK，' + fields.length + ' 个设置项锚点全部就位');
