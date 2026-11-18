// localStorage 读写的唯一薄包装（P-34 / S-05 / C-11 / U-09）。
//
// localStorage 在 WebView2 隐私模式 / 配额满时会直接抛 SecurityError / QuotaExceededError。
// 页面里的读写既在 setup 顶层（抛出去就是整页白屏），也在 watch 里（抛出去会打断状态同步），
// 所以 Proxies.vue / Connections.vue / Rules.vue 各抄了一份同样的 try/catch。本文件收敛成一份：
//   - 读失败一律返回 null，由调用方各自回退默认值
//   - 写失败静默（缓存只是「记住上次选择」，存不下不影响功能）
// 语义与各页面原先的局部实现逐行等价。
//
// 注意：store.ts / utils/theme.ts / locales/index.ts 里还有同构的私有包装，
// 因不在本轮可改文件范围内未合并，见 docs/audit-frontend/handoff-r1.md。

/** 读一个字符串缓存；失败（隐私模式 / 配额异常）返回 null，由调用方回退默认值 */
export function safeGetString(key: string): string | null {
  try {
    return localStorage.getItem(key);
  } catch {
    return null;
  }
}

/** 写一个字符串缓存；失败静默（存不下不影响功能） */
export function safeSetString(key: string, value: string): void {
  try {
    localStorage.setItem(key, value);
  } catch {
    /* 隐私模式 / 配额满：忽略，不能因为写缓存失败打断交互 */
  }
}
