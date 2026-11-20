import { useI18n } from '../../i18n';
import { PageHeader } from '../../components/ui';
import { StatusSection } from './StatusSection';
import { UsersSection } from './UsersSection';
import { MediaToolsSection, PushSection, useAdminSettings } from './SettingsSections';

/** Server administration, for admins only (the API checks it too). */
export function AdminPage() {
  const { t } = useI18n();
  const settings = useAdminSettings();
  return (
    <section className="page narrow">
      <PageHeader title={t('admin.title')} />
      <StatusSection />
      <UsersSection />
      <MediaToolsSection settings={settings} />
      <PushSection settings={settings} />
    </section>
  );
}
