import { test } from "node:test";
import assert from "node:assert/strict";
import { DemoManagerService } from "../src/lib/demo-service";
import { domainLabel, renderDomains } from "../src/lib/domain-view";

test("default address is retained while customer verification and HTTPS progress", async () => {
  const service = new DemoManagerService({ delay: 0 });
  const instance = await service.create(
    "customer-demo",
    undefined,
    false,
    "api.customer.example",
  );
  assert.equal(instance.domains?.length, 2);
  assert.equal(instance.domains?.[0].hostname, "customer-demo.opb.com");
  assert.equal(instance.domains?.[1].stage, "verification_pending");
  assert.equal(domainLabel(instance.domains?.[1]), "Verify ownership");
  await assert.rejects(
    service.removeDomain(instance.id, instance.hostname),
    /permanent/,
  );
  await service.verifyDomain(instance.id, "api.customer.example");
  let current = service
    .getSnapshot()
    .instances.find((item) => item.id === instance.id)!;
  assert.equal(current.domains?.[1].certificateStatus, "active");
  await service.removeDomain(instance.id, "api.customer.example");
  current = service
    .getSnapshot()
    .instances.find((item) => item.id === instance.id)!;
  assert.equal(current.domains?.length, 1);
  assert.equal(current.domains?.[0].hostname, instance.hostname);
  service.reset();
  assert.ok(
    service
      .getSnapshot()
      .instances.every((item) => item.domains?.[0].kind === "default"),
  );
});

test("domain UI displays verification records, in-progress certificates, and escaped errors", async () => {
  const service = new DemoManagerService({ delay: 0 });
  const instance = await service.create(
    "ui-domains",
    undefined,
    false,
    "api.customer.example",
  );
  const pending = renderDomains(instance, "46.62.175.23", false, false);
  assert.match(pending, /_pbmanager.api.customer.example/);
  assert.match(pending, /Default domain/);
  const domain = instance.domains![1];
  domain.verified = true;
  domain.dnsReady = true;
  domain.routeConfigured = true;
  domain.stage = "certificate_pending";
  domain.certificateStatus = "issuing";
  domain.lastError = "<img src=x onerror=alert(1)>";
  const rendering = renderDomains(instance, "46.62.175.23", false, false);
  assert.match(rendering, /Certificate in progress/);
  assert.ok(!rendering.includes("<img"));
  assert.match(rendering, /&lt;img/);
});
