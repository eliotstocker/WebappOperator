import {Box, Chip, Typography} from '@mui/material';
import {
    DetailsGrid,
    Link,
    NameValueTable,
    SectionBox,
    StatusLabel,
} from '@kinvolk/headlamp-plugin/lib/CommonComponents';
import {useParams} from 'react-router-dom';
import {WebsiteRevision} from '../resources/websiterevision';
import {formatBytes, getRevisionStatusColor} from '../utils/status';

export default function WebsiteRevisionDetail() {
    const {namespace, name} = useParams<{namespace: string; name: string}>();

    return <DetailsGrid
        resourceType={WebsiteRevision}
        name={name}
        namespace={namespace}
        withEvents
        extraInfo={(rev: WebsiteRevision) => {
            if(!rev) {
                return [];
            }

            return [
                {
                    name: 'Status',
                    value: <StatusLabel status={getRevisionStatusColor(rev.phase)}>{rev.phase}</StatusLabel>,
                },
                {
                    name: 'Parent Website',
                    value: rev.websiteName ? (
                        <Link
                            routeName="webapp-detail"
                            params={{namespace: rev.getNamespace(), name: rev.websiteName}}
                        >
                            {rev.websiteName}
                        </Link>
                    ) : (
                        '-'
                    ),
                },
                {
                    name: 'Image',
                    value: rev.spec?.image || '-',
                },
                {
                    name: 'Digest',
                    value: rev.digest !== '-' ? <code>{rev.digest}</code> : '-',
                },
                {
                    name: 'Synced Pods',
                    value: `${rev.readyPods.length} pods`,
                },
                {
                    name: 'Unpacked Size',
                    value: formatBytes(rev.unpackedSize),
                },
                {
                    name: 'Injection Mode',
                    value: rev.spec?.injection?.mode || 'endpoint',
                },
            ];
        }}
        extraSections={(rev: WebsiteRevision) => {
            if(!rev) {
                return [];
            }

            return [
                getSyncedPodsSection(rev.readyPods),
                getEnvSnapshotSection(rev.spec?.env),
            ].filter(Boolean);
        }}
    />;
}

function getSyncedPodsSection(readyPods: string[]) {
    //show pods that have synced the revision
    return {
        id: 'revision-pods',
        section: <SectionBox title={`Synced Pods (${readyPods.length})`}>
            {readyPods.length < 1 ? (
                <Typography variant="body2" color="textSecondary">
                    No gateway pods have reported cache readiness for this revision yet.
                </Typography>
            ) : (
                <Box sx={{display: 'flex', flexWrap: 'wrap', gap: 1, p: 1}}>
                    {readyPods.map(pod => (
                        <Chip
                            key={pod}
                            label={pod}
                            color="success"
                            variant="outlined"
                        />
                    ))}
                </Box>
            )}
        </SectionBox>,
    };
}

function getEnvSnapshotSection(env?: Record<string, string>) {
    if(!env || Object.keys(env).length < 1) {
        return null;
    }

    const rows = Object.entries(env).map(([k, v]) => ({
        name: k,
        value: <code>{v}</code>,
    }));

    return {
        id: 'revision-env',
        section: <SectionBox title="Environment Snapshot">
            <NameValueTable rows={rows} />
        </SectionBox>,
    };
}
