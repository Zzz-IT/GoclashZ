// 错误文案人话化：把内核 / 网络层 / 文件层的原始报错翻译成短句，
// 原始文本放进 detail，由 showAlert 的折叠区展示，避免用户直接看到 dial tcp / Access is denied 之类的原文。

export type UserError = { message: string; detail: string };

type ErrorRule = { re: RegExp; message: string };

// 顺序即优先级：更具体的错误码在前，helper 这类弱特征放最后
const RULES: ErrorRule[] = [
  { re: /actively refused|connection refused|10061/i, message: '内核未运行' },
  { re: /deadline exceeded|timeout|timed out|10060/i, message: '请求超时' },
  { re: /Access is denied|拒绝访问|10013/i, message: '权限不足' },
  { re: /bind:|10048|address already in use/i, message: '端口已被占用' },
  { re: /yaml:|did not find expected/i, message: '配置文件格式错误' },
  { re: /status 403|403 Forbidden/i, message: '订阅返回 403' },
  { re: /status 404|404 Not Found/i, message: '订阅返回 404' },
  { re: /helper|Helper/i, message: '需要 Helper 服务' },
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
