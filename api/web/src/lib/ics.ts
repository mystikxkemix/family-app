import type { FamilyEvent } from './types';
import { downloadFile } from './format';

// Builds a one-event iCalendar file (RFC 5545), opened by the calendar app of
// the phone or the computer: the web version of "Add to my calendar".

/** Escapes text values: backslash, semicolon, comma and new lines. */
function escape(text: string): string {
  return text.replace(/\\/g, '\\\\').replace(/;/g, '\\;').replace(/,/g, '\\,').replace(/\r?\n/g, '\\n');
}

/** UTC date-time "20261120T193000Z". */
function utc(date: Date): string {
  return date.toISOString().replace(/[-:]/g, '').replace(/\.\d{3}/, '');
}

/** Local calendar date "20261120" (all-day events have no time zone). */
function localDate(date: Date): string {
  const pad = (value: number) => String(value).padStart(2, '0');
  return `${date.getFullYear()}${pad(date.getMonth() + 1)}${pad(date.getDate())}`;
}

/** Lines over 75 octets must be folded (approximated with characters). */
function fold(line: string): string {
  const parts: string[] = [];
  for (let start = 0; start < line.length; start += 74) {
    parts.push((start === 0 ? '' : ' ') + line.slice(start, start + 74));
  }
  return parts.join('\r\n');
}

export function eventToIcs(event: FamilyEvent): string {
  const start = new Date(event.startsAt);
  const end = event.endsAt ? new Date(event.endsAt) : undefined;
  const lines = ['BEGIN:VCALENDAR', 'VERSION:2.0', 'PRODID:-//Family//Family app//EN', 'CALSCALE:GREGORIAN', 'BEGIN:VEVENT'];
  lines.push(`UID:${event.id}@family-app`);
  lines.push(`DTSTAMP:${utc(new Date())}`);
  if (event.allDay) {
    // DTEND of an all-day event is the day after the last day.
    const lastDay = end ?? start;
    const dayAfter = new Date(lastDay.getFullYear(), lastDay.getMonth(), lastDay.getDate() + 1);
    lines.push(`DTSTART;VALUE=DATE:${localDate(start)}`);
    lines.push(`DTEND;VALUE=DATE:${localDate(dayAfter)}`);
  } else {
    lines.push(`DTSTART:${utc(start)}`);
    lines.push(`DTEND:${utc(end ?? new Date(start.getTime() + 3_600_000))}`);
  }
  lines.push(`SUMMARY:${escape(event.title)}`);
  if (event.location) lines.push(`LOCATION:${escape(event.location)}`);
  if (event.notes) lines.push(`DESCRIPTION:${escape(event.notes)}`);
  lines.push('END:VEVENT', 'END:VCALENDAR');
  return lines.map(fold).join('\r\n') + '\r\n';
}

export function downloadEvent(event: FamilyEvent) {
  const name = event.title.replace(/[^\p{L}\p{N} _-]/gu, '').trim() || 'event';
  downloadFile(`${name}.ics`, eventToIcs(event), 'text/calendar;charset=utf-8');
}
