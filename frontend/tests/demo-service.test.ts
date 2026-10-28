import assert from "node:assert/strict";
import { test } from "node:test";
import {
  DemoManagerService,
  GiB,
  randomName,
  STORAGE_KEY,
  validateName,
} from "../src/lib/demo-service";

function memoryStorage() {
  const values = new Map<string, string>();
  return {
    getItem: (key: string) => values.get(key) ?? null,
    setItem: (key: string, value: string) => {
      values.set(key, value);
    },
  };
}

test("server storage stays consistent through creation and deletion", async () => {
  const manager = new DemoManagerService({ delay: 0 });
  const before = manager.getSnapshot();
  const instance = await manager.create("gronda-two");
  const after = manager.getSnapshot();
  assert.equal(after.instances.length, before.instances.length + 1);
  assert.equal(
    after.storage.instanceBytes,
    before.storage.instanceBytes + instance.storageBytes,
  );
  assert.equal(
    after.storage.usedBytes + after.storage.availableBytes,
    40 * GiB,
  );
  assert.equal(
    after.storage.instanceBytes + after.storage.otherBytes,
    after.storage.usedBytes,
  );
  assert.ok(instance.databaseBytes <= instance.storageBytes);
  await manager.remove(instance.id);
  assert.equal(
    manager.getSnapshot().storage.instanceBytes,
    before.storage.instanceBytes,
  );
});

test("blank names become six-character names; duplicate reservations are rejected", async () => {
  const manager = new DemoManagerService({ delay: 0 });
  assert.match(randomName(), /^[a-z0-9]{6}$/);
  const generated = await manager.create("   ");
  assert.match(generated.name, /^[a-z0-9]{6}$/);
  assert.equal(generated.hostname, `${generated.name}.opb.com`);
  const first = manager.create("my-project");
  await assert.rejects(manager.create("my-project"), /already in use/);
  await first;
  assert.equal(validateName("gronda"), null);
  assert.ok(validateName("../escape"));
  assert.ok(validateName("UPPERCASE"));
  assert.ok(validateName("admin"));
});

test("failed provisioning is visible and restart recovers it", async () => {
  const manager = new DemoManagerService({ delay: 0 });
  await assert.rejects(
    manager.create("failed-demo", undefined, true),
    /Simulated health check failure/,
  );
  const instance = manager
    .getSnapshot()
    .instances.find((item) => item.name === "failed-demo")!;
  assert.equal(instance.status, "failed");
  assert.ok(instance.lastError);
  await manager.control(instance.id, "restart");
  const recovered = manager
    .getSnapshot()
    .instances.find((item) => item.id === instance.id)!;
  assert.equal(recovered.status, "running");
  assert.equal(recovered.lastError, undefined);
  await manager.control(instance.id, "stop");
  assert.equal(
    manager.getSnapshot().instances.find((item) => item.id === instance.id)!
      .memoryBytes,
    0,
  );
});

test("state survives reload; corrupt data and storage failures fall back safely", async () => {
  const storage = memoryStorage();
  const manager = new DemoManagerService({ storage, delay: 0 });
  await manager.create("saved-demo");
  assert.ok(
    new DemoManagerService({ storage })
      .getSnapshot()
      .instances.some((item) => item.name === "saved-demo"),
  );
  storage.setItem(STORAGE_KEY, "{broken");
  assert.equal(
    new DemoManagerService({ storage }).getSnapshot().instances.length,
    5,
  );
  storage.setItem(
    STORAGE_KEY,
    JSON.stringify({
      version: 1,
      instances: [{}],
      activity: [],
      measuredAt: new Date().toISOString(),
    }),
  );
  assert.equal(
    new DemoManagerService({ storage }).getSnapshot().instances.length,
    5,
  );
  const unavailable = new DemoManagerService({
    storage: {
      getItem: () => {
        throw Error("blocked");
      },
      setItem: () => {
        throw Error("full");
      },
    },
    delay: 0,
  });
  await unavailable.create("memory-only");
  assert.equal(unavailable.getSnapshot().instances.length, 6);
});

test("snapshots cannot mutate service state and concurrent operations are rejected", async () => {
  const manager = new DemoManagerService({ delay: 1 });
  const snapshot = manager.getSnapshot();
  const id = snapshot.instances[0].id;
  snapshot.instances[0].name = "tampered";
  assert.notEqual(manager.getSnapshot().instances[0].name, "tampered");
  const stopping = manager.control(id, "stop");
  await assert.rejects(manager.remove(id), /current operation/);
  await assert.rejects(manager.control(id, "restart"), /already in progress/);
  await stopping;
});

test("reset cancels in-flight provisioning without restoring old state", async () => {
  const manager = new DemoManagerService({ delay: 1 });
  const creating = manager.create("reset-demo");
  manager.reset();
  await assert.rejects(creating, /Demo reset/);
  assert.equal(manager.getSnapshot().instances.length, 5);
  assert.ok(
    !manager.getSnapshot().instances.some((item) => item.name === "reset-demo"),
  );
});
