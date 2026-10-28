import type { Instance } from "./types";
import { bytes, escapeHTML as esc } from "./format";
import { pocketBaseDisabledReason } from "./domain-view";

export function renderSite(instance: Instance, pending: boolean): string {
  const site = instance.staticSite;
  const disabled =
    pending || instance.status === "provisioning" ? "disabled" : "";
  const ready = !pocketBaseDisabledReason(instance);
  return `<section class="panel site-panel"><div class="panel-top"><div><h2>Static website</h2><p class="section-description">Host your frontend alongside its PocketBase backend.</p></div><button class="button button-secondary" data-action="upload-site" data-id="${esc(instance.id)}" ${disabled}>${site ? "Replace site" : "Upload site"}</button></div><div class="site-content">${site ? `<div class="site-summary"><div><strong>${esc(site.filename)}</strong><p>${site.files} files · ${bytes(site.bytes)} · Published ${esc(new Date(site.publishedAt).toLocaleString())}</p></div><span class="pill pill-green">Published</span></div>` : '<p class="section-description">Upload a ZIP of your built HTML, CSS, JavaScript, and assets, or a single HTML file.</p>'}<dl class="site-routes"><div><dt>Website</dt><dd>${site && ready ? `<a class="text-button" href="https://${esc(instance.hostname)}/" target="_blank" rel="noopener noreferrer">${esc(instance.hostname)} ↗</a>` : `<code>${esc(instance.hostname)}/</code>`}</dd></div><div><dt>PocketBase admin</dt><dd><code>/_/</code></dd></div><div><dt>Database API</dt><dd><code>/api/</code></dd></div></dl>${site && !ready ? `<p class="field-hint">${esc(pocketBaseDisabledReason(instance) ?? "")}</p>` : ""}${site ? `<button class="text-button danger-text" data-action="remove-site" data-id="${esc(instance.id)}" ${disabled}>Remove site</button>` : ""}</div></section>`;
}
