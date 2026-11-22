import { useState } from 'react';
import { useI18n } from '../../i18n';
import { api } from '../../lib/api';
import { navigate, useLoader } from '../../lib/hooks';
import { useMe } from '../../lib/session';
import { mediaStatuses, type Media, type MediaStatus } from '../../lib/types';
import { useFeedback } from '../../components/Feedback';
import { EmptyState, IconButton, Loading, Modal, PageHeader, Segmented } from '../../components/ui';
import { Icon } from '../../components/Icon';
import { InterestLine } from '../../components/InterestLine';
import { MediaEditor } from './MediaEditor';
import { Poster } from './Poster';

/** Movies and shows to watch together. */
export function WatchlistPage() {
  const { t } = useI18n();
  const me = useMe();
  const { run } = useFeedback();
  const items = useLoader(() => api.get<Media[]>('watchlist'));
  const [status, setStatus] = useState<MediaStatus>('toWatch');
  const [adding, setAdding] = useState(false);
  const [pick, setPick] = useState<Media>();

  const visible = (items.data ?? []).filter((media) => media.status === status);

  const update = (media: Media, body: Record<string, unknown>) =>
    void run(async () => {
      const updated = await api.patch<Media>(`watchlist/${media.id}`, body);
      items.setData((current) => current?.map((other) => (other.id === updated.id ? updated : other)));
    });

  const remove = (media: Media) => {
    items.setData((current) => current?.filter((other) => other.id !== media.id));
    void run(() => api.delete(`watchlist/${media.id}`));
  };

  /** A random title to watch; titles that everyone wants come first. */
  const pickRandom = () => {
    const candidates = (items.data ?? []).filter((media) => media.status === 'toWatch');
    const everyone = candidates.filter((media) => media.interestedUserIds.length >= Math.max(me.members.length, 1));
    const pool = everyone.length > 0 ? everyone : candidates;
    setPick(pool[Math.floor(Math.random() * pool.length)]);
  };

  return (
    <section className="page">
      <PageHeader
        title={t('wl.title')}
        actions={
          <>
            <IconButton
              icon="dice"
              label={t('wl.pick')}
              onClick={pickRandom}
              compact
              disabled={!(items.data ?? []).some((media) => media.status === 'toWatch')}
            />
            <IconButton icon="plus" label={t('wl.add')} kind="primary" onClick={() => setAdding(true)} compact />
          </>
        }
      />
      <Segmented
        label={t('wl.title')}
        value={status}
        onChange={setStatus}
        options={mediaStatuses.map((value) => ({ value, label: t(`status.${value}`) }))}
      />
      {items.loading && <Loading />}
      {items.data && visible.length === 0 && <EmptyState icon="watchlist" title={t('wl.emptyTitle')} body={t('wl.emptyBody')} />}
      <ul className="cards">
        {visible.map((media) => {
          const interested = media.interestedUserIds.includes(me.user.id);
          return (
            <li key={media.id} className="card row-card">
              <button type="button" className="row-main" onClick={() => navigate(`/watchlist/${media.id}`)}>
                <Poster url={media.posterUrl} kind={media.kind} />
                <span className="row-text">
                  <strong>
                    {media.title} {media.year ? <span className="muted">{media.year}</span> : null}
                  </strong>
                  {media.note && <span className="muted small clamp">{media.note}</span>}
                  <InterestLine userIds={media.interestedUserIds} />
                  {(media.downloadRequestedAt || media.jellyfin?.available) && (
                    <span className="badges">
                      {media.jellyfin?.available && (
                        <span className="badge accent">
                          <Icon name="play" size={12} /> {t('wl.onJellyfin')}
                        </span>
                      )}
                      {media.downloadRequestedAt && (
                        <span className="badge success">
                          <Icon name="download" size={14} /> {t('wl.downloadRequested')}
                        </span>
                      )}
                    </span>
                  )}
                </span>
              </button>
              <div className="row-actions">
                <button
                  type="button"
                  className={`icon-button heart${interested ? ' active' : ''}`}
                  aria-label={interested ? t('interest.notForMe') : t('interest.want')}
                  title={interested ? t('interest.notForMe') : t('interest.want')}
                  aria-pressed={interested}
                  onClick={() => update(media, { interested: !interested })}
                >
                  <Icon name="heart" size={18} filled={interested} />
                </button>
                <select
                  className="compact-select"
                  aria-label={t('wl.moveTo')}
                  value={media.status}
                  onChange={(e) => update(media, { status: e.target.value })}
                >
                  {mediaStatuses.map((value) => (
                    <option key={value} value={value}>
                      {t(`status.${value}`)}
                    </option>
                  ))}
                </select>
                <button type="button" className="icon-button danger" aria-label={t('common.delete')} onClick={() => remove(media)}>
                  <Icon name="trash" size={18} />
                </button>
              </div>
            </li>
          );
        })}
      </ul>
      {adding && <MediaEditor onClose={() => setAdding(false)} onSaved={items.reload} />}
      {pick && (
        <Modal
          title={t('wl.pickTitle')}
          onClose={() => setPick(undefined)}
          footer={
            <>
              <button type="button" className="button" onClick={pickRandom}>
                {t('wl.pickAgain')}
              </button>
              <button
                type="button"
                className="button button-primary"
                onClick={() => {
                  update(pick, { status: 'watching' });
                  setPick(undefined);
                }}
              >
                {t('wl.letsWatch')}
              </button>
            </>
          }
        >
          <div className="pick">
            <Poster url={pick.posterUrl} kind={pick.kind} large />
            <strong>{pick.title}</strong>
          </div>
        </Modal>
      )}
    </section>
  );
}
