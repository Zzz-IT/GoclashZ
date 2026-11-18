// 窗口级 hover 清理：WebView 在窗口隐藏/最小化期间收不到 mouseleave，
// 恢复后 Chromium 可能仍按旧指针位置命中 :hover，导致按钮/卡片保持高亮。
// 这里在窗口失焦/隐藏时临时把所有含 :hover 的 CSS 规则改成永不匹配，
// 等用户真正移动鼠标或按下鼠标后再恢复，从而全局清掉陈旧 hover。
type SavedRule = { rule: CSSStyleRule; selectorText: string };

export function installHoverGuard(): () => void {
  let saved: SavedRule[] = [];
  let suppressed = false;

  const collect = (rules: CSSRuleList, out: SavedRule[]) => {
    for (let i = 0; i < rules.length; i++) {
      const rule = rules[i];
      if (rule instanceof CSSStyleRule) {
        if (rule.selectorText.includes(':hover')) {
          out.push({ rule, selectorText: rule.selectorText });
        }
        continue;
      }
      const grouped = rule as CSSGroupingRule;
      if (grouped.cssRules) {
        try { collect(grouped.cssRules, out); } catch { /* ignore inaccessible rules */ }
      }
    }
  };

  const suppress = () => {
    // U-10②：抑制期间也要重新收集。旧实现在 suppressed 为真时直接 return，
    // 于是抑制期间动态插入的 :hover 规则（例如打开 YAML 编辑器后注入的样式表）
    // 永远不会被禁用，同一个窗口状态下的行为前后不一致。
    const found: SavedRule[] = [];
    for (const sheet of Array.from(document.styleSheets)) {
      try {
        if (sheet.cssRules) collect(sheet.cssRules, found);
      } catch { /* cross-origin or not-yet-loaded stylesheet */ }
    }
    const known = new Set(saved.map((item) => item.rule));
    for (const item of found) {
      if (known.has(item.rule)) continue;
      try {
        // :hover -> :not(*) 永不匹配；用户移动鼠标后恢复原选择器
        item.rule.selectorText = item.selectorText.replace(/:hover\b/g, ':not(*)');
        saved.push(item);
      } catch (e) {
        // U-10①：部分浏览器把 selectorText 暴露为只读，改写失败时这条规则保持原样
        // （没被改坏），但必须留痕 —— 否则「某个 hover 清不掉」时完全无线索。
        console.warn('[hoverGuard] suppress failed, rule left intact', e);
      }
    }
    suppressed = saved.length > 0;
  };

  const restore = () => {
    if (!suppressed) return;
    // U-10①：恢复失败的规则不能静默丢弃 —— 一旦丢弃，它会永久停在 :not(*)，
    // 该元素的 hover 效果直到重启都不再出现。留在 saved 里等下一次 mousemove 重试。
    const retry: SavedRule[] = [];
    for (const item of saved) {
      try {
        item.rule.selectorText = item.selectorText;
      } catch (e) {
        retry.push(item);
        console.warn('[hoverGuard] restore failed, will retry', e);
      }
    }
    saved = retry;
    suppressed = saved.length > 0;
  };

  // U-10③：只在窗口真的失焦 / 隐藏时抑制。
  //   1) 旧实现连 focus 也抑制，于是「切回窗口」会把全站 :hover 改写成 :not(*)，
  //      而 restore 只挂在 mousemove/mousedown 上 —— 纯键盘用户切回窗口后
  //      所有 hover 样式一直失效，直到碰一下鼠标；
  //   2) visibilitychange 要区分方向，只有隐藏才需要清陈旧 hover。
  const onBlur = () => suppress();
  const onVisibility = () => {
    if (!document.hidden) return;
    suppress();
  };
  const onMove = () => restore();

  window.addEventListener('blur', onBlur);
  document.addEventListener('visibilitychange', onVisibility);
  window.addEventListener('mousemove', onMove, { passive: true });
  window.addEventListener('mousedown', onMove, { passive: true });

  return () => {
    window.removeEventListener('blur', onBlur);
    document.removeEventListener('visibilitychange', onVisibility);
    window.removeEventListener('mousemove', onMove);
    window.removeEventListener('mousedown', onMove);
    // 卸载时必须把改写过的选择器还回去，否则样式表会停在 :not(*)
    restore();
  };
}
