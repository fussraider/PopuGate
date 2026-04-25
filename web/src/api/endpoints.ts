import api from './client'
import type {
  LoginRequest,
  LoginResponse,
  Settings,
  Secret,
  SecretWithLink,
  Upstream,
  NetInterface,
  UpstreamTestResult,
  Instance,
  Slave,
  SlaveTestResult,
  SyncResult,
  GlobalTraffic,
  UserTraffic,
  LiveMetrics,
  ProxyStatus,
  HealthStatus,
  DockerStatus,
  EngineStatus,
  BuildResult,
  UpdateStatus,
  UpdateResult,
  BackupInfo,
  OSType,
  ServiceStatus,
} from '@/types/models'

// ─── Auth ──────────────────────────────────────────────────────

export const authApi = {
  setup: (password: string) =>
    api.post<LoginResponse>('/auth/setup', { password }).then((r) => r.data),

  login: (data: LoginRequest) =>
    api.post<LoginResponse>('/auth/login', data).then((r) => r.data),

  refresh: (refreshToken: string) =>
    api.post<LoginResponse>('/auth/refresh', { refresh_token: refreshToken }).then((r) => r.data),

  logout: () => api.post('/auth/logout'),

  changePassword: (oldPassword: string, newPassword: string) =>
    api.put('/auth/password', { old_password: oldPassword, new_password: newPassword }),
}

// ─── Config / Settings ─────────────────────────────────────────

export const configApi = {
  getAll: () => api.get<Settings>('/config').then((r) => r.data),

  update: (data: Partial<Settings>) => api.put<Settings>('/config', data).then((r) => r.data),

  get: (key: string) => api.get(`/config/${key}`).then((r) => r.data),
}

// ─── Secrets ───────────────────────────────────────────────────

export const secretsApi = {
  list: () => api.get<Secret[]>('/secrets').then((r) => r.data),

  add: (label: string, secret?: string) =>
    api.post<Secret>('/secrets', { label, secret }).then((r) => r.data),

  get: (label: string) => api.get<Secret>(`/secrets/${label}`).then((r) => r.data),

  remove: (label: string, force = false) =>
    api.delete(`/secrets/${label}`, { data: { force } }),

  rotate: (label: string) =>
    api.post<Secret>(`/secrets/${label}/rotate`).then((r) => r.data),

  toggle: (label: string, enabled: boolean) =>
    api.put(`/secrets/${label}/toggle`, { enabled }),

  setLimits: (
    label: string,
    maxConns: number,
    maxIPs: number,
    quotaBytes: number,
    expiresAt: string,
  ) =>
    api.put(`/secrets/${label}/limits`, {
      max_conns: maxConns,
      max_ips: maxIPs,
      quota_bytes: quotaBytes,
      expires_at: expiresAt,
    }),

  getLimits: (label: string) => api.get(`/secrets/${label}/limits`).then((r) => r.data),

  getLink: (label: string) =>
    api.get<SecretWithLink>(`/secrets/${label}/link`).then((r) => r.data),

  getQR: (label: string) =>
    api.get(`/secrets/${label}/qr`, { responseType: 'blob' }).then((r) => r.data),

  updateNotes: (label: string, notes: string) =>
    api.put(`/secrets/${label}/notes`, { notes }),

  resetTraffic: (label?: string) =>
    label
      ? api.post(`/secrets/${label}/reset-traffic`)
      : api.post('/secrets/reset-traffic'),
}

// ─── Upstreams ─────────────────────────────────────────────────

export const upstreamsApi = {
  list: () => api.get<Upstream[]>('/upstreams').then((r) => r.data),

  interfaces: () =>
    api.get<NetInterface[]>('/upstreams/interfaces').then((r) => r.data),

  add: (data: Omit<Upstream, 'id'>) => api.post<Upstream>('/upstreams', data).then((r) => r.data),

  remove: (name: string) => api.delete(`/upstreams/${name}`),

  toggle: (name: string, enabled: boolean) =>
    api.put(`/upstreams/${name}/toggle`, { enabled }),

  test: (name: string) => api.post(`/upstreams/${name}/test`),

  testConfig: (data: { type: string; address?: string; username?: string; password?: string; iface?: string }) =>
    api.post<UpstreamTestResult>('/upstreams/test', data).then((r) => r.data),
}

// ─── Instances ─────────────────────────────────────────────────

export const instancesApi = {
  list: () => api.get<Instance[]>('/instances').then((r) => r.data),

  add: (port: number, label: string) =>
    api.post<Instance>('/instances', { port, label }).then((r) => r.data),

  remove: (port: number) => api.delete(`/instances/${port}`),
}

// ─── Proxy Control ─────────────────────────────────────────────

export const proxyApi = {
  start: () => api.post('/proxy/start', {}, { timeout: 300000 }),
  stop: () => api.post('/proxy/stop', {}, { timeout: 300000 }),
  restart: () => api.post('/proxy/restart', {}, { timeout: 300000 }),
  reload: () => api.post('/proxy/reload', {}, { timeout: 120000 }),
  status: () => api.get<ProxyStatus>('/proxy/status').then((r) => r.data),
  logs: (tail = '100', follow = false) =>
    api.get<string>(`/proxy/logs?tail=${tail}&follow=${follow}`).then((r) => r.data),
}

// ─── Docker / Engine ───────────────────────────────────────────

export const dockerApi = {
  install: () => api.post('/docker/install', {}, { timeout: 600000 }),
  status: () => api.get<DockerStatus>('/docker/status').then((r) => r.data),
  engineStatus: () => api.get<EngineStatus>('/engine/status').then((r) => r.data),
  build: (force = false) =>
    api.post<BuildResult>('/engine/build', { force }, { timeout: 1800000 }).then((r) => r.data),
}

// ─── Geoblock ──────────────────────────────────────────────────

export const geoblockApi = {
  get: () => api.get('/geoblock').then((r) => r.data),
  add: (countryCode: string) => api.post('/geoblock/add', { country_code: countryCode }),
  remove: (countryCode: string) => api.post('/geoblock/remove', { country_code: countryCode }),
  clear: () => api.post('/geoblock/clear'),
  setMode: (mode: 'blacklist' | 'whitelist') =>
    api.put('/geoblock/mode', { mode }),
}

// ─── Traffic ───────────────────────────────────────────────────

export const trafficApi = {
  get: () =>
    api.get<{ global: GlobalTraffic; users: UserTraffic[] }>('/traffic').then((r) => r.data),
  getLive: () => api.get<LiveMetrics>('/traffic/live', { _silent: true } as any).then((r) => r.data),
  getUser: (label: string) =>
    api.get<UserTraffic>(`/traffic/${label}`).then((r) => r.data),
}

// ─── Bot ───────────────────────────────────────────────────────

export const botApi = {
  setup: (token: string, chatId: string, interval: number, label: string) =>
    api.post('/bot/setup', { token, chat_id: chatId, interval, label }),

  test: () => api.post('/bot/test'),

  status: () => api.get('/bot/status').then((r) => r.data),

  toggle: (enable: boolean) => api.put('/bot/toggle', { enable }),

  detectChatId: () => api.get('/bot/detect-chat-id').then((r) => r.data),
}

// ─── Replication ───────────────────────────────────────────────

export const replicationApi = {
  status: () => api.get('/replication/status').then((r) => r.data),

  setup: (data: { role: string; sync_interval?: number }) =>
    api.post('/replication/setup', data, { timeout: 120000 }),

  addSlave: (host: string, port: number, label: string) =>
    api.post<Slave>('/replication/slaves', { host, port, label }).then((r) => r.data),

  removeSlave: (host: string) => api.delete(`/replication/slaves/${host}`),

  listSlaves: () => api.get<Slave[]>('/replication/slaves').then((r) => r.data),

  sync: (host: string) => api.post<SyncResult>('/replication/sync', { host }, { timeout: 300000 }).then((r) => r.data),

  test: (host: string) =>
    api.post<SlaveTestResult>('/replication/test', { host }, { timeout: 120000 }).then((r) => r.data),

  sshKeygen: () => api.post('/replication/ssh-keygen', {}, { timeout: 120000 }).then((r) => r.data),
}

// ─── Update ────────────────────────────────────────────────────

export const updateApi = {
  check: () => api.get<UpdateStatus>('/update/check').then((r) => r.data),
  apply: () => api.post<UpdateResult>('/update/apply', {}, { timeout: 300000 }).then((r) => r.data),
}

// ─── Backup ────────────────────────────────────────────────────

export const backupApi = {
  list: () => api.get<BackupInfo[]>('/backups').then((r) => r.data),
  create: (label?: string) => api.post('/backups', { label }, { timeout: 300000 }),
  restore: (filename: string) => api.post('/backups/restore', { filename }, { timeout: 300000 }),
  download: (filename: string) =>
    api.get(`/backups/download/${filename}`, { responseType: 'blob', timeout: 300000 }).then((r) => r.data),
  delete: (filename: string) => api.delete(`/backups/${filename}`),
}

// ─── System ────────────────────────────────────────────────────

export const systemApi = {
  getOS: () => api.get<OSType>('/system/os').then((r) => r.data),
  installService: () => api.post('/system/service/install', {}, { timeout: 300000 }),
  uninstallService: () => api.delete('/system/service/uninstall', { timeout: 120000 }),
  serviceStatus: () =>
    api.get<ServiceStatus>('/system/service/status').then((r) => r.data),
  restartService: () => api.post('/system/service/restart', {}, { timeout: 120000 }),
  reloadService: () => api.post('/system/service/reload', {}, { timeout: 120000 }),
}

// ─── Health ────────────────────────────────────────────────────

export const healthApi = {
  check: () => api.get<HealthStatus>('/health').then((r) => r.data),
}
