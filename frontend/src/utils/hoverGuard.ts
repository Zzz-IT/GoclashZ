// 窗口级 hover 清理：WebView 在窗口隐藏/最小化期间收不到 mouseleave，
// 恢复后 Chromium 可能仍按旧指针位置命中 :hover，导致按钮/卡片保持高亮。
// 这里在窗口失焦/恢复时临时把所有含 :hover 的 CSS 规则改成永不匹配，
// 等用户真正移动鼠标或按下鼠标后再恢复，从而全局清掉陈旧 hover。
type SavedRule = { rule: CSSStyleRule; selectorText: string };

export function installHoverGuard(): () => void {
  let saved: SavedRule[] = [];
  let suppressed = false;

  const collect = (rules: CSSRuleList) => {
    for (let i = 0; i < rules.length; i++) {
      const rule = rules[i];
      if (rule instanceof CSSStyleRule) {
        if (rule.selectorText.includes(':hover')) {
          saved.push({ rule, selectorText: rule.selectorText });
        }
        continue;
      }
      const grouped = rule as CSSGroupingRule;
      if (grouped.cssRules) {
        try { collect(grouped.cssRules); } catch { /* ignore inaccessible rules */ }
      }
    }
  };

  const suppress = () => {
    if (suppressed) return;
    saved = [];
    for (const sheet of Array.from(document.styleSheets)) {
      try {
        if (sheet.cssRules) collect(sheet.cssRules);
      } catch { /* cross-origin or not-yet-loaded stylesheet */ }
    }
    if (saved.length === 0) return;
    for (const item of saved) {
      try {
        // :hover -> :not(*) 永不匹配；用户移动鼠标后恢复原选择器
        item.rule.selectorText = item.selectorText.replace(/:hover\b/g, ':not(*)');
      } catch { /* some browsers may expose read-only selectors */ }
    }
    suppressed = true;
  };

  const restore = () => {
    if (!suppressed) return;
    for (const item of saved) {
      try { item.rule.selectorText = item.selectorText; } catch { /* ignore */ }
    }
    saved = [];
    suppressed = false;
  };

  const onBlur = () => suppress();
  const onFocus = () => suppress();
  const onVisibility = () => suppress();
  const onMove = () => restore();

  window.addEventListener('blur', onBlur);
  window.addEventListener('focus', onFocus);
  document.addEventListener('visibilitychange', onVisibility);
  window.addEventListener('mousemove', onMove, { passive: true });
  window.addEventListener('mousedown', onMove, { passive: true });

  return () => {
    window.removeEventListener('blur', onBlur);
    window.removeEventListener('focus', onFocus);
    document.removeEventListener('visibilitychange', onVisibility);
    window.removeEventListener('mousemove', onMove);
    window.removeEventListener('mousedown', onMove);
    restore();
  };
}
