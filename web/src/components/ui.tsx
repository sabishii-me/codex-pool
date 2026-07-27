import type { ReactNode } from "react";

const seriesColors = ["#f87171", "#facc15", "#4ade80", "#60a5fa", "#c084fc", "#fb923c", "#2dd4bf", "#f472b6", "#a3e635"];

export interface ChartSeries { id: string; label: string; values: number[] }

export function MultiSeriesChart({ series, ariaLabel }: { series: ChartSeries[]; ariaLabel: string }) {
  const visible = series.filter(item => item.values.some(Number.isFinite));
  if (!visible.length) return <div className="spline-chart chart-empty" role="img" aria-label={`No ${ariaLabel.toLowerCase()}`}><span>No measured data in this range</span></div>;
  const count = Math.max(...visible.map(item => item.values.length));
  const max = Math.max(...visible.flatMap(item => item.values.filter(Number.isFinite)), 1);
  const x = (index: number) => count <= 1 ? 50 : index / (count - 1) * 100;
  const y = (value: number) => 94 - value / max * 78;
  return <div className="multi-series-chart" role="img" aria-label={ariaLabel}><div className="chart-legend">{visible.map((item, index) => <span key={item.id}><i style={{ background: seriesColors[index % seriesColors.length] }} />{item.label}</span>)}</div><div className="spline-chart"><svg viewBox="0 0 100 100" preserveAspectRatio="none">{visible.map((item, seriesIndex) => { const path = item.values.map((value, index) => `${index ? "L" : "M"}${x(index)},${y(value)}`).join(" "); return item.values.length > 1 ? <path key={item.id} className="chart-line" style={{ stroke: seriesColors[seriesIndex % seriesColors.length], filter: "none" }} d={path} /> : <circle key={item.id} cx="50" cy={y(item.values[0])} r="2" fill={seriesColors[seriesIndex % seriesColors.length]} />; })}</svg></div></div>;
}

export function PageFrame({ kicker, title, description, action, children }: { kicker: string; title: string; description: string; action?: ReactNode; children: ReactNode }) {
  return <div className="page-frame"><header className="page-heading"><div><span className="kicker">{kicker}</span><h1>{title}</h1><p>{description}</p></div>{action}</header>{children}</div>;
}

export function CardHeader({ title, subtitle, action }: { title: string; subtitle?: string; action?: ReactNode }) {
  return <header className="card-header"><div><h2>{title}</h2>{subtitle ? <p>{subtitle}</p> : null}</div>{action}</header>;
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
