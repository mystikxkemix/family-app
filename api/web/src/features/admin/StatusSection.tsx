import { useI18n } from '../../i18n';
import { api } from '../../lib/api';
import { useLoader } from '../../lib/hooks';
import { formatDateTime } from '../../lib/format';
import type { AdminStatus } from '../../lib/types';
import { Loading } from '../../components/ui';

/** Version, counts, and which features the server has. */
export function StatusSection() {
  const { t, locale } = useI18n();
  const status = useLoader(() => api.get<AdminStatus>('admin/status'));
  const data = status.data;
  const onOff = (value: boolean) => (value ? t('common.on') : t('common.off'));

  return (
    <>
      <h2 className="section-title">{t('admin.status')}</h2>
      <div className="card form-card">
        {!data && <Loading />}
        {data && (
          <>
            <dl className="facts">
              <dt>{t('admin.version')}</dt>
              <dd>{data.version}</dd>
              <dt>{t('admin.startedAt')}</dt>
              <dd>{formatDateTime(new Date(data.startedAt), locale)}</dd>
              <dt>{t('admin.users')}</dt>
              <dd>{data.users}</dd>
              <dt>{t('admin.households')}</dt>
              <dd>{data.households}</dd>
              <dt>{t('admin.push')}</dt>
              <dd>{onOff(data.pushEnabled)}</dd>
              <dt>Radarr</dt>
              <dd>{onOff(data.integrations.radarr)}</dd>
              <dt>Sonarr</dt>
              <dd>{onOff(data.integrations.sonarr)}</dd>
              <dt>Prowlarr</dt>
              <dd>{onOff(data.integrations.prowlarr)}</dd>
              <dt>{t('admin.settingsFile')}</dt>
              <dd>
                <code>{data.settingsFile}</code> ({data.settingsFound ? t('admin.settingsFound') : t('admin.settingsMissing')})
              </dd>
            </dl>
            {!data.settingsWritable && <p className="warning">{t('admin.notWritable')}</p>}
          </>
        )}
      </div>
    </>
  );
}
