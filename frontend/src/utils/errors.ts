// 错误文案人话化：把内核 / 网络层 / 文件层的原始报错翻译成短句，
// 原始文本放进 detail，由 showAlert 的折叠区展示，避免用户直接看到 dial tcp / Access is denied 之类的原文。

export type UserError = { message: string; detail: string };

type ErrorRule = { re: RegExp; message: string };

// 顺序即优先级：结构 / 权限 / 端口这类能一眼定性的在前，超时最泛化，放最后；
// 系统错误码必须带上下文（error 10061 / (10061) / ] 10060），避免误伤端口号或正常数字。
const RULES: ErrorRule[] = [
  // 只认 YAML 解析错误特征；不能让 open .../config.yaml: Access is denied 这类路径误命中
  { re: /yaml:\s*(line \d+|unmarshal|did not find expected|found unexpected|mapping values)|did not find expected/i, message: '配置文件格式错误' },
  { re: /Access is denied|拒绝访问|error 10013\b|\(10013\)|\]\s*10013\b/i, message: '权限不足' },
  { re: /bind:|address already in use|error 10048\b|\(10048\)|\]\s*10048\b/i, message: '端口已被占用' },
  { re: /actively refused|connection refused|error 10061\b|\(10061\)|\]\s*10061\b/i, message: '内核未运行' },
  { re: /status 403|403 Forbidden/i, message: '订阅返回 403' },
  { re: /status 404|404 Not Found/i, message: '订阅返回 404' },
  { re: /helper|Helper/i, message: '需要 Helper 服务' },
  { re: /deadline exceeded|timeout|timed out|error 10060\b|\(10060\)|\]\s*10060\b/i, message: '请求超时' },
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
