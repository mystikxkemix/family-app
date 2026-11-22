import { useCallback, useEffect, useState } from 'react';
import { useI18n } from '../../i18n';
import { ApiError, api } from '../../lib/api';
import { formatDate } from '../../lib/format';
import { navigate, useLoader } from '../../lib/hooks';
import { useMe } from '../../lib/session';
import { mediaStatuses, type DownloadResponse, type Media, type MediaDetails, type MediaSearchResult } from '../../lib/types';
import { useErrorText, useFeedback } from '../../components/Feedback';
import { IconButton, Loading, Modal, PageHeader, Segmented } from '../../components/ui';
import { Icon } from '../../components/Icon';
import { InterestLine } from '../../components/InterestLine';
import { MediaEditor } from './MediaEditor';
import { MediaSearch } from './MediaSearch';
import { Poster } from './Poster';
import { ReleasesDialog } from './ReleasesDialog';

/** What the details call answered. */
type DetailsState =
  | { kind: 'loading' }
  | { kind: 'ready'; details: MediaDetails }
  | { kind: 'notMatched' }
  | { kind: 'notConfigured' }
  | { kind: 'error'; error: unknown };

/**
 * One title of the watchlist: details from TMDB (synopsis, rating, cast...)
 * and our own data (status, who wants it, note, download tools).
 */
export function MediaDetailPage({ mediaId }: { mediaId: string }) {
  const { t, lang, locale } = useI18n();
  const me = useMe();
  const { run, showInfo } = useFeedback();
  const errorText = useErrorText();
  // There is no single-title endpoint: the watchlist is small, read it all.
  const items = useLoader(() => api.get<Media[]>('watchlist'), [mediaId]);
  const media = items.data?.find((item) => item.id === mediaId);
  const [details, setDetails] = useState<DetailsState>({ kind: 'loading' });
  const [editing, setEditing] = useState(false);
  const [matching, setMatching] = useState(false);
  const [showReleases, setShowReleases] = useState(false);

  const loadDetails = useCallback(async () => {
    setDetails({ kind: 'loading' });
    try {
      const found = await api.get<MediaDetails>(`watchlist/${mediaId}/details`, { lang });
      setDetails({ kind: 'ready', details: found });
    } catch (error) {
      if (error instanceof ApiError && error.code === 'not_matched') setDetails({ kind: 'notMatched' });
      else if (error instanceof ApiError && error.code === 'not_configured') setDetails({ kind: 'notConfigured' });
      else setDetails({ kind: 'error', error });
    }
  }, [mediaId, lang]);

  useEffect(() => {
    void loadDetails();
  }, [loadDetails]);

  const update = (body: Record<string, unknown>) =>
    void run(async () => {
      const updated = await api.patch<Media>(`watchlist/${mediaId}`, body);
      items.setData((current) => current?.map((other) => (other.id === updated.id ? updated : other)));
    });

  const match = async (result: MediaSearchResult) => {
    if (!result.externalIds.tmdb) return;
    const ok = await run(async () => {
      const updated = await api.post<Media>(`watchlist/${mediaId}/match`, { tmdbId: result.externalIds.tmdb });
      items.setData((current) => current?.map((other) => (other.id === updated.id ? updated : other)));
    });
    if (ok) {
      setMatching(false);
      await loadDetails();
    }
  };

  const download = () =>
    void run(async () => {
      const response = await api.post<DownloadResponse>(`watchlist/${mediaId}/download`);
      showInfo(response.alreadyInLibrary ? t('wl.alreadyInLibrary') : t('wl.downloadSent'));
      await items.reload();
    });

  const back = () => navigate('/watchlist');

  if (items.loading) return <Loading />;
  if (!media) {
    return (
      <section className="page narrow">
        <PageHeader title={t('wl.title')} onBack={back} />
        <p className="muted center">{t('err.not_found')}</p>
      </section>
    );
  }

  const integrations = me.integrations;
  const hasLibrary = media.kind === 'movie' ? integrations.radarr : integrations.sonarr;
  const interested = media.interestedUserIds.includes(me.user.id);
  const info = details.kind === 'ready' ? details.details : undefined;
  const releaseQuery = media.kind === 'movie' && media.year ? `${media.title} ${media.year}` : media.title;

  return (
    <section className="page narrow media-detail">
      <PageHeader
        title={media.title}
        onBack={back}
        actions={<IconButton icon="edit" label={t('common.edit')} onClick={() => setEditing(true)} compact />}
      />

      {info?.backdropUrl && (
        <div className="backdrop-image">
          <img src={info.backdropUrl} alt="" />
        </div>
      )}

      <div className="detail-hero">
        <Poster url={info?.posterUrl ?? media.posterUrl} kind={media.kind} large />
        <div className="detail-facts">
          <strong className="detail-title">{info?.title ?? media.title}</strong>
          {info && info.originalTitle && info.originalTitle !== info.title && <span className="muted small">{info.originalTitle}</span>}
          <span className="muted">{factsLine(media, info, t)}</span>
          {info && info.genres.length > 0 && (
            <span className="badges">
              {info.genres.map((genre) => (
                <span key={genre} className="badge">
                  {genre}
                </span>
              ))}
            </span>
          )}
          {info && info.tmdbVotes > 0 && <Rating rating={info.tmdbRating} votes={info.tmdbVotes} />}
        </div>
      </div>

      {details.kind === 'loading' && <Loading />}
      {details.kind === 'notMatched' && (
        <div className="card form-card">
          <p>{t('details.notMatched')}</p>
          <button type="button" className="button button-primary" onClick={() => setMatching(true)}>
            <Icon name="search" size={18} /> {t('details.find')}
          </button>
        </div>
      )}
      {details.kind === 'notConfigured' && me.user.isAdmin && <p className="muted small">{t('details.notConfigured')}</p>}
      {details.kind === 'error' && (
        <div className="card form-card">
          <p className="warning">{errorText(details.error)}</p>
          <button type="button" className="button" onClick={() => void loadDetails()}>
            {t('details.retry')}
          </button>
        </div>
      )}

      {info && (
        <>
          {info.overview && <p className="detail-overview">{info.overview}</p>}
          {info.directors.length > 0 && (
            <p className="muted">
              {media.kind === 'movie' ? t('details.directedBy') : t('details.createdBy')} <strong>{info.directors.join(', ')}</strong>
            </p>
          )}
          {info.cast.length > 0 && (
            <>
              <h2 className="section-title">{t('details.cast')}</h2>
              <ul className="cast-row">
                {info.cast.map((person) => (
                  <li key={`${person.name}-${person.character}`}>
                    <span className="cast-photo" aria-hidden="true">
                      {person.photoUrl ? <img src={person.photoUrl} alt="" loading="lazy" /> : initials(person.name)}
                    </span>
                    <strong className="small">{person.name}</strong>
                    {person.character && <span className="muted small">{person.character}</span>}
                  </li>
                ))}
              </ul>
            </>
          )}
          <div className="actions-row">
            {info.trailerUrl && (
              <a className="button" href={info.trailerUrl} target="_blank" rel="noopener noreferrer">
                <Icon name="play" size={18} /> {t('details.trailer')}
              </a>
            )}
            <a className="button" href={info.tmdbUrl} target="_blank" rel="noopener noreferrer">
              <Icon name="link" size={18} /> {t('details.openTmdb')}
            </a>
          </div>
        </>
      )}

      <h2 className="section-title">{t('details.ours')}</h2>
      <div className="card form-card">
        <Segmented
          label={t('wl.moveTo')}
          value={media.status}
          onChange={(status) => update({ status })}
          options={mediaStatuses.map((value) => ({ value, label: t(`status.${value}`) }))}
        />
        <div className="interest-row">
          <button
            type="button"
            className={`icon-button heart${interested ? ' active' : ''}`}
            aria-label={interested ? t('interest.notForMe') : t('interest.want')}
            title={interested ? t('interest.notForMe') : t('interest.want')}
            aria-pressed={interested}
            onClick={() => update({ interested: !interested })}
          >
            <Icon name="heart" size={20} filled={interested} />
          </button>
          <InterestLine userIds={media.interestedUserIds} />
        </div>
        {media.note && <p className="detail-note">{media.note}</p>}
        {(media.jellyfin || media.source === 'jellyfin') && (
          <span className="badges" data-testid="jellyfin-info">
            {media.jellyfin?.available ? (
              <span className="badge accent">
                <Icon name="play" size={12} /> {t('wl.onJellyfin')}
              </span>
            ) : (
              media.jellyfin && <span className="badge">{t('details.notInJellyfin')}</span>
            )}
            {media.jellyfin?.playedAt && (
              <span className="badge success">
                {t('details.watchedInJellyfin', { date: formatDate(new Date(media.jellyfin.playedAt), locale) })}
              </span>
            )}
            {media.source === 'jellyfin' && <span className="badge">{t('wl.fromJellyfin')}</span>}
          </span>
        )}
        {(hasLibrary || integrations.prowlarr) && (
          <div className="tool-buttons">
            {hasLibrary && (
              <button type="button" className="button" onClick={download}>
                <Icon name="download" size={18} /> {media.kind === 'movie' ? t('wl.downloadRadarr') : t('wl.downloadSonarr')}
              </button>
            )}
            {integrations.prowlarr && (
              <button type="button" className="button" onClick={() => setShowReleases(true)}>
                <Icon name="search" size={18} /> {t('wl.releases')}
              </button>
            )}
            {media.downloadRequestedAt && <span className="badge success">{t('wl.downloadRequested')}</span>}
          </div>
        )}
      </div>

      {info && (
        <p className="center">
          <button type="button" className="button" onClick={() => setMatching(true)}>
            {t('details.changeMatch')}
          </button>
        </p>
      )}

      {editing && <MediaEditor media={media} onClose={() => setEditing(false)} onSaved={items.reload} />}
      {showReleases && <ReleasesDialog kind={media.kind} initialQuery={releaseQuery} onClose={() => setShowReleases(false)} />}
      {matching && (
        <Modal title={info ? t('details.changeMatch') : t('details.find')} onClose={() => setMatching(false)}>
          <p className="muted small">{t('details.matchHint')}</p>
          <MediaSearch kind={media.kind} initialQuery={media.title} autoSearch onPick={(result) => void match(result)} />
        </Modal>
      )}
    </section>
  );
}

/** "2016 · Movie · 1 h 56 min" or "2022 · TV show · 2 seasons · 19 episodes". */
function factsLine(media: Media, info: MediaDetails | undefined, t: ReturnType<typeof useI18n>['t']): string {
  const parts: string[] = [];
  const year = info?.year ?? media.year;
  if (year) parts.push(String(year));
  parts.push(t(`kind.${media.kind}`));
  if (info?.runtimeMinutes) {
    const hours = Math.floor(info.runtimeMinutes / 60);
    const minutes = info.runtimeMinutes % 60;
    parts.push(hours > 0 ? t('details.runtimeHours', { hours, minutes }) : t('details.runtimeMinutes', { minutes }));
  }
  if (info?.seasons) parts.push(t(info.seasons === 1 ? 'details.seasons.one' : 'details.seasons.other', { count: info.seasons }));
  if (info?.episodes) parts.push(t(info.episodes === 1 ? 'details.episodes.one' : 'details.episodes.other', { count: info.episodes }));
  return parts.join(' · ');
}

/** "★ 7.6/10 · 18K votes" (TMDB users' average). */
function Rating({ rating, votes }: { rating: number; votes: number }) {
  const { t, locale } = useI18n();
  const score = new Intl.NumberFormat(locale, { maximumFractionDigits: 1, minimumFractionDigits: 1 }).format(rating);
  const count = new Intl.NumberFormat(locale, { notation: 'compact', maximumFractionDigits: 1 }).format(votes);
  return (
    <span className="rating" title={t('details.ratingSource')}>
      <Icon name="star" size={16} filled /> {t('details.rating', { score, votes: count })}
    </span>
  );
}

function initials(name: string): string {
  return name
    .split(/\s+/)
    .slice(0, 2)
    .map((part) => part.charAt(0).toUpperCase())
    .join('');
}
