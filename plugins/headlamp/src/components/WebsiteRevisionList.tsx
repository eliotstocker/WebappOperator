import {Link, ResourceListView, StatusLabel} from '@kinvolk/headlamp-plugin/lib/CommonComponents';
import {WebsiteRevision} from '../resources/websiterevision';
import {formatBytes, getRevisionStatusColor} from '../utils/status';

export default function WebsiteRevisionList() {
    return <ResourceListView
        title="Website Revisions"
        resourceClass={WebsiteRevision}
        columns={[
            'name',
            'namespace',
            {
                id: 'website',
                label: 'Website',
                getValue: (rev: WebsiteRevision) => rev.websiteName,
                render: (rev: WebsiteRevision) => (
                    <Link
                        routeName="webapp-detail"
                        params={{namespace: rev.getNamespace(), name: rev.websiteName}}
                    >
                        {rev.websiteName}
                    </Link>
                ),
            },
            {
                id: 'status',
                label: 'Status',
                getValue: (rev: WebsiteRevision) => rev.phase,
                render: (rev: WebsiteRevision) => (
                    <StatusLabel status={getRevisionStatusColor(rev.phase)}>{rev.phase}</StatusLabel>
                ),
            },
            {
                id: 'image',
                label: 'Image',
                getValue: (rev: WebsiteRevision) => rev.spec?.image || '-',
            },
            {
                id: 'synced-pods',
                label: 'Synced Pods',
                getValue: (rev: WebsiteRevision) => rev.readyPods.length,
                render: (rev: WebsiteRevision) => `${rev.readyPods.length} pods`,
            },
            {
                id: 'unpacked-size',
                label: 'Size',
                getValue: (rev: WebsiteRevision) => rev.unpackedSize,
                render: (rev: WebsiteRevision) => formatBytes(rev.unpackedSize),
            },
            'age',
        ]}
    />;
}
