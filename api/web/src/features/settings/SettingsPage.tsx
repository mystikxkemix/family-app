import { useEffect, useState, type FormEvent } from 'react';
import { useI18n, type Language } from '../../i18n';
import { api } from '../../lib/api';
import { useMe, useSession } from '../../lib/session';
import { copyText, formatDateTime } from '../../lib/format';
import type { Invite } from '../../lib/types';
import { useFeedback } from '../../components/Feedback';
import { Field, Modal, PageHeader, Segmented } from '../../components/ui';
import { Icon } from '../../components/Icon';
import { usePasswordForm } from '../auth/PasswordForm';

/** Profile, household, invites, language and account. */
export function SettingsPage() {
  const { t, lang, setLang, locale } = useI18n();
  const me = useMe();
  const { refreshMe, logout } = useSession();
  const { run, showInfo } = useFeedback();
  const [displayName, setDisplayName] = useState(me.user.displayName);
  const [householdName, setHouseholdName] = useState(me.household.name);
  const [invite, setInvite] = useState<Invite>();
  const [changingPassword, setChangingPassword] = useState(false);

  useEffect(() => setDisplayName(me.user.displayName), [me.user.displayName]);
  useEffect(() => setHouseholdName(me.household.name), [me.household.name]);

  const saveName = (event: FormEvent) => {
    event.preventDefault();
    void run(async () => {
      await api.patch('me', { displayName: displayName.trim() });
      await refreshMe();
      showInfo(t('common.saved'));
    });
  };

  const saveHousehold = (event: FormEvent) => {
    event.preventDefault();
    void run(async () => {
      await api.patch('household', { name: householdName.trim() });
      await refreshMe();
      showInfo(t('common.saved'));
    });
  };

  const createInvite = () => void run(async () => setInvite(await api.post<Invite>('household/invites')));

  const copyInvite = async () => {
    if (invite && (await copyText(invite.code))) showInfo(t('common.copied'));
  };

  return (
    <section className="page narrow">
      <PageHeader title={t('settings.title')} />

      <h2 className="section-title">{t('settings.you')}</h2>
      <form className="card form-card" onSubmit={saveName}>
        <Field label={t('settings.yourName')}>
          <div className="inline-form">
            <input value={displayName} onChange={(e) => setDisplayName(e.target.value)} maxLength={40} />
            <button type="submit" className="button" disabled={!displayName.trim() || displayName.trim() === me.user.displayName}>
              {t('common.save')}
            </button>
          </div>
        </Field>
        <p className="muted small">
          {t('settings.username')}: <strong>{me.user.username}</strong>
        </p>
      </form>

      <h2 className="section-title">{t('settings.household')}</h2>
      <form className="card form-card" onSubmit={saveHousehold}>
        <Field label={t('settings.householdName')}>
          <div className="inline-form">
            <input value={householdName} onChange={(e) => setHouseholdName(e.target.value)} maxLength={40} />
            <button type="submit" className="button" disabled={!householdName.trim() || householdName.trim() === me.household.name}>
              {t('common.save')}
            </button>
          </div>
        </Field>
        <p className="field-label">{t('settings.members')}</p>
        <ul className="members">
          {me.members.map((member) => (
            <li key={member.id}>
              {member.displayName}
              {member.id === me.user.id && <span className="muted small"> ({t('admin.you')})</span>}
            </li>
          ))}
        </ul>
      </form>

      <h2 className="section-title">{t('settings.invite')}</h2>
      <div className="card form-card">
        {invite ? (
          <>
            <div className="invite-code">
              <code>{invite.code}</code>
              <button type="button" className="button" onClick={() => void copyInvite()}>
                <Icon name="copy" size={18} /> {t('common.copy')}
              </button>
            </div>
            <p className="muted small">{t('settings.inviteValid', { date: formatDateTime(new Date(invite.expiresAt), locale) })}</p>
          </>
        ) : (
          <button type="button" className="button button-primary" onClick={createInvite}>
            {t('settings.inviteSomeone')}
          </button>
        )}
        <p className="muted small">{t('settings.inviteHint')}</p>
      </div>

      <h2 className="section-title">{t('settings.language')}</h2>
      <div className="card form-card">
        <Segmented<Language>
          label={t('settings.language')}
          value={lang}
          onChange={setLang}
          options={[
            { value: 'fr', label: 'Français' },
            { value: 'en', label: 'English' },
          ]}
        />
      </div>

      <h2 className="section-title">{t('settings.account')}</h2>
      <div className="card form-card actions-card">
        <button type="button" className="button" onClick={() => setChangingPassword(true)}>
          {t('pw.change')}
        </button>
        <button
          type="button"
          className="button button-danger"
          onClick={() => {
            if (window.confirm(t('settings.logoutConfirm'))) void logout();
          }}
        >
          <Icon name="logout" size={18} /> {t('settings.logout')}
        </button>
      </div>

      <h2 className="section-title">{t('settings.about')}</h2>
      <div className="card form-card">
        <p className="muted small">{t('settings.tmdbAttribution')}</p>
      </div>

      {changingPassword && <ChangePasswordDialog onClose={() => setChangingPassword(false)} />}
    </section>
  );
}

/** Changes the password. The other devices of this account get logged out. */
function ChangePasswordDialog({ onClose }: { onClose: () => void }) {
  const { t } = useI18n();
  const { showInfo } = useFeedback();
  const form = usePasswordForm();

  const save = async () => {
    if (await form.submit()) {
      showInfo(t('pw.changed'));
      onClose();
    }
  };

  return (
    <Modal title={t('pw.change')} onClose={onClose} onSubmit={() => void save()} canSubmit={form.valid} busy={form.busy}>
      <p className="muted small">{t('pw.otherDevices')}</p>
      {form.fields}
    </Modal>
  );
}
