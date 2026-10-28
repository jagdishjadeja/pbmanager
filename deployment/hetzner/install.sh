#!/usr/bin/env bash
set -euo pipefail

if [[ "$(id -u)" != 0 ]]; then echo "Run as root." >&2; exit 1; fi
archive="${1:?Usage: install.sh /path/to/pbmanager-release.tar.gz}"
for dependency in systemctl caddy curl unzip python3 fallocate mkfs.ext4; do command -v "$dependency" >/dev/null || { echo "Missing dependency: $dependency" >&2; exit 1; }; done
id pbmanager >/dev/null 2>&1 || useradd --system --no-create-home --shell /usr/sbin/nologin pbmanager
install -d -m 0755 /opt/pbmanager/releases /opt/pbmanager/bin /var/lib/pbmanager
install -d -o pbmanager -g pbmanager -m 0700 /var/lib/pbmanager/control
install -d -m 0711 /var/lib/pbmanager/instances
install -d -m 0700 /var/lib/pbmanager-agent
install -d -m 0750 -g pbmanager /etc/pbmanager
install -d -o caddy -g caddy -m 0750 /var/log/caddy

release="/opt/pbmanager/releases/$(date -u +%Y%m%dT%H%M%SZ)-$$"
install -d -m 0755 "$release"
tar -xzf "$archive" -C "$release"
# Archives created from mktemp directories can restore a private root mode.
chmod 0755 "$release"
chmod 0755 "$release/bin/pbmanager" "$release/bin/pbmanager-agent"

if [[ ! -f /etc/pbmanager/manager.env ]]; then
  python3 - <<'PY'
import base64, os, secrets
from pathlib import Path
values={
 'PBM_ENCRYPTION_KEY':base64.b64encode(secrets.token_bytes(32)).decode(),
 'PBM_DATA_DIR':'/var/lib/pbmanager/control/pb_data',
 'PBM_FRONTEND_DIR':'/opt/pbmanager/current/frontend',
 'PBM_AGENT_SOCKET':'/run/pbmanager-agent/agent.sock',
 'PBM_BASE_DOMAIN':os.environ.get('PBM_BASE_DOMAIN','formics.online'),
 'PBM_MANAGER_HOST':os.environ.get('PBM_MANAGER_HOST','pb.formics.online'),
 'PBM_POCKETBASE_BINARY':'/opt/pbmanager/bin/pocketbase',
 'PBM_POCKETBASE_VERSION':'0.39.0',
 'PBM_SECURE_COOKIES':'true',
}
Path('/etc/pbmanager/manager.env').write_text(''.join(f'{k}={v}\n' for k,v in values.items()))
Path('/etc/pbmanager/bootstrap.json').write_text(__import__('json').dumps({'email':'admin@formics.online','password':secrets.token_urlsafe(30)}))
os.chmod('/etc/pbmanager/bootstrap.json',0o600)
PY
  chown root:pbmanager /etc/pbmanager/manager.env
  chmod 0640 /etc/pbmanager/manager.env
fi
set -a
source /etc/pbmanager/manager.env
set +a

if [[ ! -x /opt/pbmanager/bin/pocketbase ]]; then
  download_dir="$(mktemp -d /var/tmp/pbmanager-download.XXXXXX)"
  trap 'rm -rf "$download_dir"' EXIT
  base="https://github.com/pocketbase/pocketbase/releases/download/v${PBM_POCKETBASE_VERSION}"
  filename="pocketbase_${PBM_POCKETBASE_VERSION}_linux_amd64.zip"
  curl -fLsS --retry 2 "$base/$filename" -o "$download_dir/$filename"
  curl -fLsS --retry 2 "$base/checksums.txt" -o "$download_dir/checksums.txt"
  python3 - "$download_dir" "$filename" <<'PY'
import hashlib,sys
from pathlib import Path
root=Path(sys.argv[1]); name=sys.argv[2]
expected=next((line.split()[0] for line in (root/'checksums.txt').read_text().splitlines() if line.split()[-1].lstrip('*')==name),None)
actual=hashlib.sha256((root/name).read_bytes()).hexdigest()
if not expected or actual!=expected: raise SystemExit('PocketBase release checksum verification failed')
PY
  unzip -p "$download_dir/$filename" pocketbase > /opt/pbmanager/bin/pocketbase.next
  chmod 0755 /opt/pbmanager/bin/pocketbase.next
  mv /opt/pbmanager/bin/pocketbase.next /opt/pbmanager/bin/pocketbase
fi

if [[ ! -f /var/lib/pbmanager/control/pb_data/data.db ]]; then
  "$release/bin/pbmanager" bootstrap-admin < /etc/pbmanager/bootstrap.json
  chown -R pbmanager:pbmanager /var/lib/pbmanager/control
fi

install -m 0644 "$release/deployment/pbmanager.service" /etc/systemd/system/pbmanager.service
install -m 0644 "$release/deployment/pbmanager-agent.service" /etc/systemd/system/pbmanager-agent.service
ln -s "$release" /opt/pbmanager/current.next
mv -Tf /opt/pbmanager/current.next /opt/pbmanager/current

systemctl daemon-reload
systemctl enable pbmanager-agent.service pbmanager.service
systemctl restart pbmanager-agent.service
systemctl restart pbmanager.service

for attempt in {1..30}; do
  if curl -fsS http://127.0.0.1:8095/api/health >/dev/null; then break; fi
  sleep 1
done
curl -fsS http://127.0.0.1:8095/api/health >/dev/null

python3 - <<'PY'
import os,re
from pathlib import Path
hostname=os.environ['PBM_MANAGER_HOST']
if not re.fullmatch(r'[a-z0-9.-]+',hostname): raise SystemExit('Invalid manager hostname')
Path('/etc/caddy/sites.d/pbmanager.caddy').write_text(f'''{hostname} {{
    encode zstd gzip
    header {{
        X-Content-Type-Options nosniff
        Referrer-Policy same-origin
        X-Frame-Options DENY
        -Server
    }}
    reverse_proxy 127.0.0.1:8095
}}
''')
PY
caddy validate --config /etc/caddy/Caddyfile --adapter caddyfile
systemctl reload caddy
echo "PocketBase Manager deployed at https://$PBM_MANAGER_HOST"
echo "Initial admin credentials: /etc/pbmanager/bootstrap.json (root only)"
