import { useEffect, useState } from "react";

export const strategies: Record<string, string> = {
  round_robin: "轮询",
  random: "随机",
  ip_hash: "IP 哈希",
  least_conn: "最小连接数",
  failover: "故障转移",
  p2c: "双随机择少 · P2C",
};
export const id = () => crypto.randomUUID().replaceAll("-", "");
export const clone = <T>(value: T): T => structuredClone(value);
export const bytes = (n = 0) => {
  if (n < 1024) return `${n.toFixed(0)} B`;
  const u = Math.min(4, Math.floor(Math.log(n) / Math.log(1024)));
  return `${(n / 1024 ** u).toFixed(u > 1 ? 1 : 0)} ${["B", "KiB", "MiB", "GiB", "TiB"][u]}`;
};
export const date = (s?: string) =>
  s && !s.startsWith("0001")
    ? new Date(s).toLocaleString("zh-CN", { hour12: false })
    : "尚未检查";
export async function api<T>(
  path: string,
  method = "GET",
  body?: unknown,
): Promise<T> {
  const res = await fetch(`/api/v1${path}`, {
    method,
    credentials: "same-origin",
    headers: body === undefined ? {} : { "Content-Type": "application/json" },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  const value = await res.json();
  if (!res.ok) throw new Error(value.error || `HTTP ${res.status}`);
  return value as T;
}

// useDebounced returns value once it has stopped changing for delay milliseconds.
export function useDebounced<T>(value: T, delay: number): T {
  const [settled, setSettled] = useState(value);
  useEffect(() => {
    const timer = window.setTimeout(() => setSettled(value), delay);
    return () => window.clearTimeout(timer);
  }, [value, delay]);
  return settled;
}
