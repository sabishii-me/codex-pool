const params = new URLSearchParams(location.search);
const runtime = params.get("runtime") === "operator" ? "operator" : "member";
document.body.dataset.runtime = runtime;
document.querySelector("#runtime-label").textContent = runtime === "operator" ? "Operator workspace" : "Member workspace";
document.querySelector("#activity-scope").textContent = runtime === "operator" ? "Pool-wide processed requests" : "Your processed requests";
document.querySelector("#greeting").textContent = runtime === "operator" ? "Operations overview" : "Wednesday, July 22";
document.querySelector("#dashboard-title").textContent = runtime === "operator" ? "Gateway overview" : "Good afternoon, Avery";

const metrics = {
  member: [
    ["Your requests", "184", "+12%", "Last 24 hours", "measured", false],
    ["Processed tokens", "2.8M", "+8%", "Last 24 hours", "measured", false],
    ["Models available", "12 / 14", "2 limited", "Right now", "measured", true],
    ["Provider health", "4 / 5", "1 degraded", "Right now", "measured", true]
  ],
  operator: [
    ["Pool requests", "1,842", "+9%", "Last 24 hours", "measured", false],
    ["Processed tokens", "28.4M", "+6%", "Last 24 hours", "measured", false],
    ["Models available", "12 / 14", "2 limited", "Right now", "measured", true],
    ["Healthy connections", "8 / 9", "1 degraded", "Right now", "measured", true]
  ]
};

document.querySelector("#metric-grid").innerHTML = metrics[runtime].map(([label, value, change, timeframe, evidence, warning]) => `
  <article class="metric-card">
    <span>${label}</span>
    <div class="metric-value"><strong>${value}</strong><em class="${warning ? "warning" : ""}">${change}</em></div>
    <footer><span>${timeframe}</span><span>${evidence}</span></footer>
  </article>
`).join("");

const providers = [
  ["Codex", "healthy", "Healthy", "3 / 3", "Available", "—", "C"],
  ["DeepSeek", "degraded", "Degraded", "1 / 2", "Limited", "42 min", "D"],
  ["Gemini", "healthy", "Healthy", "2 / 2", "Available", "—", "G"],
  ["Z.ai", "healthy", "Healthy", "1 / 1", "Available", "—", "Z"],
  ["Northstar AI", "healthy", "Healthy", "1 / 1", "Available", "—", "N"]
];

document.querySelector("#provider-rows").innerHTML = providers.map(([name, state, stateLabel, connections, capacity, recovery, mark]) => `
  <div class="provider-row" role="row" data-provider="${name.toLowerCase().replaceAll(" ", "-")}">
    <span class="provider-name" role="cell"><i class="provider-mark" aria-hidden="true">${mark}</i>${name}</span>
    <span class="provider-state" role="cell"><i class="status-dot ${state}" aria-hidden="true"></i>${stateLabel}</span>
    <span role="cell">${connections}</span><span role="cell">${capacity}</span><span role="cell">${recovery}</span>
  </div>
`).join("");

const activity = {
  "24h": [18, 24, 20, 31, 28, 42, 39, 48, 43, 57, 53, 61],
  "7d": [118, 132, 125, 164, 153, 177, 184],
  "30d": [82, 96, 88, 114, 120, 132, 126, 148, 143, 157, 168, 162]
};

function renderChart(range) {
  const points = activity[range];
  const max = Math.max(...points);
  document.querySelector("#activity-chart").innerHTML = points.map((value, index) =>
    `<i class="chart-bar" style="height:${Math.max(8, value / max * 92)}%" title="Period ${index + 1}: ${value} requests"></i>`
  ).join("");
  document.querySelector("#activity-chart").setAttribute("aria-label", `Request activity over ${range}. Activity is higher than the preceding period with no unusual failure spike.`);
}
renderChart("24h");

const livePoints = [28,32,30,38,35,42,39,45,48,44,52,49,58,55,61,57,64,60,69,66,72,68,75,73,79,76,82,78,84,81];
function renderLiveBars(target, points = livePoints) {
  const max = Math.max(...points);
  target.innerHTML = points.map((value, index) => `<i style="height:${Math.max(8, value / max * 94)}%;opacity:${.58 + index / points.length * .42}" title="Interval ${index + 1}: ${value}"></i>`).join("");
}
renderLiveBars(document.querySelector("#live-monitor-chart"));
renderLiveBars(document.querySelector("#focus-chart"), runtime === "operator" ? livePoints : livePoints.map((value, index) => Math.round(value * .52 + (index % 4) * 2)));

const focus = runtime === "operator" ? {
  title: "Live pool health", description: "Requests, latency, errors, and capacity · rolling 30 minutes", summary: "Routing is stable with one reduced-capacity provider.",
  values: [["Request rate","21.4/min","Measured"],["P95 latency","2.8s","Last 15 min"],["Error rate","0.7%","Normal range"],["Headroom","71%","Estimated"]]
} : {
  title: "Live personal usage", description: "Your request activity · rolling 30 minutes", summary: "Your activity is within its normal range.",
  values: [["Your requests","38","Last 30 min"],["Processed tokens","412K","Measured"],["Active route","gpt-5.6","63% of requests"],["Gateway","Healthy","12 models"]]
};
document.querySelector("#focus-title").textContent = focus.title;
document.querySelector("#focus-description").textContent = focus.description;
document.querySelector("#focus-summary").textContent = focus.summary;
document.querySelector("#focus-readouts").innerHTML = focus.values.map(([label,value,detail]) => `<article><span>${label}</span><strong>${value}</strong><small>${detail}</small></article>`).join("");

document.querySelectorAll("[data-range]").forEach(button => button.addEventListener("click", () => {
  document.querySelectorAll("[data-range]").forEach(item => {
    const active = item === button;
    item.classList.toggle("active", active);
    item.setAttribute("aria-pressed", String(active));
  });
  renderChart(button.dataset.range);
}));

const models = [
  { id: "gpt-5.6", name: "GPT-5.6", provider: "Codex", status: "available", context: "272K", output: "128K", capabilities: ["Text", "Vision", "Tools"] },
  { id: "gpt-5.6-codex", name: "GPT-5.6 Codex", provider: "Codex", status: "available", context: "272K", output: "128K", capabilities: ["Text", "Tools"] },
  { id: "deepseek-v4-flash", name: "DeepSeek V4 Flash", provider: "DeepSeek", status: "limited", context: "128K", output: "32K", capabilities: ["Text", "Tools"] },
  { id: "glm-5.2", name: "GLM-5.2", provider: "Z.ai", status: "available", context: "128K", output: "64K", capabilities: ["Text", "Tools"] },
  { id: "gemini-3-pro", name: "Gemini 3 Pro", provider: "Gemini", status: "available", context: "1M", output: "64K", capabilities: ["Text", "Vision", "Tools"] },
  { id: "northstar-reasoner", name: "Northstar Reasoner", provider: "Northstar AI", status: "available", context: "96K", output: "24K", capabilities: ["Text", "Tools"] }
];
let modelFilter = "all";

function renderModels() {
  const query = document.querySelector("#model-search").value.trim().toLowerCase();
  const visible = models.filter(model => (modelFilter === "all" || model.status === modelFilter) && `${model.name} ${model.id} ${model.provider}`.toLowerCase().includes(query));
  document.querySelector("#model-count").textContent = `${visible.length} route${visible.length === 1 ? "" : "s"}`;
  document.querySelector("#model-list").innerHTML = visible.length ? visible.map(model => `
    <article class="model-card" data-model-id="${model.id}">
      <div class="model-main"><h2>${model.name}</h2><code>${model.id}</code></div>
      <div class="model-cell model-provider-cell"><span>Provider</span><strong>${model.provider}</strong></div>
      <div class="model-cell"><span>Status</span><strong class="${model.status === "available" ? "healthy-text" : ""}">${model.status === "available" ? "Available" : "Limited"}</strong></div>
      <div class="model-cell"><span>Context</span><strong>${model.context}</strong></div>
      <div class="model-cell capabilities"><span class="sr-only">Capabilities</span>${model.capabilities.map(item => `<span>${item}</span>`).join("")}</div>
      <button class="copy-button" type="button" data-copy-model="${model.id}">Copy ID</button>
      <div class="model-mobile-meta"><span>${model.provider}</span><span>${model.status === "available" ? "Available" : "Limited"}</span><span>${model.context} context</span><span>${model.capabilities.join(" · ")}</span></div>
    </article>
  `).join("") : `<div class="empty-models"><h2>No matching models</h2><p>Try a different model name, provider, or availability filter.</p></div>`;
  document.querySelectorAll("[data-copy-model]").forEach(button => button.addEventListener("click", () => showToast(`${button.dataset.copyModel} marked as copied in this preview.`)));
}
renderModels();

document.querySelector("#model-search").addEventListener("input", renderModels);
document.querySelectorAll("[data-filter]").forEach(button => button.addEventListener("click", () => {
  modelFilter = button.dataset.filter;
  document.querySelectorAll("[data-filter]").forEach(item => {
    const active = item === button;
    item.classList.toggle("active", active);
    item.setAttribute("aria-pressed", String(active));
  });
  renderModels();
}));

const workspacePages = {
  setup: {
    title: "Setup", section: "Member workspace", copy: "Configure a supported client and verify the gateway connection.", action: "Choose client",
    content: `<div class="workspace-grid"><section class="panel"><header class="panel-header"><div><h2>Recommended setup</h2><p>Pi · macOS, Windows, or Linux</p></div><span class="status-badge success">3 steps</span></header><div class="workspace-panel-body"><p>Use the generated gateway configuration, then run one verification request.</p><ol class="step-list"><li><span><strong>Choose your client</strong><small>Pi is selected for this preview</small></span><button class="text-button" type="button" data-preview-action="change-client">Change</button></li><li><span><strong>Copy configuration</strong><small>Uses a fictional preview credential</small></span><button class="text-button" type="button" data-preview-action="copy-config">Copy</button></li><li><span><strong>Verify connection</strong><small>No live request is made in Step 0</small></span><button class="text-button" type="button" data-preview-action="verify">Verify</button></li></ol><div class="code-preview"><header><span>Pi provider configuration</span><span>Preview only</span></header><code>{ "provider": "codex-pool", "baseUrl": "https://gateway.example.invalid" }</code></div></div></section><aside class="panel"><header class="panel-header"><div><h2>Supported clients</h2><p>Setup coverage</p></div></header><div class="workspace-aside-list"><article><strong>Pi</strong><small>Recommended · configuration file</small></article><article><strong>Claude Code</strong><small>Environment configuration</small></article><article><strong>Codex CLI</strong><small>Provider profile</small></article><article><strong>Gemini CLI</strong><small>Environment configuration</small></article><article><strong>Cute Code</strong><small>Application settings</small></article></div></aside></div>`
  },
  connections: {
    title: "Provider connections", section: "Operations", copy: "Manage credentialed upstream capacity and connection lifecycle state.", action: "Add connection",
    content: `<section class="panel"><header class="panel-header"><div><h2>Connections</h2><p>8 healthy · 1 degraded · fictional preview</p></div><span class="status-badge warning">1 needs attention</span></header><div class="data-list"><article><div><strong>Codex primary</strong><small>Codex · North America</small></div><div><span class="data-label">State</span><div class="data-value healthy-text">Healthy</div></div><div><span class="data-label">Last request</span><div class="data-value">12s ago</div></div><button class="button secondary" data-preview-action="inspect">Inspect</button></article><article><div><strong>DeepSeek reserve</strong><small>DeepSeek · Global</small></div><div><span class="data-label">State</span><div class="data-value">Cooldown</div></div><div><span class="data-label">Recovery</span><div class="data-value">42 min</div></div><button class="button secondary" data-preview-action="inspect">Inspect</button></article><article><div><strong>Northstar default</strong><small>Unknown runtime provider · neutral presentation</small></div><div><span class="data-label">State</span><div class="data-value healthy-text">Healthy</div></div><div><span class="data-label">Last request</span><div class="data-value">4m ago</div></div><button class="button secondary" data-preview-action="inspect">Inspect</button></article></div></section>`
  },
  routes: {
    title: "Model routes", section: "Operations", copy: "Inspect public routes, upstream targets, eligibility, and fallback policy.", action: "Review policy",
    content: `<section class="panel"><header class="panel-header"><div><h2>Active routes</h2><p>12 available · 2 limited</p></div><span class="evidence">Measured now</span></header><div class="data-list"><article><div><strong>gpt-5.6</strong><small>Codex · GPT-5.6</small></div><div><span class="data-label">Eligible</span><div class="data-value">3 connections</div></div><div><span class="data-label">Policy</span><div class="data-value">Weighted fair</div></div><button class="button secondary" data-preview-action="route">Details</button></article><article><div><strong>deepseek-v4-flash</strong><small>DeepSeek · V4 Flash</small></div><div><span class="data-label">Eligible</span><div class="data-value">1 connection</div></div><div><span class="data-label">Policy</span><div class="data-value">Reduced capacity</div></div><button class="button secondary" data-preview-action="route">Details</button></article><article><div><strong>northstar-reasoner</strong><small>Northstar AI · Reasoner</small></div><div><span class="data-label">Eligible</span><div class="data-value">1 connection</div></div><div><span class="data-label">Policy</span><div class="data-value">Standard</div></div><button class="button secondary" data-preview-action="route">Details</button></article></div></section>`
  },
  system: {
    title: "System", section: "Operations", copy: "Review runtime, persistence, configuration, and recovery readiness.", action: "Run checks",
    content: `<section class="panel"><header class="panel-header"><div><h2>System checks</h2><p>Design-preview status snapshot</p></div><span class="status-badge success">All operational</span></header><div class="workspace-panel-body system-checks"><article><h2><span class="status-dot healthy"></span>Gateway runtime</h2><p>Healthy · started 3 days ago</p></article><article><h2><span class="status-dot healthy"></span>Canonical usage events</h2><p>Current · no pending reconciliation</p></article><article><h2><span class="status-dot healthy"></span>Provider registry</h2><p>Five definitions · last reload successful</p></article><article><h2><span class="status-dot healthy"></span>Background jobs</h2><p>Eight running · no failed jobs</p></article></div></section>`
  }
};

function renderWorkspace(page) {
  const data = workspacePages[page];
  document.querySelector("#workspace-title").textContent = data.title;
  document.querySelector("#workspace-section").textContent = data.section;
  document.querySelector("#workspace-copy").textContent = data.copy;
  document.querySelector("#workspace-actions").innerHTML = `<button class="button primary" type="button" data-preview-action="primary">${data.action}</button>`;
  document.querySelector("#workspace-content").innerHTML = data.content;
  document.querySelectorAll("#page-workspace [data-preview-action]").forEach(button => button.addEventListener("click", () => showToast(`${button.textContent.trim()} simulated in this design preview.`)));
}

function navigate(page, updateHash = true) {
  if (["monitor", "connections", "routes", "system"].includes(page) && runtime !== "operator") page = "dashboard";
  const direct = ["dashboard", "models", "usage", "monitor"].includes(page) ? page : "workspace";
  document.querySelectorAll(".page").forEach(section => {
    const active = section.id === `page-${direct}`;
    section.hidden = !active;
    section.classList.toggle("active", active);
  });
  if (direct === "workspace") renderWorkspace(page);
  document.querySelectorAll("[data-page]").forEach(link => {
    const active = link.dataset.page === page;
    link.classList.toggle("active", active);
    if (active) link.setAttribute("aria-current", "page"); else link.removeAttribute("aria-current");
  });
  if (updateHash) history.replaceState(null, "", `#${page}`);
  closeDrawer();
  requestAnimationFrame(() => {
    const heading = document.querySelector(".page:not([hidden]) h1");
    if (heading) {
      document.title = `${heading.textContent} — Codex Pool Step 0`;
      document.querySelector("#main-content").focus({ preventScroll: true });
    }
    scrollTo(0, 0);
  });
}

document.querySelectorAll("[data-page]").forEach(link => link.addEventListener("click", event => {
  event.preventDefault();
  navigate(link.dataset.page);
}));
document.querySelectorAll("[data-navigate]").forEach(button => button.addEventListener("click", () => navigate(button.dataset.navigate)));

const sidebarToggle = document.querySelector("#sidebar-toggle");
sidebarToggle.addEventListener("click", () => {
  const collapsed = document.body.classList.toggle("sidebar-collapsed");
  sidebarToggle.setAttribute("aria-expanded", String(!collapsed));
  sidebarToggle.setAttribute("aria-label", collapsed ? "Expand sidebar" : "Collapse sidebar");
  sidebarToggle.title = collapsed ? "Expand sidebar" : "Collapse sidebar";
});
const tabletLandscapeFocus = matchMedia("(min-width: 901px) and (max-width: 1200px) and (orientation: landscape)").matches;
if (tabletLandscapeFocus) {
  document.body.classList.add("sidebar-collapsed");
  sidebarToggle.setAttribute("aria-expanded", "false");
  sidebarToggle.setAttribute("aria-label", "Expand sidebar");
  sidebarToggle.title = "Expand sidebar";
}

document.querySelector("#leaderboard-privacy").addEventListener("click", () => showToast("This preview shows abbreviated fictional names and aggregate weekly usage only."));

const drawer = document.querySelector("#mobile-drawer");
const backdrop = document.querySelector("#drawer-backdrop");
const menuButtons = [document.querySelector("#menu-button"), document.querySelector("#more-button")];
function openDrawer() {
  drawer.hidden = false;
  backdrop.hidden = false;
  menuButtons.forEach(button => button.setAttribute("aria-expanded", "true"));
  document.querySelector("#close-menu").focus();
}
function closeDrawer() {
  drawer.hidden = true;
  backdrop.hidden = true;
  menuButtons.forEach(button => button.setAttribute("aria-expanded", "false"));
}
menuButtons.forEach(button => button.addEventListener("click", openDrawer));
document.querySelector("#close-menu").addEventListener("click", closeDrawer);
backdrop.addEventListener("click", closeDrawer);
document.addEventListener("keydown", event => { if (event.key === "Escape" && !drawer.hidden) closeDrawer(); });

const toast = document.querySelector("#toast");
let toastTimer;
function showToast(message) {
  toast.textContent = message;
  toast.hidden = false;
  clearTimeout(toastTimer);
  toastTimer = setTimeout(() => { toast.hidden = true; }, 2600);
}

document.querySelector("#simulate-error").addEventListener("click", () => {
  document.querySelector("#persistence-ready").hidden = true;
  document.querySelector("#persistence-error").hidden = false;
  document.querySelector("#persistence-metric").querySelector("strong").textContent = "Unavailable";
  document.querySelector("#persistence-metric").querySelector("strong").className = "";
});
document.querySelector("#retry-persistence").addEventListener("click", () => {
  document.querySelector("#persistence-error").hidden = true;
  document.querySelector("#persistence-ready").hidden = false;
  document.querySelector("#persistence-metric").querySelector("strong").textContent = "Healthy";
  document.querySelector("#persistence-metric").querySelector("strong").className = "healthy-text";
  showToast("Persistence health restored in this preview.");
});

const initialPage = location.hash.slice(1);
const compactPortrait = matchMedia("(max-width: 720px)").matches;
const compactLandscape = matchMedia("(max-height: 500px) and (orientation: landscape) and (max-width: 900px)").matches;
const defaultPage = runtime === "operator" && (compactPortrait || compactLandscape) ? "monitor" : "dashboard";
navigate(initialPage && (workspacePages[initialPage] || ["dashboard", "models", "usage", "monitor"].includes(initialPage)) ? initialPage : defaultPage, false);
