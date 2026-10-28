import type {
  Domain,
  Activity,
  Instance,
  ManagerService,
  ProvisionProgress,
  ServiceAction,
  Snapshot,
  UsageSample,
} from "./types";

export const GiB = 1024 ** 3;
export const MiB = 1024 ** 2;
export const BASE_DOMAIN = "opb.com";
export const STORAGE_KEY = "pbmanager.demo.v1";
export const PROVISION_STEPS = [
  "Preparing instance storage",
  "Initializing PocketBase",
  "Starting the systemd service",
  "Configuring the default domain",
  "Checking DNS and HTTPS certificate",
];
const TOTAL_BYTES = 40 * GiB;
const OTHER_BYTES = Math.round(13.42 * GiB);
const RESERVED = new Set([
  "www",
  "api",
  "admin",
  "manager",
  "mail",
  "localhost",
]);

interface PersistedState {
  version: 1;
  instances: Instance[];
  activity: Activity[];
  measuredAt: string;
}
interface StorageLike {
  getItem(key: string): string | null;
  setItem(key: string, value: string): void;
}
interface DemoOptions {
  storage?: StorageLike;
  delay?: number;
  now?: () => Date;
}

export function validateName(name: string): string | null {
  if (!/^[a-z0-9][a-z0-9-]{1,30}[a-z0-9]$/.test(name))
    return "Use 3–32 lowercase letters, numbers, or hyphens. Start and end with a letter or number.";
  if (RESERVED.has(name)) return "This name is reserved. Choose another name.";
  return null;
}

export function randomName(): string {
  const alphabet = "abcdefghijklmnopqrstuvwxyz0123456789";
  const random = new Uint32Array(6);
  globalThis.crypto.getRandomValues(random);
  return Array.from(random, (value) => alphabet[value % alphabet.length]).join(
    "",
  );
}

function seed(now: Date): PersistedState {
  const rows: Array<
    [string, Instance["status"], number, number, number, number, number]
  > = [
    ["gronda", "running", 124.6, 386.2, 86, 2.4, 24839],
    ["studio-api", "running", 82.4, 241.8, 64, 1.1, 14620],
    ["field-notes", "running", 48.2, 156.4, 48, 0.7, 9214],
    ["quiet-meadow", "stopped", 12.8, 32.6, 0, 0, 0],
    ["preview-api", "failed", 6.2, 18.4, 0, 0, 124],
  ];
  const instances = rows.map(
    ([name, status, db, total, memory, cpu, requests], i): Instance => ({
      id: `demo-${name}`,
      name,
      hostname: `${name}.${BASE_DOMAIN}`,
      status,
      version: "demo",
      databaseBytes: Math.round(db * MiB),
      storageBytes: Math.round(total * MiB),
      memoryBytes: memory * MiB,
      cpuPercent: cpu,
      requests,
      ingressBytes: requests * 720,
      egressBytes: requests * 12900,
      createdAt: new Date(
        now.getTime() - (18 - i * 3) * 86400000,
      ).toISOString(),
      updatedAt: new Date(now.getTime() - (i + 1) * 3600000).toISOString(),
      ...(status === "failed"
        ? {
            lastError:
              "Health check failed after service startup. Restart the demo instance to recover.",
          }
        : {}),
    }),
  );
  return {
    version: 1,
    instances,
    measuredAt: now.toISOString(),
    activity: [
      {
        id: "seed-1",
        instanceId: "demo-gronda",
        instanceName: "gronda",
        message: "Instance restarted successfully",
        kind: "success",
        at: new Date(now.getTime() - 240000).toISOString(),
      },
      {
        id: "seed-2",
        instanceId: "demo-preview-api",
        instanceName: "preview-api",
        message: "Instance health check failed",
        kind: "error",
        at: new Date(now.getTime() - 1800000).toISOString(),
      },
      {
        id: "seed-3",
        instanceId: "demo-quiet-meadow",
        instanceName: "quiet-meadow",
        message: "Instance stopped by admin",
        kind: "info",
        at: new Date(now.getTime() - 7200000).toISOString(),
      },
      {
        id: "seed-4",
        instanceId: "demo-field-notes",
        instanceName: "field-notes",
        message: "Instance created successfully",
        kind: "success",
        at: new Date(now.getTime() - 86400000).toISOString(),
      },
    ],
  };
}

function validState(value: unknown): value is PersistedState {
  if (!value || typeof value !== "object") return false;
  const state = value as PersistedState;
  if (
    state.version !== 1 ||
    !Array.isArray(state.instances) ||
    !Array.isArray(state.activity) ||
    !Number.isFinite(Date.parse(state.measuredAt))
  )
    return false;
  const ids = new Set<string>();
  const names = new Set<string>();
  for (const item of state.instances) {
    if (
      !item ||
      typeof item.id !== "string" ||
      !item.id ||
      typeof item.name !== "string" ||
      validateName(item.name) ||
      ids.has(item.id) ||
      names.has(item.name)
    )
      return false;
    if (
      item.hostname !== `${item.name}.${BASE_DOMAIN}` ||
      !["running", "stopped", "failed", "provisioning"].includes(item.status)
    )
      return false;
    if (
      ![
        "databaseBytes",
        "storageBytes",
        "memoryBytes",
        "cpuPercent",
        "requests",
        "ingressBytes",
        "egressBytes",
      ].every(
        (key) =>
          typeof item[key as keyof Instance] === "number" &&
          Number.isFinite(item[key as keyof Instance]) &&
          Number(item[key as keyof Instance]) >= 0,
      )
    )
      return false;
    if (
      item.databaseBytes > item.storageBytes ||
      !Number.isFinite(Date.parse(item.createdAt)) ||
      !Number.isFinite(Date.parse(item.updatedAt))
    )
      return false;
    ids.add(item.id);
    names.add(item.name);
  }
  return (
    state.activity.every(
      (item) =>
        item &&
        typeof item.id === "string" &&
        typeof item.instanceName === "string" &&
        typeof item.message === "string" &&
        ["success", "info", "error"].includes(item.kind) &&
        Number.isFinite(Date.parse(item.at)),
    ) &&
    OTHER_BYTES +
      state.instances.reduce((sum, item) => sum + item.storageBytes, 0) <=
      TOTAL_BYTES
  );
}

export class DemoManagerService implements ManagerService {
  async connect(): Promise<void> {}
  async loadUsage(): Promise<void> {}
  async loadLogs(): Promise<void> {}
  private state: PersistedState;
  private listeners = new Set<() => void>();
  private busy = new Set<string>();
  private delay: number;
  private now: () => Date;
  private storage?: StorageLike;
  private generation = 0;

  constructor(options: DemoOptions = {}) {
    this.now = options.now ?? (() => new Date());
    this.delay = options.delay ?? 550;
    this.storage = options.storage;
    this.state = seed(this.now());
    try {
      const saved = this.storage?.getItem(STORAGE_KEY);
      const parsed: unknown = saved ? JSON.parse(saved) : null;
      if (validState(parsed)) this.state = parsed;
    } catch {
      /* The demo also works when browser storage is unavailable. */
    }
    for (const instance of this.state.instances) {
      instance.domains ??= [this.defaultDomain(instance)];
      if (instance.status === "provisioning") {
        instance.status = "failed";
        instance.lastError =
          "The demo was reloaded during provisioning. Restart this instance to recover.";
      }
    }
  }

  private defaultDomain(instance: Instance): Domain {
    return {
      hostname: instance.hostname,
      kind: "default",
      verified: true,
      dnsReady: true,
      routeConfigured: true,
      stage: "ready",
      certificateStatus: "active",
      certificateIssuer: "Demo CA",
      certificateExpiresAt: new Date(
        this.now().getTime() + 90 * 86400000,
      ).toISOString(),
      updatedAt: this.now().toISOString(),
    };
  }
  async uploadSite(id: string, file: File): Promise<void> {
    if (
      !/\.(zip|html?)$/i.test(file.name) ||
      !file.size ||
      file.size > 50 * 1024 * 1024
    )
      throw new Error("Choose a ZIP or HTML file up to 50 MiB.");
    this.find(id).staticSite = {
      filename: file.name,
      files: 1,
      bytes: file.size,
      publishedAt: this.now().toISOString(),
    };
    this.save();
  }
  async removeSite(id: string): Promise<void> {
    delete this.find(id).staticSite;
    this.save();
  }
  async addDomain(id: string, hostname: string): Promise<void> {
    const instance = this.find(id);
    hostname = hostname.trim().toLowerCase();
    if (!/^[a-z0-9](?:[a-z0-9.-]*[a-z0-9])?\.[a-z]{2,}$/.test(hostname))
      throw new Error("Enter a hostname such as api.customer.com.");
    if (
      this.state.instances.some((item) =>
        item.domains?.some((domain) => domain.hostname === hostname),
      )
    )
      throw new Error("Domain is already in use.");
    instance.domains ??= [this.defaultDomain(instance)];
    instance.domains.push({
      hostname,
      kind: "custom",
      verificationToken: "pbm-" + crypto.randomUUID().replaceAll("-", ""),
      verified: false,
      dnsReady: false,
      routeConfigured: false,
      stage: "verification_pending",
      certificateStatus: "pending",
      updatedAt: this.now().toISOString(),
    });
    this.save();
  }
  async removeDomain(id: string, hostname: string): Promise<void> {
    const instance = this.find(id);
    if (
      instance.domains?.some(
        (domain) => domain.hostname === hostname && domain.kind === "default",
      )
    )
      throw new Error("The default domain is permanent.");
    instance.domains = instance.domains?.filter(
      (domain) => domain.hostname !== hostname,
    );
    this.save();
  }
  async verifyDomain(id: string, hostname: string): Promise<void> {
    const domain = this.find(id).domains?.find(
      (item) => item.hostname === hostname,
    );
    if (!domain) throw new Error("Domain not found.");
    domain.verified = true;
    domain.dnsReady = true;
    domain.routeConfigured = true;
    domain.stage = "certificate_pending";
    domain.certificateStatus = "issuing";
    this.save();
    await this.wait();
    domain.stage = "ready";
    domain.certificateStatus = "active";
    domain.certificateIssuer = "Demo CA";
    domain.certificateExpiresAt = new Date(
      this.now().getTime() + 90 * 86400000,
    ).toISOString();
    this.save();
  }

  getSnapshot(): Snapshot {
    const instanceBytes = this.state.instances.reduce(
      (sum, item) => sum + item.storageBytes,
      0,
    );
    const usedBytes = OTHER_BYTES + instanceBytes;
    return structuredClone({
      instances: this.state.instances,
      activity: this.state.activity,
      storage: {
        totalBytes: TOTAL_BYTES,
        usedBytes,
        availableBytes: TOTAL_BYTES - usedBytes,
        instanceBytes,
        otherBytes: OTHER_BYTES,
        measuredAt: this.state.measuredAt,
      },
    });
  }

  subscribe(listener: () => void): () => void {
    this.listeners.add(listener);
    return () => {
      this.listeners.delete(listener);
    };
  }

  private save(): void {
    try {
      this.storage?.setItem(STORAGE_KEY, JSON.stringify(this.state));
    } catch {
      /* Keep working in memory. */
    }
    this.listeners.forEach((listener) => listener());
  }

  private wait(): Promise<void> {
    return new Promise((resolve) => setTimeout(resolve, this.delay));
  }
  private find(id: string): Instance {
    const instance = this.state.instances.find((item) => item.id === id);
    if (!instance) throw new Error("This instance no longer exists.");
    return instance;
  }
  private event(
    instance: Instance,
    message: string,
    kind: Activity["kind"] = "success",
  ): void {
    this.state.activity.unshift({
      id: crypto.randomUUID(),
      instanceId: instance.id,
      instanceName: instance.name,
      message,
      kind,
      at: this.now().toISOString(),
    });
    this.state.activity = this.state.activity.slice(0, 100);
  }

  async create(
    name: string,
    progress?: (event: ProvisionProgress) => void,
    simulateFailure = false,
    customDomain?: string,
    memoryLimitMiB = 512,
  ): Promise<Instance> {
    if (
      !Number.isInteger(memoryLimitMiB) ||
      memoryLimitMiB < 128 ||
      memoryLimitMiB > 32768 ||
      memoryLimitMiB % 64 !== 0
    )
      throw new Error("Choose RAM in steps of 64 MiB, starting at 128 MiB.");
    name = name.trim();
    if (!name) {
      do {
        name = randomName();
      } while (this.state.instances.some((item) => item.name === name));
    }
    const error = validateName(name);
    if (error) throw new Error(error);
    if (this.state.instances.some((item) => item.name === name))
      throw new Error("This instance name is already in use.");
    if (this.getSnapshot().storage.availableBytes < 4 * MiB)
      throw new Error("Not enough server storage for another demo instance.");
    const instance: Instance = {
      id: crypto.randomUUID(),
      name,
      hostname: `${name}.${BASE_DOMAIN}`,
      status: "provisioning",
      version: "demo",
      databaseBytes: 2 * MiB,
      storageBytes: 4 * MiB,
      memoryBytes: 0,
      memoryLimitBytes: memoryLimitMiB * MiB,
      cpuPercent: 0,
      requests: 0,
      ingressBytes: 0,
      egressBytes: 0,
      createdAt: this.now().toISOString(),
      updatedAt: this.now().toISOString(),
    };
    this.state.instances.unshift(instance);
    instance.domains = [this.defaultDomain(instance)];
    if (customDomain) {
      try {
        await this.addDomain(instance.id, customDomain);
      } catch (error) {
        this.state.instances = this.state.instances.filter(
          (item) => item.id !== instance.id,
        );
        throw error;
      }
    }
    this.busy.add(instance.id);
    this.save();
    const generation = this.generation;
    try {
      for (let step = 0; step < PROVISION_STEPS.length; step++) {
        if (generation !== this.generation) throw new Error("Demo reset.");
        progress?.({
          instanceId: instance.id,
          step: step + 1,
          label: PROVISION_STEPS[step],
          complete: false,
        });
        await this.wait();
        if (generation !== this.generation) throw new Error("Demo reset.");
        if (simulateFailure && step === 4)
          throw new Error(
            "Simulated health check failure. You can restart the instance from the dashboard.",
          );
      }
      instance.status = "running";
      instance.memoryBytes = 32 * MiB;
      instance.cpuPercent = 0.2;
      instance.updatedAt = this.now().toISOString();
      this.event(instance, "Instance created successfully");
      progress?.({
        instanceId: instance.id,
        step: PROVISION_STEPS.length,
        label: "Your instance is ready",
        complete: true,
      });
      this.save();
      return structuredClone(instance);
    } catch (error) {
      if (generation === this.generation) {
        instance.status = "failed";
        instance.lastError =
          error instanceof Error ? error.message : "Provisioning failed.";
        this.event(instance, instance.lastError, "error");
        this.save();
      }
      throw error;
    } finally {
      this.busy.delete(instance.id);
    }
  }

  async control(id: string, action: ServiceAction): Promise<void> {
    const instance = this.find(id);
    if (this.busy.has(id))
      throw new Error("An operation is already in progress for this instance.");
    if (action === "start" && instance.status === "running") return;
    if (action === "stop" && instance.status === "stopped") return;
    const generation = this.generation;
    this.busy.add(id);
    try {
      await this.wait();
      if (generation !== this.generation) return;
      instance.status = action === "stop" ? "stopped" : "running";
      instance.memoryBytes = instance.status === "running" ? 48 * MiB : 0;
      instance.cpuPercent = instance.status === "running" ? 0.4 : 0;
      instance.updatedAt = this.now().toISOString();
      delete instance.lastError;
      this.event(
        instance,
        `Instance ${action === "stop" ? "stopped" : action === "restart" ? "restarted" : "started"} successfully`,
      );
      this.save();
    } finally {
      this.busy.delete(id);
    }
  }
  async updateResources(id: string, memoryLimitMiB: number): Promise<void> {
    if (
      !Number.isInteger(memoryLimitMiB) ||
      memoryLimitMiB < 128 ||
      memoryLimitMiB > 32768 ||
      memoryLimitMiB % 64 !== 0
    )
      throw new Error("Choose RAM in steps of 64 MiB, starting at 128 MiB.");
    const instance = this.find(id);
    if (this.busy.has(id))
      throw new Error("An operation is already in progress.");
    if (instance.memoryBytes > memoryLimitMiB * MiB)
      throw new Error("Choose a limit above current RAM usage.");
    instance.memoryLimitBytes = memoryLimitMiB * MiB;
    this.event(instance, `RAM limit updated to ${memoryLimitMiB} MiB`);
    this.save();
  }
  async createSFTPUser(
    id: string,
    username: string,
    password: string,
  ): Promise<void> {
    if (!/^[a-z][a-z0-9]{2,23}$/.test(username) || password.length < 12)
      throw new Error(
        "Use a 3–24 character username and a password of at least 12 characters.",
      );
    const instance = this.find(id);
    instance.sftpUsers ??= [];
    if (!instance.sftpUsers.some((user) => user.username === username))
      instance.sftpUsers.push({
        username,
        createdAt: this.now().toISOString(),
      });
    this.save();
  }
  async removeSFTPUser(id: string, username: string): Promise<void> {
    const instance = this.find(id);
    instance.sftpUsers = instance.sftpUsers?.filter(
      (user) => user.username !== username,
    );
    this.save();
  }

  async remove(id: string): Promise<void> {
    const instance = this.find(id);
    if (this.busy.has(id))
      throw new Error("Wait for the current operation before deleting.");
    const generation = this.generation;
    this.busy.add(id);
    try {
      await this.wait();
      if (generation !== this.generation) return;
      this.state.instances = this.state.instances.filter(
        (item) => item.id !== id,
      );
      this.event(instance, "Instance deleted", "info");
      this.save();
    } finally {
      this.busy.delete(id);
    }
  }

  getUsage(id?: string, hours = 24): UsageSample[] {
    const instances = id ? [this.find(id)] : this.state.instances;
    const requests = instances.reduce((sum, item) => sum + item.requests, 0);
    const memory = instances.reduce((sum, item) => sum + item.memoryBytes, 0);
    return Array.from({ length: 48 }, (_, index) => {
      const wave =
        0.55 +
        Math.sin(index * 0.7) * 0.16 +
        Math.cos(index * 1.9) * 0.09 +
        (index > 28 && index < 36 ? 0.25 : 0);
      const count = Math.round((requests / 48) * wave);
      return {
        at: new Date(
          this.now().getTime() - ((47 - index) / 47) * hours * 3600000,
        ).toISOString(),
        requests: count,
        cpuPercent:
          instances.reduce((sum, item) => sum + item.cpuPercent, 0) * wave,
        memoryBytes: Math.round(memory * (0.82 + wave * 0.18)),
        ingressBytes: count * 720,
        egressBytes: count * 12900,
      };
    });
  }

  getLogs(id: string): string[] {
    const item = this.find(id);
    const date = item.updatedAt.slice(0, 19).replace("T", " ");
    return [
      `${date} [systemd] pb-${item.name}.service: service ${item.status}`,
      `${date} [pocketbase] Data directory: /var/lib/pbmanager/instances/${item.name}/pb_data`,
      ...(item.status === "running"
        ? [
            `${date} [pocketbase] HTTP listener active on 127.0.0.1:<assigned-port>`,
            `${date} [proxy] HTTPS route configured for ${item.hostname}`,
            `${date} [health] GET /api/health → 200 OK`,
          ]
        : []),
      ...(item.status === "stopped"
        ? [`${date} [systemd] Stopped by administrator. Data is preserved.`]
        : []),
      ...(item.lastError ? [`${date} [error] ${item.lastError}`] : []),
      `${date} [demo] Simulated output. No services or network routes were created.`,
    ];
  }

  async refresh(): Promise<void> {
    await this.wait();
    this.state.measuredAt = this.now().toISOString();
    this.save();
  }
  reset(): void {
    this.generation++;
    this.state = seed(this.now());
    this.state.instances.forEach((instance) => {
      instance.domains = [this.defaultDomain(instance)];
    });
    this.state.instances.forEach((instance) => {
      instance.domains = [this.defaultDomain(instance)];
    });
    this.busy.clear();
    this.save();
  }
}
