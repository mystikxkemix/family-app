import { useEffect, useState } from 'react';
import { useI18n } from '../../i18n';
import { api } from '../../lib/api';
import { formatBytes, formatRelative } from '../../lib/format';
import type { MediaKind, Release } from '../../lib/types';
import { useFeedback } from '../../components/Feedback';
import { Loading, Modal } from '../../components/ui';
import { Icon } from '../../components/Icon';

/** Prowlarr interactive search: pick a release and send it to the download client. */
export function ReleasesDialog({ kind, initialQuery, onClose }: { kind: MediaKind; initialQuery: string; onClose: () => void }) {
  const { t, locale } = useI18n();
  const { run, showInfo } = useFeedback();
  const [query, setQuery] = useState(initialQuery);
  const [releases, setReleases] = useState<Release[]>();
  const [searching, setSearching] = useState(false);

  const search = async (term: string) => {
    if (!term.trim()) return;
    setSearching(true);
    setReleases(undefined);
    await run(async () => {
      const found = await api.get<Release[]>('releases', { kind, q: term.trim() });
      // Most seeded first; usenet releases (no seeders) after.
      setReleases([...found].sort((a, b) => (b.seeders ?? -1) - (a.seeders ?? -1)));
    });
    setSearching(false);
  };

  useEffect(() => {
    void search(initialQuery);
    // Only the first search is automatic.
  }, []);

  const grab = (release: Release) => {
    if (!window.confirm(t('wl.grabConfirm', { title: release.title }))) return;
    void run(async () => {
      await api.post('releases/grab', { guid: release.guid, indexerId: release.indexerId });
      showInfo(t('wl.downloadSent'));
    });
  };

  return (
    <Modal title={t('wl.releasesTitle')} onClose={onClose}>
      <div className="search-row">
        <input
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === 'Enter') {
              e.preventDefault();
              void search(query);
            }
          }}
          aria-label={t('wl.search')}
        />
        <button type="button" className="button" onClick={() => void search(query)} disabled={searching}>
          <Icon name="search" size={18} />
        </button>
      </div>
      {searching && <Loading />}
      {releases && releases.length === 0 && <p className="muted center">{t('wl.noReleases')}</p>}
      <ul className="releases">
        {releases?.map((release) => (
          <li key={`${release.indexerId}-${release.guid}`}>
            <button type="button" onClick={() => grab(release)}>
              <span className="release-title">{release.title}</span>
              <span className="muted small release-meta">
                <span>{formatBytes(release.size, locale)}</span>
                {release.seeders !== undefined && <span>{t('wl.seeders', { count: release.seeders })}</span>}
                <span>{formatRelative(new Date(release.publishDate), locale)}</span>
                {release.indexer && <span>{release.indexer}</span>}
              </span>
            </button>
          </li>
        ))}
      </ul>
    </Modal>
  );
}
