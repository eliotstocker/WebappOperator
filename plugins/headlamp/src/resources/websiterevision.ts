import {KubeObject, KubeObjectInterface} from '@kinvolk/headlamp-plugin/lib/k8s/cluster';
import {RevisionPhase, WebsiteRevisionSpec, WebsiteRevisionStatus} from '../types';

export type KubeWebsiteRevision = KubeObjectInterface & {
    spec: WebsiteRevisionSpec;
    status?: WebsiteRevisionStatus;
};

export class WebsiteRevision extends KubeObject<KubeWebsiteRevision> {
    static kind = 'WebsiteRevision';
    static apiName = 'websiterevisions';
    static apiVersion = 'webapp.io/v1alpha1';
    static isNamespaced = true;

    static get detailsRoute() {
        return '/webapps/revisions/:namespace/:name';
    }

    get spec(): WebsiteRevisionSpec {
        return this.jsonData.spec;
    }

    get status(): WebsiteRevisionStatus | undefined {
        return this.jsonData.status;
    }

    get phase(): RevisionPhase {
        return this.status?.phase || 'Pending';
    }

    get websiteName(): string {
        return this.spec?.websiteName || '-';
    }

    get readyPods(): string[] {
        return this.status?.readyPods || [];
    }

    get unpackedSize(): number {
        return this.status?.unpackedSize || 0;
    }

    get digest(): string {
        return this.spec?.digest || '-';
    }
}
