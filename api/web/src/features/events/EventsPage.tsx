import { useState } from 'react';
import { useI18n } from '../../i18n';
import { api } from '../../lib/api';
import { useLoader } from '../../lib/hooks';
import { useSession } from '../../lib/session';
import { formatDay, formatTime, fromInput, startOfDay, toDateInput, toDateTimeInput } from '../../lib/format';
import { downloadEvent } from '../../lib/ics';
import type { FamilyEvent } from '../../lib/types';
import { useFeedback } from '../../components/Feedback';
import { EmptyState, Field, IconButton, Loading, Modal, PageHeader } from '../../components/ui';
import { Icon } from '../../components/Icon';

/** Upcoming shared events, grouped by day. */
export function EventsPage() {
  const { t, locale } = useI18n();
  const { memberName } = useSession();
  const { run } = useFeedback();
  // From the start of today, so today's past events stay visible.
  const events = useLoader(() => api.get<FamilyEvent[]>('events', { from: startOfDay(new Date()).toISOString() }));
  const [editing, setEditing] = useState<FamilyEvent | 'new'>();

  const days = new Map<number, FamilyEvent[]>();
  for (const event of events.data ?? []) {
    const day = startOfDay(new Date(event.startsAt)).getTime();
    days.set(day, [...(days.get(day) ?? []), event]);
  }
  const sortedDays = [...days.entries()].sort(([a], [b]) => a - b);

  const remove = (event: FamilyEvent) => {
    if (!window.confirm(t('events.deleteConfirm', { title: event.title }))) return;
    events.setData((current) => current?.filter((other) => other.id !== event.id));
    void run(() => api.delete(`events/${event.id}`));
  };

  return (
    <section className="page">
      <PageHeader title={t('events.title')} actions={<IconButton icon="plus" label={t('events.new')} kind="primary" onClick={() => setEditing('new')} />} />
      {events.loading && <Loading />}
      {events.data && events.data.length === 0 && <EmptyState icon="events" title={t('events.emptyTitle')} body={t('events.emptyBody')} />}
      {sortedDays.map(([day, dayEvents]) => (
        <div key={day}>
          <h2 className="section-title capitalize">{formatDay(new Date(day), locale)}</h2>
          <ul className="cards">
            {dayEvents
              .sort((a, b) => a.startsAt.localeCompare(b.startsAt))
              .map((event) => (
                <li key={event.id} className="card row-card">
                  <button type="button" className="row-main" onClick={() => setEditing(event)}>
                    <span className="event-time muted">{event.allDay ? t('events.allDay') : formatTime(new Date(event.startsAt), locale)}</span>
                    <span className="row-text">
                      <strong>{event.title}</strong>
                      {event.location && <span className="muted small">{event.location}</span>}
                      <span className="muted tiny">{t('events.addedBy', { name: memberName(event.createdBy, t('common.someone')) })}</span>
                    </span>
                  </button>
                  <div className="row-actions">
                    <button
                      type="button"
                      className="icon-button"
                      aria-label={t('events.addCalendar')}
                      title={t('events.addCalendar')}
                      onClick={() => downloadEvent(event)}
                    >
                      <Icon name="calendarAdd" size={18} />
                    </button>
                    <button type="button" className="icon-button danger" aria-label={t('common.delete')} onClick={() => remove(event)}>
                      <Icon name="trash" size={18} />
                    </button>
                  </div>
                </li>
              ))}
          </ul>
        </div>
      ))}
      {editing && <EventEditor event={editing === 'new' ? undefined : editing} onClose={() => setEditing(undefined)} onSaved={events.reload} />}
    </section>
  );
}

/** The next full hour, the default start of a new event. */
function nextHour(): Date {
  const date = new Date();
  date.setMinutes(0, 0, 0);
  date.setHours(date.getHours() + 1);
  return date;
}

function EventEditor({ event, onClose, onSaved }: { event?: FamilyEvent; onClose: () => void; onSaved: () => Promise<void> }) {
  const { t } = useI18n();
  const { run } = useFeedback();
  const initialStart = event ? new Date(event.startsAt) : nextHour();
  const initialEnd = event?.endsAt ? new Date(event.endsAt) : new Date(initialStart.getTime() + 3_600_000);
  const [title, setTitle] = useState(event?.title ?? '');
  const [allDay, setAllDay] = useState(event?.allDay ?? false);
  const [start, setStart] = useState(toDateTimeInput(initialStart));
  const [hasEnd, setHasEnd] = useState(Boolean(event?.endsAt));
  const [end, setEnd] = useState(toDateTimeInput(initialEnd));
  const [location, setLocation] = useState(event?.location ?? '');
  const [notes, setNotes] = useState(event?.notes ?? '');
  const [busy, setBusy] = useState(false);

  // Inputs keep "YYYY-MM-DDTHH:mm"; in all-day mode only the date part shows.
  const dateValue = (value: string) => (allDay ? value.slice(0, 10) : value);
  const setFromInput = (setter: (value: string) => void, current: string) => (value: string) =>
    setter(allDay ? `${value}${current.slice(10) || 'T00:00'}` : value);

  const startDate = fromInput(start);
  const endDate = fromInput(end);
  const endBeforeStart = hasEnd && endDate < startDate;

  const save = async () => {
    setBusy(true);
    // An all-day event is stored at the start of its day (local time).
    const startsAt = allDay ? startOfDay(startDate) : startDate;
    const body: Record<string, unknown> = {
      title: title.trim(),
      startsAt: startsAt.toISOString(),
      allDay,
      location: location.trim(),
      notes: notes.trim(),
    };
    if (hasEnd) body.endsAt = (allDay ? startOfDay(endDate) : endDate).toISOString();
    else if (event) body.clearEnd = true;
    const saved = await run(() => (event ? api.patch(`events/${event.id}`, body) : api.post('events', body)));
    setBusy(false);
    if (saved) {
      await onSaved();
      onClose();
    }
  };

  return (
    <Modal
      title={event ? t('events.edit') : t('events.new')}
      onClose={onClose}
      onSubmit={() => void save()}
      canSubmit={title.trim() !== '' && !endBeforeStart}
      busy={busy}
      footer={
        event && (
          <button type="button" className="button" onClick={() => downloadEvent(event)}>
            <Icon name="calendarAdd" size={18} /> {t('events.addCalendar')}
          </button>
        )
      }
    >
      <Field label={t('events.titleField')}>
        <input value={title} onChange={(e) => setTitle(e.target.value)} maxLength={120} />
      </Field>
      <Field label={t('events.location')}>
        <input value={location} onChange={(e) => setLocation(e.target.value)} maxLength={200} />
      </Field>
      <label className="toggle">
        <input type="checkbox" checked={allDay} onChange={(e) => setAllDay(e.target.checked)} />
        {t('events.allDay')}
      </label>
      <Field label={t('events.starts')}>
        <input
          type={allDay ? 'date' : 'datetime-local'}
          value={dateValue(start)}
          onChange={(e) => setFromInput(setStart, start)(e.target.value)}
          required
        />
      </Field>
      <label className="toggle">
        <input type="checkbox" checked={hasEnd} onChange={(e) => setHasEnd(e.target.checked)} />
        {t('events.endTime')}
      </label>
      {hasEnd && (
        <Field label={t('events.ends')}>
          <input
            type={allDay ? 'date' : 'datetime-local'}
            value={dateValue(end)}
            min={allDay ? toDateInput(startDate) : start}
            onChange={(e) => setFromInput(setEnd, end)(e.target.value)}
          />
        </Field>
      )}
      <Field label={t('events.notes')}>
        <textarea value={notes} onChange={(e) => setNotes(e.target.value)} rows={3} maxLength={2000} />
      </Field>
    </Modal>
  );
}
