import { useState, type FormEvent } from 'react';
import { useI18n } from '../../i18n';
import { useSession } from '../../lib/session';
import { useErrorText } from '../../components/Feedback';
import { Field, Segmented } from '../../components/ui';

type Mode = 'login' | 'join';

/** Log in, or join a household with an invite code (like the iOS app). */
export function LoginPage() {
  const { t } = useI18n();
  const { login, register } = useSession();
  const errorText = useErrorText();
  const [mode, setMode] = useState<Mode>('login');
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [displayName, setDisplayName] = useState('');
  const [inviteCode, setInviteCode] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string>();

  const canSubmit =
    username.trim() !== '' && password !== '' && (mode === 'login' || (displayName.trim() !== '' && inviteCode.trim() !== ''));

  const submit = async (event: FormEvent) => {
    event.preventDefault();
    if (!canSubmit || busy) return;
    setBusy(true);
    setError(undefined);
    try {
      if (mode === 'login') {
        await login(username.trim(), password);
      } else {
        await register({ username: username.trim(), password, displayName: displayName.trim(), inviteCode: inviteCode.trim() });
      }
    } catch (failure) {
      setError(errorText(failure));
    } finally {
      setBusy(false);
    }
  };

  return (
    <main className="auth">
      <form className="auth-card" onSubmit={submit}>
        <img src="/icon-512.png" alt="" className="auth-logo" width={72} height={72} />
        <h1>{t('auth.welcome')}</h1>
        <Segmented
          label={t('auth.login')}
          value={mode}
          onChange={setMode}
          options={[
            { value: 'login', label: t('auth.login') },
            { value: 'join', label: t('auth.join') },
          ]}
        />
        <Field label={t('auth.username')}>
          <input value={username} onChange={(e) => setUsername(e.target.value)} autoComplete="username" autoCapitalize="none" required />
        </Field>
        <Field label={t('auth.password')} hint={mode === 'join' ? t('auth.passwordHint') : undefined}>
          <input
            type="password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            autoComplete={mode === 'login' ? 'current-password' : 'new-password'}
            required
          />
        </Field>
        {mode === 'join' && (
          <>
            <Field label={t('auth.displayName')}>
              <input value={displayName} onChange={(e) => setDisplayName(e.target.value)} autoComplete="given-name" required />
            </Field>
            <Field label={t('auth.inviteCode')} hint={t('auth.inviteHint')}>
              <input
                value={inviteCode}
                onChange={(e) => setInviteCode(e.target.value.toUpperCase())}
                autoCapitalize="characters"
                autoComplete="off"
                required
              />
            </Field>
          </>
        )}
        {error && (
          <p className="error" role="alert">
            {error}
          </p>
        )}
        <button type="submit" className="button button-primary button-block" disabled={!canSubmit || busy}>
          {mode === 'login' ? t('auth.login') : t('auth.createAccount')}
        </button>
        {mode === 'login' && <p className="muted small">{t('auth.firstTime')}</p>}
      </form>
    </main>
  );
}
