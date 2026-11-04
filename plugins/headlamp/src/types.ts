export type InjectionMode = 'endpoint' | 'inline' | 'both';

export type InjectionSpec = {
    mode?: InjectionMode;
    path?: string;
    versionPath?: string;
    versionPolling?: boolean;
    pollIntervalSeconds?: number;
};

export type IngressTLS = {
    hosts?: string[];
    secretName?: string;
};

export type IngressSpec = {
    enabled: boolean;
    className?: string;
    annotations?: Record<string, string>;
    tls?: IngressTLS[];
};

export type WebsitePhase = 'Pending' | 'Prewarming' | 'Ready' | 'Degraded';

export type WebsiteSpec = {
    image: string;
    hostnames: string[];
    env?: Record<string, string>;
    injection?: InjectionSpec;
    imagePullSecrets?: Array<{name: string}>;
    revisionHistoryLimit?: number;
    ingress?: IngressSpec;
};

export type Condition = {
    type: string;
    status: 'True' | 'False' | 'Unknown';
    reason?: string;
    message?: string;
    lastTransitionTime?: string;
};

export type WebsiteStatus = {
    activeRevision?: string;
    activeImage?: string;
    phase?: WebsitePhase;
    observedGeneration?: number;
    conditions?: Condition[];
};

export type RevisionPhase = 'Pending' | 'Prewarming' | 'Ready' | 'Active' | 'Retired' | 'Failed';

export type WebsiteRevisionSpec = {
    websiteName: string;
    image: string;
    digest?: string;
    env?: Record<string, string>;
    injection?: InjectionSpec;
    imagePullSecrets?: Array<{name: string}>;
};

export type WebsiteRevisionStatus = {
    phase?: RevisionPhase;
    readyPods?: string[];
    unpackedSize?: number;
    observedGeneration?: number;
    conditions?: Condition[];
};
