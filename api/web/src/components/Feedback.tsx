import { createContext, useCallback, useContext, useMemo, useState, type ReactNode } from 'react';
import { ApiError } from '../lib/api';
import { useI18n, type MessageKey } from '../i18n';
import { en } from '../i18n/en';

interface Toast {
  id: number;
  text: string;
  kind: 'error' | 'info';
}

interface Feedback {
  /** Shows an error message (translated when the API code is known). */
  showError: (error: unknown) => void;
  showInfo: (text: string) => void;
  /** Runs an action and shows its error, if any. Returns false on failure. */
  run: (action: () => Promise<unknown>) => Promise<boolean>;
}

const FeedbackContext = createContext<Feedback | null>(null);

/** Translated text for an error: the API code when known, else its message. */
export function useErrorText() {
  const { t } = useI18n();
  return useCallback(
    (error: unknown) => {
      if (error instanceof ApiError) {
        const key = `err.${error.code}`;
        if (key in en) return t(key as MessageKey);
        if (error.message) return error.message;
      }
      return t('err.generic');
    },
    [t],
  );
}

export function FeedbackProvider({ children }: { children: ReactNode }) {
  const [toasts, setToasts] = useState<Toast[]>([]);
  const errorText = useErrorText();

  const push = useCallback((text: string, kind: Toast['kind']) => {
    const id = Date.now() + Math.random();
    setToasts((current) => [...current, { id, text, kind }]);
    window.setTimeout(() => setToasts((current) => current.filter((toast) => toast.id !== id)), kind === 'error' ? 6000 : 3000);
  }, []);

  const showError = useCallback(
    (error: unknown) => {
      // These two are handled by the session (login page, password page).
      if (error instanceof ApiError && (error.status === 401 || error.code === 'password_change_required')) return;
      push(errorText(error), 'error');
    },
    [errorText, push],
  );
  const showInfo = useCallback((text: string) => push(text, 'info'), [push]);

  const run = useCallback(
    async (action: () => Promise<unknown>) => {
      try {
        await action();
        return true;
      } catch (error) {
        showError(error);
        return false;
      }
    },
    [showError],
  );

  const value = useMemo(() => ({ showError, showInfo, run }), [showError, showInfo, run]);
  return (
    <FeedbackContext.Provider value={value}>
      {children}
      <div className="toasts" role="status" aria-live="polite">
        {toasts.map((toast) => (
          <div key={toast.id} className={`toast toast-${toast.kind}`}>
            {toast.text}
          </div>
        ))}
      </div>
    </FeedbackContext.Provider>
  );
}

export function useFeedback(): Feedback {
  const context = useContext(FeedbackContext);
  if (!context) throw new Error('useFeedback must be used inside FeedbackProvider');
  return context;
}
