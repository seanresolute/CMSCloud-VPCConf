import {DashboardPage} from './view/dashboard.js';
import {AccountPage} from './view/account.js';
import {AccountsPage} from './view/accounts.js';
import {VPCPage} from './view/vpc.js';
import {ManagedTransitGatewayAttachmentsPage} from './view/mtgas.js';
import {ManagedResolverRulesPage} from './view/rrs.js';
import {SecurityGroupSetsPage} from './view/sgs.js';
import {BatchTasksPage} from './view/batch.js';
import {VPCRequestPage} from './view/vpcreqs.js';
import {IPUsagePage} from './view/usage.js';
import {SearchPage} from './view/search.js';

const serverPrefix = '/provision/';
const contentDiv = document.getElementById('mainContent');
// The view returned by "getView" must implement:
//   init(contentDiv) - initialize the view and render it to contentDiv
// If the view uses window.pushState then it must provide a non-null
// state and it must implement
//   uninit() - prepare to be replaced by a new view (e.g. clear timeouts)
const routes = [
	{
		re: /^$/,
		getView: () =>
			new DashboardPage({
				ServerPrefix: serverPrefix,
			})
	},
	{
		re: /^usage$/,
		getView: () =>
			new IPUsagePage({
				ServerPrefix: serverPrefix,
			})
	},
	{
		re: /^accounts$/,
		getView: () =>
			new AccountsPage({
				ServerPrefix: serverPrefix,
			})
	},		
	{
		re: new RegExp('^accounts/([^/]+)$'),
		getView: (result) =>
			new AccountPage({
				ServerPrefix: serverPrefix,
				AccountID: result[1],
			})
	},
	{
		re: new RegExp('^accounts/([^/]+)/vpc/([^/]+)/([^/]+)$'),			
		getView: (result) =>
			new VPCPage({
				ServerPrefix: serverPrefix,
				Region: result[2],
				AccountID: result[1],
				VPCID: result[3],
			})
	},
	{
		re: /^mtgas$/,
		getView: () =>
			new ManagedTransitGatewayAttachmentsPage({
				ServerPrefix: serverPrefix,
			})
	},
	{
		re: /^mrrs$/,
		getView: () =>
			new ManagedResolverRulesPage({
				ServerPrefix: serverPrefix,
			})
	},
	{
		re: /^sgs$/,
		getView: () =>
			new SecurityGroupSetsPage({
				ServerPrefix: serverPrefix,
			})
	},
	{
		re: /^batch$/,
		getView: () =>
			new BatchTasksPage({
				ServerPrefix: serverPrefix,
			})
	},
	{
		re: new RegExp('^vpcreqs/?([0-9]+)?$'),
		getView: (result) =>
			new VPCRequestPage({
				ServerPrefix: serverPrefix,
				RequestID: result[1],
			})
	},
	{
		re: /^search$/,
		getView: () =>
			new SearchPage({
				ServerPrefix: serverPrefix,
			})
	},	
]

let currentView = null;

function route() {
	if (serverPrefix != window.location.pathname.substring(0, serverPrefix.length)) {
		alert('Path does not match expected prefix ' + serverPrefix);
		return;
	}

	const path = window.location.pathname.substring(serverPrefix.length).trim();
	
	for (let i = 0; i < routes.length; i++) {
		const route = routes[i];
		const result = route.re.exec(path);

		if (result) {
			if (currentView !== null && currentView.uninit) {
				currentView.uninit();
			}
			currentView = route.getView(result);
			currentView.init(contentDiv);
			return;
		}
	}
}

window.addEventListener('load', () => {
	route();
	if (currentView !== null && currentView.uninit) {
		// This view uses "pushstate".
		// Set an initial state so that "back" to the view first loaded
		// will not be treated as a hash change.
		history.replaceState('initial', document.title, window.location.href);
	}
});
window.addEventListener('popstate', (e) => {
	if (e.state === null) {
		// No state set means that this is just a hash change (because views
		// are required to set a state when using pushState).
		return;
	}
	route();
});
