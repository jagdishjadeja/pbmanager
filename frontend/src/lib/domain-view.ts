import type { Domain, Instance } from "./types";
import { escapeHTML as esc } from "./format";

const labels: Record<string, string> = {
  verification_pending: "Verify ownership",
  dns_pending: "Waiting for DNS",
  proxy_configuring: "Configuring proxy",
  certificate_pending: "Certificate in progress",
  retrying: "Certificate retry scheduled",
  ready: "HTTPS ready",
  failed: "Configuration failed",
};
export function domainLabel(domain?: Domain): string {
  return domain
    ? (labels[domain.stage] ?? "Checking domain")
    : "Checking HTTPS";
}

export function pocketBaseDisabledReason(
  instance: Instance,
): string | undefined {
  if (instance.status === "provisioning")
    return instance.provisioningMessage ?? "The instance is being provisioned.";
  if (instance.status === "stopped")
    return "Start this instance to open PocketBase.";
  if (instance.status === "failed")
    return (
      instance.lastError ??
      "The instance needs attention before PocketBase can open."
    );
  const domain = instance.domains?.find(
    (item) => item.kind === "default" && item.hostname === instance.hostname,
  );
  if (domain?.stage !== "ready" || domain.certificateStatus !== "active")
    return `${domain?.stage === "ready" ? "Waiting for HTTPS certificate" : domainLabel(domain)}. Open PocketBase will be available when HTTPS is ready.`;
  return undefined;
}

export function renderPocketBaseButton(
  instance: Instance,
  icon: string,
  compact = false,
): string {
  const reason = pocketBaseDisabledReason(instance);
  const title = reason ?? "Open PocketBase dashboard";
  return `<span class="pocketbase-action" title="${esc(title)}"${reason ? ` tabindex="0" role="group" aria-label="${esc(title)}"` : ""}><button class="${compact ? "icon-button" : "button button-secondary"}" data-action="open-pb" data-id="${esc(instance.id)}" aria-label="Open PocketBase for ${esc(instance.name)}"${reason ? " disabled" : ""}>${icon}${compact ? "" : "Open PocketBase"}</button></span>`;
}

export function websiteBadge(instance: Instance): string {
  const domain = instance.domains?.find((item) => item.kind === "default");
  if (!domain) return "";
  const color =
    domain.stage === "ready"
      ? "pill-green"
      : domain.stage === "failed"
        ? "pill-red"
        : "pill-amber";
  return `<span class="pill ${color}" title="${esc(instance.provisioningMessage ?? domainLabel(domain))}"><span class="status-dot"></span>${esc(domainLabel(domain))}</span>`;
}

export function renderDomains(
  instance: Instance,
  ip: string,
  pending: boolean,
  demo: boolean,
): string {
  const domains = instance.domains ?? [];
  const disabled = pending ? "disabled" : "";
  const rows = domains
    .map((domain) => {
      const active =
        domain.stage === "ready" && domain.certificateStatus === "active";
      const steps = [
        ...(domain.kind === "custom"
          ? [{ label: "Ownership", done: domain.verified }]
          : []),
        { label: "DNS", done: domain.dnsReady },
        { label: "Proxy", done: domain.routeConfigured },
        {
          label: "HTTPS certificate",
          done: domain.certificateStatus === "active",
        },
      ];
      const firstPending = steps.findIndex((step) => !step.done);
      const records =
        domain.kind === "custom" && !domain.verified
          ? `<tr><td>TXT</td><td><code>_pbmanager.${esc(domain.hostname)}</code></td><td><code>${esc(domain.verificationToken ?? "")}</code></td></tr>`
          : "";
      return `<article class="domain-card"><div class="domain-card-top"><div><strong>${active && instance.status === "running" ? `<a href="https://${esc(domain.hostname)}/_/" target="_blank" rel="noopener noreferrer">${esc(domain.hostname)} ↗</a>` : esc(domain.hostname)}</strong><span class="domain-kind">${domain.kind === "default" ? "Default domain · always included" : "Customer domain"}</span></div><span class="pill ${active ? "pill-green" : domain.stage === "failed" ? "pill-red" : "pill-amber"}">${domain.certificateStatus === "issuing" ? '<span class="spinner"></span>' : '<span class="status-dot"></span>'}${esc(domainLabel(domain))}</span></div><ol class="domain-steps">${steps.map((step, index) => `<li class="${step.done ? "done" : index === firstPending ? "current" : ""}"><span class="step-circle">${step.done ? "✓" : index + 1}</span>${step.label}</li>`).join("")}</ol>${!domain.dnsReady || records ? `<div class="domain-dns"><p>Add these DNS records at your domain provider, then check DNS. Use DNS-only while setting up HTTPS.</p><div class="table-scroll"><table><thead><tr><th>Type</th><th>Name</th><th>Value</th></tr></thead><tbody><tr><td>A</td><td><code>${esc(domain.hostname)}</code></td><td><code>${esc(ip)}</code></td></tr>${records}</tbody></table></div></div>` : ""}${domain.certificateStatus === "active" ? `<p class="domain-certificate">Certificate active${domain.certificateIssuer ? ` · ${esc(domain.certificateIssuer)}` : ""}${domain.certificateExpiresAt ? ` · expires ${new Date(domain.certificateExpiresAt).toLocaleDateString()}` : ""}</p>` : domain.verified && domain.dnsReady ? '<p class="domain-certificate">Caddy is requesting and installing the HTTPS certificate. This page updates automatically.</p>' : ""}${domain.lastError ? `<p class="domain-error">${esc(domain.lastError)}${domain.nextRetryAt ? `<br />Next retry: ${esc(new Date(domain.nextRetryAt).toLocaleString())}` : ""}</p>` : ""}<div class="domain-card-actions"><button class="text-button" data-action="verify-domain" data-id="${esc(instance.id)}" data-hostname="${esc(domain.hostname)}" ${disabled}>${active ? "Recheck status" : "Check DNS & HTTPS"}</button>${domain.kind === "custom" ? `<button class="text-button danger-text" data-action="remove-domain" data-id="${esc(instance.id)}" data-hostname="${esc(domain.hostname)}" ${disabled}>Remove domain</button>` : ""}</div></article>`;
    })
    .join("");
  return `<section class="panel domains-panel"><div class="panel-top"><div><h2>Domains & HTTPS</h2><p class="section-description">Your default domain stays available when customer domains are connected.</p></div><button class="button button-secondary" data-action="add-domain" data-id="${esc(instance.id)}" ${disabled}>+ Add domain</button></div>${instance.status === "provisioning" ? `<div class="domain-operation"><span class="spinner"></span>${esc(instance.provisioningMessage ?? "Provisioning instance…")}</div>` : ""}<div class="domain-list">${rows || '<p class="section-description">Checking domain configuration…</p>'}</div><p class="domain-panel-note">${demo ? "Simulated DNS and certificate statuses." : "Statuses are saved in PocketBase and refreshed automatically."}</p></section>`;
}
