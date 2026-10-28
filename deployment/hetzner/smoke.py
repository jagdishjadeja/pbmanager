#!/usr/bin/env python3
"""Exercise a disposable real instance on the Hetzner host. No secrets are printed."""
import json
import io
import zipfile
import os
import re
import subprocess
import sqlite3
import secrets
import select
import signal
import pty
import time
import urllib.error
import urllib.request
from pathlib import Path

BASE = "http://127.0.0.1:8095/api/manager"
COOKIE = ""


def call(method, path, body=None, expected=200, origin=None):
    headers = {"Content-Type": "application/json"}
    if COOKIE:
        headers["Cookie"] = COOKIE
    if origin:
        headers["Origin"] = origin
    request = urllib.request.Request(BASE + path, data=None if body is None else json.dumps(body).encode(), headers=headers, method=method)
    try:
        response = urllib.request.urlopen(request, timeout=160)
    except urllib.error.HTTPError as error:
        response = error
    data = response.read()
    if response.code != expected:
        raise AssertionError(f"{method} {path}: HTTP {response.code}; {data.decode()[:2000]}")
    return (json.loads(data) if data else None), response


def wait(operation):
    deadline = time.monotonic() + 180
    while time.monotonic() < deadline:
        current, _ = call("GET", "/operations/" + operation["id"])
        if current["state"] == "failed":
            raise AssertionError(current["error"])
        if current["state"] == "succeeded":
            return
        time.sleep(1)
    raise AssertionError("Operation timed out")


def upload_site(instance_id, filename, content, expected=200):
    boundary = "pbmanager-static-smoke-boundary"
    body = (f'--{boundary}\r\nContent-Disposition: form-data; name="site"; filename="{filename}"\r\nContent-Type: application/octet-stream\r\n\r\n').encode() + content + f"\r\n--{boundary}--\r\n".encode()
    request = urllib.request.Request(BASE + "/instances/" + instance_id + "/site", data=body, headers={"Cookie": COOKIE, "Content-Type": "multipart/form-data; boundary=" + boundary}, method="POST")
    try:
        response = urllib.request.urlopen(request, timeout=160)
    except urllib.error.HTTPError as error:
        response = error
    data = response.read()
    assert response.code == expected, f"Site upload: {response.code}: {data.decode()[:1000]}"
    return json.loads(data) if data else None


def site_zip(files):
    buffer = io.BytesIO()
    with zipfile.ZipFile(buffer, "w", zipfile.ZIP_DEFLATED) as archive:
        for name, content in files.items():
            archive.writestr(name, content)
    return buffer.getvalue()


def memory_checks(instance):
    unit = "pbm-" + instance["id"] + ".service"
    def value(property):
        return subprocess.check_output(["systemctl", "show", unit, "--property=" + property, "--value"]).strip()
    assert int(value("MemoryMax")) == 256 * 1048576
    original_pid = value("MainPID")
    operation, _ = call("PATCH", "/instances/" + instance["id"] + "/resources", {"memoryLimitMiB":768}, expected=202)
    wait(operation)
    assert int(value("MemoryMax")) == 768 * 1048576 and value("MainPID") == original_pid
    operation, _ = call("POST", "/instances/" + instance["id"] + "/actions/stop", expected=202)
    wait(operation)
    operation, _ = call("PATCH", "/instances/" + instance["id"] + "/resources", {"memoryLimitMiB":384}, expected=202)
    wait(operation)
    state, _ = call("POST", "/refresh")
    current = next(item for item in state["instances"] if item["id"] == instance["id"])
    assert current["status"] == "stopped" and current["memoryLimitBytes"] == 384 * 1048576
    operation, _ = call("POST", "/instances/" + instance["id"] + "/actions/start", expected=202)
    wait(operation)
    assert int(value("MemoryMax")) == 384 * 1048576
    print("PASS: chosen RAM limit, live update without restart, and persistence for stopped/started instances", flush=True)


def mcp_checks(instance):
    identity, _ = call("POST", "/agent-tokens", {"name":"smoke-" + instance["name"], "instanceIds":[instance["id"]]}, expected=201)
    token = identity["token"]
    task_id = None
    def rpc(method, params, expected=200):
        request = urllib.request.Request("http://127.0.0.1:8095/mcp", data=json.dumps({"jsonrpc":"2.0", "id":1, "method":method, "params":params}).encode(), headers={"Authorization":"Bearer " + token, "Content-Type":"application/json", "Accept":"application/json, text/event-stream", "MCP-Protocol-Version":"2025-06-18"}, method="POST")
        try:
            response = urllib.request.urlopen(request, timeout=60)
        except urllib.error.HTTPError as error:
            response = error
        data = json.load(response)
        assert response.code == expected, "MCP request returned unexpected HTTP status"
        if expected != 200:
            return data
        assert "error" not in data, "MCP protocol returned an error: " + str(data.get("error"))
        return data["result"]
    def tool(name, arguments):
        result = rpc("tools/call", {"name":name,"arguments":arguments})
        assert not result.get("isError"), "MCP tool failed: " + str(result.get("content"))
        return json.loads(result["content"][0]["text"])
    try:
        initialized = rpc("initialize", {"protocolVersion":"2025-06-18", "capabilities":{}, "clientInfo":{"name":"pbmanager-smoke","version":"1"}})
        assert initialized["serverInfo"]["name"] == "PocketBase Manager"
        state, _ = call("GET", "/snapshot")
        public_endpoint = "https://" + state["server"]["managerHost"] + "/mcp"
        public_payload = json.dumps({"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"public-transport-check","version":"1"}}})
        config = "header = " + json.dumps("Authorization: Bearer " + token) + "\n"
        public_response = subprocess.run(["curl","--silent","--show-error","--max-time","20","--config","-","--header","Content-Type: application/json","--header","Accept: application/json, text/event-stream","--header","MCP-Protocol-Version: 2025-06-18","--data-raw",public_payload,"--write-out","\n%{http_code}",public_endpoint], input=config, text=True, capture_output=True, check=True)
        public_body, public_status = public_response.stdout.rsplit("\n",1)
        assert public_status == "200", "Public MCP handshake returned HTTP " + public_status
        assert json.loads(public_body)["result"]["serverInfo"]["name"] == "PocketBase Manager"
        names = {item["name"] for item in rpc("tools/list", {})["tools"]}
        assert {"list_instances","pocketbase_request","create_task","update_task","list_tasks"} <= names
        listing = tool("list_instances", {})
        assert len(listing) == 1 and listing[0]["id"] == instance["id"]
        def db(method, path, body=None):
            args = {"instanceId":instance["id"],"method":method,"path":path}
            if body is not None:
                args["body"] = body
            return tool("pocketbase_request", args)["body"]
        collection = db("POST", "/api/collections", {"name":"mcp_probe","type":"base","fields":[{"name":"title","type":"text","required":True}]})
        record = db("POST", "/api/collections/mcp_probe/records", {"title":"Created by MCP"})
        updated = db("PATCH", "/api/collections/mcp_probe/records/" + record["id"], {"title":"Updated by MCP"})
        assert updated["title"] == "Updated by MCP"
        assert db("GET", "/api/collections/mcp_probe/records")["items"][0]["id"] == record["id"]
        db("DELETE", "/api/collections/mcp_probe/records/" + record["id"])
        db("DELETE", "/api/collections/" + collection["id"])
        task = tool("create_task", {"title":"Temporary smoke task","instanceId":instance["id"],"assignee":"smoke-agent"})
        task_id = task["id"]
        assert tool("update_task", {"id":task_id,"status":"done"})["status"] == "done"
        assert any(item["id"] == task_id for item in tool("list_tasks", {"status":"done"}))
    finally:
        if task_id:
            call("DELETE", "/agent-tasks/" + task_id, expected=204)
        call("DELETE", "/agent-tokens/" + identity["agent"]["id"], expected=204)
    rpc("tools/list", {}, expected=401)
    print("PASS: MCP handshake, scoped discovery, superadmin schema/record CRUD, shared tasks, and token revocation", flush=True)


def password_session(argv, password, commands=None):
    child, descriptor = pty.fork()
    if child == 0:
        os.execvp(argv[0], argv)
    output = bytearray()
    sent_password = False
    sent_commands = False
    deadline = time.monotonic() + 35
    try:
        while time.monotonic() < deadline:
            readable, _, _ = select.select([descriptor], [], [], 0.2)
            if readable:
                try:
                    chunk = os.read(descriptor, 65536)
                except OSError:
                    break
                if not chunk:
                    break
                output.extend(chunk)
                if not sent_password and b"password:" in output.lower():
                    os.write(descriptor, (password + "\n").encode())
                    sent_password = True
                if commands and sent_password and not sent_commands and b"sftp>" in output:
                    os.write(descriptor, commands.encode())
                    sent_commands = True
            done, status = os.waitpid(child, os.WNOHANG)
            if done:
                return os.waitstatus_to_exitcode(status), bytes(output)
        else:
            os.kill(child, signal.SIGKILL)
            os.waitpid(child, 0)
            raise AssertionError("SSH/SFTP test timed out (credential output withheld)")
        _, status = os.waitpid(child, 0)
        return os.waitstatus_to_exitcode(status), bytes(output)
    finally:
        os.close(descriptor)


def sftp_checks(instance):
    username = "pbtest" + instance["name"]
    password = secrets.token_urlsafe(24)
    path = "/instances/" + instance["id"] + "/sftp-users"
    upload_site(instance["id"], "index.html", b"<!doctype html><h1>SFTP test site</h1>")
    operation, _ = call("POST", path, {"username":username,"password":password}, expected=202)
    wait(operation)
    probe = Path("/var/tmp/pbmanager-sftp-probe.txt")
    probe.write_text("SFTP upload works")
    options = ["-o", "StrictHostKeyChecking=accept-new", "-o", "UserKnownHostsFile=/var/tmp/pbmanager-sftp-known-hosts", "-o", "PreferredAuthentications=password", "-o", "NumberOfPasswordPrompts=1", "-o", "ConnectTimeout=10"]
    try:
        code, output = password_session(["sftp", *options, username + "@127.0.0.1"], password,
            "pwd\nput /var/tmp/pbmanager-sftp-probe.txt /current/probe.txt\nget /etc/passwd /var/tmp/pbmanager-forbidden-passwd\nget /pb_data/data.db /var/tmp/pbmanager-forbidden-db\nln -s /etc/passwd /current/escape\nln /current/probe.txt /current/hardlink\nput /var/tmp/pbmanager-sftp-probe.txt /forbidden.txt\nbye\n")
        assert code == 0 and b"Uploading" in output, "SFTP password login or upload failed (credential output withheld)"
        assert not Path("/var/tmp/pbmanager-forbidden-passwd").exists() and not Path("/var/tmp/pbmanager-forbidden-db").exists()
        root = Path("/var/lib/pbmanager/instances") / instance["id"]
        assert (root / "pb_public/probe.txt").read_text() == "SFTP upload works"
        assert not (root / "pb_public/escape").is_symlink() and not (root / "pb_public/hardlink").exists()
        assert not (Path("/var/lib/pbmanager/sftp") / username / "forbidden.txt").exists()
        code, output = password_session(["ssh", "-T", *options, username + "@127.0.0.1", "id"], password)
        assert code != 0 and b"uid=" not in output, "A file account obtained shell access"
        new_password = secrets.token_urlsafe(24)
        operation, _ = call("POST", path, {"username":username,"password":new_password}, expected=202)
        wait(operation)
        code, _ = password_session(["sftp", *options, username + "@127.0.0.1"], password, "bye\n")
        assert code != 0, "Old file password still works"
        code, _ = password_session(["sftp", *options, username + "@127.0.0.1"], new_password, "bye\n")
        assert code == 0, "New file password failed"
        upload_site(instance["id"], "replacement.html", b"<!doctype html><h1>ZIP and SFTP share the current website</h1>")
        code, _ = password_session(["sftp", *options, username + "@127.0.0.1"], new_password, "put /var/tmp/pbmanager-sftp-probe.txt /current/after-replace.txt\nbye\n")
        assert code == 0 and (root / "pb_public/after-replace.txt").exists(), "SFTP did not follow the new website release"
    finally:
        operation, _ = call("DELETE", path + "/" + username, expected=202)
        wait(operation)
        probe.unlink(missing_ok=True)
    assert subprocess.run(["id", username], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL).returncode != 0
    assert not (Path("/var/lib/pbmanager/sftp") / username).exists()
    print("PASS: scoped SFTP password upload, blocked shell/database/system paths and links, password reset, ZIP compatibility, and account removal", flush=True)


def static_site_checks(instance):
    instance_id = instance["id"]
    unit = "pbm-" + instance_id + ".service"
    hostname = instance["hostname"]
    def fetch(path):
        return subprocess.check_output(["curl", "--fail", "--silent", "--show-error", "--max-time", "20", "--resolve", hostname + ":443:127.0.0.1", "https://" + hostname + path])
    def pid():
        return subprocess.check_output(["systemctl", "show", unit, "--property=MainPID", "--value"]).strip()
    original_pid = pid()
    sentinel = Path("/var/lib/pbmanager/instances") / instance_id / "pb_data" / "static-isolation.txt"
    sentinel.write_text("database directory must remain untouched")
    site = upload_site(instance_id, "build.zip", site_zip({"dist/index.html":"<!doctype html><h1>PB Manager static smoke</h1>", "dist/assets/app.js":"console.log('static-site-ok')"}))
    assert site["files"] == 2 and site["publishedAt"]
    assert pid() == original_pid, "A site upload restarted a correctly configured instance"
    assert b"PB Manager static smoke" in fetch("/")
    assert b"static-site-ok" in fetch("/assets/app.js")
    assert json.loads(fetch("/api/health"))["code"] == 200
    assert b"<html" in fetch("/_/").lower()
    upload_site(instance_id, "unsafe.zip", site_zip({"index.html":"bad", "../pb_data/static-isolation.txt":"overwritten"}), expected=422)
    upload_site(instance_id, "reserved.zip", site_zip({"index.html":"bad", "api/health":"overwritten"}), expected=422)
    assert b"PB Manager static smoke" in fetch("/")
    assert sentinel.read_text() == "database directory must remain untouched"
    state, _ = call("POST", "/refresh")
    measured = next(item for item in state["instances"] if item["id"] == instance_id)
    assert measured["staticSite"]["files"] == 2 and measured["storageBytes"] > 0
    connection = sqlite3.connect("file:/var/lib/pbmanager/control/pb_data/data.db?mode=ro", uri=True)
    stored = json.loads(connection.execute("SELECT data FROM manager_instances WHERE id=?", (instance_id,)).fetchone()[0])
    connection.close()
    assert stored["staticSite"]["filename"] == "build.zip"
    # Exercise the one-time configuration upgrade using only this disposable service.
    service_path = Path("/etc/systemd/system") / unit
    contents = service_path.read_text()
    public_flag = " --publicDir=" + str(sentinel.parent.parent / "pb_public")
    assert public_flag in contents
    service_path.write_text(contents.replace(public_flag, ""))
    subprocess.run(["systemctl", "daemon-reload"], check=True)
    subprocess.run(["systemctl", "restart", unit], check=True)
    legacy_pid = pid()
    upload_site(instance_id, "landing.html", b"<!doctype html><h1>Replacement HTML site</h1>")
    assert public_flag in service_path.read_text() and pid() != legacy_pid
    assert b"Replacement HTML site" in fetch("/")
    assert json.loads(fetch("/api/health"))["code"] == 200
    call("DELETE", "/instances/" + instance_id + "/site", expected=204)
    state, _ = call("POST", "/refresh")
    measured = next(item for item in state["instances"] if item["id"] == instance_id)
    assert not measured.get("staticSite") and measured["status"] == "running"
    assert sentinel.read_text() == "database directory must remain untouched"
    assert json.loads(fetch("/api/health"))["code"] == 200 and b"<html" in fetch("/_/").lower()
    print("PASS: static ZIP/HTML hosting, atomic replacement, rejected unsafe archives, legacy service upgrade, metadata persistence, and database isolation", flush=True)


def main():
    global COOKIE
    call("GET", "/snapshot", expected=401)
    credentials = json.loads(Path("/etc/pbmanager/bootstrap.json").read_text())
    _, response = call("POST", "/session", credentials)
    COOKIE = response.headers["Set-Cookie"].split(";", 1)[0]
    assert "HttpOnly" in response.headers["Set-Cookie"] and "Secure" in response.headers["Set-Cookie"]
    call("POST", "/instances", {"name": "forbidden"}, expected=403, origin="https://evil.invalid")
    before, _ = call("POST", "/refresh")
    assert before["storage"]["totalBytes"] > 0
    assert before["server"]["memoryTotalBytes"] > 0
    result, _ = call("POST", "/instances", {"name": "", "memoryLimitMiB":256}, expected=202)
    instance = result["instance"]
    Path("/var/tmp/pbmanager-smoke-instance.json").write_text(json.dumps(instance))
    print("Created temporary instance:", instance["name"], flush=True)
    try:
        assert re.fullmatch(r"[a-z0-9]{6}", instance["name"])
        call("POST", "/instances", {"name": instance["name"]}, expected=409)
        wait(result["operation"])
        unit = "pbm-" + instance["id"] + ".service"
        subprocess.run(["systemctl", "is-active", "--quiet", unit], check=True)
        data_root = Path("/var/lib/pbmanager/instances") / instance["id"]
        subprocess.run(["mountpoint", "-q", str(data_root)], check=True)
        filesystem = os.statvfs(data_root)
        assert filesystem.f_blocks * filesystem.f_frsize <= 1 << 30
        route = Path("/etc/caddy/sites.d") / ("pbmanager-" + instance["id"] + ".caddy")
        assert instance["hostname"] in route.read_text()
        deadline = time.monotonic() + 180
        while time.monotonic() < deadline:
            state, _ = call("POST", "/refresh")
            measured = next(item for item in state["instances"] if item["id"] == instance["id"])
            default = next(domain for domain in measured["domains"] if domain["kind"] == "default")
            if default["certificateStatus"] == "active":
                assert default["stage"] == "ready" and default["certificateExpiresAt"]
                break
            time.sleep(3)
        else:
            raise AssertionError("Default HTTPS certificate did not become active: " + json.dumps(default))
        connection = sqlite3.connect("file:/var/lib/pbmanager/control/pb_data/data.db?mode=ro", uri=True)
        stored = json.loads(connection.execute("SELECT data FROM manager_instances WHERE id=?", (instance["id"],)).fetchone()[0])
        connection.close()
        assert stored["domains"][0]["certificateStatus"] == "active" and stored["provisioningStage"] == "ready"
        print("PASS: real HTTPS certificate issued and completion persisted in PocketBase", flush=True)
        resolve = instance["hostname"] + ":443:127.0.0.1"
        subprocess.run(["curl", "--fail", "--silent", "--show-error", "--max-time", "20", "--resolve", resolve, "https://" + instance["hostname"] + "/api/health"], check=True, stdout=subprocess.DEVNULL)
        subprocess.run(["curl", "--silent", "--show-error", "--max-time", "20", "--resolve", resolve, "--header", "Content-Type: application/json", "--data", json.dumps({"identity":"traffic-check@example.invalid", "password":"x" * 2048}), "https://" + instance["hostname"] + "/api/collections/_superusers/auth-with-password"], check=True, stdout=subprocess.DEVNULL)
        for _ in range(10):
            state, _ = call("POST", "/refresh")
            measured = next(item for item in state["instances"] if item["id"] == instance["id"])
            if measured["requests"] >= 2 and measured["ingressBytes"] > 0 and measured["egressBytes"] > 0:
                break
            time.sleep(1)
        else:
            raise AssertionError("HTTP traffic counters did not update")
        custom = "customer-" + instance["name"] + ".example.com"
        operation, _ = call("POST", "/instances/" + instance["id"] + "/domains", {"hostname":custom}, expected=202)
        wait(operation)
        state, _ = call("POST", "/refresh")
        measured = next(item for item in state["instances"] if item["id"] == instance["id"])
        domain = next(domain for domain in measured["domains"] if domain["hostname"] == custom)
        assert domain["stage"] == "verification_pending" and domain["verificationToken"] and not domain["routeConfigured"]
        assert custom not in route.read_text()
        call("DELETE", "/instances/" + instance["id"] + "/domains/" + instance["hostname"], expected=400)
        operation, _ = call("DELETE", "/instances/" + instance["id"] + "/domains/" + custom, expected=202)
        wait(operation)
        assert instance["hostname"] in route.read_text()
        print("PASS: customer ownership gate, default domain preservation, and real HTTP traffic accounting", flush=True)
        static_site_checks(instance)
        memory_checks(instance)
        mcp_checks(instance)
        sftp_checks(instance)
        login, _ = call("POST", "/instances/" + instance["id"] + "/credentials")
        login_again, _ = call("POST", "/instances/" + instance["id"] + "/credentials")
        assert login_again == login, "Initial superadmin credentials changed after viewing"
        request = urllib.request.Request(f"http://127.0.0.1:{instance['port']}/api/collections/_superusers/auth-with-password", data=json.dumps({"identity": login["email"], "password": login["password"]}).encode(), headers={"Content-Type": "application/json"})
        with urllib.request.urlopen(request) as response:
            assert json.load(response)["token"]
        state, _ = call("POST", "/refresh")
        measured = next(item for item in state["instances"] if item["id"] == instance["id"])
        assert measured["databaseBytes"] > 0 and measured["storageBytes"] > 0
        assert measured["status"] == "running"
        assert measured["memoryBytes"] > 0
        logs, _ = call("GET", "/instances/" + instance["id"] + "/logs")
        assert logs
        for action, expected in [("stop", "stopped"), ("start", "running"), ("restart", "running")]:
            operation, _ = call("POST", "/instances/" + instance["id"] + "/actions/" + action, expected=202)
            wait(operation)
            state, _ = call("POST", "/refresh")
            assert next(item for item in state["instances"] if item["id"] == instance["id"])["status"] == expected
        print("PASS: systemd lifecycle, 1 GiB volume, initial login, Caddy route, real metrics, and logs", flush=True)
    finally:
        operation, _ = call("DELETE", "/instances/" + instance["id"], expected=202)
        wait(operation)
        state, _ = call("POST", "/refresh")
        assert all(item["id"] != instance["id"] for item in state["instances"])
        assert not (Path("/var/lib/pbmanager-agent/volumes") / (instance["id"] + ".img")).exists()
        Path("/var/tmp/pbmanager-smoke-instance.json").unlink(missing_ok=True)
        print("PASS: temporary instance and volume cleaned up", flush=True)
    with urllib.request.urlopen("http://127.0.0.1:8090/api/health") as response:
        assert response.status == 200
    print("PASS: existing PocketBase app remains healthy", flush=True)


if __name__ == "__main__":
    main()
