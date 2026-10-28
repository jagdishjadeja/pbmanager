import type { Instance } from "./types";
import { bytes, escapeHTML as esc } from "./format";

export function maximumMemoryMiB(total?: number): number {
  return total
    ? Math.min(
        32768,
        Math.max(128, Math.floor((total / 1048576 - 512) / 64) * 64),
      )
    : 32768;
}

export function renderResources(
  instance: Instance,
  pending: boolean,
  total?: number,
): string {
  const limit = (instance.memoryLimitBytes ?? 512 * 1048576) / 1048576;
  return `<section class="panel resources-panel"><div class="panel-top"><div><h2>RAM allocation</h2><p class="section-description">Set the maximum RAM this instance can use.</p></div><span class="pill pill-gray">${bytes(instance.memoryBytes)} used</span></div><form id="resources-form" class="resources-form" data-id="${esc(instance.id)}"><div><label class="field-label" for="instance-ram">RAM limit (MiB)</label><input class="resource-input" id="instance-ram" name="memoryLimitMiB" type="number" min="128" max="${maximumMemoryMiB(total)}" step="64" value="${limit}" required ${pending || instance.status === "provisioning" ? "disabled" : ""} /><p class="field-hint">128–${maximumMemoryMiB(total)} MiB, in steps of 64. RAM is used on demand; the limit persists after restart.</p></div><button class="button button-secondary" type="submit" ${pending || instance.status === "provisioning" ? "disabled" : ""}>Save RAM limit</button></form><p class="form-error resource-error" id="resources-error" role="alert"></p></section>`;
}
