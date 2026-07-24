import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { DashboardPage } from "./features/dashboard";
import { UsagePage } from "./features/usage";
import type { ResourceState } from "./resource-state";
import { ModelsPage } from "./features/models";
import type { ModelDescriptor, PoolUserStats } from "./types";

const model: ModelDescriptor = { id: "model-1", name: "Model One", provider: "provider", protocol: "openai", available_now: true, capabilities: { tools: true } };

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
    const members: ResourceState<PoolUserStats[]> = { status: "idle" };
    const html = renderToStaticMarkup(<UsagePage isElevated={false} members={members} />);
    expect(html).toContain("Loading measured usage");
    expect(html).not.toContain("Choose a model");
    expect(html).not.toContain("Configure a client");
  });

  it("member Models omits Admin routing context", () => {
    const html = renderToStaticMarkup(<ModelsPage models={[model]} isElevated={false} />);
    expect(html).toContain("Model One");
    expect(html).not.toContain("Routing projection unavailable");
  });
});
