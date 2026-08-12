import { useLayoutEffect, useRef, useState, type ReactNode } from "react";
import { scaleLinear, scalePoint } from "d3-scale";
import { curveMonotoneX, line } from "d3-shape";

const seriesColors = ["#f87171", "#facc15", "#4ade80", "#60a5fa", "#c084fc", "#fb923c", "#2dd4bf", "#f472b6", "#a3e635"];

export interface ChartSeries { id: string; label: string; values: number[] }

export function MultiSeriesChart({ series, xLabels, ariaLabel }: { series: ChartSeries[]; xLabels: string[]; ariaLabel: string }) {
  const plotRef = useRef<HTMLDivElement>(null);
  const [plotWidth, setPlotWidth] = useState(960);
  useLayoutEffect(() => {
    const element = plotRef.current;
    if (!element) return;
    const update = () => setPlotWidth(Math.max(320, Math.round(element.getBoundingClientRect().width)));
    update();
    const observer = new ResizeObserver(update);
    observer.observe(element);
    return () => observer.disconnect();
  }, []);

  const visible = series.filter(item => item.values.some(value => Number.isFinite(value) && value > 0));
  if (!xLabels.length) return <div className="spline-chart chart-empty" role="img" aria-label={`No ${ariaLabel.toLowerCase()}`}><span>Usage timeline unavailable</span></div>;
  const count = xLabels.length;
  const height = 250;
  const top = 18;
  const bottom = 220;
  const bucketTotals = Array.from({ length: count }, (_, index) => visible.reduce((total, item) => total + Math.max(0, Number.isFinite(item.values[index]) ? item.values[index] : 0), 0));
  const maxValue = Math.max(...bucketTotals, 1);
  const magnitude = 10 ** Math.floor(Math.log10(maxValue));
  const yMax = Math.ceil(maxValue / magnitude) * magnitude;
  const yTicks = [yMax, yMax / 2, 0];
  const side = 10;
  const bucketIndexes = Array.from({ length: count }, (_, index) => index);
  const xScale = scalePoint<number>().domain(bucketIndexes).range([side, plotWidth - side]);
  const yScale = scaleLinear().domain([0, yMax]).range([bottom, top]);
  const x = (index: number) => xScale(index) ?? plotWidth / 2;
  const y = (value: number) => yScale(value);
  const xTickIndexes = [...new Set([0, Math.floor((count - 1) / 2), count - 1])];
  const curve = line<{ x: number; y: number }>().x(point => point.x).y(point => point.y).curve(curveMonotoneX);
  const columnWidth = 14;

  return <div className="multi-series-chart" role="img" aria-label={ariaLabel}>
    {visible.length ? <div className="chart-legend">{visible.map((item, index) => <span key={item.id}><i style={{ background: seriesColors[index % seriesColors.length] }} />{item.label}</span>)}</div> : <div className="chart-zero-summary">No usage in this range</div>}
    <div className="chart-frame">
      <div className="chart-y-axis" aria-hidden="true"><b>Billable tokens</b>{yTicks.map(value => <span key={value}>{compact(value)}</span>)}</div>
      <div className="chart-plot" ref={plotRef}>
        <svg width={plotWidth} height={height} viewBox={`0 0 ${plotWidth} ${height}`} aria-hidden="true">
          {yTicks.map(value => <line key={value} className="chart-grid-line" x1="0" x2={plotWidth} y1={y(value)} y2={y(value)} />)}
          {Array.from({ length: count }, (_, index) => <line key={`bucket-${index}`} className="chart-bucket-line" x1={x(index)} x2={x(index)} y1={top} y2={bottom} />)}
          {Array.from({ length: count }, (_, bucketIndex) => {
            let stacked = 0;
            return <g key={`column-${bucketIndex}`}>{visible.map((item, seriesIndex) => {
              const value = Math.max(0, Number.isFinite(item.values[bucketIndex]) ? item.values[bucketIndex] : 0);
              if (value === 0) return null;
              const segmentTop = y(stacked + value);
              const segmentBottom = y(stacked);
              stacked += value;
              return <rect key={item.id} className="usage-series-column" x={x(bucketIndex) - columnWidth / 2} y={segmentTop} width={columnWidth} height={Math.max(1, segmentBottom - segmentTop)} fill={seriesColors[seriesIndex % seriesColors.length]} />;
            })}</g>;
          })}
          {visible.map((item, seriesIndex) => {
            const points = item.values.slice(0, count).map((value, index) => ({ x: x(index), y: y(Math.max(0, Number.isFinite(value) ? value : 0)) }));
            return <path key={item.id} className="usage-series-line" style={{ stroke: seriesColors[seriesIndex % seriesColors.length] }} d={curve(points) ?? ""} />;
          })}
        </svg>
        <div className="chart-x-axis" aria-hidden="true">{xTickIndexes.map(index => <span key={index} style={{ left: `${x(index) / plotWidth * 100}%` }}>{xLabels[index]}</span>)}</div>
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

const pieColors = ["#60a5fa", "#4ade80", "#facc15", "#f87171", "#c084fc", "#2dd4bf", "#fb923c", "#f472b6", "#a3e635", "#818cf8", "#34d399", "#fbbf24", "#22d3ee", "#a78bfa", "#f97316"];

export interface PieSlice { label: string; value: number; color?: string }

export function PieChart({ slices, size = 320, selectedLabel, onSelect }: { slices: PieSlice[]; size?: number; selectedLabel?: string; onSelect?: (label: string) => void }) {
  const total = slices.reduce((sum, slice) => sum + Math.max(0, slice.value), 0);
  if (total <= 0) return <div className="pie-empty">No usage in this range</div>;
  const cx = size / 2, cy = size / 2, r = size / 2 - 2;
  let angle = -Math.PI / 2;
  const paths: ReactNode[] = [];
  const legend: ReactNode[] = [];
  slices.forEach((slice, index) => {
    const value = Math.max(0, slice.value);
    if (value <= 0) return;
    const fraction = value / total;
    const startAngle = angle;
    const endAngle = angle + fraction * 2 * Math.PI;
    angle = endAngle;
    const largeArc = fraction > 0.5 ? 1 : 0;
    const x1 = cx + r * Math.cos(startAngle), y1 = cy + r * Math.sin(startAngle);
    const x2 = cx + r * Math.cos(endAngle), y2 = cy + r * Math.sin(endAngle);
    const color = slice.color ?? pieColors[index % pieColors.length];
    const selected = selectedLabel === slice.label;
    if (fraction >= 0.9999) {
      // A single full circle cannot be drawn as an SVG arc from a point back to
      // itself; render it as a plain circle instead.
      paths.push(<circle key={slice.label} cx={cx} cy={cy} r={r} fill={color} fillOpacity={selected ? 1 : 0.78} stroke="var(--color-surface)" strokeWidth={1.5} style={{ cursor: onSelect ? "pointer" : "default", transition: "fill-opacity .15s" }} onClick={onSelect ? () => onSelect(slice.label) : undefined} role={onSelect ? "button" : undefined} aria-label={slice.label} />);
    } else {
      const x1 = cx + r * Math.cos(startAngle), y1 = cy + r * Math.sin(startAngle);
      const x2 = cx + r * Math.cos(endAngle), y2 = cy + r * Math.sin(endAngle);
      paths.push(<path key={slice.label} d={`M ${cx} ${cy} L ${x1} ${y1} A ${r} ${r} 0 ${largeArc} 1 ${x2} ${y2} Z`} fill={color} fillOpacity={selected ? 1 : 0.78} stroke="var(--color-surface)" strokeWidth={1.5} style={{ cursor: onSelect ? "pointer" : "default", transition: "fill-opacity .15s" }} onClick={onSelect ? () => onSelect(slice.label) : undefined} role={onSelect ? "button" : undefined} aria-label={slice.label} />);
    }
    legend.push(<button key={slice.label} type="button" className={`pie-legend-item${selected ? " active" : ""}`} onClick={onSelect ? () => onSelect(slice.label) : undefined}><span className="pie-legend-dot" style={{ background: color }} /><span className="pie-legend-label">{slice.label}</span><small>{Math.round(fraction * 100)}%</small></button>);
  });
  return <div className="pie-wrap"><svg width={size} height={size} viewBox={`0 0 ${size} ${size}`} role="img" aria-label="Usage distribution">{paths}</svg><div className="pie-legend">{legend}</div></div>;
}
