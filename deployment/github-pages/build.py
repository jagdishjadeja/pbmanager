#!/usr/bin/env python3
"""Build the interactive demo as one HTML file for GitHub Pages."""

import base64
import os
from pathlib import Path
import re
import subprocess


ROOT = Path(__file__).resolve().parents[2]
DIST = ROOT / "frontend" / "dist"
DEMO_URL = "https://jagdishjadeja.github.io/pbmanager/"


def asset(url: str) -> str:
    path = (DIST / url.lstrip("/")).resolve()
    if not path.is_relative_to(DIST.resolve()):
        raise ValueError(f"Asset is outside the build directory: {url}")
    return path.read_text()


def main() -> None:
    environment = dict(os.environ, PUBLIC_DEMO_MODE="true")
    subprocess.run(
        ["pnpm", "--dir", str(ROOT / "frontend"), "build"],
        env=environment,
        check=True,
    )
    html = (DIST / "index.html").read_text()
    if 'data-mode="demo"' not in html:
        raise RuntimeError("Refusing to publish a build without demo mode enabled.")

    html = re.sub(
        r'<link rel="stylesheet" href="([^"]+)"[^>]*>',
        lambda match: "<style>" + asset(match.group(1)) + "</style>",
        html,
    )
    html = re.sub(
        r'<script type="module" src="([^"]+)"[^>]*></script>',
        lambda match: '<script type="module">'
        + re.sub(r"</script", r"<\\/script", asset(match.group(1)), flags=re.I)
        + "</script>",
        html,
    )
    if re.search(r'<script[^>]+\bsrc=|<link[^>]+\brel="stylesheet"', html):
        raise RuntimeError("The demo still references a separate script or stylesheet.")

    favicon = base64.b64encode((DIST / "favicon.svg").read_bytes()).decode()
    html = html.replace('href="/favicon.svg"', f'href="data:image/svg+xml;base64,{favicon}"')
    html = html.replace(
        "<title>Overview · PocketBase Manager</title>",
        "<title>PocketBase Manager · Interactive Demo</title>",
    )
    description = "Try PocketBase Manager: an interactive dashboard demo with browser-local sample data."
    html = html.replace(
        "An internal dashboard for your PocketBase instances. Provision, monitor, and manage from one place.",
        description,
    )
    preview = (
        '<meta property="og:type" content="website">'
        '<meta property="og:title" content="PocketBase Manager · Interactive Demo">'
        f'<meta property="og:description" content="{description}">'
        f'<meta property="og:url" content="{DEMO_URL}">'
        f'<meta property="og:image" content="{DEMO_URL}images/pocketbase-manager-dashboard.png">'
    )
    html = html.replace("</head>", preview + "</head>", 1)

    destination = ROOT / "docs"
    destination.mkdir(exist_ok=True)
    (destination / "index.html").write_text(html)
    (destination / ".nojekyll").write_text("Static PocketBase Manager demo.\n")
    print(f"GitHub Pages demo: {destination / 'index.html'}")


if __name__ == "__main__":
    main()
