// 错误文案人话化：把内核 / 网络层 / 文件层的原始报错翻译成短句，
// 原始文本放进 detail，由 showAlert 的折叠区展示，避免用户直接看到 dial tcp / Access is denied 之类的原文。
// U-03：兜底后缀（「（已记录日志）」）改走 i18n，语言包补齐前回退到原中文。
import { t } from '../locales';

/** t() 在 key 缺失时原样返回 key 本身（locales/index.ts:64-67），据此回退到内置文案 */
const tr = (key: string, fallback: string, params?: Record<string, string | number>) => {
  const value = t(key, params);
  return value === key ? fallback : value;
};

export type UserError = { message: string; detail: string };

type ErrorRule = { re: RegExp; message: string };

// 顺序即优先级：结构 / 权限 / 端口这类能一眼定性的在前；
// 超时最泛化，必须排在 Helper 之前（含「通过 Helper 启动内核失败: context deadline exceeded」这类），
// 否则会被 Helper 规则抢走（U-05）。
// 系统错误码必须带上下文（error 10061 / (10061) / ] 10060），避免误伤端口号或正常数字。
const RULES: ErrorRule[] = [
  // 只认 YAML 解析错误特征；不能让 open .../config.yaml: Access is denied 这类路径误命中
  { re: /yaml:\s*(line \d+|unmarshal|did not find expected|found unexpected|mapping values)|did not find expected/i, message: '配置文件格式错误' },
  { re: /Access is denied|拒绝访问|error 10013\b|\(10013\)|\]\s*10013\b/i, message: '权限不足' },
  { re: /bind:|address already in use|error 10048\b|\(10048\)|\]\s*10048\b/i, message: '端口已被占用' },
  { re: /actively refused|connection refused|error 10061\b|\(10061\)|\]\s*10061\b/i, message: '内核未运行' },
  { re: /status 403|403 Forbidden/i, message: '订阅返回 403' },
  { re: /status 404|404 Not Found/i, message: '订阅返回 404' },
  // U-05：原来这里是 /helper|Helper/i —— 过宽。「启动 helper 超时」「helper.exe 已被占用」
  // 都会显示成「需要 Helper 服务」，把用户引向错误的修复路径（去装 Helper，实际是超时/占用）。
  // 现在只认后端真正的缺失/需安装语义：supervisor.go:19 的 helper_install_required /
  // helper_repair_required code，以及服务未安装/未启动/未注册的文案。
  { re: /deadline exceeded|timeout|timed out|error 10060\b|\(10060\)|\]\s*10060\b/i, message: '请求超时' },
  { re: /helper_install_required|HELPER_NOT_INSTALLED|helper_repair_required|HELPER_REPAIR_REQUIRED|需要\s*Helper\s*服务|Helper\s*服务未(安装|启动|注册)/i, message: '需要 Helper 服务' },
];

// 把 unknown 提取成可读文本（Wails 的 reject 常见为字符串，也可能是 Error / 带 message 的对象）
function rawText(e: unknown): string {
  if (e === null || e === undefined) return '';
  if (typeof e === 'string') return e;
  if (e instanceof Error) return e.message || String(e);
  if (typeof e === 'object') {
    const m = (e as { message?: unknown }).message;
    if (typeof m === 'string' && m) return m;
  }
  try {
    return String(e);
  } catch {
    return '';
  }
}

/**
 * 把任意异常转换成「短句 + 原始详情」。
 * - 命中内置规则：message 为不超过 16 字的名词短语，detail 为原始字符串
 * - 未命中：message 取原始文本 trim 后前 60 字，detail 为原始完整文本
 */
export function toUserError(e: unknown): UserError {
  const raw = rawText(e);
  const text = raw.trim();

  for (const rule of RULES) {
    if (rule.re.test(text)) {
      return { message: rule.message, detail: raw };
    }
  }

  return { message: text.slice(0, 60) || '未知错误', detail: raw };
}

/* ============================================================
 * 错误分类（可编程语义，替代散落各处的文案 includes）
 * ------------------------------------------------------------
 * 只认「确定的、后端确实会产出的」特征串，绝不出现泛化的
 * Wintun / busy / error 这类模糊匹配；未命中一律 unknown，
 * 由调用方按 unknown 处理（展示通用文案 + 原始 detail，绝不
 * 因此触发安装驱动等高代价动作）。
 * 对应后端生产端：
 *   wintun_missing        → core/appcore/supervisor.go:167（ErrWintunMissing）
 *   缺少 Wintun 依赖      → core/appcore/controller.go:1778
 *   wintun 仍不可用       → core/runtimeassets/status.go:69（经 controller.go:661 包装）
 *   helper_install_required / helper_repair_required → core/appcore/supervisor.go:18-19
 *   no active config      → core/appcore/supervisor.go:17（ErrNoActiveConfig）
 * ========================================================== */
export type ErrorKind =
  | 'wintun_missing'
  | 'wintun_unavailable'
  | 'helper_install_required'
  | 'helper_repair_required'
  | 'no_active_config'
  | 'unknown';

/** A=静默重试；B=需干预但不阻塞；C=不可恢复/需用户处理 */
export type ErrorLevel = 'A' | 'B' | 'C';

export type ErrorClass = { kind: ErrorKind; level: ErrorLevel };

// 顺序即优先级：wintun_missing 必须早于 wintun_unavailable（前者含后者上下文）
const CLASSIFY_RULES: { re: RegExp; kind: ErrorKind; level: ErrorLevel }[] = [
  { re: /wintun_missing|ASSET_WINTUN_MISSING|缺少\s*Wintun\s*依赖/i, kind: 'wintun_missing', level: 'C' },
  { re: /wintun\s*仍不可用|wintun\s*已损坏|不是有效\s*Windows\s*DLL/i, kind: 'wintun_unavailable', level: 'B' },
  { re: /helper_install_required|HELPER_NOT_INSTALLED/i, kind: 'helper_install_required', level: 'B' },
  { re: /helper_repair_required/i, kind: 'helper_repair_required', level: 'B' },
  { re: /no active config/i, kind: 'no_active_config', level: 'C' },
];

/**
 * 把任意异常归类成 { kind, level }，只用于「分支决策」
 * （是否走装驱动 / 是否可重试），不负责展示文案。
 * 展示请用 displayErrorMessage()：它会保留 toUserError 的人话映射。
 * 未命中任何规则时返回 unknown。
 */
export function classifyError(e: unknown): ErrorClass {
  const text = rawText(e).trim();
  if (text) {
    for (const rule of CLASSIFY_RULES) {
      if (rule.re.test(text)) return { kind: rule.kind, level: rule.level };
    }
  }
  return { kind: 'unknown', level: 'C' };
}

/**
 * toUserError 未命中人话规则时的兜底后缀：告诉用户「已记录日志」，别再让他们截图原文。
 * U-03：原来是导出的中文常量（8 语言下固定中文）；改成函数才能跟随运行时语言切换。
 * 全仓库只有本文件使用，故不再导出。
 */
function errorLoggedSuffix(): string {
  return tr('common.errorLogged', '（已记录日志）');
}

/* ------------------------------------------------------------
 * 后端「语义」适配层（F-A4-14）
 * ------------------------------------------------------------
 * 现状：这两处语义（长连接延迟、路由切换中）后端只以中文消息文本表达，
 * 没有 code 字段，前端只能按字面量判定。为了「只有一处需要改」，
 * 这里统一收口；等后端补 DEFERRED_* / ROUTE_SWITCHING 之类的 code 后，
 * 只需替换这两个函数体，调用点（store.ts / useTunToggle.ts）不用动。
 * ---------------------------------------------------------- */
const LONG_CONNECTION_DEFERRAL_RE = /由于长连接活动而延迟|由于长连接一直存在，已放弃/;

/** 该错误/消息是否属于「长连接保护导致的自动延后」（不是失败，不该弹模态） */
export function isLongConnectionDeferral(text: string | null | undefined): boolean {
  return !!text && LONG_CONNECTION_DEFERRAL_RE.test(text);
}

/** 出站 IP 探测结果的消息是否表示「路由切换中」（应保留旧值，不当作失败） */
export function isRouteSwitchingMessage(text: string | null | undefined): boolean {
  return !!text && text.includes('路由切换中');
}

/** toUserError 是否命中了内置的 8 条人话规则（端口占用/权限/超时/内核未运行…） */
export function matchesKnownErrorRule(e: unknown): boolean {
  const text = rawText(e).trim();
  if (!text) return false;
  return RULES.some((rule) => rule.re.test(text));
}

/**
 * 用户可见文案：先走 toUserError 的人话规则，只有
 * 「classifyError 归为 unknown 且没命中任何人话规则」时，
 * 才在原始短文本末尾追加「（已记录日志）」，避免丢掉原有映射。
 * 原始完整文本仍由 toUserError(e).detail 传给模态的折叠区。
 */
export function displayErrorMessage(e: unknown): string {
  const message = toUserError(e).message;
  if (classifyError(e).kind !== 'unknown') return message;
  if (matchesKnownErrorRule(e)) return message;
  return message + errorLoggedSuffix();
}
