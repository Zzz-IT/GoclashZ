<template>
  <div class="settings-page">
    <div class="glass-card setting-group">
      <h3>{{ t('settings.home.networkGroup') }}</h3>

      <div class="setting-item clickable" @click="$emit('navigate', 'network')">
        <div class="info">
          <h4>{{ t('settings.home.network') }}</h4>
          <p>{{ t('settings.home.networkDesc') }}</p>
        </div>
        <span class="arrow">➔</span>
      </div>

      <div class="setting-item clickable" @click="$emit('navigate', 'dns')">
        <div class="info">
          <h4>{{ t('settings.home.dns') }}</h4>
          <p>{{ t('settings.home.dnsDesc') }}</p>
        </div>
        <span class="arrow">➔</span>
      </div>

      <div class="setting-item clickable" @click="$emit('navigate', 'tun')">
        <div class="info">
          <h4>{{ t('settings.home.tun') }}</h4>
          <p>{{ t('settings.home.tunDesc') }}</p>
        </div>
        <span class="arrow">➔</span>
      </div>

      <div class="setting-item clickable" @click="$emit('navigate', 'lan')">
        <div class="info">
          <h4>{{ t('settings.home.lan') }}</h4>
          <p>{{ t('settings.home.lanDesc') }}</p>
        </div>
        <span class="arrow">➔</span>
      </div>
    </div>

    <div class="glass-card setting-group">
      <h3>{{ t('settings.home.appGroup') }}</h3>

      <div class="setting-item clickable" @click="$emit('navigate', 'behavior')">
        <div class="info">
          <h4>{{ t('settings.home.behavior') }}</h4>
          <p>{{ t('settings.home.behaviorDesc') }}</p>
        </div>
        <span class="arrow">➔</span>
      </div>

      <div class="setting-item clickable" @click="$emit('enter-uwp')">
        <div class="info">
          <h4>{{ t('settings.home.uwp') }}</h4>
          <p>{{ t('settings.home.uwpDesc') }}</p>
        </div>
        <span class="arrow">➔</span>
      </div>

      <div class="setting-item clickable" @click="$emit('navigate', 'update')">
        <div class="info">
          <h4>{{ t('settings.home.update') }}</h4>
          <p>{{ t('settings.home.updateDesc') }}</p>
        </div>
        <span class="arrow">➔</span>
      </div>

      <div class="setting-item clickable" @click="$emit('navigate', 'about')">
        <div class="info">
          <h4>{{ t('settings.home.about') }}</h4>
          <p>{{ t('settings.home.aboutDesc') }}</p>
        </div>
        <span class="arrow">➔</span>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { t } from '../locales';

defineEmits<{
  navigate: [view: 'network' | 'dns' | 'tun' | 'lan' | 'behavior' | 'update' | 'about'];
  'enter-uwp': [];
}>();
</script>

<style scoped>
.settings-page {
  display: flex;
  flex-direction: column;
  flex: 1;
  min-height: 100%;
  overflow: visible;
}
.setting-group { padding: 20px 24px; margin-bottom: 12px; }
h3 { margin: 0 0 8px 0; color: var(--text-main); font-size: 1.25rem; padding-bottom: 4px; }
h4 { margin: 0 0 6px 0; color: var(--text-main); font-size: 1rem; }
p { margin: 0; font-size: 0.85rem; color: var(--text-sub); max-width: 100%; line-height: 1.5; }
.info { flex: 1; padding-right: 24px; min-width: 0; }
.setting-item { display: flex; justify-content: space-between; align-items: center; padding: 14px 0; }
.setting-item.clickable { cursor: pointer; padding: 16px; border-radius: 12px; margin: 0 -16px; transition: 0.2s; }
.setting-item.clickable:hover { background: var(--surface-hover); }
.arrow { color: var(--text-sub); font-size: 1.2rem; }
</style>
