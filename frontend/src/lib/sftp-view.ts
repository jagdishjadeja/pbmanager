import type { Instance } from "./types";
import { escapeHTML as esc } from "./format";

export function renderSFTP(
  instance: Instance,
  pending: boolean,
  ip: string,
): string {
  const disabled =
    pending || instance.status === "provisioning" ? "disabled" : "";
  return `<section class="panel sftp-panel"><div class="panel-top"><div><h2>SSH file access</h2><p class="section-description">Password-protected SFTP accounts for this instance’s website folder.</p></div><button class="button button-secondary" data-action="add-sftp" data-id="${esc(instance.id)}" ${disabled}>Add file account</button></div><div class="site-content">${instance.sftpUsers?.length ? instance.sftpUsers.map((user) => `<div class="agent-list-row"><div><strong>${esc(user.username)}</strong><p><code>sftp ${esc(user.username)}@${esc(ip)}</code> · folder <code>/current</code></p></div><div class="sftp-actions"><button class="text-button" data-action="reset-sftp" data-id="${esc(instance.id)}" data-username="${esc(user.username)}" ${disabled}>Reset password</button><button class="text-button danger-text" data-action="remove-sftp" data-id="${esc(instance.id)}" data-username="${esc(user.username)}" ${disabled}>Remove</button></div></div>`).join("") : '<p class="section-description">No file accounts yet. Add a username and password to upload and edit files using an SFTP client.</p>'}<p class="field-hint">SFTP uses SSH on port 22. Accounts can edit only this website’s files; terminal commands, software installation, forwarding, and database files are unavailable. Use the server IP because proxied domains may not accept SSH connections.</p></div></section>`;
}
