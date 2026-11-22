import { useCallback, useEffect, useState } from 'react';
import { useI18n } from '../../i18n';
import { api } from '../../lib/api';
import type { ConnectionResult } from '../../lib/types';
import { Loading } from '../../components/ui';

/**
 * Tests every outside service the server uses. It runs once when the page
 * opens and on "Test all", never on a timer: each run calls the services.
 */
export function ConnectionsSection() {
  const { t } = useI18n();
  const [results, setResults] = useState<ConnectionResult[] | undefined>(undefined);
  const [testing, setTesting] = useState(false);
  const [error, setError] = useState<string | undefined>(undefined);

  const run = useCallback(async () => {
    setTesting(true);
    setError(undefined);
    try {
      setResults(await api.get<ConnectionResult[]>('admin/connections'));
    } catch (failure) {
      setError(failure instanceof Error ? failure.message : String(failure));
    } finally {
      setTesting(false);
    }
  }, []);

  useEffect(() => {
    void run();
  }, [run]);

  // Tool names stay as they are; only these two are translated.
  const name = (service: string) => {
    if (service === 'database') return t('connections.database');
    if (service === 'push') return t('connections.push');
    if (service === 'tmdb') return 'TMDB';
    return service.charAt(0).toUpperCase() + service.slice(1);
  };

  return (
    <>
      <h2 className="section-title">{t('connections.title')}</h2>
      <div className="card form-card">
        {!results && !error && <Loading />}
        {error && <p className="warning">{error}</p>}
        {results && (
          <ul className="connections">
            {results.map((result) => (
              <li key={result.service} className={`connection ${!result.configured ? 'off' : result.ok ? 'ok' : 'failed'}`}>
                <span className="connection-status" aria-hidden="true">
                  {!result.configured ? '–' : result.ok ? '✓' : '✕'}
                </span>
                <span className="connection-text">
                  <strong>{name(result.service)}</strong>
                  <span className="muted small">
                    {!result.configured
                      ? t('connections.notSetUp')
                      : result.ok
                        ? `${result.message} · ${result.latencyMs} ms`
                        : result.message}
                  </span>
                </span>
              </li>
            ))}
          </ul>
        )}
        <button type="button" className="button" onClick={() => void run()} disabled={testing}>
          {testing ? t('connections.testing') : t('connections.testAll')}
        </button>
        <p className="muted small">{t('connections.hint')}</p>
      </div>
    </>
  );
}
