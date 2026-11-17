import { ref, onUnmounted } from 'vue';

/**
 * 切页期间的"裁切处淡出"控制。
 *
 * 带宽与四边取值都在 CSS 里（--ui-fade-* 由 layout.css 注册成 <length>，
 * 可被 transition 驱动）。这里只负责一件事：什么时候把边推到满、什么时候收回。
 *
 * 为什么收回不能等过渡结束钩子：
 *   after-enter / after-leave 都在滑动**结束之后**才触发，收缩就只能滞后到
 *   滑动结束之后，观感是"滑动完了才补一段动画"。
 *   改成按时间提前触发，让收缩正好在滑动落定那一刻收干净，两段合成一段。
 *
 * 开始时刻由 CSS 反推：滑动时长 - 收缩时长。这样：
 *   · 调大 --pane-duration，收缩自动跟着往后推
 *   · 调大 --pane-edge-fade-out，收缩自动提前，越长越早开始
 * 不需要在 JS 里再写一份时长常量。
 */

function cssMs(name: string, fallbackMs: number, host?: HTMLElement | null): number {
  if (typeof document === 'undefined') return fallbackMs;
  const target = host ?? document.documentElement;
  const raw = getComputedStyle(target).getPropertyValue(name).trim();
  const value = parseFloat(raw);
  if (!Number.isFinite(value)) return fallbackMs;
  return raw.endsWith('ms') ? value : value * 1000;
}

/**
 * @param host 可选：从哪个元素读取 --pane-duration / --pane-edge-fade-out。
 *   组件可以借此给自己的局部过渡单独定时长（例如规则页翻页比整页切页快），
 *   计时器就会跟着那个局部值算，不会和位移时长错位。
 */
export function useEdgeFade(host?: () => HTMLElement | null) {
  /** 方向类：edge-fade-y（上下）/ edge-fade-x（左右）。常驻，收起后也不移除。 */
  const axis = ref('');
  /** 带宽开关：只控制 edge-fade-on 这个类。 */
  const on = ref(false);

  let timer: number | null = null;
  let safetyTimer: number | null = null;

  const clear = () => {
    if (timer !== null) {
      clearTimeout(timer);
      timer = null;
    }
    if (safetyTimer !== null) {
      clearTimeout(safetyTimer);
      safetyTimer = null;
    }
  };

  /** 切页开始时调用：定方向、推带宽、安排提前收回。 */
  const begin = (nextAxis: string) => {
    clear();
    axis.value = nextAxis;
    on.value = true;

    const hostEl = host?.() ?? null;
    const paneMs = cssMs('--pane-duration', 500, hostEl);
    const fadeOutMs = cssMs('--pane-edge-fade-out', 220, hostEl);
    const startMs = Math.max(0, paneMs - fadeOutMs);

    timer = window.setTimeout(() => {
      on.value = false;
      timer = null;
    }, startMs);

    // 兜底：即使过渡钩子没触发（中断、快速连切），也保证一定收回。
    // 正常情况下提前收回已经生效，这里只是防止 edge-fade-on 卡住导致
    // 淡化带持续遮住页面内容和弹窗。
    safetyTimer = window.setTimeout(() => {
      on.value = false;
      safetyTimer = null;
    }, paneMs + 600);
  };

  /** 立即收起。只给 resize 兜底用，不要接过渡结束钩子：
      连切时上一个过渡的 after-leave 会在新过渡跑到一半才触发，
      它分不清自己属于哪一次过渡，会把当前的带子提前收掉。 */
  const end = () => {
    clear();
    on.value = false;
  };

  // 窗口 resize（含全屏切换）时强制收起淡化带。
  // resize 不触发 Vue 过渡钩子，但计时器可能被浏览器节流，
  // 那时 edge-fade-on 会一直挂着，把整页内容持续淡化。这里兜底。
  const onResize = () => { end(); };
  window.addEventListener('resize', onResize);
  onUnmounted(() => window.removeEventListener('resize', onResize));

  return { axis, on, begin };
}
