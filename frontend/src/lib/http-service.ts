import type {
  Instance,
  ManagerService,
  ProvisionProgress,
  ServiceAction,
  Snapshot,
  UsageSample,
} from "./types";

export class UnauthorizedError extends Error {}
interface Operation {
  id: string;
  instanceId: string;
  action: string;
  state: "queued" | "running" | "succeeded" | "failed";
  step: number;
  label: string;
  error?: string;
}

export class HttpManagerService implements ManagerService {
  private snapshot: Snapshot = {
    instances: [],
    activity: [],
    storage: {
      totalBytes: 0,
      usedBytes: 0,
      availableBytes: 0,
      instanceBytes: 0,
      otherBytes: 0,
      measuredAt: "",
    },
  };
  private listeners = new Set<() => void>();
  private usage = new Map<string, UsageSample[]>();
  private logs = new Map<string, string[]>();
  email = "";

  async request<T>(path: string, init: RequestInit = {}): Promise<T> {
    const headers = new Headers(init.headers);
    if (!(init.body instanceof FormData) && !headers.has("Content-Type"))
      headers.set("Content-Type", "application/json");
    const response = await fetch("/api/manager" + path, {
      credentials: "same-origin",
      ...init,
      headers,
      signal: init.signal ?? AbortSignal.timeout(100000),
    });
    if (response.status === 401) {
      window.dispatchEvent(new Event("manager:unauthorized"));
      throw new UnauthorizedError("Sign in to continue.");
    }
    if (!response.ok) {
      const error = await response
        .json()
        .catch(() => ({ message: response.statusText }));
      throw new Error(error.message ?? "Request failed.");
    }
    if (response.status === 204) return undefined as T;
    return response.json() as Promise<T>;
  }
  private publish(): void {
    this.listeners.forEach((listener) => listener());
  }
  getSnapshot(): Snapshot {
    return structuredClone(this.snapshot);
  }
  subscribe(listener: () => void): () => void {
    this.listeners.add(listener);
    return () => {
      this.listeners.delete(listener);
    };
  }
  async connect(): Promise<void> {
    const session = await this.request<{ email: string }>("/session");
    this.email = session.email;
    await this.refresh();
  }
  async login(email: string, password: string): Promise<void> {
    await this.request("/session", {
      method: "POST",
      body: JSON.stringify({ email, password }),
    });
    await this.connect();
  }
  async logout(): Promise<void> {
    await this.request("/session", { method: "DELETE" });
    this.email = "";
  }
  async refresh(force = false): Promise<void> {
    this.snapshot = await this.request<Snapshot>(
      force ? "/refresh" : "/snapshot",
      force ? { method: "POST" } : {},
    );
    this.publish();
  }
  async loadUsage(id?: string, hours = 24): Promise<void> {
    const params = new URLSearchParams({ hours: String(hours) });
    if (id) params.set("id", id);
    this.usage.set(
      `${id ?? "all"}:${hours}`,
      await this.request<UsageSample[]>("/usage?" + params),
    );
  }
  getUsage(id?: string, hours = 24): UsageSample[] {
    return structuredClone(this.usage.get(`${id ?? "all"}:${hours}`) ?? []);
  }
  async loadLogs(id: string): Promise<void> {
    this.logs.set(
      id,
      await this.request<string[]>(`/instances/${encodeURIComponent(id)}/logs`),
    );
  }
  getLogs(id: string): string[] {
    return this.logs.get(id) ?? ["Loading service logs…"];
  }
  async credentials(id: string): Promise<{ email: string; password: string }> {
    return this.request(`/instances/${encodeURIComponent(id)}/credentials`, {
      method: "POST",
    });
  }
  async uploadSite(id: string, file: File): Promise<void> {
    const body = new FormData();
    body.set("site", file);
    await this.request(`/instances/${encodeURIComponent(id)}/site`, {
      method: "POST",
      body,
    });
    await this.refresh(true);
  }
  async removeSite(id: string): Promise<void> {
    await this.request(`/instances/${encodeURIComponent(id)}/site`, {
      method: "DELETE",
    });
    await this.refresh(true);
  }
  private async wait(
    operation: Operation,
    progress?: (event: ProvisionProgress) => void,
  ): Promise<void> {
    const deadline = Date.now() + 5 * 60 * 1000;
    while (Date.now() < deadline) {
      progress?.({
        instanceId: operation.instanceId,
        step: operation.step,
        label: operation.label,
        complete: operation.state === "succeeded",
      });
      if (operation.state === "succeeded") return;
      if (operation.state === "failed")
        throw new Error(operation.error ?? "Operation failed.");
      await new Promise((resolve) => setTimeout(resolve, 1000));
      operation = await this.request<Operation>(
        `/operations/${encodeURIComponent(operation.id)}`,
      );
    }
    throw new Error(
      "The operation is still running. Check the instance status before retrying.",
    );
  }
  async create(
    name: string,
    progress?: (event: ProvisionProgress) => void,
    _simulateFailure?: boolean,
    customDomain?: string,
    memoryLimitMiB = 512,
  ): Promise<Instance> {
    const result = await this.request<{
      instance: Instance;
      operation: Operation;
    }>("/instances", {
      method: "POST",
      body: JSON.stringify({ name, customDomain, memoryLimitMiB }),
    });
    this.snapshot.instances.unshift(result.instance);
    this.publish();
    try {
      await this.wait(result.operation, progress);
    } finally {
      await this.refresh(true);
    }
    return (
      this.snapshot.instances.find((item) => item.id === result.instance.id) ??
      result.instance
    );
  }
  async control(id: string, action: ServiceAction): Promise<void> {
    const operation = await this.request<Operation>(
      `/instances/${encodeURIComponent(id)}/actions/${action}`,
      { method: "POST" },
    );
    try {
      await this.wait(operation);
    } finally {
      await this.refresh(true);
    }
  }
  async updateResources(id: string, memoryLimitMiB: number): Promise<void> {
    const operation = await this.request<Operation>(
      `/instances/${encodeURIComponent(id)}/resources`,
      {
        method: "PATCH",
        body: JSON.stringify({ memoryLimitMiB }),
      },
    );
    try {
      await this.wait(operation);
    } finally {
      await this.refresh(true);
    }
  }
  async createSFTPUser(
    id: string,
    username: string,
    password: string,
  ): Promise<void> {
    const operation = await this.request<Operation>(
      `/instances/${encodeURIComponent(id)}/sftp-users`,
      { method: "POST", body: JSON.stringify({ username, password }) },
    );
    try {
      await this.wait(operation);
    } finally {
      await this.refresh(true);
    }
  }
  async removeSFTPUser(id: string, username: string): Promise<void> {
    const operation = await this.request<Operation>(
      `/instances/${encodeURIComponent(id)}/sftp-users/${encodeURIComponent(username)}`,
      { method: "DELETE" },
    );
    try {
      await this.wait(operation);
    } finally {
      await this.refresh(true);
    }
  }
  async remove(id: string): Promise<void> {
    const operation = await this.request<Operation>(
      `/instances/${encodeURIComponent(id)}`,
      { method: "DELETE" },
    );
    try {
      await this.wait(operation);
    } finally {
      await this.refresh(true);
    }
  }
  private async domainAction(
    id: string,
    path: string,
    method: string,
    body?: unknown,
  ): Promise<void> {
    const operation = await this.request<Operation>(
      `/instances/${encodeURIComponent(id)}/domains${path}`,
      { method, body: body === undefined ? undefined : JSON.stringify(body) },
    );
    try {
      await this.wait(operation);
    } finally {
      await this.refresh(true);
    }
  }
  async addDomain(id: string, hostname: string): Promise<void> {
    await this.domainAction(id, "", "POST", { hostname });
  }
  async removeDomain(id: string, hostname: string): Promise<void> {
    await this.domainAction(id, `/${encodeURIComponent(hostname)}`, "DELETE");
  }
  async verifyDomain(id: string, hostname: string): Promise<void> {
    await this.domainAction(
      id,
      `/${encodeURIComponent(hostname)}/verify`,
      "POST",
    );
  }
  reset(): void {
    throw new Error("Reset is only available in demo mode.");
  }
}
