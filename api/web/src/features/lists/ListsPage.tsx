import { useState } from 'react';
import { useI18n } from '../../i18n';
import { api } from '../../lib/api';
import { navigate, useLoader } from '../../lib/hooks';
import { listKinds, type ListKind, type SharedList } from '../../lib/types';
import { useFeedback } from '../../components/Feedback';
import { EmptyState, Field, IconButton, Loading, Modal, PageHeader } from '../../components/ui';
import { Icon } from '../../components/Icon';

const defaultEmoji: Record<ListKind, string> = { grocery: '🛒', todo: '✅', other: '📝' };

export function listEmoji(list: SharedList): string {
  return list.emoji || defaultEmoji[list.kind];
}

/** All shared lists of the household, with what is left to do. */
export function ListsPage() {
  const { t, tp } = useI18n();
  const { run } = useFeedback();
  const lists = useLoader(() => api.get<SharedList[]>('lists'));
  const [editing, setEditing] = useState<SharedList | 'new'>();

  const remove = (list: SharedList) => {
    if (!window.confirm(t('lists.deleteConfirm', { name: list.name }))) return;
    lists.setData((current) => current?.filter((item) => item.id !== list.id));
    void run(() => api.delete(`lists/${list.id}`)).then(() => lists.reload());
  };

  return (
    <section className="page">
      <PageHeader title={t('lists.title')} actions={<IconButton icon="plus" label={t('lists.new')} kind="primary" onClick={() => setEditing('new')} />} />
      {lists.loading && <Loading />}
      {lists.data && lists.data.length === 0 && (
        <EmptyState icon="lists" title={t('lists.emptyTitle')} body={t('lists.emptyBody')} />
      )}
      <ul className="cards">
        {lists.data?.map((list) => (
          <li key={list.id} className="card row-card">
            <button type="button" className="row-main" onClick={() => navigate(`/lists/${list.id}`)}>
              <span className="emoji" aria-hidden="true">
                {listEmoji(list)}
              </span>
              <span className="row-text">
                <strong>{list.name}</strong>
                <span className="muted small">
                  {list.totalCount === 0 ? t('lists.empty') : list.openCount === 0 ? t('lists.allDone') : tp('lists.left', list.openCount)}
                </span>
              </span>
            </button>
            <div className="row-actions">
              <button type="button" className="icon-button" aria-label={t('lists.edit')} onClick={() => setEditing(list)}>
                <Icon name="edit" size={18} />
              </button>
              <button type="button" className="icon-button danger" aria-label={t('common.delete')} onClick={() => remove(list)}>
                <Icon name="trash" size={18} />
              </button>
            </div>
          </li>
        ))}
      </ul>
      {editing && <ListEditor list={editing === 'new' ? undefined : editing} onClose={() => setEditing(undefined)} onSaved={lists.reload} />}
    </section>
  );
}

/** Creates or renames a list. */
function ListEditor({ list, onClose, onSaved }: { list?: SharedList; onClose: () => void; onSaved: () => Promise<void> }) {
  const { t } = useI18n();
  const { run } = useFeedback();
  const [name, setName] = useState(list?.name ?? '');
  const [kind, setKind] = useState<ListKind>(list?.kind ?? 'grocery');
  const [emoji, setEmoji] = useState(list?.emoji ?? '');
  const [busy, setBusy] = useState(false);

  const save = async () => {
    setBusy(true);
    const body = { name: name.trim(), kind, emoji: emoji.trim() };
    const saved = await run(() => (list ? api.patch(`lists/${list.id}`, body) : api.post('lists', body)));
    setBusy(false);
    if (saved) {
      await onSaved();
      onClose();
    }
  };

  return (
    <Modal
      title={list ? t('lists.edit') : t('lists.new')}
      onClose={onClose}
      onSubmit={() => void save()}
      canSubmit={name.trim() !== ''}
      busy={busy}
    >
      <Field label={t('lists.name')}>
        <input value={name} onChange={(e) => setName(e.target.value)} maxLength={60} />
      </Field>
      <Field label={t('lists.type')}>
        <select value={kind} onChange={(e) => setKind(e.target.value as ListKind)}>
          {listKinds.map((value) => (
            <option key={value} value={value}>
              {t(`kind.${value}`)}
            </option>
          ))}
        </select>
      </Field>
      <Field label={t('lists.emoji')}>
        <input value={emoji} onChange={(e) => setEmoji(e.target.value)} maxLength={8} placeholder={defaultEmoji[kind]} />
      </Field>
    </Modal>
  );
}
