export interface Settings {
  proxy_port: number
  proxy_metrics_port: number
  proxy_domain: string
  proxy_concurrency: number
  proxy_cpus: string
  proxy_memory: string
  custom_ip: string
  fake_cert_len: number
  proxy_protocol: boolean
  proxy_protocol_trusted_cidrs: string
  ad_tag: string
  geoblock_mode: 'blacklist' | 'whitelist'
  blocklist_countries: string
  masking_enabled: boolean
  masking_host: string
  masking_port: number
  unknown_sni_action: 'mask' | 'drop'
  telegram_enabled: boolean
  telegram_bot_token: string
  telegram_chat_id: string
  telegram_interval: number
  telegram_alerts_enabled: boolean
  telegram_server_label: string
  auto_update_enabled: boolean
  replication_enabled: boolean
  replication_role: 'standalone' | 'master' | 'slave'
  replication_sync_interval: number
  replication_ssh_port: number
  replication_ssh_user: string
  replication_delete_extra: boolean
  replication_ssh_key_path: string
  replication_exclude: string
  replication_restart_on_change: boolean
  replication_log: string
  debug: boolean
}

export interface Secret {
  id: number
  label: string
  secret_key: string
  created_at: number
  enabled: boolean
  max_conns: number
  max_ips: number
  quota_bytes: number
  expires_at: string
  notes: string
  traffic_in?: number
  traffic_out?: number
}

export interface SecretWithLink extends Secret {
  tg_link?: string
  web_link?: string
}

export interface Upstream {
  id: number
  name: string
  type: 'direct' | 'socks5' | 'socks4'
  address: string
  username: string
  password: string
  weight: number
  iface: string
  enabled: boolean
}

export interface NetInterface {
  name: string
  addresses: string[]
}

export interface UpstreamTestResult {
  ok: boolean
  exit_ip?: string
  latency_ms?: number
  error?: string
}

export interface Instance {
  id: number
  port: number
  metrics_port: number
  enabled: boolean
  label: string
}

export interface Slave {
  id: number
  host: string
  port: number
  label: string
  enabled: boolean
  last_sync: number
  status: string
}

export interface SlaveTestResult {
  host: string
  ssh_ok: boolean
  docker_status: string
  error: string
}

export interface SyncResult {
  host: string
  files_sent: number
  deleted: number
  error: string
}

export interface UserTraffic {
  label: string
  bytes_in: number
  bytes_out: number
  bytes_in_delta: number
  bytes_out_delta: number
}

export interface GlobalTraffic {
  bytes_in: number
  bytes_out: number
  bytes_in_delta: number
  bytes_out_delta: number
}

export interface LiveMetrics {
  connections: number
  connections_total: number
  connections_bad_total: number
  connections_me_current: number
  connections_direct_current: number
  upstream_attempt_total: number
  upstream_success_total: number
  upstream_fail_total: number
  me_writers_active: number
  me_writers_warm: number
  uptime_seconds: number
  user_metrics: Record<string, UserLiveMetric>
}

export interface UserLiveMetric {
  octets_from_client: number
  octets_to_client: number
  connections: number
  unique_ips: number
}

export interface ProxyStatus {
  running: boolean
  port: number
  uptime?: string
  uptime_seconds?: number
  container_id?: string
  started_at?: string
  conns_current?: number
  conns_total?: number
  traffic_in?: number
  traffic_out?: number
  instances?: InstanceStatus[]
}

export interface InstanceStatus {
  port: number
  running: boolean
  label: string
}

export interface HealthStatus {
  docker: string
  container: string
  port: string
  metrics: string
}

export interface DockerStatus {
  installed: boolean
  version?: string
}

export interface EngineStatus {
  version: string
  image_exists: boolean
}

export interface BuildResult {
  method: string
  version: string
  message: string
}

export interface UpdateStatus {
  current: string
  latest: string
  update_available: boolean
  url?: string
  mode: 'docker' | 'binary'
}

export interface UpdateResult {
  ok?: boolean
  previous_version: string
  new_version: string
  binary_path?: string
  backup_path?: string
  image_pulled?: string
  web_image_pulled?: string
  container_name?: string
  web_container_name?: string
  web_dist_path?: string
  message?: string
}

export interface BackupInfo {
  filename: string
  size: number
  created_at: string
}

export interface OSType {
  family: string
  version: string
  arch: string
}

export interface ServiceStatus {
  supported: boolean
  installed: boolean
  active: string
  enabled: boolean
  pid?: string
  uptime?: string
}

export interface LoginRequest {
  username: string
  password: string
}

export interface LoginResponse {
  access_token: string
  refresh_token: string
}

export interface AuthTokens {
  accessToken: string
  refreshToken: string
}
