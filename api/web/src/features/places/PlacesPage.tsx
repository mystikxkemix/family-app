import { useState } from 'react';
import { useI18n } from '../../i18n';
import { api } from '../../lib/api';
import { useLoader } from '../../lib/hooks';
import { useMe } from '../../lib/session';
import { mapsLink } from '../../lib/format';
import { placeCategories, placeStatuses, type Place, type PlaceCategory, type PlaceStatus } from '../../lib/types';
import { useFeedback } from '../../components/Feedback';
import { EmptyState, Field, IconButton, Loading, Modal, PageHeader, Segmented } from '../../components/ui';
import { Icon } from '../../components/Icon';
import { InterestLine } from '../../components/InterestLine';

const categoryEmoji: Record<PlaceCategory, string> = {
  restaurant: '🍽️',
  bar: '🍷',
  cafe: '☕️',
  activity: '🎯',
  shop: '🛍️',
  other: '📍',
};

/** Restaurants, bars and activities to try together. */
export function PlacesPage() {
  const { t } = useI18n();
  const me = useMe();
  const { run } = useFeedback();
  const places = useLoader(() => api.get<Place[]>('places'));
  const [status, setStatus] = useState<PlaceStatus>('toTry');
  const [editing, setEditing] = useState<Place | 'new'>();

  const visible = (places.data ?? [])
    .filter((place) => place.status === status)
    // Tried places: best rated first.
    .sort((a, b) => (status === 'tried' ? b.rating - a.rating : 0));

  const update = (place: Place, body: Record<string, unknown>) =>
    void run(async () => {
      const updated = await api.patch<Place>(`places/${place.id}`, body);
      places.setData((current) => current?.map((other) => (other.id === updated.id ? updated : other)));
    });

  const remove = (place: Place) => {
    places.setData((current) => current?.filter((other) => other.id !== place.id));
    void run(() => api.delete(`places/${place.id}`));
  };

  return (
    <section className="page">
      <PageHeader title={t('places.title')} actions={<IconButton icon="plus" label={t('places.new')} kind="primary" onClick={() => setEditing('new')} />} />
      <Segmented
        label={t('places.title')}
        value={status}
        onChange={setStatus}
        options={placeStatuses.map((value) => ({ value, label: t(`status.${value}`) }))}
      />
      {places.loading && <Loading />}
      {places.data && visible.length === 0 && <EmptyState icon="places" title={t('places.emptyTitle')} body={t('places.emptyBody')} />}
      <ul className="cards">
        {visible.map((place) => {
          const interested = place.interestedUserIds.includes(me.user.id);
          return (
            <li key={place.id} className="card row-card">
              <button type="button" className="row-main" onClick={() => setEditing(place)}>
                <span className="emoji" aria-hidden="true">
                  {categoryEmoji[place.category]}
                </span>
                <span className="row-text">
                  <strong>{place.name}</strong>
                  {place.address && <span className="muted small">{place.address}</span>}
                  {place.note && <span className="muted small clamp">{place.note}</span>}
                  {place.status === 'tried' && place.rating > 0 ? (
                    <span className="stars" aria-label={`${place.rating}/5`}>
                      {'★'.repeat(place.rating)}
                      {'☆'.repeat(5 - place.rating)}
                    </span>
                  ) : (
                    <InterestLine userIds={place.interestedUserIds} />
                  )}
                </span>
              </button>
              <div className="row-actions">
                <a className="icon-button" href={mapsLink(place.name, place.address)} target="_blank" rel="noreferrer" aria-label={t('places.openMaps')} title={t('places.openMaps')}>
                  <Icon name="map" size={18} />
                </a>
                {place.url && (
                  <a className="icon-button" href={place.url} target="_blank" rel="noreferrer" aria-label={t('places.openWebsite')} title={t('places.openWebsite')}>
                    <Icon name="link" size={18} />
                  </a>
                )}
                {place.status === 'toTry' ? (
                  <>
                    <button
                      type="button"
                      className={`icon-button heart${interested ? ' active' : ''}`}
                      aria-label={interested ? t('interest.notForMe') : t('interest.wantGo')}
                      title={interested ? t('interest.notForMe') : t('interest.wantGo')}
                      aria-pressed={interested}
                      onClick={() => update(place, { interested: !interested })}
                    >
                      <Icon name="heart" size={18} filled={interested} />
                    </button>
                    <button type="button" className="icon-button" aria-label={t('places.markTried')} title={t('places.markTried')} onClick={() => update(place, { status: 'tried' })}>
                      <Icon name="check" size={18} />
                    </button>
                  </>
                ) : (
                  <>
                    <button type="button" className="icon-button" aria-label={t('places.markToTry')} title={t('places.markToTry')} onClick={() => update(place, { status: 'toTry' })}>
                      <Icon name="back" size={18} />
                    </button>
                  <select
                    className="compact-select"
                    aria-label={t('places.rate')}
                    value={place.rating}
                    onChange={(e) => update(place, { rating: Number(e.target.value) })}
                  >
                    <option value={0}>{t('places.rate')}</option>
                    {[5, 4, 3, 2, 1].map((stars) => (
                      <option key={stars} value={stars}>
                        {'★'.repeat(stars)}
                      </option>
                    ))}
                  </select>
                  </>
                )}
                <button type="button" className="icon-button danger" aria-label={t('common.delete')} onClick={() => remove(place)}>
                  <Icon name="trash" size={18} />
                </button>
              </div>
            </li>
          );
        })}
      </ul>
      {editing && <PlaceEditor place={editing === 'new' ? undefined : editing} onClose={() => setEditing(undefined)} onSaved={places.reload} />}
    </section>
  );
}

function PlaceEditor({ place, onClose, onSaved }: { place?: Place; onClose: () => void; onSaved: () => Promise<void> }) {
  const { t } = useI18n();
  const { run } = useFeedback();
  const [name, setName] = useState(place?.name ?? '');
  const [category, setCategory] = useState<PlaceCategory>(place?.category ?? 'restaurant');
  const [address, setAddress] = useState(place?.address ?? '');
  const [website, setWebsite] = useState(place?.url ?? '');
  const [note, setNote] = useState(place?.note ?? '');
  const [busy, setBusy] = useState(false);

  const save = async () => {
    setBusy(true);
    let link = website.trim();
    if (link && !link.includes('://')) link = `https://${link}`;
    const body = { name: name.trim(), category, address: address.trim(), url: link, note: note.trim() };
    const saved = await run(() => (place ? api.patch(`places/${place.id}`, body) : api.post('places', body)));
    setBusy(false);
    if (saved) {
      await onSaved();
      onClose();
    }
  };

  return (
    <Modal title={place ? t('places.edit') : t('places.new')} onClose={onClose} onSubmit={() => void save()} canSubmit={name.trim() !== ''} busy={busy}>
      <Field label={t('places.name')}>
        <input value={name} onChange={(e) => setName(e.target.value)} maxLength={120} />
      </Field>
      <Field label={t('places.category')}>
        <select value={category} onChange={(e) => setCategory(e.target.value as PlaceCategory)}>
          {placeCategories.map((value) => (
            <option key={value} value={value}>
              {categoryEmoji[value]} {t(`cat.${value}`)}
            </option>
          ))}
        </select>
      </Field>
      <Field label={t('places.address')}>
        <input value={address} onChange={(e) => setAddress(e.target.value)} maxLength={300} autoComplete="street-address" />
      </Field>
      <Field label={t('places.website')}>
        <input value={website} onChange={(e) => setWebsite(e.target.value)} maxLength={1000} inputMode="url" autoCapitalize="none" />
      </Field>
      <Field label={t('places.note')}>
        <textarea value={note} onChange={(e) => setNote(e.target.value)} rows={3} maxLength={1000} />
      </Field>
    </Modal>
  );
}
