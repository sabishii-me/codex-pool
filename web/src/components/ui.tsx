import type { ReactNode } from "react";

export function PageFrame({ kicker, title, description, action, children }: { kicker: string; title: string; description: string; action?: ReactNode; children: ReactNode }) {
  return <div className="page-frame"><header className="page-heading"><div><span className="kicker">{kicker}</span><h1>{title}</h1><p>{description}</p></div>{action}</header>{children}</div>;
}

export function CardHeader({ title, subtitle, action }: { title: string; subtitle: string; action?: ReactNode }) {
  return <header className="card-header"><div><h2>{title}</h2><p>{subtitle}</p></div>{action}</header>;
}

export function Metric({ label, value, note }: { label: string; value: string; note: string }) {
  return <article className="metric"><span>{label}</span><strong>{value}</strong><small>{note}</small></article>;
}

export function StatusBadge({ tone, children }: { tone: "success" | "warning"; children: ReactNode }) {
  return <span className={`status-badge ${tone}`}>{children}</span>;
}

export function compact(value: number) {
  return new Intl.NumberFormat("en-US", { notation: "compact", maximumFractionDigits: 1 }).format(value || 0);
}

export function SplineChart({ values }: { values: number[] }) {
  const points = values.filter(Number.isFinite);
  if (points.length === 0) return <div className="spline-chart chart-empty" role="img" aria-label="No request activity in this range"><span>No requests in this range</span></div>;
  const max = Math.max(...points, 1);
  const coordinate = (value: number) => 94 - (value / max) * 78;
  const x = (index: number) => points.length === 1 ? 50 : (index / (points.length - 1)) * 100;
  const path = points.map((value, index) => `${index ? "L" : "M"}${x(index)},${coordinate(value)}`).join(" ");
  return <div className="spline-chart" role="img" aria-label={`Request activity trend, ${points.length} measured ${points.length === 1 ? "point" : "points"}`}><svg viewBox="0 0 100 100" preserveAspectRatio="none"><defs><linearGradient id="red-area" x1="0" y1="0" x2="0" y2="1"><stop offset="0" stopColor="var(--color-accent-primary)" stopOpacity=".28" /><stop offset="1" stopColor="var(--color-accent-primary)" stopOpacity="0" /></linearGradient></defs>{points.length > 1 ? <><path className="chart-area" d={`${path} L100,100 L0,100 Z`} /><path className="chart-line" d={path} /></> : <circle className="chart-point" cx="50" cy={coordinate(points[0])} r="2.5" />}</svg></div>;
}
