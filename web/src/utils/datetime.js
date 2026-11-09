/** Date/time formatting: date YYYY-MM-DD, time HH:MM (24-hour). */

function pad(n) {
  return String(n).padStart(2, '0')
}

const RE_DATE = /^(\d{4})-(\d{2})-(\d{2})$/
const RE_TIME = /^(\d{1,2}):(\d{2})(?::\d{2})?$/
// YYYY-MM-DD[T ]HH:MM[:SS][optional fraction/timezone]
const RE_DATETIME =
  /^(\d{4})-(\d{2})-(\d{2})[T ](\d{1,2}):(\d{2})(?::\d{2})?(?:\.\d+)?(?:Z|[+-]\d{2}:?\d{2})?$/i

/**
 * Parse to local calendar parts without UTC date-only shift.
 * Returns { y, m, d, h, min } or null.
 */
function parseParts(v) {
  if (v == null || v === '') return null

  if (v instanceof Date) {
    if (Number.isNaN(v.getTime())) return null
    return {
      y: v.getFullYear(),
      m: v.getMonth() + 1,
      d: v.getDate(),
      h: v.getHours(),
      min: v.getMinutes(),
    }
  }

  const s = String(v).trim()

  let m = s.match(RE_DATE)
  if (m) {
    return { y: +m[1], m: +m[2], d: +m[3], h: 0, min: 0 }
  }

  m = s.match(RE_TIME)
  if (m) {
    return { y: 0, m: 0, d: 0, h: +m[1], min: +m[2] }
  }

  m = s.match(RE_DATETIME)
  if (m) {
    // Prefer component extraction for naive (no Z/offset) strings so local wall time is kept.
    const hasTz = /(?:Z|[+-]\d{2}:?\d{2})$/i.test(s)
    if (!hasTz) {
      return { y: +m[1], m: +m[2], d: +m[3], h: +m[4], min: +m[5] }
    }
  }

  // RFC3339 / ISO with timezone (or other Date-parseable values)
  const dt = new Date(s)
  if (Number.isNaN(dt.getTime())) return null
  return {
    y: dt.getFullYear(),
    m: dt.getMonth() + 1,
    d: dt.getDate(),
    h: dt.getHours(),
    min: dt.getMinutes(),
  }
}

/** Format as YYYY-MM-DD */
export function formatDate(v) {
  if (v == null || v === '') return '—'
  const p = parseParts(v)
  if (!p || !p.y) return String(v)
  return `${p.y}-${pad(p.m)}-${pad(p.d)}`
}

/** Format as HH:MM (24-hour) */
export function formatTime(v) {
  if (v == null || v === '') return '—'
  const p = parseParts(v)
  if (!p) return String(v)
  // Time-only or full datetime both have h/min
  return `${pad(p.h)}:${pad(p.min)}`
}

/** Format as YYYY-MM-DD HH:MM */
export function formatDateTime(v) {
  if (v == null || v === '') return '—'
  const p = parseParts(v)
  if (!p || !p.y) return String(v)
  return `${p.y}-${pad(p.m)}-${pad(p.d)} ${pad(p.h)}:${pad(p.min)}`
}

/** Value for report form inputs: YYYY-MM-DD HH:MM (24-hour) */
export function toLocalInput(v) {
  if (v == null || v === '') return ''
  const p = parseParts(v)
  if (!p || !p.y) return ''
  return `${p.y}-${pad(p.m)}-${pad(p.d)} ${pad(p.h)}:${pad(p.min)}`
}

/**
 * Convert form datetime (YYYY-MM-DD HH:MM or datetime-local) to API form "YYYY-MM-DD HH:MM:00".
 */
export function fromLocalInput(s) {
  if (!s) return null
  const normalized = String(s).trim().replace('T', ' ')
  const m = normalized.match(
    /^(\d{4})-(\d{2})-(\d{2})[ ](\d{1,2}):(\d{2})(?::(\d{2}))?$/,
  )
  if (!m) return null
  return `${m[1]}-${m[2]}-${m[3]} ${pad(+m[4])}:${pad(+m[5])}:${pad(+(m[6] || 0))}`
}

/** Current local month as YYYY-MM */
export function currentMonth() {
  const d = new Date()
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}`
}
