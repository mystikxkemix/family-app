import { useEffect, useId, useRef, type FormEvent, type ReactNode } from 'react';
import { Icon, type IconName } from './Icon';
import { useI18n } from '../i18n';

/** Page title with optional actions on the right. */
export function PageHeader({ title, actions, onBack }: { title: string; actions?: ReactNode; onBack?: () => void }) {
  const { t } = useI18n();
  return (
    <header className="page-header">
      {onBack && (
        <button type="button" className="icon-button" onClick={onBack} aria-label={t('common.back')}>
          <Icon name="back" />
        </button>
      )}
      <h1>{title}</h1>
      <div className="page-actions">{actions}</div>
    </header>
  );
}

/** A button with an icon and a label (the label is hidden on small screens if compact). */
export function IconButton({
  icon,
  label,
  onClick,
  compact = false,
  kind = 'plain',
  disabled,
}: {
  icon: IconName;
  label: string;
  onClick: () => void;
  compact?: boolean;
  kind?: 'plain' | 'primary' | 'danger';
  disabled?: boolean;
}) {
  return (
    <button
      type="button"
      className={`button button-${kind}${compact ? ' button-compact' : ''}`}
      onClick={onClick}
      aria-label={label}
      title={label}
      disabled={disabled}
    >
      <Icon name={icon} size={18} />
      <span className="button-label">{label}</span>
    </button>
  );
}

/** Tabs like the iOS segmented picker. */
export function Segmented<T extends string>({
  value,
  options,
  onChange,
  label,
}: {
  value: T;
  options: { value: T; label: string }[];
  onChange: (value: T) => void;
  label: string;
}) {
  return (
    <div className="segmented" role="tablist" aria-label={label}>
      {options.map((option) => (
        <button
          key={option.value}
          type="button"
          role="tab"
          aria-selected={option.value === value}
          className={option.value === value ? 'selected' : ''}
          onClick={() => onChange(option.value)}
        >
          {option.label}
        </button>
      ))}
    </div>
  );
}

export function EmptyState({ icon, title, body, action }: { icon: IconName; title: string; body: string; action?: ReactNode }) {
  return (
    <div className="empty">
      <Icon name={icon} size={44} />
      <h2>{title}</h2>
      <p>{body}</p>
      {action}
    </div>
  );
}

export function Loading() {
  const { t } = useI18n();
  return <p className="muted center">{t('common.loading')}</p>;
}

/**
 * A modal form. Esc and the backdrop close it; submitting calls onSubmit.
 * The submit button is disabled while `busy` or when `canSubmit` is false.
 */
export function Modal({
  title,
  children,
  onClose,
  onSubmit,
  submitLabel,
  canSubmit = true,
  busy = false,
  footer,
}: {
  title: string;
  children: ReactNode;
  onClose: () => void;
  onSubmit?: () => void;
  submitLabel?: string;
  canSubmit?: boolean;
  busy?: boolean;
  footer?: ReactNode;
}) {
  const { t } = useI18n();
  const titleId = useId();
  const panel = useRef<HTMLFormElement>(null);
  // The latest onClose, so the effect below runs only once (focus stays put).
  const close = useRef(onClose);
  close.current = onClose;

  useEffect(() => {
    const onKey = (event: KeyboardEvent) => {
      if (event.key === 'Escape') close.current();
    };
    window.addEventListener('keydown', onKey);
    panel.current?.querySelector<HTMLElement>('input, textarea, select, button')?.focus();
    return () => window.removeEventListener('keydown', onKey);
  }, []);

  const submit = (event: FormEvent) => {
    event.preventDefault();
    if (onSubmit && canSubmit && !busy) onSubmit();
  };

  return (
    <div className="backdrop" onMouseDown={(event) => event.target === event.currentTarget && onClose()}>
      <form ref={panel} className="modal" role="dialog" aria-modal="true" aria-labelledby={titleId} onSubmit={submit}>
        <h2 id={titleId}>{title}</h2>
        <div className="modal-body">{children}</div>
        <div className="modal-footer">
          {footer}
          <span className="spacer" />
          <button type="button" className="button" onClick={onClose}>
            {onSubmit ? t('common.cancel') : t('common.close')}
          </button>
          {onSubmit && (
            <button type="submit" className="button button-primary" disabled={!canSubmit || busy}>
              {submitLabel ?? t('common.save')}
            </button>
          )}
        </div>
      </form>
    </div>
  );
}

/** A labelled form field. */
export function Field({ label, children, hint }: { label: string; children: ReactNode; hint?: string }) {
  return (
    <label className="field">
      <span className="field-label">{label}</span>
      {children}
      {hint && <span className="field-hint">{hint}</span>}
    </label>
  );
}
