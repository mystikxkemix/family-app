import { useCallback, useEffect, useState } from 'react';
import { useI18n } from '../../i18n';
import { api } from '../../lib/api';
import { useSession } from '../../lib/session';
import type { AdminSettings, AdminSettingsInput, ArrTool, TestResult } from '../../lib/types';
import { useFeedback } from '../../components/Feedback';
import { Field, Loading } from '../../components/ui';
import { Icon } from '../../components/Icon';

// The settings file (settings.json on the server) as an editable draft.
// Secrets are never sent back by the API: an empty key field keeps the saved
// one, a filled field replaces it.

interface ArrDraft {
  url: string;
  apiKey: string;
  apiKeySet: boolean;
  qualityProfileId: string;
  rootFolder: string;
}

interface ApnsDraft {
  teamId: string;
  keyId: string;
  topicsText: string;
  key: string;
  keySet: boolean;
}

interface Draft {
  radarr: ArrDraft;
  sonarr: ArrDraft;
  prowlarr: ArrDraft;
  apns: ApnsDraft[];
}

function toDraft(settings: AdminSettings): Draft {
  const arr = (tool: ArrTool): ArrDraft => ({
    url: settings[tool].url ?? '',
    apiKey: '',
    apiKeySet: settings[tool].apiKeySet,
    qualityProfileId: settings[tool].qualityProfileId ? String(settings[tool].qualityProfileId) : '',
    rootFolder: settings[tool].rootFolder ?? '',
  });
  return {
    radarr: arr('radarr'),
    sonarr: arr('sonarr'),
    prowlarr: arr('prowlarr'),
    apns: settings.apns.credentials.map((credential) => ({
      teamId: credential.teamId,
      keyId: credential.keyId,
      topicsText: credential.topics.join(', '),
      key: '',
      keySet: credential.keySet,
    })),
  };
}

function toInput(draft: Draft): AdminSettingsInput {
  const arr = (value: ArrDraft) => ({
    url: value.url.trim(),
    apiKey: value.apiKey.trim(),
    qualityProfileId: Number.parseInt(value.qualityProfileId, 10) || 0,
    rootFolder: value.rootFolder.trim(),
  });
  return {
    radarr: arr(draft.radarr),
    sonarr: arr(draft.sonarr),
    prowlarr: { url: draft.prowlarr.url.trim(), apiKey: draft.prowlarr.apiKey.trim() },
    apns: {
      credentials: draft.apns.map((credential) => ({
        teamId: credential.teamId.trim(),
        keyId: credential.keyId.trim(),
        topics: credential.topicsText
          .split(',')
          .map((topic) => topic.trim())
          .filter(Boolean),
        key: credential.key,
      })),
    },
  };
}

/** Loads the settings once (no auto refresh: it would erase what is being typed). */
export function useAdminSettings() {
  const { t } = useI18n();
  const { refreshMe } = useSession();
  const { run, showInfo } = useFeedback();
  const [draft, setDraft] = useState<Draft>();
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    void run(async () => setDraft(toDraft(await api.get<AdminSettings>('admin/settings'))));
  }, [run]);

  const save = useCallback(async () => {
    if (!draft) return;
    setBusy(true);
    await run(async () => {
      const saved = await api.put<AdminSettings>('admin/settings', toInput(draft));
      setDraft(toDraft(saved));
      showInfo(t('common.saved'));
      // The app shows the tools (download, search) from me.integrations.
      await refreshMe();
    });
    setBusy(false);
  }, [draft, refreshMe, run, showInfo, t]);

  const test = useCallback(
    async (tool: ArrTool) => {
      await run(async () => {
        const result = await api.post<TestResult>('admin/settings/test', { tool });
        showInfo(result.ok ? `${t('admin.testOk')} — ${result.message}` : result.message);
      });
    },
    [run, showInfo, t],
  );

  return { draft, setDraft, save, test, busy };
}

type AdminSettingsState = ReturnType<typeof useAdminSettings>;

const toolNames: Record<ArrTool, string> = { radarr: 'Radarr', sonarr: 'Sonarr', prowlarr: 'Prowlarr' };

export function MediaToolsSection({ settings }: { settings: AdminSettingsState }) {
  const { t } = useI18n();
  const { draft, setDraft, save, test, busy } = settings;

  const update = (tool: ArrTool, patch: Partial<ArrDraft>) =>
    setDraft((current) => (current ? { ...current, [tool]: { ...current[tool], ...patch } } : current));

  return (
    <>
      <h2 className="section-title">{t('admin.mediaTools')}</h2>
      <div className="card form-card">
        <p className="muted small">{t('admin.mediaToolsIntro')}</p>
        {!draft && <Loading />}
        {draft &&
          (['radarr', 'sonarr', 'prowlarr'] as ArrTool[]).map((tool) => (
            <fieldset key={tool} className="tool">
              <legend>{toolNames[tool]}</legend>
              <Field label={t('admin.url')}>
                <input
                  value={draft[tool].url}
                  onChange={(e) => update(tool, { url: e.target.value })}
                  placeholder={tool === 'radarr' ? 'http://radarr:7878' : tool === 'sonarr' ? 'http://sonarr:8989' : 'http://prowlarr:9696'}
                  inputMode="url"
                  autoCapitalize="none"
                />
              </Field>
              <Field label={t('admin.apiKey')} hint={draft[tool].apiKeySet ? t('admin.apiKeySaved') : undefined}>
                <input
                  type="password"
                  value={draft[tool].apiKey}
                  onChange={(e) => update(tool, { apiKey: e.target.value })}
                  placeholder={draft[tool].apiKeySet ? '••••••••' : ''}
                  autoComplete="off"
                />
              </Field>
              {tool !== 'prowlarr' && (
                <div className="two-columns">
                  <Field label={t('admin.qualityProfile')}>
                    <input
                      value={draft[tool].qualityProfileId}
                      onChange={(e) => update(tool, { qualityProfileId: e.target.value.replace(/\D/g, '') })}
                      inputMode="numeric"
                    />
                  </Field>
                  <Field label={t('admin.rootFolder')}>
                    <input value={draft[tool].rootFolder} onChange={(e) => update(tool, { rootFolder: e.target.value })} />
                  </Field>
                </div>
              )}
              <button type="button" className="button" onClick={() => void test(tool)} disabled={!draft[tool].apiKeySet || !draft[tool].url}>
                {t('admin.test')} {toolNames[tool]}
              </button>
            </fieldset>
          ))}
        {draft && (
          <button type="button" className="button button-primary" onClick={() => void save()} disabled={busy}>
            {t('common.save')}
          </button>
        )}
      </div>
    </>
  );
}

export function PushSection({ settings }: { settings: AdminSettingsState }) {
  const { t } = useI18n();
  const { draft, setDraft, save, busy } = settings;

  const update = (index: number, patch: Partial<ApnsDraft>) =>
    setDraft((current) =>
      current ? { ...current, apns: current.apns.map((credential, i) => (i === index ? { ...credential, ...patch } : credential)) } : current,
    );

  const add = () =>
    setDraft((current) =>
      current ? { ...current, apns: [...current.apns, { teamId: '', keyId: '', topicsText: '', key: '', keySet: false }] } : current,
    );

  const remove = (index: number) => setDraft((current) => (current ? { ...current, apns: current.apns.filter((_, i) => i !== index) } : current));

  /** Reads the chosen .p8 file as text; it is sent on save. */
  const pickKey = async (index: number, file: File | undefined) => {
    if (!file) return;
    const key = await file.text();
    // AuthKey_<KEYID>.p8: fill the key id when it is still empty.
    const match = /AuthKey_([A-Z0-9]+)\.p8$/i.exec(file.name);
    update(index, { key, ...(match && !draft?.apns[index]?.keyId ? { keyId: match[1].toUpperCase() } : {}) });
  };

  return (
    <>
      <h2 className="section-title">{t('admin.push')}</h2>
      <div className="card form-card">
        <p className="muted small">{t('admin.pushIntro')}</p>
        {!draft && <Loading />}
        {draft && draft.apns.length === 0 && <p className="muted">{t('admin.noKeys')}</p>}
        {draft?.apns.map((credential, index) => (
          <fieldset key={index} className="tool">
            <legend>
              APNs {credential.keyId || index + 1}
              <button type="button" className="icon-button danger" aria-label={t('admin.removeKey')} title={t('admin.removeKey')} onClick={() => remove(index)}>
                <Icon name="trash" size={16} />
              </button>
            </legend>
            <div className="two-columns">
              <Field label={t('admin.teamId')}>
                <input value={credential.teamId} onChange={(e) => update(index, { teamId: e.target.value.toUpperCase() })} maxLength={10} />
              </Field>
              <Field label={t('admin.keyId')}>
                <input value={credential.keyId} onChange={(e) => update(index, { keyId: e.target.value.toUpperCase() })} maxLength={10} />
              </Field>
            </div>
            <Field label={t('admin.topics')}>
              <input value={credential.topicsText} onChange={(e) => update(index, { topicsText: e.target.value })} placeholder="com.example.family" autoCapitalize="none" />
            </Field>
            <Field
              label={t('admin.keyFile')}
              hint={credential.key ? t('admin.keyPicked') : credential.keySet ? t('admin.keySaved') : undefined}
            >
              <input type="file" accept=".p8" onChange={(e) => void pickKey(index, e.target.files?.[0])} />
            </Field>
          </fieldset>
        ))}
        {draft && (
          <div className="actions-row">
            <button type="button" className="button" onClick={add}>
              <Icon name="plus" size={18} /> {t('admin.addKey')}
            </button>
            <button type="button" className="button button-primary" onClick={() => void save()} disabled={busy}>
              {t('common.save')}
            </button>
          </div>
        )}
      </div>
    </>
  );
}
