import {WebsitePhase, RevisionPhase} from '../types';

export function getWebsiteStatusColor(phase: WebsitePhase): 'success' | 'warning' | 'error' | '' {
    switch(phase) {
        case 'Ready':
            return 'success';
        case 'Prewarming':
            return 'warning';
        case 'Degraded':
            return 'error';
        default:
            return '';
    }
}

export function getRevisionStatusColor(phase: RevisionPhase): 'success' | 'warning' | 'error' | '' {
    switch(phase) {
        case 'Active':
        case 'Ready':
            return 'success';
        case 'Prewarming':
            return 'warning';
        case 'Failed':
            return 'error';
        default:
            return '';
    }
}

export function formatBytes(bytes: number): string {
    if(bytes < 1) {
        return '0 B';
    }
    const units = ['B', 'KB', 'MB', 'GB'];
    let val = bytes;
    let unitIdx = 0;
    while(val >= 1024 && unitIdx < units.length -1) {
        val /= 1024;
        unitIdx++;
    }
    return `${val.toFixed(1)} ${units[unitIdx]}`;
}
