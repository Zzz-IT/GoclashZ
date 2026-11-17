import { ref } from 'vue';

/**
 * 设置页搜索词（模块级，跨组件重建保留）。
 * 为什么放在模块作用域：设置首页在切到子页时会被卸载，
 * 返回时若用组件内 ref，搜索词会丢失、结果退回成普通菜单。
 */
export const settingsSearchQuery = ref('');
