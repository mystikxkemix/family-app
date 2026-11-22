// Small helpers for dates, sizes, links and files. Locale-aware where it shows text.

/** Local date "YYYY-MM-DD" for <input type="date">. */
export function toDateInput(date: Date): string {
  const pad = (value: number) => String(value).padStart(2, '0');
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}`;
}

/** Local date-time "YYYY-MM-DDTHH:mm" for <input type="datetime-local">. */
export function toDateTimeInput(date: Date): string {
  const pad = (value: number) => String(value).padStart(2, '0');
  return `${toDateInput(date)}T${pad(date.getHours())}:${pad(date.getMinutes())}`;
}

/** Parses the value of a date or datetime-local input as local time. */
export function fromInput(value: string): Date {
  return value.length <= 10 ? new Date(`${value}T00:00`) : new Date(value);
}

export function startOfDay(date: Date): Date {
  return new Date(date.getFullYear(), date.getMonth(), date.getDate());
}

/** A date in the past, with its year: "12 March 2025". */
export function formatDate(date: Date, locale: string): string {
  return new Intl.DateTimeFormat(locale, { day: 'numeric', month: 'long', year: 'numeric' }).format(date);
}

export function formatDay(date: Date, locale: string): string {
  return new Intl.DateTimeFormat(locale, { weekday: 'long', day: 'numeric', month: 'long' }).format(date);
}

export function formatTime(date: Date, locale: string): string {
  return new Intl.DateTimeFormat(locale, { hour: 'numeric', minute: '2-digit' }).format(date);
}

export function formatDateTime(date: Date, locale: string): string {
  return new Intl.DateTimeFormat(locale, { dateStyle: 'medium', timeStyle: 'short' }).format(date);
}

/** "3 days ago", "in 2 hours"... */
export function formatRelative(date: Date, locale: string): string {
  const seconds = (date.getTime() - Date.now()) / 1000;
  const units: [Intl.RelativeTimeFormatUnit, number][] = [
    ['year', 31_536_000],
    ['month', 2_592_000],
    ['day', 86_400],
    ['hour', 3_600],
    ['minute', 60],
  ];
  const format = new Intl.RelativeTimeFormat(locale, { numeric: 'auto' });
  for (const [unit, size] of units) {
    if (Math.abs(seconds) >= size) return format.format(Math.round(seconds / size), unit);
  }
  return format.format(Math.round(seconds), 'second');
}

export function formatBytes(bytes: number, locale: string): string {
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  let value = bytes;
  let unit = 0;
  while (value >= 1024 && unit < units.length - 1) {
    value /= 1024;
    unit++;
  }
  return `${new Intl.NumberFormat(locale, { maximumFractionDigits: unit >= 3 ? 2 : 0 }).format(value)} ${units[unit]}`;
}

/** Names joined the local way: "Alex and Sam" / "Alex et Sam". */
export function formatNames(names: string[], locale: string): string {
  return new Intl.ListFormat(locale, { type: 'conjunction' }).format(names);
}

/** Apple Maps search for a name and an address (opens Google Maps elsewhere). */
export function mapsLink(name: string, address: string): string {
  const query = [name, address].filter(Boolean).join(', ');
  return `https://maps.apple.com/?q=${encodeURIComponent(query)}`;
}

/** Copies text, with a fallback for plain-http pages where the Clipboard API is off. */
export async function copyText(text: string): Promise<boolean> {
  try {
    if (navigator.clipboard && window.isSecureContext) {
      await navigator.clipboard.writeText(text);
      return true;
    }
  } catch {
    // Fall back below.
  }
  const area = document.createElement('textarea');
  area.value = text;
  area.setAttribute('readonly', '');
  area.style.position = 'fixed';
  area.style.opacity = '0';
  document.body.appendChild(area);
  area.select();
  const copied = document.execCommand('copy');
  document.body.removeChild(area);
  return copied;
}

/** Saves a file built in the page (for example an .ics event). */
export function downloadFile(name: string, content: string, type: string) {
  const url = URL.createObjectURL(new Blob([content], { type }));
  const link = document.createElement('a');
  link.href = url;
  link.download = name;
  document.body.appendChild(link);
  link.click();
  document.body.removeChild(link);
  window.setTimeout(() => URL.revokeObjectURL(url), 1000);
}
