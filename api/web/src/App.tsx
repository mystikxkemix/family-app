import { useEffect } from 'react';
import { useI18n, type MessageKey } from './i18n';
import { navigate, usePath } from './lib/hooks';
import { useSession } from './lib/session';
import { Icon, type IconName } from './components/Icon';
import { Loading } from './components/ui';
import { LoginPage } from './features/auth/LoginPage';
import { ChoosePasswordPage } from './features/auth/ChoosePasswordPage';
import { ListsPage } from './features/lists/ListsPage';
import { ListDetailPage } from './features/lists/ListDetailPage';
import { WatchlistPage } from './features/watchlist/WatchlistPage';
import { MediaDetailPage } from './features/watchlist/MediaDetailPage';
import { PlacesPage } from './features/places/PlacesPage';
import { EventsPage } from './features/events/EventsPage';
import { SettingsPage } from './features/settings/SettingsPage';
import { AdminPage } from './features/admin/AdminPage';

interface Tab {
  path: string;
  label: MessageKey;
  icon: IconName;
  adminOnly?: boolean;
}

const tabs: Tab[] = [
  { path: '/lists', label: 'nav.lists', icon: 'lists' },
  { path: '/watchlist', label: 'nav.watchlist', icon: 'watchlist' },
  { path: '/places', label: 'nav.places', icon: 'places' },
  { path: '/events', label: 'nav.events', icon: 'events' },
  { path: '/settings', label: 'nav.settings', icon: 'settings' },
  { path: '/admin', label: 'nav.admin', icon: 'admin', adminOnly: true },
];

/** Login, forced password change, or the app with its navigation. */
export function App() {
  const { me, passwordChangeRequired } = useSession();
  if (me === undefined) return <Loading />;
  if (me === null) return <LoginPage />;
  if (passwordChangeRequired) return <ChoosePasswordPage />;
  return <Shell isAdmin={me.user.isAdmin} />;
}

function Shell({ isAdmin }: { isAdmin: boolean }) {
  const { t } = useI18n();
  const path = usePath();
  const visibleTabs = tabs.filter((tab) => !tab.adminOnly || isAdmin);

  // Unknown paths (and "/") go to the lists; non-admins leave /admin.
  const known = visibleTabs.some((tab) => path === tab.path || path.startsWith(`${tab.path}/`));
  useEffect(() => {
    if (!known) navigate('/lists');
  }, [known]);

  return (
    <div className="shell">
      <nav className="nav" aria-label={t('app.name')}>
        <div className="brand">
          <img src="/icon-512.png" alt="" width={32} height={32} />
          <span>{t('app.name')}</span>
        </div>
        {visibleTabs.map((tab) => {
          const active = path === tab.path || path.startsWith(`${tab.path}/`);
          return (
            <a
              key={tab.path}
              href={tab.path}
              className={active ? 'active' : ''}
              aria-current={active ? 'page' : undefined}
              onClick={(event) => {
                if (event.metaKey || event.ctrlKey || event.shiftKey) return;
                event.preventDefault();
                navigate(tab.path);
              }}
            >
              <Icon name={tab.icon} size={22} />
              <span>{t(tab.label)}</span>
            </a>
          );
        })}
      </nav>
      <main className="content">{known ? <Route path={path} /> : null}</main>
    </div>
  );
}

function Route({ path }: { path: string }) {
  const listMatch = /^\/lists\/([0-9a-f]{24})$/.exec(path);
  if (listMatch) return <ListDetailPage listId={listMatch[1]} />;
  const mediaMatch = /^\/watchlist\/([0-9a-f]{24})$/.exec(path);
  if (mediaMatch) return <MediaDetailPage mediaId={mediaMatch[1]} />;
  switch (path) {
    case '/watchlist':
      return <WatchlistPage />;
    case '/places':
      return <PlacesPage />;
    case '/events':
      return <EventsPage />;
    case '/settings':
      return <SettingsPage />;
    case '/admin':
      return <AdminPage />;
    default:
      return <ListsPage />;
  }
}
