import {Box, Chip, Link as MuiLink, Typography} from '@mui/material';
import {
    DateLabel,
    DetailsGrid,
    Link,
    NameValueTable,
    SectionBox,
    SimpleTable,
    StatusLabel,
} from '@kinvolk/headlamp-plugin/lib/CommonComponents';
import {useParams} from 'react-router-dom';
import {Website} from '../resources/website';
import {WebsiteRevision} from '../resources/websiterevision';
import {formatBytes, getRevisionStatusColor, getWebsiteStatusColor} from '../utils/status';

export default function WebsiteDetail() {
    const {namespace, name} = useParams<{namespace: string; name: string}>();
    const [revisions] = WebsiteRevision.useList({namespace});

    const siteRevisions = (revisions || []).filter(rev => rev.websiteName === name);

    return <DetailsGrid
        resourceType={Website}
        name={name}
        namespace={namespace}
        withEvents
        extraInfo={(site: Website) => {
            if(!site) {
                return [];
            }

            return [
                {
                    name: 'Status',
                    value: <StatusLabel status={getWebsiteStatusColor(site.phase)}>{site.phase}</StatusLabel>,
                },
                {
                    name: 'Desired Image',
                    value: site.spec?.image || '-',
                },
                {
                    name: 'Active Image',
                    value: site.activeImage,
                },
                {
                    name: 'Active Revision',
                    value: site.status?.activeRevision ? (
                        <Link
                            routeName="webapp-revision-detail"
                            params={{namespace: site.getNamespace(), name: site.status.activeRevision}}
                        >
                            {site.status.activeRevision}
                        </Link>
                    ) : (
                        '-'
                    ),
                },
                {
                    name: 'Hostnames',
                    value: site.hostnames.length < 1 ? '-' : (
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
                    name: 'Injection Mode',
                    value: site.spec?.injection?.mode || 'endpoint',
                },
                {
                    name: 'Config Endpoint',
                    value: site.spec?.injection?.path || '/_config.js',
                },
                {
                    name: 'Version Endpoint',
                    value: site.spec?.injection?.versionPath || '/_version',
                },
                {
                    name: 'Version Polling',
                    value: site.spec?.injection?.versionPolling === false ? 'Disabled' : `Every ${site.spec?.injection?.pollIntervalSeconds || 30}s`,
                },
                {
                    name: 'Revision History Limit',
                    value: site.spec?.revisionHistoryLimit ?? 3,
                },
                {
                    name: 'Ingress Managed',
                    value: site.spec?.ingress?.enabled ? 'Yes' : 'No',
                },
            ];
        }}
        extraSections={(site: Website) => {
            if(!site) {
                return [];
            }

            return [
                getRevisionsSection(siteRevisions),
                getEnvSection(site.spec?.env),
                getIngressSection(site.spec?.ingress),
            ].filter(Boolean);
        }}
    />;
}

function getRevisionsSection(revisions: WebsiteRevision[]) {
    //show table of child revisions
    return {
        id: 'website-revisions',
        section: <SectionBox title={`Revisions (${revisions.length})`}>
            {revisions.length < 1 ? (
                <Typography variant="body2" color="textSecondary">
                    No revisions found for this website.
                </Typography>
            ) : (
                <SimpleTable
                    columns={[
                        {
                            label: 'Name',
                            getter: (rev: WebsiteRevision) => (
                                <Link
                                    routeName="webapp-revision-detail"
                                    params={{namespace: rev.getNamespace(), name: rev.metadata.name}}
                                >
                                    {rev.metadata.name}
                                </Link>
                            ),
                        },
                        {
                            label: 'Phase',
                            getter: (rev: WebsiteRevision) => (
                                <StatusLabel status={getRevisionStatusColor(rev.phase)}>
                                    {rev.phase}
                                </StatusLabel>
                            ),
                        },
                        {
                            label: 'Ready Pods',
                            getter: (rev: WebsiteRevision) => (
                                <Box sx={{display: 'flex', flexWrap: 'wrap', gap: 0.5}}>
                                    {rev.readyPods.length < 1 ? (
                                        <Typography variant="body2" color="textSecondary">0 pods</Typography>
                                    ) : (
                                        rev.readyPods.map(pod => (
                                            <Chip key={pod} label={pod} size="small" variant="outlined" />
                                        ))
                                    )}
                                </Box>
                            ),
                        },
                        {
                            label: 'Unpacked Size',
                            getter: (rev: WebsiteRevision) => formatBytes(rev.unpackedSize),
                        },
                        {
                            label: 'Age',
                            getter: (rev: WebsiteRevision) => (
                                <DateLabel date={rev.metadata.creationTimestamp} />
                            ),
                        },
                    ]}
                    data={revisions}
                />
            )}
        </SectionBox>,
    };
}

function getEnvSection(env?: Record<string, string>) {
    if(!env || Object.keys(env).length < 1) {
        return null;
    }

    const rows = Object.entries(env).map(([k, v]) => ({
        name: k,
        value: <code>{v}</code>,
    }));

    return {
        id: 'website-env',
        section: <SectionBox title="Injected Environment Variables">
            <NameValueTable rows={rows} />
        </SectionBox>,
    };
}

function getIngressSection(ingress?: Website['spec']['ingress']) {
    if(!ingress || !ingress.enabled) {
        return null;
    }

    const rows = [
        {name: 'Class Name', value: ingress.className || '-'},
        {
            name: 'TLS Secrets',
            value: ingress.tls && ingress.tls.length > 0 ? (
                ingress.tls.map(t => `${t.secretName || 'unnamed'} (${(t.hosts || []).join(', ')})`).join('; ')
            ) : 'Disabled',
        },
    ];

    return {
        id: 'website-ingress',
        section: <SectionBox title="Ingress Settings">
            <NameValueTable rows={rows} />
        </SectionBox>,
    };
}
