import { useState } from 'react';
import { useI18n } from '../../i18n';
import { api } from '../../lib/api';
import { useMe } from '../../lib/session';
import { mediaKinds, type DownloadResponse, type ExternalIds, type Media, type MediaKind, type MediaSearchResult } from '../../lib/types';
import { useFeedback } from '../../components/Feedback';
import { Field, Modal, Segmented } from '../../components/ui';
import { Icon } from '../../components/Icon';
import { ReleasesDialog } from './ReleasesDialog';

/** A small poster, or the movie/show icon when there is none. */
export function Poster({ url, kind, large = false }: { url?: string; kind: MediaKind; large?: boolean }) {
  return (
    <span className={`poster${large ? ' large' : ''}`} aria-hidden="true">
      {url ? <img src={url} alt="" loading="lazy" /> : <Icon name={kind === 'movie' ? 'film' : 'tv'} size={large ? 40 : 22} />}
    </span>
  );
}

/**
 * Adds or edits a movie or a show. When Radarr/Sonarr is set up on the
 * server, a search fills the title, year, poster and ids. In edit mode it also
 * offers the download and the Prowlarr release search.
 */
export function MediaEditor({ media, onClose, onSaved }: { media?: Media; onClose: () => void; onSaved: () => Promise<void> }) {
  const { t } = useI18n();
  const me = useMe();
  const { run, showInfo } = useFeedback();
  const [kind, setKind] = useState<MediaKind>(media?.kind ?? 'movie');
  const [title, setTitle] = useState(media?.title ?? '');
  const [year, setYear] = useState(media?.year ? String(media.year) : '');
  const [note, setNote] = useState(media?.note ?? '');
  const [posterUrl, setPosterUrl] = useState(media?.posterUrl ?? '');
  const [overview, setOverview] = useState(media?.overview ?? '');
  const [externalIds, setExternalIds] = useState<ExternalIds>(media?.externalIds ?? {});
  const [query, setQuery] = useState('');
  const [results, setResults] = useState<MediaSearchResult[]>([]);
  const [searching, setSearching] = useState(false);
  const [busy, setBusy] = useState(false);
  const [showReleases, setShowReleases] = useState(false);

  const integrations = me.integrations;
  const canSearch = kind === 'movie' ? integrations.radarr : integrations.sonarr;

  const search = async () => {
    const term = query.trim();
    if (!term) return;
    setSearching(true);
    await run(async () => setResults(await api.get<MediaSearchResult[]>('search/media', { kind, q: term })));
    setSearching(false);
  };

  const choose = (result: MediaSearchResult) => {
    setTitle(result.title);
    setYear(result.year ? String(result.year) : '');
    setPosterUrl(result.posterUrl ?? '');
    setOverview(result.overview ?? '');
    setExternalIds(result.externalIds);
    setResults([]);
  };

  const save = async () => {
    setBusy(true);
    const body: Record<string, unknown> = {
      title: title.trim(),
      year: Number.parseInt(year, 10) || 0,
      note: note.trim(),
      posterUrl,
      overview,
      externalIds,
    };
    if (!media) body.kind = kind;
    const saved = await run(() => (media ? api.patch(`watchlist/${media.id}`, body) : api.post('watchlist', body)));
    setBusy(false);
    if (saved) {
      await onSaved();
      onClose();
    }
  };

  const download = async () => {
    if (!media) return;
    await run(async () => {
      const response = await api.post<DownloadResponse>(`watchlist/${media.id}/download`);
      showInfo(response.alreadyInLibrary ? t('wl.alreadyInLibrary') : t('wl.downloadSent'));
      await onSaved();
    });
  };

  const hasLibrary = media && (media.kind === 'movie' ? integrations.radarr : integrations.sonarr);
  const releaseQuery = media ? (media.kind === 'movie' && media.year ? `${media.title} ${media.year}` : media.title) : '';

  return (
    <>
      <Modal
        title={media ? t('wl.editTitle') : t('wl.add')}
        onClose={onClose}
        onSubmit={() => void save()}
        canSubmit={title.trim() !== ''}
        busy={busy}
      >
        {!media && (
          <Segmented
            label={t('lists.type')}
            value={kind}
            onChange={(value) => {
              setKind(value);
              setResults([]);
            }}
            options={mediaKinds.map((value) => ({ value, label: t(`kind.${value}`) }))}
          />
        )}
        {!media && canSearch && (
          <div className="search-box">
            <div className="search-row">
              <input
                value={query}
                onChange={(e) => setQuery(e.target.value)}
                onKeyDown={(e) => {
                  // Enter searches; it must not submit (save) the whole form.
                  if (e.key === 'Enter') {
                    e.preventDefault();
                    void search();
                  }
                }}
                placeholder={t('wl.search')}
                aria-label={t('wl.search')}
              />
              <button type="button" className="button" onClick={() => void search()} disabled={searching || !query.trim()}>
                <Icon name="search" size={18} />
              </button>
            </div>
            {results.length > 0 && (
              <ul className="results">
                {results.map((result) => (
                  <li key={`${result.externalIds.tmdb ?? ''}-${result.externalIds.tvdb ?? ''}-${result.title}`}>
                    <button type="button" onClick={() => choose(result)}>
                      <Poster url={result.posterUrl} kind={result.kind} />
                      <span className="row-text">
                        <strong>{result.title}</strong>
                        {result.year ? <span className="muted small">{result.year}</span> : null}
                        {result.inLibrary && <span className="badge success">{t('wl.inLibrary')}</span>}
                      </span>
                    </button>
                  </li>
                ))}
              </ul>
            )}
            <p className="field-hint">{t('wl.searchHint')}</p>
          </div>
        )}
        <div className="with-poster">
          {posterUrl && <Poster url={posterUrl} kind={kind} />}
          <div className="grow">
            <Field label={t('wl.titleField')}>
              <input value={title} onChange={(e) => setTitle(e.target.value)} maxLength={200} />
            </Field>
            <Field label={t('wl.year')}>
              <input value={year} onChange={(e) => setYear(e.target.value.replace(/\D/g, ''))} inputMode="numeric" maxLength={4} />
            </Field>
          </div>
        </div>
        <Field label={t('wl.note')}>
          <textarea value={note} onChange={(e) => setNote(e.target.value)} rows={3} maxLength={1000} />
        </Field>
        {media && (hasLibrary || integrations.prowlarr) && (
          <div className="tool-buttons">
            {hasLibrary && (
              <button type="button" className="button" onClick={() => void download()}>
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
      </Modal>
      {showReleases && media && <ReleasesDialog kind={media.kind} initialQuery={releaseQuery} onClose={() => setShowReleases(false)} />}
    </>
  );
}
