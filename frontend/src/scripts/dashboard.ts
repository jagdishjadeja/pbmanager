import {
  BASE_DOMAIN,
  DemoManagerService,
  GiB,
  PROVISION_STEPS,
  randomName,
  validateName,
} from "../lib/demo-service";
import {
  renderDomains,
  websiteBadge,
  domainLabel,
  renderPocketBaseButton,
  pocketBaseDisabledReason,
} from "../lib/domain-view";
import { HttpManagerService, UnauthorizedError } from "../lib/http-service";
import { renderSite } from "../lib/site-view";
import { renderSFTP } from "../lib/sftp-view";
import { renderResources, maximumMemoryMiB } from "../lib/resources-view";
import { bytes, escapeHTML as esc, number, relativeTime } from "../lib/format";
import type {
  Instance,
  ManagerService,
  ServiceAction,
  Snapshot,
  UsageSample,
  View,
} from "../lib/types";

const $ = <T extends HTMLElement = HTMLElement>(selector: string): T => {
  const element = document.querySelector<T>(selector);
  if (!element) throw new Error(`Missing element: ${selector}`);
  return element;
};
const iconPaths: Record<string, string> = {
  server: "M4 3h16v7H4zM4 14h16v7H4zM7 6.5h.01M7 17.5h.01M16 6.5h2M16 17.5h2",
  disk: "m5 3-3 12v5h20v-5L19 3ZM2 15h20M17 18h.01M19 18h.01",
  cpu: "M7 7h10v10H7zM9 1v3M15 1v3M9 20v3M15 20v3M1 9h3M1 15h3M20 9h3M20 15h3",
  chart: "M3 3v18h18M7 14l4-5 4 3 6-8",
  external: "M14 3h7v7M21 3 10 14M10 3H3v18h18v-7",
  refresh:
    "M20 7v5h-5M4 17v-5h5M5.6 7a7 7 0 0 1 11.6-2L20 8M4 16l2.8 3A7 7 0 0 0 18.4 17",
  stop: "M6 6h12v12H6z",
  play: "m8 4 12 8-12 8Z",
  trash: "M3 6h18M9 6V3h6v3M6 6l1 15h10l1-15M10 10v7M14 10v7",
  check: "m5 12 4 4L19 6",
  close: "m6 6 12 12M6 18 18 6",
  chevron: "m9 5 7 7-7 7",
  arrow: "M12 19V5M5 12l7-7 7 7",
  "sort-up": "M12 19V5m-5 7 5-7 5 7",
  "sort-down": "M12 5v14m-5-7 5 7 5-7",
  sort: "m8 9 4-4 4 4m-8 6 4 4 4-4",
  copy: "M8 8h13v13H8zM16 8V3H3v13h5",
};
const icon = (name: string, size = 16) =>
  `<svg width="${size}" height="${size}" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="${iconPaths[name] ?? iconPaths.server}"/></svg>`;
let storage: Storage | undefined;
try {
  storage = window.localStorage;
} catch {
  /* In-memory demo fallback. */
}
const demoMode = document.documentElement.dataset.mode === "demo";
const liveService = demoMode ? undefined : new HttpManagerService();
const service: ManagerService =
  liveService ?? new DemoManagerService({ storage });
const baseDomain = () =>
  service.getSnapshot().server?.baseDomain ?? BASE_DOMAIN;
let snapshot: Snapshot = service.getSnapshot();
let currentView: View = "overview";
let detailId: string | undefined;
let sort: keyof Pick<
  Instance,
  "name" | "status" | "databaseBytes" | "storageBytes"
> = "name";
let sortDirection = 1;
const pending = new Set<string>();
let suggestedName = randomName();
let creating = false;
let domainInstanceId: string | undefined;
let siteInstanceId: string | undefined;
let sftpInstanceId: string | undefined;
let toastTimer: ReturnType<typeof setTimeout>;
let confirmAction: (() => Promise<void>) | undefined;

interface AgentToken {
  id: string;
  name: string;
  instanceIds?: string[];
  createdAt: string;
  lastUsedAt?: string;
}
interface AgentTask {
  id: string;
  title: string;
  status: string;
  assignee?: string;
}
async function loadAgents(): Promise<void> {
  if (!liveService) return;
  const [tokens, tasks] = await Promise.all([
    liveService.request<AgentToken[]>("/agent-tokens"),
    liveService.request<AgentTask[]>("/agent-tasks"),
  ]);
  $("#agent-token-list").innerHTML =
    tokens
      .map(
        (token) =>
          `<div class="agent-list-row"><div><strong>${esc(token.name)}</strong><p>${token.instanceIds?.length ? `${token.instanceIds.length} assigned instance(s)` : "All managed instances"} · ${token.lastUsedAt ? `Last used ${esc(relativeTime(token.lastUsedAt))}` : "Never used"}</p></div><button class="text-button danger-text" data-action="revoke-agent" data-token-id="${esc(token.id)}">Revoke</button></div>`,
      )
      .join("") || '<p class="field-hint">No agent tokens yet.</p>';
  $("#agent-task-list").innerHTML =
    tasks
      .map(
        (task) =>
          `<div class="agent-list-row"><strong>${esc(task.title)}</strong><span class="pill pill-gray">${esc(task.status.replaceAll("_", " "))}</span></div>`,
      )
      .join("") || '<p class="field-hint">No agent tasks yet.</p>';
}

function toast(message: string, error = false): void {
  clearTimeout(toastTimer);
  $("#toast-region").innerHTML =
    `<div class="toast${error ? " error" : ""}">${icon(error ? "close" : "check", 16)}<span>${esc(message)}</span></div>`;
  toastTimer = setTimeout(() => {
    $("#toast-region").replaceChildren();
  }, 4500);
}

const statusPill = (status: Instance["status"]) =>
  `<span class="pill ${status === "running" ? "pill-green" : status === "failed" ? "pill-red" : status === "provisioning" ? "pill-amber" : "pill-gray"}"><span class="status-dot ${status}"></span>${status[0].toUpperCase() + status.slice(1)}</span>`;
const metric = (
  label: string,
  value: string,
  description: string,
  iconName: string,
) =>
  `<div class="metric-card"><div class="metric-label"><span>${label}</span>${icon(iconName, 18)}</div><div class="metric-value">${value}</div><div class="metric-description">${description}</div></div>`;

function renderMetrics(): void {
  const running = snapshot.instances.filter(
    (item) => item.status === "running",
  ).length;
  const failed = snapshot.instances.filter(
    (item) => item.status === "failed",
  ).length;
  const cpu = snapshot.instances.reduce(
    (sum, item) => sum + item.cpuPercent,
    0,
  );
  const memory = snapshot.instances.reduce(
    (sum, item) => sum + item.memoryBytes,
    0,
  );
  $("#overview-metrics").innerHTML = [
    metric(
      "Total instances",
      String(snapshot.instances.length).padStart(2, "0"),
      `<span class="good">${running} running</span><span>·</span>${snapshot.instances.length - running} inactive`,
      "server",
    ),
    metric(
      "Instance storage",
      bytes(snapshot.storage.instanceBytes),
      `<span class="good">${((snapshot.storage.instanceBytes / (snapshot.storage.totalBytes || 1)) * 100).toFixed(1)}%</span> of server capacity`,
      "disk",
    ),
    metric(
      "CPU usage",
      `${cpu.toFixed(1)}<small> %</small>`,
      "Across running instances",
      "cpu",
    ),
    metric(
      "Memory usage",
      bytes(memory),
      `<span class="good">${((memory / (snapshot.server?.memoryTotalBytes || 4 * GiB)) * 100).toFixed(1)}%</span> of ${bytes(snapshot.server?.memoryTotalBytes || 4 * GiB)} RAM`,
      "chart",
    ),
  ].join("");
  $("#nav-count").textContent = String(snapshot.instances.length);
  $("#storage-used").textContent = bytes(snapshot.storage.usedBytes, 2);
  $("#storage-total").textContent =
    ` / ${bytes(snapshot.storage.totalBytes)} used`;
  $("#storage-percent").textContent =
    `${((snapshot.storage.usedBytes / (snapshot.storage.totalBytes || 1)) * 100).toFixed(1)}% used`;
  $("#storage-available").textContent = bytes(
    snapshot.storage.availableBytes,
    2,
  );
  $("#storage-bar").innerHTML =
    `<span class="instance-segment" style="width:${(snapshot.storage.instanceBytes / (snapshot.storage.totalBytes || 1)) * 100}%"></span><span class="other-segment" style="width:${(snapshot.storage.otherBytes / (snapshot.storage.totalBytes || 1)) * 100}%"></span>`;
  $("#storage-bar").setAttribute(
    "aria-label",
    `${bytes(snapshot.storage.instanceBytes)} instances, ${bytes(snapshot.storage.otherBytes)} system and other, ${bytes(snapshot.storage.availableBytes)} available`,
  );
  $("#storage-legend").innerHTML = [
    ["green", "Instances", snapshot.storage.instanceBytes],
    ["sage", "System & reserved", snapshot.storage.otherBytes],
    ["gray", "Available", snapshot.storage.availableBytes],
  ]
    .map(
      ([color, label, size]) =>
        `<div><span class="legend-dot ${color}"></span><div>${label}<strong>${bytes(Number(size), 2)}</strong></div></div>`,
    )
    .join("");
  $("#last-updated").textContent =
    `${demoMode ? "Sample data" : "Live data"} · updated ${snapshot.storage.measuredAt ? relativeTime(snapshot.storage.measuredAt).toLowerCase() : "pending"}`;
  if (snapshot.server) {
    $("#server-name").textContent = snapshot.server.name;
    $("#server-os").textContent = snapshot.server.os;
    $("#server-region").textContent = "Hetzner";
    $("#server-resources").textContent =
      `${snapshot.server.cpuCount} vCPU · ${bytes(snapshot.server.memoryTotalBytes)} RAM`;
    document.querySelectorAll(".domain-suffix").forEach((element) => {
      element.textContent = `.${baseDomain()}`;
    });
  }
  $("#storage-mode-label").textContent = demoMode
    ? "Sample volume"
    : "Live filesystem";
  $("#create-mode-note").textContent = demoMode
    ? "Demo · no server changes"
    : `1 GiB storage · point DNS to ${snapshot.server?.publicIP ?? "46.62.175.23"}`;
  $("#usage-description").textContent = demoMode
    ? "Illustrative traffic across your instances."
    : "Measured HTTP traffic across your instances.";
  const serverWarning = snapshot.server?.error ?? "";
  $("#connection-error").hidden = !serverWarning;
  $("#connection-error").textContent = serverWarning;
  const health = $("#workspace-health");
  health.dataset.health = failed ? "attention" : "healthy";
  health.textContent = failed
    ? `${failed} ${failed === 1 ? "instance needs" : "instances need"} attention`
    : "No instance errors";
}

function renderTable(): void {
  const query = $<HTMLInputElement>("#instance-search")
    .value.toLowerCase()
    .trim();
  const status = $<HTMLSelectElement>("#status-filter").value;
  const instances = snapshot.instances.filter(
    (item) =>
      (item.name.includes(query) || item.hostname.includes(query)) &&
      (status === "all" || item.status === status),
  );
  instances.sort((a, b) => {
    const first = a[sort];
    const second = b[sort];
    return (
      (typeof first === "number" && typeof second === "number"
        ? first - second
        : String(first).localeCompare(String(second))) * sortDirection
    );
  });
  $("#table-count").textContent = String(snapshot.instances.length);
  $("#table-summary").textContent =
    `Showing ${instances.length} of ${snapshot.instances.length} instances`;
  $("#instances-panel").setAttribute("aria-busy", "false");
  document
    .querySelectorAll<HTMLButtonElement>("[data-sort]")
    .forEach((button) => {
      button
        .closest("th")
        ?.setAttribute(
          "aria-sort",
          button.dataset.sort === sort
            ? sortDirection === 1
              ? "ascending"
              : "descending"
            : "none",
        );
      button.querySelector("span")!.innerHTML = icon(
        button.dataset.sort === sort
          ? sortDirection === 1 ? "sort-up" : "sort-down"
          : "sort",
        12,
      );
    });
  if (!instances.length) {
    $("#instances-body").innerHTML =
      `<tr><td colspan="7" class="empty-state"><strong>${snapshot.instances.length ? "No matching instances" : "A clean slate for your next idea."}</strong>${snapshot.instances.length ? "Try a different search or status filter." : "Create your first PocketBase instance to get started."}</td></tr>`;
    return;
  }
  $("#instances-body").innerHTML = instances
    .map((item) => {
      const isPending = pending.has(item.id) || item.status === "provisioning";
      const disabled = isPending ? "disabled" : "";
      const action = item.status === "running" ? "stop" : "start";
      return `<tr data-instance="${esc(item.id)}"><td><a class="instance-link" href="#instance/${encodeURIComponent(item.id)}"><span class="instance-avatar ${item.name === "studio-api" ? "lavender" : item.name === "field-notes" ? "sand" : ""}">${icon("server", 16)}</span><span><strong>${esc(item.name)}</strong><small>${esc(item.hostname)}</small></span></a></td><td>${isPending && item.status !== "provisioning" ? '<span class="spinner" aria-label="Working"></span>' : statusPill(item.status)}<div class="website-status">${websiteBadge(item)}</div></td><td class="number-cell db-cell">${bytes(item.databaseBytes)}</td><td class="number-cell storage-cell"><strong>${bytes(item.storageBytes)}</strong><div class="mini-storage" aria-hidden="true"><span style="width:${Math.min(100, (item.storageBytes / GiB) * 100)}%"></span></div></td><td class="number-cell">${item.status === "running" ? `${item.cpuPercent.toFixed(1)}<span class="subtle-label">%</span>` : "—"}</td><td class="number-cell">${item.status === "running" ? bytes(item.memoryBytes, 0) : "—"}</td><td><div class="row-actions">${renderPocketBaseButton(item, icon("external", 13), true)}<details><summary aria-label="Actions for ${esc(item.name)}">···</summary><div class="row-menu"><button data-action="${action}" data-id="${esc(item.id)}" ${disabled}>${icon(action === "start" ? "play" : "stop", 13)}${action === "start" ? "Start" : "Stop"} instance</button><button data-action="restart" data-id="${esc(item.id)}" ${disabled}>${icon("refresh", 13)}Restart instance</button><button data-action="copy" data-id="${esc(item.id)}">${icon("copy", 13)}Copy URL</button><button class="danger-text" data-action="delete" data-id="${esc(item.id)}" ${disabled}>${icon("trash", 13)}Delete instance</button></div></details></div></td></tr>`;
    })
    .join("");
}

function renderActivity(): void {
  let activity = detailId
    ? snapshot.activity.filter((item) => item.instanceId === detailId)
    : snapshot.activity;
  if (currentView !== "activity") activity = activity.slice(0, 3);
  $("#activity-title").textContent = detailId
    ? "Instance activity"
    : currentView === "activity"
      ? "All activity"
      : "Recent activity";
  $("#all-activity-link").hidden = currentView === "activity";
  $("#activity-list").innerHTML = activity.length
    ? activity
        .map(
          (item) =>
            `<div class="activity-list-item"><span class="activity-icon ${item.kind}">${icon(item.kind === "success" ? "check" : item.kind === "error" ? "close" : "stop", 12)}</span><div><strong>${esc(item.message)}</strong><p>${esc(item.instanceName)}.${esc(baseDomain())}</p></div><time class="activity-time" datetime="${esc(item.at)}" title="${esc(new Date(item.at).toLocaleString())}">${relativeTime(item.at)}</time></div>`,
        )
        .join("")
    : '<div class="empty-state">No activity yet. Your next action will appear here.</div>';
}

function chart(samples: UsageSample[], suffix: string): string {
  if (!samples.length)
    return `<div class="empty-state">${demoMode ? "No usage samples." : "Collecting usage history. The first measurement will appear shortly."}</div>`;
  const width = 900;
  const height = 140;
  const max = Math.max(1, ...samples.map((sample) => sample.requests)) * 1.2;
  const points = samples.map((sample, index) => [
    (index / Math.max(1, samples.length - 1)) * width,
    height - (sample.requests / max) * height,
  ]);
  const path = points
    .map(
      ([x, y], index) => `${index ? "L" : "M"}${x.toFixed(1)} ${y.toFixed(1)}`,
    )
    .join(" ");
  const area = `${path} L${width} ${height} L0 ${height} Z`;
  return `<div class="chart-wrap"><svg class="chart-svg" viewBox="0 0 ${width} ${height + 10}" preserveAspectRatio="none" role="img" aria-label="Illustrative HTTP request history"><defs><linearGradient id="area-${suffix}" x1="0" y1="0" x2="0" y2="1"><stop offset="0%" stop-color="#71717a" stop-opacity=".3"/><stop offset="100%" stop-color="#71717a" stop-opacity=".02"/></linearGradient></defs>${[0, 1, 2, 3].map((i) => `<line x1="0" y1="${(i * height) / 3}" x2="${width}" y2="${(i * height) / 3}" stroke="#e4e4e7" stroke-width="1" stroke-dasharray="4 5"/>`).join("")}<path d="${area}" fill="url(#area-${suffix})"/><path d="${path}" fill="none" stroke="#18181b" stroke-width="2" vector-effect="non-scaling-stroke"/></svg><div class="chart-labels">${[
    0, 0.25, 0.5, 0.75, 1,
  ]
    .map((ratio) => samples[Math.round((samples.length - 1) * ratio)])
    .map(
      (sample) =>
        `<span>${new Date(sample.at).toLocaleString("en", { month: "short", day: "numeric", hour: "2-digit", minute: "2-digit", hour12: false })}</span>`,
    )
    .join("")}</div></div>`;
}

function renderUsage(): void {
  const hours = Number($<HTMLSelectElement>("#usage-range").value);
  const samples = service.getUsage(undefined, hours);
  const totals = samples.reduce(
    (acc, item) => ({
      requests: acc.requests + item.requests,
      ingress: acc.ingress + item.ingressBytes,
      egress: acc.egress + item.egressBytes,
    }),
    { requests: 0, ingress: 0, egress: 0 },
  );
  $("#usage-metrics").innerHTML =
    `<div><span>HTTP requests</span><strong>${number(totals.requests)}</strong></div><div><span>Incoming HTTP traffic${snapshot.instances.some((item) => item.ingressEstimated) ? " (estimated)" : ""}</span><strong>${bytes(totals.ingress)}</strong></div><div><span>Outgoing HTTP traffic</span><strong>${bytes(totals.egress)}</strong></div>`;
  $("#usage-chart").innerHTML = chart(samples, "usage");
}

function renderDetail(): void {
  if (!detailId) return;
  const editingRAM = document.activeElement?.id === "instance-ram";
  const editedRAM = editingRAM
    ? $<HTMLInputElement>("#instance-ram").value
    : undefined;
  const item = snapshot.instances.find((instance) => instance.id === detailId);
  if (!item) {
    location.hash = "#instances";
    return;
  }
  $("#page-title").textContent = item.name;
  $("#page-description").textContent =
    `${item.hostname} · A dedicated PocketBase instance.`;
  $("#breadcrumb").textContent = `Instances / ${item.name}`;
  document.title = `${item.name} · PocketBase Manager`;
  const disabled =
    pending.has(item.id) || item.status === "provisioning" ? "disabled" : "";
  $("#detail-panel").innerHTML =
    `${item.lastError ? `<div class="error-banner">${esc(item.lastError)}</div>` : ""}<div class="detail-top"><div>${statusPill(item.status)}<span class="subtle-label">Created ${new Date(item.createdAt).toLocaleDateString("en", { month: "short", day: "numeric", year: "numeric" })} · ${demoMode ? "Demo instance" : "Managed instance"}</span></div><div class="detail-actions">${!demoMode ? `<button class="button button-secondary" data-action="credentials" data-id="${esc(item.id)}">Superadmin login</button>` : ""}${renderPocketBaseButton(item, icon("external", 14))}<button class="button button-secondary" data-action="${item.status === "running" ? "stop" : "start"}" data-id="${esc(item.id)}" ${disabled}>${icon(item.status === "running" ? "stop" : "play", 14)}${item.status === "running" ? "Stop" : "Start"}</button><button class="button button-secondary" data-action="restart" data-id="${esc(item.id)}" ${disabled}>${icon("refresh", 14)}Restart</button><button class="button button-secondary" data-action="delete" data-id="${esc(item.id)}" ${disabled} aria-label="Delete ${esc(item.name)}">${icon("trash", 14)}</button></div></div>${renderDomains(item, snapshot.server?.publicIP ?? "46.62.175.23", pending.has(item.id), demoMode)}${renderSite(item, pending.has(item.id))}${renderResources(item, pending.has(item.id), snapshot.server?.memoryTotalBytes)}${renderSFTP(item, pending.has(item.id), snapshot.server?.publicIP ?? "46.62.175.23")}<div class="metric-grid">${metric("Database size", bytes(item.databaseBytes), "SQLite databases + WAL files", "disk")}${metric("Total storage", bytes(item.storageBytes), "Database, uploads & backups", "server")}${metric("CPU usage", `${item.cpuPercent.toFixed(1)}<small> %</small>`, demoMode ? "Sample measurement" : "Current measurement", "cpu")}${metric("Memory usage", bytes(item.memoryBytes), demoMode ? "Sample measurement" : "Current measurement", "chart")}</div><div class="detail-grid"><section class="panel"><div class="panel-top"><h2>HTTP requests</h2><span class="subtle-label">Last 24 hours${demoMode ? " · illustrative" : ""}</span></div>${chart(service.getUsage(item.id), "detail")}</section><section class="panel detail-info"><h2>Instance configuration</h2><dl><div><dt>Hostname</dt><dd>${esc(item.hostname)}</dd></div><div><dt>Service</dt><dd class="mono">${demoMode ? `pb-${esc(item.name)}` : `pbm-${esc(item.id)}`}.service</dd></div><div><dt>Reverse proxy</dt><dd>Caddy · HTTPS</dd></div><div><dt>Incoming HTTP traffic${item.ingressEstimated ? " (estimated)" : ""}</dt><dd>${bytes(item.ingressBytes)}</dd></div><div><dt>Outgoing HTTP traffic</dt><dd>${bytes(item.egressBytes)}</dd></div></dl></section></div><section class="panel logs-panel"><h2>Service logs<span class="subtle-label">${demoMode ? "Simulated systemd journal" : "systemd journal"}</span></h2><pre>${esc(service.getLogs(item.id).join("\n"))}</pre></section>`;
  if (editedRAM !== undefined) {
    const input = $<HTMLInputElement>("#instance-ram");
    input.value = editedRAM;
    input.focus();
  }
}

function render(): void {
  snapshot = service.getSnapshot();
  renderMetrics();
  renderTable();
  renderActivity();
  if (currentView === "usage") renderUsage();
  if (detailId) renderDetail();
}

function route(): void {
  const hash = location.hash.replace(/^#/, "");
  detailId = undefined;
  if (hash.startsWith("instance/")) {
    try {
      detailId = decodeURIComponent(hash.slice(9));
    } catch {
      location.hash = "#instances";
      return;
    }
    currentView = "instances";
  } else
    currentView = ["overview", "instances", "usage", "activity"].includes(hash)
      ? (hash as View)
      : "overview";
  const titles = {
    overview: [
      "Overview",
      "A home for your backends. Keep an eye on everything in one place.",
    ],
    instances: [
      "Instances",
      "Small backends. Big possibilities. Manage every instance here.",
    ],
    usage: [
      "Usage & analytics",
      "Understand how your instances use your infrastructure.",
    ],
    activity: ["Activity", "A running history of your instance operations."],
  };
  $("#page-title").textContent = titles[currentView][0];
  $("#page-description").textContent = titles[currentView][1];
  $("#breadcrumb").textContent = titles[currentView][0];
  document.title = `${titles[currentView][0]} · PocketBase Manager`;
  $("#overview-metrics").hidden =
    Boolean(detailId) || currentView === "activity";
  $("#overview-panels").hidden =
    Boolean(detailId) || !["overview", "usage"].includes(currentView);
  $("#instances-panel").hidden =
    Boolean(detailId) || !["overview", "instances"].includes(currentView);
  $("#analytics-panel").hidden = Boolean(detailId) || currentView !== "usage";
  $("#detail-panel").hidden = !detailId;
  $("#activity-panel").hidden = currentView === "usage";
  document.querySelectorAll<HTMLAnchorElement>("[data-nav]").forEach((link) => {
    link.classList.toggle("active", link.dataset.nav === currentView);
    if (link.dataset.nav === currentView)
      link.setAttribute("aria-current", "page");
    else link.removeAttribute("aria-current");
  });
  closeMobile();
  render();
  void loadSelected();
}

function closeMenus(): void {
  document
    .querySelectorAll<HTMLDetailsElement>(".row-actions details[open]")
    .forEach((detail) => {
      detail.open = false;
    });
}
function closeMobile(): void {
  $("#sidebar").classList.remove("open");
  $("#mobile-overlay").hidden = true;
  $(".mobile-menu").setAttribute("aria-expanded", "false");
}

function openCreate(): void {
  if (!creating) {
    $<HTMLFormElement>("#create-form").reset();
    suggestedName = randomName();
    updatePreview();
    $("#name-error").textContent = "";
    $("#provision-progress").hidden = true;
    $("#create-submit").innerHTML =
      `${icon("plus", 17)}<span>Create instance</span>`;
  }
  const maximum = maximumMemoryMiB(snapshot.server?.memoryTotalBytes);
  $<HTMLInputElement>("#create-ram").max = String(maximum);
  $("#create-ram-hint").textContent =
    `128–${maximum} MiB in steps of 64. Default 512 MiB; RAM is used on demand.`;
  $<HTMLDialogElement>("#create-dialog").showModal();
  if (!creating) $<HTMLInputElement>("#instance-name").focus();
}

function updatePreview(): void {
  $("#hostname-preview").textContent =
    $<HTMLInputElement>("#instance-name").value.trim() || suggestedName;
}

async function control(id: string, action: ServiceAction): Promise<void> {
  pending.add(id);
  render();
  try {
    await service.control(id, action);
    toast(
      `Instance ${action === "start" ? "started" : action === "stop" ? "stopped" : "restarted"}.`,
    );
  } catch (error) {
    toast(error instanceof Error ? error.message : "Operation failed.", true);
  } finally {
    pending.delete(id);
    render();
  }
}

function confirm(
  title: string,
  description: string,
  label: string,
  action: () => Promise<void>,
): void {
  $("#confirm-title").textContent = title;
  $("#confirm-description").textContent = description;
  $("#confirm-submit").textContent = label;
  confirmAction = action;
  $<HTMLDialogElement>("#confirm-dialog").showModal();
  $<HTMLButtonElement>('[data-close="confirm-dialog"]').focus();
}

document.addEventListener("click", async (event) => {
  const target = event.target as HTMLElement;
  const button = target.closest<HTMLButtonElement>("[data-action]");
  const closeButton = target.closest<HTMLButtonElement>("[data-close]");
  if (closeButton)
    $<HTMLDialogElement>(`#${closeButton.dataset.close}`).close();
  if (!target.closest(".row-actions details")) closeMenus();
  if (!button) return;
  const action = button.dataset.action;
  const id = button.dataset.id;
  closeMenus();
  if (action === "create") openCreate();
  else if (action === "agent-access" && liveService) {
    $("#mcp-endpoint").textContent = `${location.origin}/mcp`;
    $("#agent-access-error").textContent = "";
    $("#agent-instance").innerHTML =
      '<option value="">All managed instances</option>' +
      snapshot.instances
        .map(
          (item) =>
            `<option value="${esc(item.id)}">${esc(item.name)}</option>`,
        )
        .join("");
    $<HTMLDialogElement>("#agents-dialog").showModal();
    try {
      await loadAgents();
    } catch (error) {
      $("#agent-access-error").textContent =
        error instanceof Error ? error.message : "Could not load agents.";
    }
  } else if (action === "revoke-agent" && liveService) {
    const tokenId = button.dataset.tokenId;
    if (!tokenId) return;
    confirm(
      "Revoke agent access?",
      "This token will stop working immediately for every MCP request.",
      "Revoke token",
      async () => {
        await liveService.request(
          `/agent-tokens/${encodeURIComponent(tokenId)}`,
          { method: "DELETE" },
        );
        await loadAgents();
        toast("Agent access revoked.");
      },
    );
  } else if (action === "logout" && liveService) {
    await liveService.logout();
    showLogin();
  } else if (action === "about")
    $<HTMLDialogElement>("#about-dialog").showModal();
  else if (action === "reset") {
    confirm(
      "Start fresh?",
      "This resets all simulated instances and activity in this browser. No real services are affected.",
      "Reset demo",
      async () => {
        service.reset();
        location.hash = "#overview";
        toast("Demo reset.");
      },
    );
  } else if (action === "refresh") {
    button.disabled = true;
    button.classList.add("is-refreshing");
    await service.refresh(true);
    button.disabled = false;
    button.classList.remove("is-refreshing");
    toast(`${demoMode ? "Sample" : "Live"} measurements refreshed.`);
  } else if (id) {
    const instance = snapshot.instances.find((item) => item.id === id);
    if (!instance) return;
    if (action === "open-pb") {
      const reason = pocketBaseDisabledReason(instance);
      if (reason) {
        toast(reason);
        return;
      }
    }
    if (action === "add-sftp" || action === "reset-sftp") {
      sftpInstanceId = id;
      $<HTMLFormElement>("#sftp-form").reset();
      const username = $<HTMLInputElement>("#sftp-username");
      username.value = button.dataset.username ?? "";
      username.readOnly = action === "reset-sftp";
      $("#sftp-title").textContent =
        action === "reset-sftp"
          ? "Reset file account password"
          : "Add a file account";
      $("#sftp-error").textContent = "";
      $<HTMLDialogElement>("#sftp-dialog").showModal();
    } else if (action === "remove-sftp") {
      const username = button.dataset.username;
      if (!username) return;
      confirm(
        `Remove ${username}?`,
        "This revokes SSH/SFTP access and disconnects that account. The website and database are kept.",
        "Remove file account",
        async () => {
          pending.add(id);
          render();
          try {
            await service.removeSFTPUser(id, username);
            toast("File account removed.");
          } finally {
            pending.delete(id);
            render();
          }
        },
      );
    } else if (action === "upload-site") {
      siteInstanceId = id;
      $<HTMLFormElement>("#site-form").reset();
      $("#site-form-error").textContent = "";
      $("#site-upload-status").textContent = "";
      $<HTMLDialogElement>("#site-dialog").showModal();
    } else if (action === "remove-site") {
      confirm(
        `Remove the website for ${instance.name}?`,
        "This removes the uploaded static website. PocketBase records, uploads, and admin access remain available.",
        "Remove site",
        async () => {
          pending.add(id);
          render();
          try {
            await service.removeSite(id);
            toast("Static site removed.");
          } finally {
            pending.delete(id);
            render();
          }
        },
      );
    } else if (action === "add-domain") {
      domainInstanceId = id;
      $<HTMLFormElement>("#domain-form").reset();
      $("#domain-form-error").textContent = "";
      $<HTMLDialogElement>("#domain-dialog").showModal();
    } else if (action === "verify-domain" || action === "remove-domain") {
      const hostname = button.dataset.hostname;
      if (!hostname) return;
      if (action === "remove-domain")
        confirm(
          `Remove ${hostname}?`,
          `The default domain ${instance.hostname} and all instance data will stay available.`,
          "Remove domain",
          async () => {
            await service.removeDomain(id, hostname);
            toast("Customer domain removed.");
          },
        );
      else {
        pending.add(id);
        render();
        try {
          await service.verifyDomain(id, hostname);
          toast("Domain status updated.");
        } catch (error) {
          toast(
            error instanceof Error ? error.message : "Domain check failed.",
            true,
          );
        } finally {
          pending.delete(id);
          render();
        }
      }
    } else if (action === "start" || action === "stop" || action === "restart")
      await control(id, action);
    else if (action === "delete")
      confirm(
        `Delete ${instance.name}?`,
        `This removes ${instance.hostname} and its ${bytes(instance.storageBytes)} of ${demoMode ? "sample data from this demo" : "data, uploads, and backups from the server. This cannot be undone"}.`,
        "Delete instance",
        async () => {
          await service.remove(id);
          if (detailId === id) location.hash = "#instances";
          toast("Instance deleted.");
        },
      );
    else if (action === "copy") {
      try {
        await navigator.clipboard.writeText(`https://${instance.hostname}`);
        toast("Instance URL copied.");
      } catch {
        toast(`Instance URL: https://${instance.hostname}`);
      }
    } else if (action === "credentials" && liveService) {
      try {
        const credentials = await liveService.credentials(id);
        $("#credential-email").textContent = credentials.email;
        $("#credential-password").textContent = credentials.password;
        $<HTMLDialogElement>("#credentials-dialog").showModal();
      } catch (error) {
        toast(
          error instanceof Error
            ? error.message
            : "Unable to reveal credentials.",
          true,
        );
      }
    } else if (action === "open-pb" && !demoMode)
      window.open(
        `https://${instance.hostname}/_/`,
        "_blank",
        "noopener,noreferrer",
      );
    else if (action === "open-pb")
      toast(
        `Demo only: PocketBase will open at https://${instance.hostname}/_/.`,
      );
  }
});

$("#confirm-submit").addEventListener("click", async () => {
  const button = $<HTMLButtonElement>("#confirm-submit");
  button.disabled = true;
  try {
    await confirmAction?.();
    $<HTMLDialogElement>("#confirm-dialog").close();
  } catch (error) {
    toast(error instanceof Error ? error.message : "Operation failed.", true);
  } finally {
    button.disabled = false;
  }
});

$("#create-form").addEventListener("submit", async (event) => {
  event.preventDefault();
  if (creating) return;
  const name =
    $<HTMLInputElement>("#instance-name").value.trim() || suggestedName;
  const error = validateName(name);
  const input = $<HTMLInputElement>("#instance-name");
  if (error || snapshot.instances.some((instance) => instance.name === name)) {
    $("#name-error").textContent =
      error ?? "This instance name is already in use.";
    input.setAttribute("aria-invalid", "true");
    input.focus();
    return;
  }
  const memoryLimitMiB = $<HTMLInputElement>("#create-ram").valueAsNumber;
  creating = true;
  $("#name-error").textContent = "";
  input.removeAttribute("aria-invalid");
  input.disabled = true;
  $<HTMLInputElement>("#create-ram").disabled = true;
  $<HTMLInputElement>("#create-custom-domain").disabled = true;
  $<HTMLButtonElement>("#regenerate-name").disabled = true;
  $<HTMLInputElement>("#simulate-failure").disabled = true;
  const submit = $<HTMLButtonElement>("#create-submit");
  submit.disabled = true;
  submit.innerHTML =
    '<span class="spinner"></span><span>Creating instance…</span>';
  $("#provision-progress").hidden = false;
  let newId: string | undefined;
  try {
    const instance = await service.create(
      name,
      (progress) => {
        newId = progress.instanceId;
        $("#provision-progress").innerHTML =
          PROVISION_STEPS.map(
            (label, index) =>
              `<div class="provision-step ${progress.complete || index + 1 < progress.step ? "done" : index + 1 === progress.step ? "current" : ""}"><span class="step-circle">${progress.complete || index + 1 < progress.step ? icon("check", 11) : index + 1}</span>${esc(label)}</div>`,
          ).join("") +
          `<p class="section-description">${esc(progress.label)}</p>`;
      },
      $<HTMLInputElement>("#simulate-failure").checked,
      $<HTMLInputElement>("#create-custom-domain").value.trim(),
      memoryLimitMiB,
    );
    $<HTMLDialogElement>("#create-dialog").close();
    location.hash = `#instance/${encodeURIComponent(instance.id)}`;
    toast(
      demoMode
        ? `${instance.hostname} is ready in the demo.`
        : `${instance.name} created. ${domainLabel(instance.domains?.[0])}.`,
    );
  } catch (error) {
    $("#name-error").textContent =
      error instanceof Error ? error.message : "Provisioning failed.";
    toast("Provisioning failed. Check the instance details.", true);
    if (!$<HTMLDialogElement>("#create-dialog").open && newId)
      location.hash = `#instance/${encodeURIComponent(newId)}`;
  } finally {
    creating = false;
    input.disabled = false;
    $<HTMLInputElement>("#create-ram").disabled = false;
    $<HTMLInputElement>("#create-custom-domain").disabled = false;
    $<HTMLButtonElement>("#regenerate-name").disabled = false;
    $<HTMLInputElement>("#simulate-failure").disabled = false;
    submit.disabled = false;
    submit.innerHTML = `${icon("plus", 17)}<span>Create instance</span>`;
  }
});

$("#instance-name").addEventListener("input", () => {
  updatePreview();
  $("#name-error").textContent = "";
  $("#instance-name").removeAttribute("aria-invalid");
});
$("#regenerate-name").addEventListener("click", () => {
  suggestedName = randomName();
  $<HTMLInputElement>("#instance-name").value = "";
  updatePreview();
});
$("#instance-search").addEventListener("input", renderTable);
$("#status-filter").addEventListener("change", renderTable);
$("#usage-range").addEventListener("change", () => {
  void loadSelected();
});
document.querySelectorAll<HTMLButtonElement>("[data-sort]").forEach((button) =>
  button.addEventListener("click", () => {
    const nextSort = button.dataset.sort as typeof sort;
    sortDirection = sort === nextSort ? -sortDirection : 1;
    sort = nextSort;
    renderTable();
  }),
);

document.addEventListener(
  "toggle",
  (event) => {
    const details = event.target;
    if (
      !(details instanceof HTMLDetailsElement) ||
      !details.closest(".row-actions") ||
      !details.open
    )
      return;
    document
      .querySelectorAll<HTMLDetailsElement>(".row-actions details[open]")
      .forEach((other) => {
        if (other !== details) other.open = false;
      });
    const menu = details.querySelector<HTMLElement>(".row-menu")!;
    const rect = details.getBoundingClientRect();
    menu.style.left = `${Math.max(8, Math.min(rect.right - 145, window.innerWidth - 153))}px`;
    menu.style.top = `${Math.min(rect.bottom + 5, window.innerHeight - 165)}px`;
  },
  true,
);
document.addEventListener("keydown", (event) => {
  if (event.key === "Escape") {
    closeMenus();
    closeMobile();
  }
});
window.addEventListener("scroll", closeMenus, true);
window.addEventListener("resize", closeMenus);
$(".mobile-menu").addEventListener("click", () => {
  const open = $("#sidebar").classList.toggle("open");
  $("#mobile-overlay").hidden = !open;
  $(".mobile-menu").setAttribute("aria-expanded", String(open));
});
$("#mobile-overlay").addEventListener("click", closeMobile);
window.addEventListener("hashchange", route);
service.subscribe(render);
let connected = demoMode;
let refreshPending = false;
async function loadSelected(): Promise<void> {
  if (!connected) return;
  const selectedId = detailId;
  try {
    if (selectedId)
      await Promise.all([
        service.loadUsage(selectedId),
        service.loadLogs(selectedId),
      ]);
    else if (currentView === "usage")
      await service.loadUsage(
        undefined,
        Number($<HTMLSelectElement>("#usage-range").value),
      );
    if (selectedId === detailId) render();
  } catch (error) {
    if (!(error instanceof UnauthorizedError))
      toast(
        error instanceof Error ? error.message : "Could not load measurements.",
        true,
      );
  }
}
function showLogin(): void {
  connected = false;
  $(".app-shell").hidden = true;
  $("#connection-screen").hidden = true;
  document
    .querySelectorAll<HTMLDialogElement>("dialog[open]")
    .forEach((dialog) => dialog.close());
  $<HTMLDialogElement>("#login-dialog").showModal();
}
async function connectedUI(): Promise<void> {
  connected = true;
  $("#connection-screen").hidden = true;
  $(".app-shell").hidden = false;
  $("#admin-email").textContent = liveService?.email ?? "Admin";
  route();
}
window.addEventListener("manager:unauthorized", showLogin);
$("#login-dialog").addEventListener("cancel", (event) =>
  event.preventDefault(),
);
$("#credentials-dialog").addEventListener("close", () => {
  $("#credential-email").textContent = "";
  $("#credential-password").textContent = "";
});
$("#login-form").addEventListener("submit", async (event) => {
  event.preventDefault();
  if (!liveService) return;
  const submit = $<HTMLButtonElement>("#login-submit");
  submit.disabled = true;
  $("#login-error").textContent = "";
  try {
    await liveService.login(
      $<HTMLInputElement>("#login-email").value,
      $<HTMLInputElement>("#login-password").value,
    );
    $<HTMLInputElement>("#login-password").value = "";
    $<HTMLDialogElement>("#login-dialog").close();
    await connectedUI();
  } catch (error) {
    $("#login-error").textContent =
      error instanceof Error ? error.message : "Sign-in failed.";
  } finally {
    submit.disabled = false;
  }
});
async function start(): Promise<void> {
  if (demoMode) {
    await connectedUI();
    return;
  }
  try {
    await service.connect();
    await connectedUI();
  } catch (error) {
    showLogin();
    if (!(error instanceof UnauthorizedError))
      $("#login-error").textContent =
        error instanceof Error ? error.message : "Backend unavailable.";
  }
}
setInterval(() => {
  if (!connected || document.hidden || refreshPending) return;
  refreshPending = true;
  void service
    .refresh()
    .then(loadSelected)
    .catch((error) => {
      if (!(error instanceof UnauthorizedError)) {
        $("#connection-error").hidden = false;
        $("#connection-error").textContent =
          error instanceof Error ? error.message : "Connection lost.";
      }
    })
    .finally(() => {
      refreshPending = false;
    });
}, 5000);
$("#domain-form").addEventListener("submit", async (event) => {
  event.preventDefault();
  if (!domainInstanceId) return;
  const button = $<HTMLButtonElement>("#domain-submit");
  button.disabled = true;
  $("#domain-form-error").textContent = "";
  try {
    await service.addDomain(
      domainInstanceId,
      $<HTMLInputElement>("#domain-hostname").value.trim(),
    );
    $<HTMLDialogElement>("#domain-dialog").close();
    toast("Domain added. Follow the DNS verification instructions.");
  } catch (error) {
    $("#domain-form-error").textContent =
      error instanceof Error ? error.message : "Could not add domain.";
  } finally {
    button.disabled = false;
  }
});
$("#site-form").addEventListener("submit", async (event) => {
  event.preventDefault();
  const id = siteInstanceId;
  const file = $<HTMLInputElement>("#site-file").files?.[0];
  if (!id || !file) return;
  $("#site-form-error").textContent = "";
  if (
    !/\.(zip|html?)$/i.test(file.name) ||
    !file.size ||
    file.size > 50 * 1024 * 1024
  ) {
    $("#site-form-error").textContent =
      "Choose a ZIP or HTML file between 1 byte and 50 MiB.";
    return;
  }
  const button = $<HTMLButtonElement>("#site-submit");
  button.disabled = true;
  button.innerHTML = '<span class="spinner"></span> Publishing…';
  $("#site-upload-status").textContent =
    "Uploading, checking files, and publishing your site…";
  pending.add(id);
  render();
  try {
    await service.uploadSite(id, file);
    $<HTMLDialogElement>("#site-dialog").close();
    toast("Static site published.");
  } catch (error) {
    $("#site-form-error").textContent =
      error instanceof Error ? error.message : "Could not publish site.";
  } finally {
    pending.delete(id);
    button.disabled = false;
    button.textContent = "Publish site";
    $("#site-upload-status").textContent = "";
    render();
  }
});
document.addEventListener("submit", async (event) => {
  const form = event.target;
  if (!(form instanceof HTMLFormElement) || form.id !== "resources-form")
    return;
  event.preventDefault();
  const id = form.dataset.id;
  if (!id) return;
  const memoryLimitMiB = $<HTMLInputElement>("#instance-ram").valueAsNumber;
  pending.add(id);
  render();
  try {
    await service.updateResources(id, memoryLimitMiB);
    toast(`RAM limit set to ${memoryLimitMiB} MiB.`);
  } catch (error) {
    const message =
      error instanceof Error ? error.message : "Could not update RAM.";
    toast(message, true);
  } finally {
    pending.delete(id);
    render();
  }
});
$("#agent-token-form").addEventListener("submit", async (event) => {
  event.preventDefault();
  if (!liveService) return;
  const button = $<HTMLButtonElement>("#agent-token-submit");
  button.disabled = true;
  $("#agent-access-error").textContent = "";
  try {
    const instanceId = $<HTMLSelectElement>("#agent-instance").value;
    const result = await liveService.request<{
      token: string;
      endpoint: string;
    }>("/agent-tokens", {
      method: "POST",
      body: JSON.stringify({
        name: $<HTMLInputElement>("#agent-name").value.trim(),
        instanceIds: instanceId ? [instanceId] : [],
      }),
    });
    $("#agent-new-token").hidden = false;
    $("#agent-secret").textContent = result.token;
    $("#agent-cursor-config").textContent = JSON.stringify(
      {
        mcpServers: {
          pbmanager: {
            url: result.endpoint,
            headers: { Authorization: `Bearer ${result.token}` },
          },
        },
      },
      null,
      2,
    );
    $("#agent-codex-config").textContent =
      `[mcp_servers.pbmanager]\nurl = "${result.endpoint}"\nbearer_token_env_var = "PBMANAGER_MCP_TOKEN"`;
    await loadAgents();
  } catch (error) {
    $("#agent-access-error").textContent =
      error instanceof Error ? error.message : "Could not create agent token.";
  } finally {
    button.disabled = false;
  }
});
$("#agents-dialog").addEventListener("close", () => {
  $("#agent-new-token").hidden = true;
  $("#agent-secret").textContent = "";
  $("#agent-cursor-config").textContent = "";
  $("#agent-codex-config").textContent = "";
});
$("#sftp-form").addEventListener("submit", async (event) => {
  event.preventDefault();
  const id = sftpInstanceId;
  if (!id) return;
  const username = $<HTMLInputElement>("#sftp-username").value.trim();
  const password = $<HTMLInputElement>("#sftp-password").value;
  const button = $<HTMLButtonElement>("#sftp-submit");
  button.disabled = true;
  button.textContent = "Saving…";
  $("#sftp-error").textContent = "";
  pending.add(id);
  render();
  try {
    await service.createSFTPUser(id, username, password);
    $<HTMLDialogElement>("#sftp-dialog").close();
    toast(`File account ${username} is ready.`);
  } catch (error) {
    $("#sftp-error").textContent =
      error instanceof Error ? error.message : "Could not save file account.";
  } finally {
    button.disabled = false;
    button.textContent = "Save file account";
    pending.delete(id);
    render();
  }
});
$("#sftp-dialog").addEventListener("close", () => {
  $<HTMLInputElement>("#sftp-password").value = "";
});
void start();
