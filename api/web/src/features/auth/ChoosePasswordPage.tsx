import type { FormEvent } from 'react';
import { useI18n } from '../../i18n';
import { useSession } from '../../lib/session';
import { usePasswordForm } from './PasswordForm';

/** Shown instead of the app while the account must choose its own password. */
export function ChoosePasswordPage() {
  const { t } = useI18n();
  const { loginPassword, logout } = useSession();
  const form = usePasswordForm(loginPassword);

  const submit = (event: FormEvent) => {
    event.preventDefault();
    void form.submit();
  };

  return (
    <main className="auth">
      <form className="auth-card" onSubmit={submit}>
        <img src="/icon-512.png" alt="" className="auth-logo" width={72} height={72} />
        <h1>{t('pw.title')}</h1>
        <p className="muted">{t('pw.intro')}</p>
        {form.fields}
        <button type="submit" className="button button-primary button-block" disabled={!form.valid || form.busy}>
          {t('pw.save')}
        </button>
        <button type="button" className="button button-block" onClick={() => void logout()}>
          {t('settings.logout')}
        </button>
      </form>
    </main>
  );
}
