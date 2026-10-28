export type InstanceStatus = "running" | "stopped" | "provisioning" | "failed";
export type ServiceAction = "start" | "stop" | "restart";
export type View = "overview" | "instances" | "usage" | "activity";

export interface Instance {
  id: string;
  name: string;
  hostname: string;
  dnsReady?: boolean;
  status: InstanceStatus;
  version: string;
  databaseBytes: number;
  storageBytes: number;
  memoryBytes: number;
  memoryLimitBytes?: number;
  cpuPercent: number;
  requests: number;
  ingressBytes: number;
  ingressEstimated?: boolean;
  egressBytes: number;
  createdAt: string;
  updatedAt: string;
  lastError?: string;
  domains?: Domain[];
  provisioningStage?: string;
  provisioningMessage?: string;
  staticSite?: StaticSite;
  sftpUsers?: { username: string; createdAt: string }[];
}

export interface StaticSite {
  filename: string;
  files: number;
  bytes: number;
  publishedAt: string;
}

export interface Domain {
  hostname: string;
  kind: "default" | "custom";
  verificationToken?: string;
  verified: boolean;
  dnsReady: boolean;
  routeConfigured: boolean;
  stage:
    | "verification_pending"
    | "dns_pending"
    | "proxy_configuring"
    | "certificate_pending"
    | "ready"
    | "retrying"
    | "failed";
  certificateStatus: "pending" | "issuing" | "active" | "retrying";
  certificateIssuer?: string;
  certificateExpiresAt?: string;
  lastError?: string;
  nextRetryAt?: string;
  updatedAt: string;
}

export interface StorageSummary {
  totalBytes: number;
  usedBytes: number;
  availableBytes: number;
  instanceBytes: number;
  otherBytes: number;
  measuredAt: string;
}

export interface UsageSample {
  at: string;
  requests: number;
  cpuPercent: number;
  memoryBytes: number;
  ingressBytes: number;
  egressBytes: number;
}

export interface Activity {
  id: string;
  instanceId?: string;
  instanceName: string;
  message: string;
  kind: "success" | "info" | "error";
  at: string;
}

export interface Snapshot {
  instances: Instance[];
  activity: Activity[];
  storage: StorageSummary;
  server?: {
    name: string;
    os: string;
    cpuCount: number;
    memoryTotalBytes: number;
    baseDomain: string;
    managerHost: string;
    pocketbaseVersion: string;
    trafficAvailable: boolean;
    publicIP?: string;
    error?: string;
  };
}

export interface ProvisionProgress {
  instanceId: string;
  step: number;
  label: string;
  complete: boolean;
}

/** Replace this adapter with Go HTTP calls after the demo workflows are approved. */
export interface ManagerService {
  connect(): Promise<void>;
  loadUsage(id?: string, hours?: number): Promise<void>;
  loadLogs(id: string): Promise<void>;
  getSnapshot(): Snapshot;
  subscribe(listener: () => void): () => void;
  create(
    name: string,
    progress?: (event: ProvisionProgress) => void,
    simulateFailure?: boolean,
    customDomain?: string,
    memoryLimitMiB?: number,
  ): Promise<Instance>;
  addDomain(id: string, hostname: string): Promise<void>;
  removeDomain(id: string, hostname: string): Promise<void>;
  verifyDomain(id: string, hostname: string): Promise<void>;
  uploadSite(id: string, file: File): Promise<void>;
  removeSite(id: string): Promise<void>;
  updateResources(id: string, memoryLimitMiB: number): Promise<void>;
  createSFTPUser(id: string, username: string, password: string): Promise<void>;
  removeSFTPUser(id: string, username: string): Promise<void>;
  control(id: string, action: ServiceAction): Promise<void>;
  remove(id: string): Promise<void>;
  getUsage(id?: string, hours?: number): UsageSample[];
  getLogs(id: string): string[];
  refresh(force?: boolean): Promise<void>;
  reset(): void;
}
