async function fetchJSON(url, options) {
  const resp = await fetch(url, options)
  if (!resp.ok) {
    throw new Error(await resp.text())
  }
  return resp.json()
}

export function queryString(obj) {
  const params = new URLSearchParams()
  for (const [key, value] of Object.entries(obj)) {
    if (value !== undefined && value !== null && value !== '') {
      params.set(key, value)
    }
  }
  return params.toString()
}

export async function fetchStats(filters) {
  const hours = Number.parseInt(filters.since_hours || '1', 10) || 1
  return fetchJSON(`/_proxy/stats?${queryString({ hours, ...filters })}`)
}

export async function fetchRequests(limit, offset, filters) {
  return fetchJSON(`/_proxy/requests?${queryString({ limit, offset, ...filters })}`)
}

export async function fetchRequest(id) {
  return fetchJSON(`/_proxy/request/${encodeURIComponent(id)}`)
}

export async function deleteRequest(id) {
  return fetchJSON(`/_proxy/request/${encodeURIComponent(id)}`, { method: 'DELETE' })
}

export async function fetchRaw(id, part) {
  const resp = await fetch(`/_proxy/raw/${encodeURIComponent(id)}/${part}`)
  if (!resp.ok) {
    return null
  }
  const text = await resp.text()
  try {
    return JSON.stringify(JSON.parse(text), null, 2)
  } catch {
    return text
  }
}

export async function fetchModels() {
  const data = await fetchJSON('/_proxy/models')
  return Array.isArray(data.items) ? data.items : []
}

export async function fetchBackends() {
  const data = await fetchJSON('/_proxy/backends')
  return Array.isArray(data.items) ? data.items : []
}

export async function fetchStatsByBackend(hours) {
  const data = await fetchJSON(`/_proxy/stats-by-backend?${queryString({ hours })}`)
  return Array.isArray(data.items) ? data.items : []
}

export async function fetchDailyStats(days, filters = {}) {
  const data = await fetchJSON(`/_proxy/daily-stats?${queryString({ days, ...filters })}`)
  return {
    items: Array.isArray(data.items) ? data.items : [],
    days: data.days || days,
  }
}
