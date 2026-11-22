import { useCallback, useEffect, useState } from 'react';
import { useI18n } from '../../i18n';
import { api } from '../../lib/api';
import { formatRelative } from '../../lib/format';
import type { JellyfinState, JellyfinSyncResult, JellyfinUser } from '../../lib/types';
import { Field, Loading } from '../../components/ui';
import type { AdminSettingsState, JellyfinDraft } from './SettingsSections';

/** The message of a failed call (Jellyfin errors are clearer than a generic text). */
function messageOf(failure: unknown): string {
  return failure instanceof Error ? failure.message : String(failure);
}

/**
 * Jellyfin: address and key (saved with the other settings), the Jellyfin
 * account of the household, and the sync (every 15 minutes, or now).
 */
export function JellyfinSection({ settings }: { settings: AdminSettingsState }) {
  const { t, locale } = useI18n();
  const { draft, setDraft, save, test, busy } = settings;
  const [state, setState] = useState<JellyfinState>();
  const [users, setUsers] = useState<JellyfinUser[]>();
  const [error, setError] = useState<string>();
  const [syncing, setSyncing] = useState(false);

  const load = useCallback(async () => {
    setError(undefined);
    try {
      const next = await api.get<JellyfinState>('admin/jellyfin');
      setState(next);
      setUsers(next.configured ? await api.get<JellyfinUser[]>('admin/jellyfin/users') : undefined);
    } catch (failure) {
      setError(messageOf(failure));
    }
  }, []);

  // Load at start, and again when the saved address or key changes.
  const savedKey = draft ? `${draft.jellyfin.url}|${draft.jellyfin.apiKeySet}` : '';
  useEffect(() => {
    void load();
  }, [load, savedKey]);

  const update = (patch: Partial<JellyfinDraft>) =>
    setDraft((current) => (current ? { ...current, jellyfin: { ...current.jellyfin, ...patch } } : current));

  const chooseUser = async (jellyfinUserId: string) => {
    setError(undefined);
    try {
      setState(await api.put<JellyfinState>('admin/jellyfin/household', { jellyfinUserId }));
    } catch (failure) {
      setError(messageOf(failure));
    }
  };

  const syncNow = async () => {
    setSyncing(true);
    setError(undefined);
    try {
      const result = await api.post<JellyfinSyncResult>('admin/jellyfin/sync');
      setState((current) => (current ? { ...current, lastSync: result } : current));
    } catch (failure) {
      setError(messageOf(failure));
    } finally {
      setSyncing(false);
    }
  };

  const last = state?.lastSync;
  const when = last ? formatRelative(new Date(last.at), locale) : '';

  return (
    <>
      <h2 className="section-title">Jellyfin</h2>
      <div className="card form-card">
        <p className="muted small">{t('admin.jellyfinHelp')}</p>
        {!draft && <Loading />}
        {draft && (
          <fieldset className="tool">
            <legend>Jellyfin</legend>
            <Field label={t('admin.url')}>
              <input
                value={draft.jellyfin.url}
                onChange={(e) => update({ url: e.target.value })}
                placeholder="http://host.docker.internal:8096"
                inputMode="url"
                autoCapitalize="none"
              />
            </Field>
            <Field label={t('admin.apiKey')} hint={draft.jellyfin.apiKeySet ? t('admin.apiKeySaved') : undefined}>
              <input
                type="password"
                value={draft.jellyfin.apiKey}
                onChange={(e) => update({ apiKey: e.target.value })}
                placeholder={draft.jellyfin.apiKeySet ? '••••••••' : ''}
                autoComplete="off"
              />
            </Field>
            <div className="actions-row">
              <button type="button" className="button" onClick={() => void test('jellyfin')} disabled={!draft.jellyfin.apiKeySet || !draft.jellyfin.url}>
                {t('admin.test')} Jellyfin
              </button>
              <button type="button" className="button button-primary" onClick={() => void save()} disabled={busy}>
                {t('common.save')}
              </button>
            </div>
          </fieldset>
        )}
        {state && !state.configured && <p className="muted small">{t('admin.jellyfinSaveFirst')}</p>}
        {state?.configured && (
          <>
            <Field label={t('admin.jellyfinUser')}>
              <select value={state.jellyfinUserId} onChange={(e) => void chooseUser(e.target.value)} disabled={!users} data-testid="jellyfin-user">
                <option value="">{t('admin.jellyfinNoUser')}</option>
                {users?.map((user) => (
                  <option key={user.id} value={user.id}>
                    {user.name}
                  </option>
                ))}
              </select>
            </Field>
            <p className="muted small" data-testid="jellyfin-last-sync">
              {!last
                ? t('admin.jellyfinNever')
                : last.ok
                  ? t('admin.jellyfinLastSync', {
                      when,
                      matched: String(last.matched),
                      watched: String(last.markedWatched),
                      imported: String(last.imported),
                    })
                  : t('admin.jellyfinLastError', { when, message: last.message ?? '' })}
            </p>
            <button type="button" className="button" onClick={() => void syncNow()} disabled={syncing || state.running || !state.jellyfinUserId}>
              {syncing || state.running ? t('admin.jellyfinSyncing') : t('admin.jellyfinSync')}
            </button>
          </>
        )}
        {error && <p className="warning">{error}</p>}
      </div>
    </>
  );
}
