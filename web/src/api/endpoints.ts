import api from './client'
import type {
    AuditEntry,
    BackupInfo,
    BuildResult,
    DockerStatus,
    DockerUpdateStatus,
    EngineStatus,
    GlobalTraffic,
    HealthStatus,
    Instance,
    LiveMetrics,
    LoginRequest,
    LoginResponse,
    NetInterface,
    OSType,
    ProxyStatus,
    SchedulerHistoryRecord,
    SchedulerTask,
    Secret,
    SecretImportItem,
    SecretTemplate,
    SecretWithLink,
    ServiceStatus,
    Settings,
    Slave,
    SlaveTestResult,
    SyncResult,
    SystemResources,
    TelemtReleaseListItem,
    TelemtUpdateStatus,
    TrafficHistoryRecord,
    UpdateResult,
    UpdateStatus,
    Upstream,
    UpstreamTestResult,
    UserTraffic,
} from '@/types/models'

// ─── Auth ──────────────────────────────────────────────────────

export const authApi = {
  setup: (password: string) =>
    api.post<LoginResponse>('/auth/setup', { password }).then((r) => r.data),

  login: (data: LoginRequest) =>
    api.post<LoginResponse>('/auth/login', data).then((r) => r.data),

  refresh: (refreshToken: string) =>
    api.post<LoginResponse>('/auth/refresh', { refresh_token: refreshToken }, { _noRetry: true } as any).then((r) => r.data),

  logout: () => api.post('/auth/logout', {}, { _noRetry: true } as any),

  changePassword: (currentPassword: string, newPassword: string) =>
    api.put('/auth/password', { current: currentPassword, new: newPassword }),
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
    rateLimitUpBps?: number,
    rateLimitDownBps?: number,
  ) =>
    api.put(`/secrets/${label}/limits`, {
      max_conns: maxConns,
      max_ips: maxIPs,
      quota_bytes: quotaBytes,
      expires_at: expiresAt,
      rate_limit_up_bps: rateLimitUpBps,
      rate_limit_down_bps: rateLimitDownBps,
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

  setTags: (label: string, tags: string) =>
    api.put(`/secrets/${label}/tags`, { tags }),

  archive: (label: string) =>
    api.post(`/secrets/${label}/archive`),

  unarchive: (label: string) =>
    api.post(`/secrets/${label}/unarchive`),

  clone: (label: string, newLabel: string) =>
    api.post<Secret>(`/secrets/${label}/clone`, { new_label: newLabel }).then((r) => r.data),

  rename: (label: string, newLabel: string) =>
    api.put(`/secrets/${label}/rename`, { new_label: newLabel }),

  extend: (label: string, days: number) =>
    api.post<Secret>(`/secrets/${label}/extend`, { days }).then((r) => r.data),

  disableExpired: () =>
    api.post<{ ok: boolean; disabled: number }>('/secrets/disable-expired').then((r) => r.data),

  bulkExtend: (labels: string[], days: number, tag?: string) =>
    api.post<{ ok: boolean; updated: number }>('/secrets/bulk-extend', { labels: tag ? undefined : labels, tag: tag || undefined, days }).then((r) => r.data),

  bulkRotate: (labels: string[], tag?: string) =>
    api.post<{ ok: boolean; updated: number; labels: string[] }>('/secrets/bulk-rotate', { labels: tag ? undefined : labels, tag: tag || undefined }).then((r) => r.data),

  bulkToggle: (labels: string[], enable: boolean, tag?: string) =>
    api.post<{ ok: boolean; updated: number }>('/secrets/bulk-toggle', { labels: tag ? undefined : labels, tag: tag || undefined, enable }).then((r) => r.data),

  bulkSetLimits: (
    labels: string[],
    limits: { max_conns?: number; max_ips?: number; quota_bytes?: number; expires_at?: string; rate_limit_up_bps?: number; rate_limit_down_bps?: number },
    tag?: string,
  ) =>
    api.post<{ ok: boolean; updated: number }>('/secrets/bulk-set-limits', { labels: tag ? undefined : labels, tag: tag || undefined, ...limits }).then((r) => r.data),

  listTags: () =>
    api.get<{ tags: string[] }>('/secrets/tags').then((r) => r.data),

  listByTag: (tag: string) =>
    api.get<Secret[]>(`/secrets/by-tag/${tag}`).then((r) => r.data),

  search: (query: string) =>
    api.get<Secret[]>('/secrets/search', { params: { q: query } }).then((r) => r.data),

  top: (limit = 10) =>
    api.get<Secret[]>('/secrets/top', { params: { limit } }).then((r) => r.data),

  exportAll: () =>
    api.get<Secret[]>('/secrets/export').then((r) => r.data),

  importSecrets: (secrets: SecretImportItem[]) =>
    api.post<{ ok: boolean; imported: string[]; skipped: string[]; errors: string[] }>('/secrets/import', { secrets }).then((r) => r.data),
}

// ─── Upstreams ─────────────────────────────────────────────────

export const upstreamsApi = {
  list: () => api.get<Upstream[]>('/upstreams').then((r) => r.data),

  interfaces: () =>
    api.get<NetInterface[]>('/upstreams/interfaces').then((r) => r.data),

  add: (data: Omit<Upstream, 'id'>) => api.post<Upstream>('/upstreams', data).then((r) => r.data),

  update: (name: string, data: Omit<Upstream, 'id' | 'name' | 'enabled'>) =>
    api.put<Upstream>(`/upstreams/${name}`, data).then((r) => r.data),

  remove: (name: string) => api.delete(`/upstreams/${name}`),

  toggle: (name: string, enabled: boolean) =>
    api.put(`/upstreams/${name}/toggle`, { enabled }),

  test: (name: string) => api.post<UpstreamTestResult>(`/upstreams/${name}/test`).then((r) => r.data),

  testConfig: (data: { type: string; address?: string; username?: string; password?: string; iface?: string }) =>
    api.post<UpstreamTestResult>('/upstreams/test', data).then((r) => r.data),

  bulkAdd: (data: {
    upstreams: { type: string; address?: string; username?: string; password?: string; url?: string; weight?: number; iface?: string }[]
  }) => api.post<{ ok: boolean; count: number; skipped: number; skipped_middle_proxy?: string[]; names?: string[] }>('/upstreams/bulk', data).then((r) => r.data),
}

// ─── Instances ─────────────────────────────────────────────────

export const instancesApi = {
  list: () => api.get<Instance[]>('/instances').then((r) => r.data),

  add: (data: {
    port: number
    label: string
    tls_domain: string
    tls_domains?: string
    fake_tls?: boolean
    mask_host?: string
    mask_port?: number
    tags?: string
    metrics_port?: number
    tcp_mss_enabled?: boolean
    tcp_mss?: number
    tls_fronting?: boolean
  }) => api.post<Instance>('/instances', data).then((r) => r.data),

  update: (id: number, data: Partial<Instance>) =>
    api.put<Instance>(`/instances/${id}`, data).then((r) => r.data),

  remove: (id: number) => api.delete(`/instances/${id}`),

  start: (id: number) =>
    api.post(`/instances/${id}/start`, {}, { timeout: 300000 }),

  stop: (id: number) =>
    api.post(`/instances/${id}/stop`, {}, { timeout: 300000 }),

  reload: (id: number) =>
    api.post(`/instances/${id}/reload`, {}, { timeout: 120000 }),
  restart: (id: number) =>
    api.post(`/instances/${id}/restart`, {}, { timeout: 300000 }),
  reloadConfig: (id: number) =>
    api.post(`/instances/${id}/reload-config`, {}, { timeout: 120000 }),

  status: (id: number) =>
    api.get<{ id: number; status: string }>(`/instances/${id}/status`).then((r) => r.data),

  logs: (id: number, tail = '100') =>
    api.get<string>(`/instances/${id}/logs?tail=${tail}`).then((r) => r.data),

  checkPort: (port: number, excludeId?: number) =>
    api.get<{ available: boolean; reason?: string }>('/instances/check-port', {
      params: { port, exclude: excludeId || undefined },
    }).then((r) => r.data),

  refreshFronting: (id: number) =>
    api.post(`/instances/${id}/refresh-fronting`).then((r) => r.data),
}

// ─── Proxy Control ─────────────────────────────────────────────

export const proxyApi = {
  start: () => api.post('/proxy/start', {}, { timeout: 300000 }),
  stop: () => api.post('/proxy/stop', {}, { timeout: 300000 }),
  restart: () => api.post('/proxy/restart', {}, { timeout: 300000 }),
  reload: () => api.post('/proxy/reload', {}, { timeout: 120000 }),
  reloadZeroDowntime: () => api.post('/proxy/reload-zero-downtime', {}, { timeout: 300000 }),
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
  engineUpdateStatus: () => api.get<TelemtUpdateStatus>('/engine/update').then((r) => r.data),
  engineReleases: () => api.get<TelemtReleaseListItem[]>('/engine/releases').then((r) => r.data),
  engineCheckRemote: () => api.post<TelemtUpdateStatus>('/engine/check', {}).then((r) => r.data),
  engineApplyUpdate: (version: string, commit: string) =>
    api.post<{ ok: boolean; message: string; version: string }>('/engine/update', { version, commit }, { timeout: 2100000 }).then((r) => r.data),
  engineCancelUpdate: () =>
    api.post<{ ok: boolean; message: string }>('/engine/update/cancel', {}).then((r) => r.data),
  engineCancelBuild: () =>
    api.post<{ ok: boolean; message: string }>('/engine/build/cancel', {}).then((r) => r.data),
  engineBuildLogs: () =>
    api.get<string>('/engine/update/logs').then((r) => r.data),

  // Host Docker daemon update methods
  updateStatus: () => api.get<DockerUpdateStatus>('/docker/update/status').then((r) => r.data),
  updateCheck: () => api.post<DockerUpdateStatus>('/docker/update/check', {}).then((r) => r.data),
  updateApply: () => api.post<{ ok: boolean }>('/docker/update/apply', {}, { timeout: 900000 }).then((r) => r.data),
}

// ─── Geoblock ──────────────────────────────────────────────────

export const geoblockApi = {
  get: () => api.get('/geoblock').then((r) => r.data),
  add: (countryCode: string) => api.post('/geoblock/add', { country: countryCode }),
  remove: (countryCode: string) => api.post('/geoblock/remove', { country: countryCode }),
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
  getHistory: (start: number, end: number, label?: string, aggregate?: string) =>
    api.get<{ history: TrafficHistoryRecord[] }>('/traffic/history', {
      params: { start, end, label: label || undefined, aggregate: aggregate || 'none' },
    }).then((r) => r.data),
}

// ─── Bot ───────────────────────────────────────────────────────

export const botApi = {
  setup: (token: string, chatId: string, interval: number, label: string) =>
    api.post('/bot/setup', { token, chat_id: chatId, interval, label }),

  test: () => api.post('/bot/test'),

  status: () => api.get('/bot/status').then((r) => r.data),

  toggle: (enable: boolean) => api.put('/bot/toggle', { enable }),

  detectChatId: () => api.get('/bot/detect-chat-id').then((r) => r.data),

  setCommands: () => api.post('/bot/commands'),
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

  sshKey: () => api.get('/replication/ssh-key').then((r) => r.data),
}

// ─── Update ────────────────────────────────────────────────────

export const updateApi = {
  check: () => api.get<UpdateStatus>('/update/check').then((r) => r.data),
  apply: () => api.post<UpdateResult>('/update/apply', {}, { timeout: 300000 }).then((r) => r.data),
}

// ─── Backup ────────────────────────────────────────────────────

export const backupApi = {
  list: () => api.get<{ backups: BackupInfo[]; encryption_enabled: boolean }>('/backups').then((r) => r.data),
  create: (label?: string) => api.post('/backups', { label }, { timeout: 300000 }),
  restore: (filename: string) => api.post('/backups/restore', { filename }, { timeout: 300000 }),
  download: (filename: string) =>
    api.get(`/backups/download/${filename}`, { responseType: 'blob', timeout: 300000 }).then((r) => r.data),
  delete: (filename: string) => api.delete(`/backups/${filename}`),
}

// ─── System ────────────────────────────────────────────────────

export const systemApi = {
  getOS: () => api.get<OSType>('/system/os').then((r) => r.data),
  getResources: () => api.get<SystemResources>('/system/resources').then((r) => r.data),
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

// ─── Scheduler ─────────────────────────────────────────────────

export const schedulerApi = {
  listTasks: () =>
    api.get<SchedulerTask[]>('/scheduler/tasks').then((r) => r.data),

  updateTask: (name: string, data: { enabled?: boolean; schedule?: string }) =>
    api.put(`/scheduler/tasks/${name}`, data).then((r) => r.data),

  runTask: (name: string) =>
    api.post<SchedulerHistoryRecord>(`/scheduler/tasks/${name}/run`).then((r) => r.data),

  getTaskHistory: (name: string, limit = 20, offset = 0) =>
    api.get<SchedulerHistoryRecord[]>(`/scheduler/tasks/${name}/history`, { params: { limit, offset } }).then((r) => r.data),

  getAllHistory: (limit = 50, offset = 0) =>
    api.get<SchedulerHistoryRecord[]>('/scheduler/history', { params: { limit, offset } }).then((r) => r.data),
}

// ─── Audit ─────────────────────────────────────────────────────

export const auditApi = {
  list: (limit = 100, offset = 0, users?: string[], actions?: string[], from?: number, to?: number) =>
    api.get<AuditEntry[]>('/audit', {
      params: {
        limit,
        offset,
        users: users?.join(','),
        actions: actions?.join(','),
        from,
        to,
      }
    }).then((r) => r.data),

  filters: () =>
    api.get<{ users: string[]; actions: string[] }>('/audit/filters').then((r) => r.data),
}

// ─── Templates ──────────────────────────────────────────────────

export const templatesApi = {
  list: () =>
    api.get<SecretTemplate[]>('/templates').then((r) => r.data),

  get: (name: string) =>
    api.get<SecretTemplate>(`/templates/${name}`).then((r) => r.data),

  create: (data: Omit<SecretTemplate, 'id'>) =>
    api.post<SecretTemplate>('/templates', data).then((r) => r.data),

  remove: (name: string) =>
    api.delete(`/templates/${name}`),

  apply: (templateName: string, secretLabel: string) =>
    api.post(`/templates/${templateName}/apply`, { secret_label: secretLabel }).then((r) => r.data),
}
