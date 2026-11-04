import {registerRoute, registerSidebarEntry} from '@kinvolk/headlamp-plugin/lib';
import WebsiteDetail from './components/WebsiteDetail';
import WebsiteList from './components/WebsiteList';
import WebsiteRevisionDetail from './components/WebsiteRevisionDetail';
import WebsiteRevisionList from './components/WebsiteRevisionList';

//register webapps parent sidebar item
registerSidebarEntry({
    parent: null,
    name: 'webapps',
    label: 'Webapps',
    icon: 'mdi:web',
    url: '/webapps',
});

registerSidebarEntry({
    parent: 'webapps',
    name: 'webapps-list',
    label: 'Websites',
    url: '/webapps',
});

registerSidebarEntry({
    parent: 'webapps',
    name: 'webapp-revisions-list',
    label: 'Revisions',
    url: '/webapps/revisions',
});

//register routes
registerRoute({
    path: '/webapps',
    sidebar: 'webapps-list',
    name: 'webapp-list',
    exact: true,
    component: () => <WebsiteList />,
});

registerRoute({
    path: '/webapps/revisions',
    sidebar: 'webapp-revisions-list',
    name: 'webapp-revisions-list',
    exact: true,
    component: () => <WebsiteRevisionList />,
});

registerRoute({
    path: '/webapps/:namespace/:name',
    sidebar: 'webapps-list',
    name: 'webapp-detail',
    exact: true,
    component: () => <WebsiteDetail />,
});

registerRoute({
    path: '/webapps/revisions/:namespace/:name',
    sidebar: 'webapp-revisions-list',
    name: 'webapp-revision-detail',
    exact: true,
    component: () => <WebsiteRevisionDetail />,
});
