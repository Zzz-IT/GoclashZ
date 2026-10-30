<template>
  <div class="settings-page">
    <!-- 设置项搜索：输入页名或字段名，结果按页分组，点击直达该字段 -->
    <div class="settings-search">
      <span class="settings-search__icon" v-html="ICONS.search"></span>
      <input v-model="query" :placeholder="t('settings.searchPlaceholder')" />
      <button v-if="query" class="settings-search__clear" @click="query = ''">✕</button>
    </div>

    <template v-if="!q">
      <div class="glass-card setting-group">
        <h3>{{ t('settings.home.networkGroup') }}</h3>
        <div v-for="e in networkEntries" :key="e.id" class="setting-item clickable" @click="go(e)">
          <div class="info">
            <h4>{{ t(e.titleKey) }}</h4>
            <p>{{ t(e.descKey) }}</p>
          </div>
          <span class="arrow">➔</span>
        </div>
      </div>

      <div class="glass-card setting-group">
        <h3>{{ t('settings.home.appGroup') }}</h3>
        <div v-for="e in appEntries" :key="e.id" class="setting-item clickable" @click="go(e)">
          <div class="info">
            <h4>{{ t(e.titleKey) }}</h4>
            <p>{{ t(e.descKey) }}</p>
          </div>
          <span class="arrow">➔</span>
        </div>
      </div>
    </template>

    <template v-else>
      <div v-for="g in groups" :key="g.page" class="search-group">
        <h3 class="search-group__title">{{ t(SETTING_PAGE_TITLES[g.page]) }}</h3>
        <div v-for="f in g.items" :key="f.id" class="search-row" @click="jump(f)">
          <span class="search-row__title">{{ labelOf(f) }}</span>
          <span v-if="sectionLabel(f)" class="search-row__section">{{ sectionLabel(f) }}</span>
        </div>
      </div>
      <EmptyState v-if="!groups.length" :icon="ICONS.search" :title="t('settings.searchEmpty')" />
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { t } from '../locales';
import { ICONS } from '../utils/icons';
import EmptyState from './EmptyState.vue';
import { SETTING_FIELDS, SETTING_PAGE_TITLES, type SettingField, type SettingPage } from '../settings/manifest';
import { settingsSearchQuery } from '../settings/searchState';

const emit = defineEmits<{
  navigate: [view: SettingPage, anchor?: string];
  'enter-uwp': [];
}>();

type EntryId = SettingPage;
type Entry = { id: EntryId; titleKey: string; descKey: string };

const entries: Entry[] = [
  { id: 'network', titleKey: 'settings.home.network', descKey: 'settings.home.networkDesc' },
  { id: 'dns', titleKey: 'settings.home.dns', descKey: 'settings.home.dnsDesc' },
  { id: 'tun', titleKey: 'settings.home.tun', descKey: 'settings.home.tunDesc' },
  { id: 'lan', titleKey: 'settings.home.lan', descKey: 'settings.home.lanDesc' },
  { id: 'behavior', titleKey: 'settings.home.behavior', descKey: 'settings.home.behaviorDesc' },
  { id: 'uwp', titleKey: 'settings.home.uwp', descKey: 'settings.home.uwpDesc' },
  { id: 'update', titleKey: 'settings.home.update', descKey: 'settings.home.updateDesc' },
  { id: 'about', titleKey: 'settings.home.about', descKey: 'settings.home.aboutDesc' },
];

// 搜索词来自模块级 ref：从子页返回时结果不丢
const query = settingsSearchQuery;
const q = computed(() => query.value.trim().toLowerCase());

const networkEntries = computed(() => entries.filter((e) => e.id === 'network' || e.id === 'dns' || e.id === 'tun' || e.id === 'lan'));
const appEntries = computed(() => entries.filter((e) => networkEntries.value.indexOf(e) < 0));

// 参数化标题：Geo 数据库四行共用 settings.update.dbFile({name})
const dbNames: Record<string, string> = { Geoip: 'GeoIP', Geosite: 'GeoSite', Mmdb: 'MMDB', Asn: 'ASN' };

const labelOf = (f: SettingField): string => {
  if (f.titleKey === 'settings.update.dbFile') {
    const suffix = f.id.replace('set-update-dbFile', '');
    return t('settings.update.dbFile', { name: dbNames[suffix] || suffix });
  }
  if (!f.titleKey) return t(SETTING_PAGE_TITLES[f.page]);
  return t(f.titleKey);
};

// 只有当分组标题与页标题不同时才显示（避免每行重复一遍页名）
const sectionLabel = (f: SettingField): string => {
  const pageTitle = t(SETTING_PAGE_TITLES[f.page]);
  const group = t(f.group);
  return group === pageTitle ? '' : group;
};

const groups = computed(() => {
  if (!q.value) return [];
  const hit = SETTING_FIELDS.filter((f) => {
    if (labelOf(f).toLowerCase().includes(q.value)) return true;
    return f.keywords.some((k) => k.toLowerCase().includes(q.value));
  });
  const order: SettingPage[] = ['dns', 'network', 'tun', 'lan', 'behavior', 'update', 'about', 'uwp'];
  const map = new Map<SettingPage, SettingField[]>();
  for (const f of hit) {
    const arr = map.get(f.page) || [];
    arr.push(f);
    map.set(f.page, arr);
  }
  return order.filter((p) => map.has(p)).map((p) => ({ page: p, items: map.get(p) as SettingField[] }));
});

const go = (e: Entry) => {
  if (e.id === 'uwp') emit('enter-uwp');
  else emit('navigate', e.id);
};

const jump = (f: SettingField) => {
  emit('navigate', f.page, f.id);
};
</script>

<style scoped>
.settings-page {
  display: flex;
  flex-direction: column;
  flex: 1;
  min-height: 100%;
  overflow: visible;
}
.settings-search {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 12px;
  padding: 0 12px;
  height: 40px;
  border-radius: 10px;
  background: var(--surface);
}
.settings-search__icon {
  display: flex;
  align-items: center;
  color: var(--text-muted);
}
.settings-search__icon :deep(svg) {
  width: 16px;
  height: 16px;
}
.settings-search input {
  flex: 1;
  min-width: 0;
  border: none;
  background: transparent;
  color: var(--text-main);
  font-family: inherit;
  font-size: 0.88rem;
}
.settings-search__clear {
  border: none;
  background: none;
  color: var(--text-muted);
  font-size: 0.85rem;
  cursor: pointer;
  padding: 0 2px;
}
.settings-search__clear:hover {
  color: var(--text-main);
}
.setting-group { padding: 20px 24px; margin-bottom: 12px; }

/* 搜索结果：紧凑列表，不再复用浏览态的宽松行距 */
.search-group {
  padding: 10px 14px 8px;
  margin-bottom: 8px;
  border-radius: 12px;
  background: var(--surface-panel);
}
.search-group__title {
  margin: 0 0 6px 0;
  padding: 2px 4px 0;
  font-size: 1.15rem;
  font-weight: 600;
  color: var(--text-main);
}
.search-row {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 7px 8px;
  border-radius: 8px;
  cursor: pointer;
}
.search-row:hover { background: var(--surface-hover); }
.search-row__title {
  flex: 1;
  min-width: 0;
  font-size: 0.9rem;
  color: var(--text-main);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.search-row__section {
  flex: 0 0 auto;
  font-size: 0.75rem;
  color: var(--text-muted);
}

h3 { margin: 0 0 8px 0; color: var(--text-main); font-size: 1.25rem; padding-bottom: 4px; }
h4 { margin: 0 0 6px 0; color: var(--text-main); font-size: 1rem; }
p { margin: 0; font-size: 0.85rem; color: var(--text-sub); max-width: 100%; line-height: 1.5; }
.info { flex: 1; padding-right: 24px; min-width: 0; }
.setting-item { display: flex; justify-content: space-between; align-items: center; padding: 14px 0; }
.setting-item.clickable { cursor: pointer; padding: 16px; border-radius: 12px; margin: 0 -16px; transition: 0.2s; }
.setting-item.clickable:hover { background: var(--surface-hover); }
.arrow { color: var(--text-sub); font-size: 1.2rem; }
</style>
