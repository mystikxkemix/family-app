import { useState } from 'react';
import { useI18n } from '../../i18n';
import { api } from '../../lib/api';
import { useLoader } from '../../lib/hooks';
import { useMe, useSession } from '../../lib/session';
import { copyText, formatDateTime } from '../../lib/format';
import type { AdminUser } from '../../lib/types';
import { useFeedback } from '../../components/Feedback';
import { Loading, Modal } from '../../components/ui';
import { Icon } from '../../components/Icon';

/** Every account: promote/demote admin, reset password, delete. */
export function UsersSection() {
  const { t, locale } = useI18n();
  const me = useMe();
  const { refreshMe } = useSession();
  const { run, showInfo } = useFeedback();
  const users = useLoader(() => api.get<AdminUser[]>('admin/users'));
  const [temporary, setTemporary] = useState<{ name: string; password: string }>();

  const setAdmin = (user: AdminUser, isAdmin: boolean) =>
    void run(async () => {
      const updated = await api.patch<AdminUser>(`admin/users/${user.id}`, { isAdmin });
      users.setData((current) => current?.map((other) => (other.id === updated.id ? updated : other)));
      // Removing your own admin role hides this page.
      if (user.id === me.user.id) await refreshMe();
    });

  const resetPassword = (user: AdminUser) => {
    if (!window.confirm(t('admin.resetConfirm', { name: user.displayName }))) return;
    void run(async () => {
      const response = await api.post<{ temporaryPassword: string }>(`admin/users/${user.id}/reset-password`);
      setTemporary({ name: user.displayName, password: response.temporaryPassword });
      await users.reload();
    });
  };

  const remove = (user: AdminUser) => {
    if (!window.confirm(t('admin.deleteConfirm', { name: user.displayName }))) return;
    void run(async () => {
      await api.delete(`admin/users/${user.id}`);
      users.setData((current) => current?.filter((other) => other.id !== user.id));
      await refreshMe();
    });
  };

  return (
    <>
      <h2 className="section-title">{t('admin.users')}</h2>
      {users.loading && <Loading />}
      <ul className="cards">
        {users.data?.map((user) => {
          const isMe = user.id === me.user.id;
          return (
            <li key={user.id} className="card user-card">
              <div className="row-text">
                <strong>
                  {user.displayName} <span className="muted small">@{user.username}</span>
                  {isMe && <span className="muted small"> ({t('admin.you')})</span>}
                </strong>
                <span className="muted small">
                  {user.householdName} · {formatDateTime(new Date(user.createdAt), locale)}
                </span>
                <span className="badges">
                  <span className={`badge${user.isAdmin ? ' accent' : ''}`}>{user.isAdmin ? t('admin.roleAdmin') : t('admin.roleMember')}</span>
                  {user.mustChangePassword && <span className="badge warning">{t('admin.mustChange')}</span>}
                </span>
              </div>
              <div className="user-actions">
                <button type="button" className="button" onClick={() => setAdmin(user, !user.isAdmin)}>
                  {user.isAdmin ? t('admin.removeAdmin') : t('admin.makeAdmin')}
                </button>
                {!isMe && (
                  <>
                    <button type="button" className="button" onClick={() => resetPassword(user)}>
                      {t('admin.resetPassword')}
                    </button>
                    <button type="button" className="button button-danger" onClick={() => remove(user)}>
                      <Icon name="trash" size={18} /> {t('admin.deleteUser')}
                    </button>
                  </>
                )}
              </div>
            </li>
          );
        })}
      </ul>
      {temporary && (
        <Modal title={t('admin.tempPasswordTitle', { name: temporary.name })} onClose={() => setTemporary(undefined)}>
          <div className="invite-code">
            <code>{temporary.password}</code>
            <button
              type="button"
              className="button"
              onClick={() => void copyText(temporary.password).then((ok) => ok && showInfo(t('common.copied')))}
            >
              <Icon name="copy" size={18} /> {t('common.copy')}
            </button>
          </div>
          <p className="muted small">{t('admin.tempPasswordHint', { name: temporary.name })}</p>
        </Modal>
      )}
    </>
  );
}
