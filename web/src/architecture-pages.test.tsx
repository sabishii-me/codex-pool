import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { DashboardPage } from "./features/dashboard";
import { UsagePage } from "./features/usage";
import { ModelsPage } from "./features/models";
import type { ModelDescriptor, SignalAnalytics } from "./types";

const model: ModelDescriptor = { id: "model-1", name: "Model One", provider: "provider", protocol: "openai", available_now: true, capabilities: { tools: true } };
const signal: SignalAnalytics = {
  generated_at: "2026-07-24T00:00:00Z", origin_data_since: "2026-01-01T00:00:00Z", economics: [],
  hourly: [{ hour: "2026-07-24T00:00:00Z", account_type: "provider", input_tokens: 1, cached_tokens: 0, output_tokens: 1, reasoning_tokens: 0, billable_tokens: 2, request_count: 1 }],
  origin_weekly: [], model_daily: [], quota_capacity: [], model_efficiency: [], reset_observations: [],
};

describe("route-level page jobs", () => {
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
    const html = renderToStaticMarkup(<UsagePage stats={null} signal={signal} originId="missing-origin" isElevated={false} />);
    expect(html).toContain("Personal usage is unavailable for this session");
    expect(html).not.toContain("Choose a model");
    expect(html).not.toContain("Configure a client");
  });

  it("member Models omits Admin routing context", () => {
    const html = renderToStaticMarkup(<ModelsPage models={[model]} isElevated={false} />);
    expect(html).toContain("Model One");
    expect(html).not.toContain("Routing projection unavailable");
  });
});
