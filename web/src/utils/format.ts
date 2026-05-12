export function formatBytes(bytes: number): string {
  if (bytes === undefined || bytes === null || isNaN(bytes)) return '0 B'
  if (bytes === 0) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  const k = 1024
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + ' ' + units[i]
}

export function formatDuration(seconds: number): string {
  if (seconds < 60) return `${seconds}s`
  if (seconds < 3600) return `${Math.floor(seconds / 60)}m ${seconds % 60}s`
  if (seconds < 86400) {
    const h = Math.floor(seconds / 3600)
    const m = Math.floor((seconds % 3600) / 60)
    return `${h}h ${m}m`
  }
  const d = Math.floor(seconds / 86400)
  const h = Math.floor((seconds % 86400) / 3600)
  return `${d}d ${h}h`
}

export function formatDate(timestamp: number): string {
  return new Date(timestamp * 1000).toLocaleString()
}

export function formatTimestamp(ts: number): string {
  return new Date(ts * 1000).toLocaleDateString()
}

export function formatISODate(iso: string): string {
  if (!iso || iso === '0') return 'Never'
  try {
    return new Date(iso).toLocaleDateString()
  } catch {
    return iso
  }
}

export function formatNumber(n: number): string {
  if (n >= 1_000_000) return (n / 1_000_000).toFixed(1) + 'M'
  if (n >= 1_000) return (n / 1_000).toFixed(1) + 'K'
  return n.toString()
}

export function sleep(ms: number) {
  return new Promise((r) => setTimeout(r, ms))
}

export function parseJSONTags(tags?: string): string[] {
  if (!tags || tags === '[]') return []
  try { return JSON.parse(tags) } catch { return [] }
}
