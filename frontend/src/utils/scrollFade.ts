/**
 * 滚动边缘渐隐：只负责根据滚动位置写 data-fade-* 属性。
 *
 * 淡入淡出由 CSS 的 mask 渐变完成 —— 渐变位置用一个 @property 注册过的
 * 长度变量控制，所以可以 transition（mask-image 本身不可过渡，
 * 但"由自定义属性驱动的停靠点"可以，这是唯一既不变色又能平滑渐入的写法）。
 */

const VERTICAL_SELECTOR = '.ui-page-body';
const HORIZONTAL_SELECTOR = '.proxy-tabs-track, .rules-tabs-track, .conn-tabs-track';

type Axis = 'y' | 'x';

function apply(el: HTMLElement, axis: Axis) {
  if (axis === 'y') {
    const max = el.scrollHeight - el.clientHeight;
    el.dataset.fadeTop = el.scrollTop > 2 ? '1' : '0';
    el.dataset.fadeBottom = max > 2 && el.scrollTop < max - 2 ? '1' : '0';
    return;
  }
  const max = el.scrollWidth - el.clientWidth;
  el.dataset.fadeLeft = el.scrollLeft > 2 ? '1' : '0';
  el.dataset.fadeRight = max > 2 && el.scrollLeft < max - 2 ? '1' : '0';
}

function update(el: HTMLElement) {
  if (el.matches(VERTICAL_SELECTOR)) apply(el, 'y');
  if (el.matches(HORIZONTAL_SELECTOR)) apply(el, 'x');
}

function updateAll() {
  document.querySelectorAll<HTMLElement>(VERTICAL_SELECTOR).forEach(update);
  document.querySelectorAll<HTMLElement>(HORIZONTAL_SELECTOR).forEach(update);
}

let started = false;

export function initScrollFades() {
  if (started) return;
  started = true;

  // 滚动事件不冒泡，用捕获阶段统一接住所有容器
  document.addEventListener(
    'scroll',
    (e) => {
      const target = e.target as HTMLElement | null;
      if (target && typeof target.matches === 'function') update(target);
    },
    true
  );

  if (typeof ResizeObserver !== 'undefined') {
    const ro = new ResizeObserver(() => updateAll());
    ro.observe(document.body);
  }

  if (typeof MutationObserver !== 'undefined') {
    const mo = new MutationObserver(() => updateAll());
    mo.observe(document.body, { childList: true, subtree: true, attributes: true, attributeFilter: ['class', 'style'] });
  }

  window.addEventListener('resize', updateAll);
  requestAnimationFrame(updateAll);
  window.setTimeout(updateAll, 300);
}
