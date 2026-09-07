export function fmtMs(value) {
  if (!Number.isFinite(value) || value <= 0) return '-'
  return `${Math.round(value)} ms`
}

export function fmtRate(value) {
  if (!Number.isFinite(value) || value <= 0) return '-'
  return value.toFixed(value >= 100 ? 0 : 1)
}

export function fmtNum(value) {
  if (!Number.isFinite(value)) return '0'
  return new Intl.NumberFormat('en-US').format(value)
}

export function fmtPercent(value) {
  if (!Number.isFinite(value)) return '0%'
  return `${(value * 100).toFixed(2)}%`
}

export function fmtBytes(value) {
  if (!Number.isFinite(value) || value <= 0) return '0 B'
  if (value < 1024) return `${value} B`
  if (value < 1024 * 1024) return `${(value / 1024).toFixed(1)} KB`
  return `${(value / (1024 * 1024)).toFixed(2)} MB`
}

export function fmtTime(value) {
  const d = new Date(value)
  if (Number.isNaN(d.getTime())) return value || '-'
  return d.toLocaleTimeString([], {
    hour: '2-digit', minute: '2-digit', second: '2-digit', hour12: false,
  })
}

export function fmtDate(value) {
  const d = new Date(value)
  if (Number.isNaN(d.getTime())) return value || '-'
  return d.toLocaleTimeString([], {
    hour: '2-digit', minute: '2-digit', second: '2-digit', hour12: false,
  })
}

export function fmtDateTime(value) {
  const d = new Date(value)
  if (Number.isNaN(d.getTime())) return value || '-'
  return d.toLocaleString([], {
    year: 'numeric', month: 'short', day: '2-digit',
    hour: '2-digit', minute: '2-digit', second: '2-digit', hour12: false,
  })
}

export function isCompleted(item) {
  return (item.status_code || 0) > 0 || !!item.response_raw_path || !!item.error_text
}

export function statusText(item) {
  return isCompleted(item) ? String(item.status_code || 0) : 'LIVE'
}

export function statusClass(code) {
  if (code === 0) return 'status-live'
  if (code >= 500) return 'status-err'
  if (code >= 400) return 'status-mid'
  return 'status-ok'
}

export function latencyTone(ms) {
  if (!Number.isFinite(ms) || ms <= 0) return ''
  if (ms >= 20000) return 'tone-critical'
  if (ms >= 8000) return 'tone-hot'
  if (ms >= 2500) return 'tone-warm'
  return 'tone-cool'
}

export function rateTone(rate) {
  if (!Number.isFinite(rate) || rate <= 0) return ''
  if (rate < 10) return 'tone-critical'
  if (rate < 25) return 'tone-hot'
  if (rate < 45) return 'tone-warm'
  return 'tone-good'
}

export function cacheTone(pct) {
  if (!Number.isFinite(pct) || pct <= 0) return ''
  if (pct >= 50) return 'tone-good'
  if (pct >= 20) return 'tone-warm'
  return 'tone-hot'
}

export function shortenBackendUrl(url) {
  return String(url || '').replace(/^https?:\/\//, '').replace(/\/+$/, '')
}

export function buildStructuredResponse(raw, rec) {
  if (!raw) return ''
  try {
    const parsed = JSON.parse(raw)
    if (!rec?.is_streaming) {
      return JSON.stringify(parsed, null, 2)
    }
    return raw
  } catch {
    return raw
  }
}
