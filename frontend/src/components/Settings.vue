<template>
  <div class="settings-container">
    <Transition name="slide-fade" mode="out-in">
      <div :key="view" class="settings-view-wrapper">
        <SettingsHome v-if="view === 'main'" @navigate="view = $event" @enter-uwp="view = 'uwp'; uwpEntry += 1" />
        <SettingsUpdate v-else-if="view === 'update'" :behavior="behavior" @navigate="view = $event" @save-behavior="saveBehavior" />
        <SettingsTun v-else-if="view === 'tun'" :reset-key="resetVersions.tun" @navigate="view = $event" @reset="confirmReset" />
        <SettingsDns v-else-if="view === 'dns'" :reset-key="resetVersions.dns" @navigate="view = $event" @reset="confirmReset" />
        <SettingsNetwork v-else-if="view === 'network'" :reset-key="resetVersions.network" @navigate="view = $event" @reset="confirmReset" />
        <SettingsBehavior v-else-if="view === 'behavior'" :behavior="behavior" :reset-key="resetVersions.behavior" @navigate="view = $event" @reset="confirmReset" @save="saveBehavior" @startup-change="handleStartupWithOSChange" />
        <SettingsAbout v-else-if="view === 'about'" :behavior="behavior" @navigate="view = $event" @save="saveBehavior" />
        <SettingsUwp v-else-if="view === 'uwp'" :entry="uwpEntry" @navigate="view = $event" />
      </div>
    </Transition>
    <Transition name="pop"><div v-if="showResetConfirm" class="modal-overlay" @click="showResetConfirm = false"><div class="custom-modal-card" @click.stop><div class="modal-header"><h3 class="danger-text">确认重置</h3></div><div class="modal-body"><p class="global-modal-msg">确定要将 <strong>{{ resetModuleName }}</strong> 恢复为默认设置吗？此操作不可撤销，程序将重新加载配置。</p><div class="modal-footer"><button class="action-btn flex-1" @click="showResetConfirm = false">取消</button><button class="primary-btn accent-btn red-text-btn flex-1" @click="handleReset">确认重置</button></div></div></div></div></Transition>
  </div>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue';
import * as API from '../../wailsjs/go/main/App';
import { globalState, showAlert } from '../store';
import SettingsAbout from './SettingsAbout.vue';
import SettingsBehavior from './SettingsBehavior.vue';
import SettingsDns from './SettingsDns.vue';
import SettingsHome from './SettingsHome.vue';
import SettingsNetwork from './SettingsNetwork.vue';
import SettingsTun from './SettingsTun.vue';
import SettingsUpdate from './SettingsUpdate.vue';
import SettingsUwp from './SettingsUwp.vue';

const props = defineProps({ initialView: { type: String, default: 'main' } });
const view = ref(props.initialView as 'main' | 'uwp' | 'tun' | 'dns' | 'network' | 'behavior' | 'update' | 'about');
const uwpEntry = ref(0);
const showResetConfirm = ref(false);
const resetModule = ref('');
const resetModuleName = ref('');
const resetVersions = ref({ tun: 0, dns: 0, network: 0, behavior: 0 });
const modules: Record<string, string> = { network: '基础网络设置', dns: 'DNS 服务器设置', tun: '虚拟网卡设置', behavior: '应用行为设置' };
const behavior = ref<any>({ silentStart: false, closeToTray: true, startupWithOS: false, restoreOnStartup: false, colorDelay: false, delayRetention: false, delayRetentionTime: 'long', proxyTrafficOnly: false, logLevel: 'info', appLogLevel: 'info', hideLogs: false, subUA: '', activeConfig: '', activeMode: '', geoIpLink: '', geoSiteLink: '', mmdbLink: '', asnLink: '', autoUpdate: true, updateMethod: 'startup', updateInterval: 3 });

const loadBehavior = async () => { try { const value = await API.GetAppBehavior(); if (value) behavior.value = value; } catch (error) { console.error('加载配置失败', error); } };
const saveBehavior = async () => { try { await API.SaveAppBehavior(behavior.value); if (behavior.value.appLogLevel) globalState.appLogLevel = behavior.value.appLogLevel; if (behavior.value.logLevel) globalState.logLevel = behavior.value.logLevel; } catch (error) { console.error('应用行为保存失败', error); } };
const handleStartupWithOSChange = async () => { if (!behavior.value.startupWithOS) behavior.value.restoreOnStartup = false; await saveBehavior(); };
const confirmReset = (module: string) => { resetModule.value = module; resetModuleName.value = modules[module]; showResetConfirm.value = true; };
const handleReset = async () => {
  try {
    await API.ResetComponentSettings(resetModule.value);
    showResetConfirm.value = false;
    resetVersions.value[resetModule.value as keyof typeof resetVersions.value] += 1;
    if (resetModule.value === 'behavior') await loadBehavior();
    showAlert(`${resetModuleName.value} 已重置为默认值`, '成功');
  } catch (error) {
    console.error('重置失败:', error);
    showAlert('重置失败: ' + error, '错误');
  }
};

void loadBehavior();
watch(() => props.initialView, value => { view.value = value as typeof view.value; });
watch(view, () => { setTimeout(() => document.querySelector('.view-scroller')?.scrollTo({ top: 0, behavior: 'auto' }), 250); });
</script>

<style scoped>
.settings-container{display:flex;flex-direction:column;min-height:100%;overflow:visible;position:relative}.settings-view-wrapper{display:flex;flex-direction:column;width:100%}.slide-fade-enter-active,.slide-fade-leave-active{transition:all .25s cubic-bezier(.4,0,.2,1);width:100%;height:100%}.slide-fade-enter-from{opacity:0;transform:translateX(12px)}.slide-fade-leave-to{opacity:0;transform:translateX(-12px)}.modal-footer .action-btn,.modal-footer .primary-btn{height:42px;display:flex;align-items:center;justify-content:center}
</style>
