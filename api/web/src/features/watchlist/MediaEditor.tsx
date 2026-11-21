import { useState } from 'react';
import { useI18n } from '../../i18n';
import { api } from '../../lib/api';
import { useMe } from '../../lib/session';
import { mediaKinds, type ExternalIds, type Media, type MediaKind, type MediaSearchResult } from '../../lib/types';
import { useFeedback } from '../../components/Feedback';
import { Field, Modal, Segmented } from '../../components/ui';
import { MediaSearch } from './MediaSearch';
import { Poster } from './Poster';

/** Whether the server can search titles of this kind (TMDB, or Radarr/Sonarr). */
export function canSearchTitles(integrations: { tmdb: boolean; radarr: boolean; sonarr: boolean }, kind: MediaKind) {
  return integrations.tmdb || (kind === 'movie' ? integrations.radarr : integrations.sonarr);
}

/**
 * Adds or edits a movie or a show. When the server can search (TMDB, or
 * Radarr/Sonarr), picking a result fills the title, year, poster and ids.
 * Download and release search live on the details page.
 */
export function MediaEditor({ media, onClose, onSaved }: { media?: Media; onClose: () => void; onSaved: () => Promise<void> }) {
  const { t } = useI18n();
  const me = useMe();
  const { run } = useFeedback();
  const [kind, setKind] = useState<MediaKind>(media?.kind ?? 'movie');
  const [title, setTitle] = useState(media?.title ?? '');
  const [year, setYear] = useState(media?.year ? String(media.year) : '');
  const [note, setNote] = useState(media?.note ?? '');
  const [posterUrl, setPosterUrl] = useState(media?.posterUrl ?? '');
  const [overview, setOverview] = useState(media?.overview ?? '');
  const [externalIds, setExternalIds] = useState<ExternalIds>(media?.externalIds ?? {});
  const [busy, setBusy] = useState(false);

  const choose = (result: MediaSearchResult) => {
    setTitle(result.title);
    setYear(result.year ? String(result.year) : '');
    setPosterUrl(result.posterUrl ?? '');
    setOverview(result.overview ?? '');
    setExternalIds(result.externalIds);
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

  return (
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
          onChange={setKind}
          options={mediaKinds.map((value) => ({ value, label: t(`kind.${value}`) }))}
        />
      )}
      {!media && canSearchTitles(me.integrations, kind) && (
        <>
          <MediaSearch kind={kind} onPick={choose} />
          <p className="field-hint">{t('wl.searchHint')}</p>
        </>
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
    </Modal>
  );
}
