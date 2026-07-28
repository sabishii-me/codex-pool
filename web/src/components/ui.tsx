import type { ReactNode } from "react";

const seriesColors = ["#f87171", "#facc15", "#4ade80", "#60a5fa", "#c084fc", "#fb923c", "#2dd4bf", "#f472b6", "#a3e635"];

export interface ChartSeries { id: string; label: string; values: number[] }

function monotoneCurvePath(points: Array<{ x: number; y: number }>) {
  if (points.length === 0) return "";
  if (points.length === 1) return `M${points[0].x},${points[0].y}`;
  const slopes = points.slice(0, -1).map((point, index) => (points[index + 1].y - point.y) / (points[index + 1].x - point.x));
  const tangents = points.map((_, index) => {
    if (index === 0) return slopes[0];
    if (index === points.length - 1) return slopes.at(-1) ?? 0;
    const before = slopes[index - 1];
    const after = slopes[index];
    return before * after <= 0 ? 0 : 2 * before * after / (before + after);
  });
  for (let index = 0; index < slopes.length; index++) {
    if (slopes[index] === 0) {
      tangents[index] = 0;
      tangents[index + 1] = 0;
      continue;
    }
    const before = tangents[index] / slopes[index];
    const after = tangents[index + 1] / slopes[index];
    const magnitude = Math.hypot(before, after);
    if (magnitude > 3) {
      const scale = 3 / magnitude;
      tangents[index] = scale * before * slopes[index];
      tangents[index + 1] = scale * after * slopes[index];
    }
  }
  return points.slice(0, -1).reduce((path, point, index) => {
    const next = points[index + 1];
    const third = (next.x - point.x) / 3;
    return `${path} C${point.x + third},${point.y + tangents[index] * third} ${next.x - third},${next.y - tangents[index + 1] * third} ${next.x},${next.y}`;
  }, `M${points[0].x},${points[0].y}`);
}

export function MultiSeriesChart({ series, xLabels, ariaLabel }: { series: ChartSeries[]; xLabels: string[]; ariaLabel: string }) {
  const visible = series.filter(item => item.values.some(value => Number.isFinite(value) && value > 0));
  if (!xLabels.length) return <div className="spline-chart chart-empty" role="img" aria-label={`No ${ariaLabel.toLowerCase()}`}><span>Usage timeline unavailable</span></div>;
  const count = xLabels.length;
  const maxValue = Math.max(...visible.flatMap(item => item.values.slice(0, count).map(value => Math.max(0, Number.isFinite(value) ? value : 0))), 1);
  const magnitude = 10 ** Math.floor(Math.log10(maxValue));
  const yMax = Math.ceil(maxValue / magnitude) * magnitude;
  const yTicks = [yMax, yMax / 2, 0];
  const x = (index: number) => count <= 1 ? 50 : index / (count - 1) * 100;
  const y = (value: number) => 94 - value / yMax * 84;
  const xTickIndexes = [...new Set([0, Math.floor((count - 1) / 2), count - 1])];
  return <div className="multi-series-chart" role="img" aria-label={ariaLabel}>
    {visible.length ? <div className="chart-legend">{visible.map((item, index) => <span key={item.id}><i style={{ background: seriesColors[index % seriesColors.length] }} />{item.label}</span>)}</div> : <div className="chart-zero-summary">No usage in this range</div>}
    <div className="chart-frame">
      <div className="chart-y-axis" aria-hidden="true"><b>Billable tokens</b>{yTicks.map(value => <span key={value}>{compact(value)}</span>)}</div>
      <div className="chart-plot">
        <svg viewBox="0 0 100 100" preserveAspectRatio="none" aria-hidden="true">
          {yTicks.map(value => <line key={value} className="chart-grid-line" x1="0" x2="100" y1={y(value)} y2={y(value)} />)}
          {Array.from({ length: count + 1 }, (_, index) => <line key={`bucket-${index}`} className="chart-bucket-line" x1={index / count * 100} x2={index / count * 100} y1="10" y2="94" />)}
          {visible.map((item, seriesIndex) => {
            const points = item.values.slice(0, count).map((value, index) => ({ x: x(index), y: y(Math.max(0, Number.isFinite(value) ? value : 0)) }));
            return <path key={item.id} className="usage-series-line" style={{ stroke: seriesColors[seriesIndex % seriesColors.length] }} d={monotoneCurvePath(points)} />;
          })}
        </svg>
        <div className="chart-x-axis" aria-hidden="true">{xTickIndexes.map(index => <span key={index} style={{ left: `${x(index)}%` }}>{xLabels[index]}</span>)}</div>
        <b className="chart-x-title">Time</b>
      </div>
    </div>
  </div>;
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
  const coordinates = points.map((value, index) => ({ x: x(index), y: coordinate(value) }));
  const path = coordinates.map((point, index) => `${index === 0 ? "M" : "L"}${point.x},${point.y}`).join(" ");
  return <div className="spline-chart" role="img" aria-label={`Request activity trend, ${points.length} measured ${points.length === 1 ? "point" : "points"}`}><svg viewBox="0 0 100 100" preserveAspectRatio="none"><defs><linearGradient id="red-area" x1="0" y1="0" x2="0" y2="1"><stop offset="0" stopColor="var(--color-accent-primary)" stopOpacity=".28" /><stop offset="1" stopColor="var(--color-accent-primary)" stopOpacity="0" /></linearGradient></defs>{points.length > 1 ? <><path className="chart-area" d={`${path} L100,100 L0,100 Z`} /><path className="chart-line" d={path} /></> : <circle className="chart-point" cx="50" cy={coordinate(points[0])} r="2.5" />}</svg></div>;
}
