# Product

<!-- impeccable:product-schema 1 -->

## Platform

web

## Users

Developers building applications with PocketBase. The public page introduces managed hosting. The private management dashboard is operated by an administrator.

## Product Purpose

PBManager presents managed PocketBase hosting and provides a control plane for provisioning and operating independent PocketBase instances.

## Positioning

Managed hosting built around PocketBase, with instance management, domain and HTTPS progress, service controls, logs, and resource visibility in one dashboard. The user confirmed managed hosting as the public positioning.

## Operating Context

The manager uses Go and PocketBase, Caddy for HTTPS and routing, systemd for instance services, and an Astro frontend served from the manager. GitHub Pages hosts the public landing page and a separate interactive dashboard demo.

## Capabilities and Constraints

- Provision dedicated Linux accounts, bounded storage volumes, RAM limits, systemd services, and Caddy routes.
- Start, stop, restart, inspect logs, and monitor storage, CPU, memory, and traffic.
- Attach custom domains and display DNS, ownership verification, and HTTPS progress.
- Publish static websites alongside PocketBase.
- Public dashboard demo uses browser-local sample data and does not provision real hosting.
- The application currently has administrator authentication, not customer signup or billing.
- Hosting prices, service availability commitments, customer numbers, and support response commitments are undecided. Do not invent them.
- Hosting requests can link to the project's GitHub issue form. Do not collect email addresses in a nonfunctional form.

## Brand Commitments

PBManager / PocketBase Manager. The user requested Impeccable design for the public landing page and administrator interface, with deployment to the existing GitHub Pages site.

## Evidence on Hand

- The running dashboard, repository documentation, and browser-local interactive demo.
- `docs/images/pocketbase-manager-dashboard.png`, a user-provided screenshot already published in the repository.
- No supplied testimonials, customer logos, hosting prices, or performance benchmarks.

## Product Principles

- Keep the hosting offer clear and PocketBase-specific.
- Make operational state readable and actionable.
- Preserve existing instance operations and administrator workflows.
- Distinguish demonstration data from live infrastructure.
