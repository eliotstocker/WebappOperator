import {KubeObject, KubeObjectInterface} from '@kinvolk/headlamp-plugin/lib/k8s/cluster';
import {WebsitePhase, WebsiteSpec, WebsiteStatus} from '../types';

export type KubeWebsite = KubeObjectInterface & {
    spec: WebsiteSpec;
    status?: WebsiteStatus;
};

export class Website extends KubeObject<KubeWebsite> {
    static kind = 'Website';
    static apiName = 'websites';
    static apiVersion = 'webapp.io/v1alpha1';
    static isNamespaced = true;

    static get detailsRoute() {
        return '/webapps/:namespace/:name';
    }

    get spec(): WebsiteSpec {
        return this.jsonData.spec;
    }

    get status(): WebsiteStatus | undefined {
        return this.jsonData.status;
    }

    get phase(): WebsitePhase {
        return this.status?.phase || 'Pending';
    }

    get hostnames(): string[] {
        return this.spec?.hostnames || [];
    }

    get activeRevision(): string {
        return this.status?.activeRevision || '-';
    }

    get activeImage(): string {
        return this.status?.activeImage || this.spec?.image || '-';
    }
}
