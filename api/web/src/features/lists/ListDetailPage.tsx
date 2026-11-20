import { useEffect, useRef, useState, type FormEvent } from 'react';
import { useI18n } from '../../i18n';
import { api } from '../../lib/api';
import { navigate, useLoader } from '../../lib/hooks';
import type { ListItem, SharedList } from '../../lib/types';
import { useFeedback } from '../../components/Feedback';
import { Field, IconButton, Loading, Modal, PageHeader } from '../../components/ui';
import { Icon } from '../../components/Icon';
import { listEmoji } from './ListsPage';

/** The items of one list: add quickly, tap to check, edit, delete. */
export function ListDetailPage({ listId }: { listId: string }) {
  const { t } = useI18n();
  const { run } = useFeedback();
  const list = useLoader(() => api.get<SharedList[]>('lists').then((all) => all.find((item) => item.id === listId) ?? null), [listId]);
  const items = useLoader(() => api.get<ListItem[]>(`lists/${listId}/items`), [listId]);
  const [newText, setNewText] = useState('');
  const [editing, setEditing] = useState<ListItem>();
  const addField = useRef<HTMLInputElement>(null);

  const all = items.data ?? [];
  const open = all.filter((item) => !item.checked);
  const checked = all
    .filter((item) => item.checked)
    .sort((a, b) => (b.checkedAt ?? '').localeCompare(a.checkedAt ?? ''));

  /** Adds the item and keeps the focus in the field for the next one. */
  const add = (event: FormEvent) => {
    event.preventDefault();
    const text = newText.trim();
    if (!text) return;
    setNewText('');
    addField.current?.focus();
    void run(async () => {
      const item = await api.post<ListItem>(`lists/${listId}/items`, { text });
      items.setData((current) => [...(current ?? []), item]);
    });
  };

  /** Checks or unchecks at once, then saves. A failure reloads the truth. */
  const toggle = (item: ListItem) => {
    const nextChecked = !item.checked;
    items.setData((current) =>
      current?.map((other) =>
        other.id === item.id ? { ...other, checked: nextChecked, checkedAt: nextChecked ? new Date().toISOString() : undefined } : other,
      ),
    );
    void run(async () => {
      const updated = await api.patch<ListItem>(`lists/${listId}/items/${item.id}`, { checked: nextChecked });
      items.setData((current) => current?.map((other) => (other.id === updated.id ? updated : other)));
    }).then((ok) => {
      if (!ok) void items.reload();
    });
  };

  const remove = (item: ListItem) => {
    items.setData((current) => current?.filter((other) => other.id !== item.id));
    void run(() => api.delete(`lists/${listId}/items/${item.id}`));
  };

  const removeChecked = () => {
    items.setData((current) => current?.filter((item) => !item.checked));
    void run(() => api.post(`lists/${listId}/clear-checked`));
  };

  // The list was deleted (here or by someone else): back to all lists.
  useEffect(() => {
    if (list.data === null) navigate('/lists');
  }, [list.data]);

  const title = list.data ? `${listEmoji(list.data)} ${list.data.name}` : '';
  return (
    <section className="page">
      <PageHeader
        title={title}
        onBack={() => navigate('/lists')}
        actions={
          checked.length > 0 && <IconButton icon="trash" label={t('items.removeChecked')} onClick={removeChecked} compact />
        }
      />
      <form className="add-row card" onSubmit={add}>
        <Icon name="plus" />
        <input
          ref={addField}
          value={newText}
          onChange={(e) => setNewText(e.target.value)}
          placeholder={t('items.add')}
          aria-label={t('items.add')}
          maxLength={200}
        />
      </form>
      {items.loading && <Loading />}
      <ItemGroup items={open} onToggle={toggle} onEdit={setEditing} onRemove={remove} />
      {checked.length > 0 && (
        <>
          <h2 className="section-title">{list.data?.kind === 'grocery' ? t('items.inCart') : t('items.done')}</h2>
          <ItemGroup items={checked} onToggle={toggle} onEdit={setEditing} onRemove={remove} />
        </>
      )}
      {editing && <ItemEditor item={editing} onClose={() => setEditing(undefined)} onSaved={items.reload} />}
    </section>
  );
}

function ItemGroup({
  items,
  onToggle,
  onEdit,
  onRemove,
}: {
  items: ListItem[];
  onToggle: (item: ListItem) => void;
  onEdit: (item: ListItem) => void;
  onRemove: (item: ListItem) => void;
}) {
  const { t } = useI18n();
  if (items.length === 0) return null;
  return (
    <ul className="cards">
      {items.map((item) => (
        <li key={item.id} className={`card row-card item${item.checked ? ' checked' : ''}`}>
          <button type="button" className="row-main" onClick={() => onToggle(item)} aria-pressed={item.checked}>
            <span className="check" aria-hidden="true">
              <Icon name={item.checked ? 'check' : 'circle'} />
            </span>
            <span className="row-text">
              <span className="item-text">{item.text}</span>
              {item.note && <span className="muted small">{item.note}</span>}
            </span>
          </button>
          <div className="row-actions">
            <button type="button" className="icon-button" aria-label={t('items.edit')} onClick={() => onEdit(item)}>
              <Icon name="edit" size={18} />
            </button>
            <button type="button" className="icon-button danger" aria-label={t('common.delete')} onClick={() => onRemove(item)}>
              <Icon name="trash" size={18} />
            </button>
          </div>
        </li>
      ))}
    </ul>
  );
}

function ItemEditor({ item, onClose, onSaved }: { item: ListItem; onClose: () => void; onSaved: () => Promise<void> }) {
  const { t } = useI18n();
  const { run } = useFeedback();
  const [text, setText] = useState(item.text);
  const [note, setNote] = useState(item.note);
  const [busy, setBusy] = useState(false);

  const save = async () => {
    setBusy(true);
    const saved = await run(() => api.patch(`lists/${item.listId}/items/${item.id}`, { text: text.trim(), note: note.trim() }));
    setBusy(false);
    if (saved) {
      await onSaved();
      onClose();
    }
  };

  return (
    <Modal title={t('items.edit')} onClose={onClose} onSubmit={() => void save()} canSubmit={text.trim() !== ''} busy={busy}>
      <Field label={t('items.item')}>
        <input value={text} onChange={(e) => setText(e.target.value)} maxLength={200} />
      </Field>
      <Field label={t('items.note')}>
        <textarea value={note} onChange={(e) => setNote(e.target.value)} rows={3} maxLength={500} />
      </Field>
    </Modal>
  );
}
