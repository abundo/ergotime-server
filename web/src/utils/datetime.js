/** Date/time formatting: date YYYY-MM-DD, time HH:MM (24-hour).
 *
 * Stored instants are UTC. Display uses the profile timezone when one is set,
 * otherwise the browser's IANA timezone. The browser language only picks
 * weekday names; it does not choose the zone.
 */

function pad(n) {
  return String(n).padStart(2, "0");
}

const RE_DATE = /^(\d{4})-(\d{2})-(\d{2})$/;
const RE_TIME = /^(\d{1,2}):(\d{2})(?::\d{2})?$/;
const RE_DATETIME = /^(\d{4})-(\d{2})-(\d{2})[T ](\d{1,2}):(\d{2})(?::(\d{2}))?(?:\.\d+)?(Z|[+-]\d{2}:?\d{2})?$/i;

export function browserTimeZone() {
  try {
    return Intl.DateTimeFormat().resolvedOptions().timeZone || "UTC";
  } catch {
    return "UTC";
  }
}

let zone = browserTimeZone();

/** tz is an IANA name. Blank keeps the browser timezone. */
export function setDisplayTimeZone(tz) {
  const next = String(tz || "").trim();
  zone = next || browserTimeZone();
}

export function displayTimeZone() {
  return zone || "UTC";
}

function zonedParts(date, timeZone) {
  const fmt = new Intl.DateTimeFormat("en-US", {
    timeZone: timeZone || displayTimeZone(),
    hourCycle: "h23",
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
  });
  const bag = {};
  for (const part of fmt.formatToParts(date)) {
    if (part.type !== "literal") bag[part.type] = part.value;
  }
  let h = Number(bag.hour);
  if (h === 24) h = 0;
  return {
    y: Number(bag.year),
    m: Number(bag.month),
    d: Number(bag.day),
    h,
    min: Number(bag.minute),
    s: Number(bag.second),
  };
}

/** Wall clock in timeZone → UTC Date. */
function wallToUtc(y, m, d, h, min, sec, timeZone) {
  const want = Date.UTC(y, m - 1, d, h, min, sec);
  let utc = want;
  for (let i = 0; i < 3; i++) {
    const shown = zonedParts(new Date(utc), timeZone);
    const got = Date.UTC(shown.y, shown.m - 1, shown.d, shown.h, shown.min, shown.s);
    const delta = got - want;
    if (delta === 0) break;
    utc -= delta;
  }
  return new Date(utc);
}

/**
 * Parse to calendar parts in the display timezone.
 * A date-only or time-only string is not shifted.
 * A zone-less datetime is a UTC instant.
 */
function parseParts(v) {
  if (v == null || v === "") return null;

  if (v instanceof Date) {
    if (Number.isNaN(v.getTime())) return null;
    return zonedParts(v, displayTimeZone());
  }

  const s = String(v).trim();

  let m = s.match(RE_DATE);
  if (m) {
    return { y: +m[1], m: +m[2], d: +m[3], h: 0, min: 0 };
  }

  m = s.match(RE_TIME);
  if (m) {
    return { y: 0, m: 0, d: 0, h: +m[1], min: +m[2] };
  }

  m = s.match(RE_DATETIME);
  if (m) {
    const hasTz = Boolean(m[7]);
    if (!hasTz) {
      const utc = wallToUtc(+m[1], +m[2], +m[3], +m[4], +m[5], +(m[6] || 0), "UTC");
      return zonedParts(utc, displayTimeZone());
    }
    const dt = new Date(s);
    if (Number.isNaN(dt.getTime())) return null;
    return zonedParts(dt, displayTimeZone());
  }

  const dt = new Date(s);
  if (Number.isNaN(dt.getTime())) return null;
  return zonedParts(dt, displayTimeZone());
}

/** Format as YYYY-MM-DD */
export function formatDate(v) {
  if (v == null || v === "") return "—";
  const p = parseParts(v);
  if (!p || !p.y) return String(v);
  return `${p.y}-${pad(p.m)}-${pad(p.d)}`;
}

/** Format as HH:MM (24-hour) */
export function formatTime(v) {
  if (v == null || v === "") return "—";
  const p = parseParts(v);
  if (!p) return String(v);
  return `${pad(p.h)}:${pad(p.min)}`;
}

/** Format as YYYY-MM-DD HH:MM in the display timezone */
export function formatDateTime(v) {
  if (v == null || v === "") return "—";
  const p = parseParts(v);
  if (!p || !p.y) return String(v);
  return `${p.y}-${pad(p.m)}-${pad(p.d)} ${pad(p.h)}:${pad(p.min)}`;
}

/** Value for report form inputs: YYYY-MM-DD HH:MM in the display timezone */
export function toLocalInput(v) {
  if (v == null || v === "") return "";
  const p = parseParts(v);
  if (!p || !p.y) return "";
  return `${p.y}-${pad(p.m)}-${pad(p.d)} ${pad(p.h)}:${pad(p.min)}`;
}

/**
 * Convert a display-timezone wall time to a UTC instant for the API.
 * Returns RFC3339 with a Z suffix.
 */
export function fromLocalInput(s) {
  if (!s) return null;
  const normalized = String(s).trim().replace("T", " ");
  const m = normalized.match(/^(\d{4})-(\d{2})-(\d{2})[ ](\d{1,2}):(\d{2})(?::(\d{2}))?$/);
  if (!m) return null;
  const utc = wallToUtc(+m[1], +m[2], +m[3], +m[4], +m[5], +(m[6] || 0), displayTimeZone());
  return utc.toISOString().replace(/\.\d{3}Z$/, "Z");
}

/** Current month in the display timezone, as YYYY-MM */
export function currentMonth() {
  const p = zonedParts(new Date(), displayTimeZone());
  return `${p.y}-${pad(p.m)}`;
}

/** Weekday name for a calendar date YYYY-MM-DD, using the browser language. */
export function weekdayName(date) {
  const m = String(date || "").match(/^(\d{4})-(\d{2})-(\d{2})/);
  if (!m) return "";
  const utc = new Date(Date.UTC(Number(m[1]), Number(m[2]) - 1, Number(m[3]), 12));
  return new Intl.DateTimeFormat(navigator.language || "en", {
    weekday: "long",
    timeZone: "UTC",
  }).format(utc);
}
