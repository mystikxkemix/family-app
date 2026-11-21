import { useEffect, useState } from 'react';
import { useI18n } from '../../i18n';
import { api } from '../../lib/api';
import type { MediaKind, MediaSearchResult } from '../../lib/types';
import { useFeedback } from '../../components/Feedback';
import { Icon } from '../../components/Icon';
import { Loading } from '../../components/ui';
import { Poster } from './Poster';

/**
 * Title search (TMDB when the server has a key, else Radarr/Sonarr), in the
 * website's language. Used by the editor and by "Find the details".
 */
export function MediaSearch({
  kind,
  initialQuery = '',
  autoSearch = false,
  onPick,
}: {
  kind: MediaKind;
  initialQuery?: string;
  autoSearch?: boolean;
  onPick: (result: MediaSearchResult) => void;
}) {
  const { t, lang } = useI18n();
  const { run } = useFeedback();
  const [query, setQuery] = useState(initialQuery);
  const [results, setResults] = useState<MediaSearchResult[]>();
  const [searching, setSearching] = useState(false);

  const search = async (term: string) => {
    const text = term.trim();
    if (!text) return;
    setSearching(true);
    await run(async () => setResults(await api.get<MediaSearchResult[]>('search/media', { kind, q: text, lang })));
    setSearching(false);
  };

  useEffect(() => {
    if (autoSearch) void search(initialQuery);
    // Only the first search is automatic.
  }, []);

  // A new kind (movie/show) makes the old results wrong.
  useEffect(() => setResults(undefined), [kind]);

  return (
    <div className="search-box">
      <div className="search-row">
        <input
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          onKeyDown={(e) => {
            // Enter searches; it must not submit (save) the surrounding form.
            if (e.key === 'Enter') {
              e.preventDefault();
              void search(query);
            }
          }}
          placeholder={t('wl.search')}
          aria-label={t('wl.search')}
        />
        <button type="button" className="button" onClick={() => void search(query)} disabled={searching || !query.trim()} aria-label={t('wl.search')}>
          <Icon name="search" size={18} />
        </button>
      </div>
      {searching && <Loading />}
      {results && results.length === 0 && !searching && <p className="muted small center">{t('details.noResult')}</p>}
      {results && results.length > 0 && (
        <ul className="results">
          {results.map((result) => (
            <li key={`${result.externalIds.tmdb ?? ''}-${result.externalIds.tvdb ?? ''}-${result.title}-${result.year ?? ''}`}>
              <button type="button" onClick={() => onPick(result)}>
                <Poster url={result.posterUrl} kind={result.kind} />
                <span className="row-text">
                  <strong>{result.title}</strong>
                  {result.year ? <span className="muted small">{result.year}</span> : null}
                  {result.overview && <span className="muted small clamp">{result.overview}</span>}
                  {result.inLibrary && <span className="badge success">{t('wl.inLibrary')}</span>}
                </span>
              </button>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
