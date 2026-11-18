/**
 * 滚动边缘渐隐：只负责根据滚动位置写 data-fade-* 属性。
 *
 * 淡入淡出由 CSS 的 mask 渐变完成 —— 渐变位置用一个 @property 注册过的
 * 长度变量控制，所以可以 transition（mask-image 本身不可过渡，
 * 但"由自定义属性驱动的停靠点"可以，这是唯一既不变色又能平滑渐入的写法）。
 */

const VERTICAL_SELECTOR = '.ui-page-body';
const HORIZONTAL_SELECTOR = '.proxy-tabs-track, .rules-tabs-track, .conn-tabs-track';
/** 两类滚动容器的合集：判断一次 mutation「要不要重算」用 */
const ANY_SELECTOR = VERTICAL_SELECTOR + ', ' + HORIZONTAL_SELECTOR;

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

/**
 * U-11 ①：把观察器触发合并到一帧。
 * updateAll() 每次都要跑两遍 document.querySelectorAll + 逐元素读 scrollHeight/clientHeight
 * （强制同步布局）；原来一次列表刷新里的每批 mutation 都会各跑一遍。
 */
let scheduled = false;
function schedule() {
  if (scheduled) return;
  scheduled = true;
  requestAnimationFrame(() => {
    scheduled = false;
    updateAll();
  });
}

/** 节点是否落在某个滚动容器内（含自身是容器的情况） */
function inScrollArea(node: Node | null): boolean {
  const el = node instanceof Element ? node : node?.parentElement ?? null;
  if (!el) return false;
  return !!el.closest(ANY_SELECTOR);
}

/**
 * U-11 ②：只关心「落在滚动容器里 / 新增移除的是滚动容器」的 mutation。
 * 应用里 toast 入场、侧栏状态、主题切换这些 :class 变化原来也会触发一次全文档查询，
 * 而它们不可能改变任何滚动容器的 scrollHeight —— 这里逐条过滤掉。
 * 注意不能干脆去掉 attributes：容器内后代用 v-show / 展开类改高度时不会产生 childList，
 * 去掉就会漏掉「底边渐隐」的更新。
 */
function isRelevantMutation(rec: MutationRecord): boolean {
  if (rec.type === 'attributes') return inScrollArea(rec.target);
  if (inScrollArea(rec.target)) return true;
  for (let i = 0; i < rec.addedNodes.length; i++) {
    const node = rec.addedNodes[i];
    if (node instanceof Element && (node.matches(ANY_SELECTOR) || !!node.querySelector(ANY_SELECTOR))) return true;
  }
  for (let i = 0; i < rec.removedNodes.length; i++) {
    const node = rec.removedNodes[i];
    // 已脱离文档，closest 查不到，只能看它本身是不是容器
    if (node instanceof Element && (node.matches(ANY_SELECTOR) || !!node.querySelector(ANY_SELECTOR))) return true;
  }
  return false;
}

/** 已注册的清理动作；initScrollFades() 返回的 dispose 会执行它（U-11 ③：原来没有任何释放路径） */
let disposable: (() => void) | null = null;

/**
 * 启动滚动渐隐。返回释放函数（幂等），供 App 卸载 / HMR / 测试使用。
 */
export function initScrollFades(): () => void {
  if (disposable) return disposable;
  const cleanups: Array<() => void> = [];

  // 滚动事件不冒泡，用捕获阶段统一接住所有容器
  const onScroll = (e: Event) => {
    const target = e.target as HTMLElement | null;
    if (target && typeof target.matches === 'function') update(target);
  };
  document.addEventListener('scroll', onScroll, true);
  cleanups.push(() => document.removeEventListener('scroll', onScroll, true));

  if (typeof ResizeObserver !== 'undefined') {
    const ro = new ResizeObserver(() => schedule());
    ro.observe(document.body);
    cleanups.push(() => ro.disconnect());
  }

  if (typeof MutationObserver !== 'undefined') {
    const mo = new MutationObserver((records) => {
      for (const rec of records) {
        if (isRelevantMutation(rec)) {
          schedule();
          return;
        }
      }
    });
    mo.observe(document.body, { childList: true, subtree: true, attributes: true, attributeFilter: ['class', 'style'] });
    cleanups.push(() => mo.disconnect());
  }

  const onResize = () => schedule();
  window.addEventListener('resize', onResize);
  cleanups.push(() => window.removeEventListener('resize', onResize));

  requestAnimationFrame(updateAll);
  const initialTimer = window.setTimeout(updateAll, 300);
  cleanups.push(() => window.clearTimeout(initialTimer));

  const dispose = () => {
    for (const fn of cleanups) fn();
    disposable = null;
  };
  disposable = dispose;
  return dispose;
}

/** 显式释放（正常运行时是幂等的空操作） */
export function disposeScrollFades(): void {
  disposable?.();
}
