import { useI18n } from '../i18n';
import { useMe, useSession } from '../lib/session';
import { formatNames } from '../lib/format';
import { Icon } from './Icon';

/** Who wants to watch / to go, with a special line when it is everyone. */
export function InterestLine({ userIds }: { userIds: string[] }) {
  const { t, locale } = useI18n();
  const me = useMe();
  const { memberName } = useSession();
  if (userIds.length === 0) return null;
  if (me.members.length > 1 && userIds.length >= me.members.length) {
    return (
      <span className="interest everyone">
        <Icon name="heart" size={14} filled /> {t('interest.everyone')}
      </span>
    );
  }
  const names = userIds.map((id) => memberName(id, t('common.someone')));
  return (
    <span className="interest">
      <Icon name="heart" size={14} /> {formatNames(names, locale)}
    </span>
  );
}
