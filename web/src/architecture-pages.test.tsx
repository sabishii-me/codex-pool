import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { DashboardPage } from "./features/dashboard";
import { UsagePage, buildModelChart } from "./features/usage";
import type { ResourceState } from "./resource-state";
import { ModelsPage } from "./features/models";
import { MultiSeriesChart, SplineChart } from "./components/ui";
import type { ModelDescriptor, PoolUserStats, UsageProjection } from "./types";

const model: ModelDescriptor = { id: "model-1", name: "Model One", provider: "provider", protocol: "openai", available_now: true, capabilities: { tools: true } };

describe("route-level page jobs", () => {
  it("never invents activity when the measured series is empty", () => {
    const html = renderToStaticMarkup(<SplineChart values={[]} />);
    expect(html).toContain("No requests in this range");
    expect(html).not.toContain("chart-line");
    expect(html).not.toContain("chart-area");
  });

  it("renders one measured point without manufacturing a trend", () => {
    const html = renderToStaticMarkup(<SplineChart values={[7]} />);
    expect(html).toContain("1 measured point");
    expect(html).toContain("chart-point");
    expect(html).not.toContain("chart-line");
  });
  it("keeps real time gaps and changes chart density with the selected range", () => {
    const projection = {
      range_hours: 168,
      range_days: 7,
      evidence: { generated_at: "2026-07-27T23:30:00Z" },
      model_hourly: [
        { hour: "2026-07-27T22:00:00Z", model_id: "gpt", provider_id: "codex", billable_tokens: 100, requests: 1 },
        { hour: "2026-07-25T12:00:00Z", model_id: "gpt", provider_id: "codex", billable_tokens: 50, requests: 1 },
      ],
    } as UsageProjection;
    const hourly = buildModelChart(projection, "24h");
    expect(hourly.labels).toHaveLength(24);
    expect(hourly.series[0].values.filter(Boolean)).toEqual([100]);
    const daily = buildModelChart(projection, "7d");
    expect(daily.labels).toHaveLength(7);
    expect(daily.series[0].values.filter(Boolean)).toEqual([50, 100]);
    expect(daily.labels.at(-1)).toBe(new Date("2026-07-27T00:00:00Z").toLocaleDateString([], { month: "short", day: "numeric" }));
  });

  it("keeps midnight UTC usage in the correct daily bucket", () => {
    const projection = {
      range_hours: 168,
      range_days: 7,
      evidence: { generated_at: "2026-07-27T23:30:00Z" },
      model_hourly: [
        { hour: "2026-07-27T00:00:00Z", model_id: "gpt", provider_id: "codex", billable_tokens: 25, requests: 1 },
      ],
    } as UsageProjection;
    const daily = buildModelChart(projection, "7d");
    expect(daily.series[0].values).toHaveLength(7);
    expect(daily.series[0].values.at(-1)).toBe(25);
    expect(daily.series[0].values.slice(0, -1).every(value => value === 0)).toBe(true);
  });

  it("falls back to truthful provider buckets when model detail is unavailable", () => {
    const projection = {
      range_hours: 24,
      range_days: 1,
      evidence: { generated_at: "2026-07-27T23:30:00Z" },
      model_hourly: [],
      hourly: [
        { hour: "2026-07-27T22", account_type: "codex", billable_tokens: 40, input_tokens: 30, cached_tokens: 0, output_tokens: 10, reasoning_tokens: 0, request_count: 1 },
      ],
    } as unknown as UsageProjection;
    const chart = buildModelChart(projection, "24h");
    expect(chart.granularity).toBe("provider");
    expect(chart.series).toHaveLength(1);
    expect(chart.series[0].label).toBe("codex");
    expect(chart.series[0].values.filter(Boolean)).toEqual([40]);
  });

  it("keeps an exact timeline and zero baseline for an empty measured range", () => {
    const html = renderToStaticMarkup(<MultiSeriesChart ariaLabel="Billable tokens by model over time" xLabels={["10 AM", "11 AM", "12 PM"]} series={[]} />);
    expect(html).toContain("No usage in this range");
    expect(html).toContain("Billable tokens");
    expect(html).toContain("10 AM");
    expect(html).toContain('class="chart-bucket-line"');
    expect(html).not.toContain("No measured data in this range");
  });

  it("renders a shape-preserving curve with a non-scaling stroke", () => {
    const html = renderToStaticMarkup(<MultiSeriesChart ariaLabel="Billable tokens by model over time" xLabels={["10 AM", "11 AM", "12 PM"]} series={[{ id: "codex:gpt", label: "gpt", values: [10, 30, 20] }]} />);
    expect(html).toContain("Billable tokens");
    expect(html).toContain("Time");
    expect(html).toContain("10 AM");
    expect(html).toContain('class="usage-series-line"');
    expect(html).toMatch(/class="usage-series-line"[^>]+d="M[^"]+ C/);
    expect(html).not.toContain("<circle");
  });

  it("Home is orientation and does not render usage analytics", () => {
    const html = renderToStaticMarkup(<DashboardPage stats={null} models={[model]} connections={{ status: "idle" }} isElevated={false} onNavigate={() => {}} />);
    expect(html).toContain("Ready to use");
    expect(html).toContain("Choose a model");
    expect(html).toContain("Connect a client");
    expect(html).not.toContain("Billable tokens");
    expect(html).not.toContain("Pool request history");
    expect(html).not.toContain("Economics");
  });

  it("Usage owns measured activity and does not repeat Home orientation", () => {
    const members: ResourceState<PoolUserStats[]> = { status: "idle" };
    const html = renderToStaticMarkup(<UsagePage isElevated={false} members={members} />);
    expect(html).toContain("Loading usage");
    expect(html).not.toContain("Choose a model");
    expect(html).not.toContain("Configure a client");
  });

  it("member Models omits Admin routing context", () => {
    const html = renderToStaticMarkup(<ModelsPage models={[model]} isElevated={false} />);
    expect(html).toContain("Model One");
    expect(html).not.toContain("Routing projection unavailable");
  });
});
