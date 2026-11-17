// 前端错误上报统一入口（F-A4-11）
//
// 背景：此前大量 catch 只写 console.error 甚至完全空 catch，失败在界面上、
// 日志里都不留痕；排查时只能靠复现。这里给所有「不打算弹窗」的失败一个
// 统一出口：控制台 + 内存环形缓冲（供日志面板/诊断包以后接入）。
// 不引入任何依赖，也不依赖后端：后端目前没有前端日志 API
// （frontend/bindings 下无 Log* 导出），等 A2/A3 的落盘通道就绪后，
// 只需在 reportError 内部加一行转发即可，调用点不用改。
import { toUserError } from './errors';

export type ErrorRecord = {
  at: number;
  ctx: string;
  message: string;
  detail: string;
};

const MAX_RECORDS = 50;
const records: ErrorRecord[] = [];

/**
 * 记录一个「不弹窗」的错误。
 * @param e   任意异常（字符串 / Error / 后端 reject 值）
 * @param ctx 稳定的上下文名，用点号分层，例如 'proxies.load' / 'tun.installDriver'
 */
export function reportError(e: unknown, ctx: string): void {
  const u = toUserError(e);
  try {
    console.error(`[${ctx}] ${u.message}`, e);
  } catch {
    // console 不可用时也不能让上报本身抛错
  }
  records.push({ at: Date.now(), ctx, message: u.message, detail: u.detail });
  if (records.length > MAX_RECORDS) records.shift();
}

/** 供日志面板 / 诊断复制的最近错误快照（只读） */
export function getRecentErrors(): readonly ErrorRecord[] {
  return records;
}
