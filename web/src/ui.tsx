import React, { useEffect } from "react";
import { Check, GitBranch, X } from "lucide-react";
import type { Health, Event } from "./types";
import { date } from "./lib";

export function Badge({ health, label }: { health?: Health; label?: string }) {
  const s = health?.status || "inactive";
  return (
    <span className={`badge ${s}`} title={health?.error}>
      <i />
      {label ||
        {
          healthy: `${health?.latency_ms ?? 0} ms`,
          unhealthy: "不可用",
          unknown: "待探测",
          inactive: "未启用",
          disabled: "已禁用",
          unsupported: "不支持",
        }[s] ||
        s}
    </span>
  );
}
export function Empty({
  title,
  text,
  action,
}: {
  title: string;
  text: string;
  action?: React.ReactNode;
}) {
  return (
    <div className="empty">
      <div className="empty-icon">
        <GitBranch size={30} />
      </div>
      <h3>{title}</h3>
      <p>{text}</p>
      {action}
    </div>
  );
}
export function Modal({
  title,
  children,
  close,
  wide = false,
}: {
  title: string;
  children: React.ReactNode;
  close: () => void;
  wide?: boolean;
}) {
  useEffect(() => {
    const f = (e: KeyboardEvent) => {
      if (e.key === "Escape") close();
    };
    document.addEventListener("keydown", f);
    return () => document.removeEventListener("keydown", f);
  }, [close]);
  return (
    <div
      className="modal-shade"
      onMouseDown={(e) => {
        if (e.target === e.currentTarget) close();
      }}
    >
      <section
        className={`modal ${wide ? "wide" : ""}`}
        role="dialog"
        aria-modal="true"
        aria-label={title}
      >
        <header>
          <h2>{title}</h2>
          <button className="icon-button" onClick={close} aria-label="关闭">
            <X size={20} />
          </button>
        </header>
        {children}
      </section>
    </div>
  );
}
export function Field({
  label,
  hint,
  children,
}: {
  label: string;
  hint?: string;
  children: React.ReactNode;
}) {
  return (
    <label className="field">
      <span>{label}</span>
      {children}
      {hint && <small>{hint}</small>}
    </label>
  );
}
export function Sparkline({ points }: { points: number[] }) {
  const max = Math.max(1, ...points);
  const line = points
    .map(
      (n, i) =>
        `${(i * 100) / Math.max(1, points.length - 1)},${31 - (n / max) * 26}`,
    )
    .join(" ");
  return (
    <svg viewBox="0 0 100 34" preserveAspectRatio="none" className="sparkline">
      <polyline
        points={`0,34 ${line} 100,34`}
        fill="rgba(88,215,173,.08)"
        stroke="none"
      />
      <polyline
        points={line}
        fill="none"
        stroke="currentColor"
        strokeWidth="1.3"
        vectorEffect="non-scaling-stroke"
      />
    </svg>
  );
}
export function Metric({
  label,
  value,
  sub,
  icon,
  children,
}: {
  label: string;
  value: string;
  sub: string;
  icon: React.ReactNode;
  children?: React.ReactNode;
}) {
  return (
    <article className="metric">
      <div>
        <span>{label}</span>
        {icon}
      </div>
      <strong>{value}</strong>
      <small>{sub}</small>
      {children}
    </article>
  );
}
export function SystemRow({ name, value }: { name: string; value: string }) {
  return (
    <div className="system-row">
      <span>{name}</span>
      <strong>{value}</strong>
    </div>
  );
}
export function EventList({ events }: { events: Event[] }) {
  return events.length ? (
    <div className="event-list">
      {events.map((e) => (
        <div className="event" key={e.id}>
          <span className={`event-kind ${e.kind}`}>{e.kind.toUpperCase()}</span>
          <p>{e.message}</p>
          <time>{date(e.created)}</time>
        </div>
      ))}
    </div>
  ) : (
    <div className="event-empty">暂时没有事件，配置变化会显示在这里。</div>
  );
}
export function SaveButton({ busy }: { busy: boolean }) {
  return (
    <div className="form-actions">
      <button className="button primary" disabled={busy}>
        <Check size={16} />
        {busy ? "正在应用…" : "保存并应用"}
      </button>
    </div>
  );
}
