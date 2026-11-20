import { useState } from 'react';
import { useI18n } from '../../i18n';
import { useSession } from '../../lib/session';
import { useErrorText } from '../../components/Feedback';
import { Field } from '../../components/ui';

/**
 * Current / new / confirm password fields with their rules. Shared by the
 * forced first-login page and the "change password" dialog. The current
 * password field is hidden when it is already known (just typed at login).
 */
export function usePasswordForm(knownCurrent?: string) {
  const { t } = useI18n();
  const { changePassword } = useSession();
  const errorText = useErrorText();
  const [current, setCurrent] = useState(knownCurrent ?? '');
  const [next, setNext] = useState('');
  const [confirmation, setConfirmation] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string>();

  const mismatch = next !== '' && confirmation !== '' && next !== confirmation;
  const valid = current !== '' && next.length >= 8 && next === confirmation && next !== current;

  const submit = async (): Promise<boolean> => {
    if (!valid || busy) return false;
    setBusy(true);
    setError(undefined);
    try {
      await changePassword(current, next);
      return true;
    } catch (failure) {
      setError(errorText(failure));
      return false;
    } finally {
      setBusy(false);
    }
  };

  const fields = (
    <>
      {knownCurrent === undefined && (
        <Field label={t('pw.current')}>
          <input type="password" value={current} onChange={(e) => setCurrent(e.target.value)} autoComplete="current-password" />
        </Field>
      )}
      <Field label={t('pw.new')}>
        <input type="password" value={next} onChange={(e) => setNext(e.target.value)} autoComplete="new-password" />
      </Field>
      <Field label={t('pw.confirm')} hint={mismatch ? undefined : t('pw.rule')}>
        <input type="password" value={confirmation} onChange={(e) => setConfirmation(e.target.value)} autoComplete="new-password" />
      </Field>
      {mismatch && <p className="error">{t('pw.mismatch')}</p>}
      {error && (
        <p className="error" role="alert">
          {error}
        </p>
      )}
    </>
  );

  return { fields, valid, busy, submit };
}
