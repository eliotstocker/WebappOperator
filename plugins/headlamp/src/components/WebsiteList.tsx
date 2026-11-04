import {Box, Link as MuiLink} from '@mui/material';
import {Link, ResourceListView, StatusLabel} from '@kinvolk/headlamp-plugin/lib/CommonComponents';
import {Website} from '../resources/website';
import {getWebsiteStatusColor} from '../utils/status';

export default function WebsiteList() {
    return <ResourceListView
        title="Websites"
        resourceClass={Website}
        columns={[
            'name',
            'namespace',
            {
                id: 'status',
                label: 'Status',
                getValue: (site: Website) => site.phase,
                render: (site: Website) => (
                    <StatusLabel status={getWebsiteStatusColor(site.phase)}>{site.phase}</StatusLabel>
                ),
            },
            {
                id: 'active-revision',
                label: 'Active Revision',
                getValue: (site: Website) => site.activeRevision,
                render: (site: Website) => {
                    if(!site.status?.activeRevision) {
                        return '-';
                    }
                    return <Link
                        routeName="webapp-revision-detail"
                        params={{namespace: site.getNamespace(), name: site.status.activeRevision}}
                    >
                        {site.status.activeRevision}
                    </Link>;
                },
            },
            {
                id: 'hostnames',
                label: 'Hostnames',
                getValue: (site: Website) => site.hostnames.join(', '),
                render: (site: Website) => (
                    <Box sx={{display: 'flex', flexWrap: 'wrap', gap: 0.5}}>
                        {site.hostnames.map(host => (
                            <MuiLink
                                key={host}
                                href={host.startsWith('http') ? host : `https://${host}`}
                                target="_blank"
                                rel="noopener noreferrer"
                                underline="hover"
                            >
                                {host}
                            </MuiLink>
                        ))}
                    </Box>
                ),
            },
            {
                id: 'image',
                label: 'Image',
                getValue: (site: Website) => site.activeImage,
            },
            'age',
        ]}
    />;
}
