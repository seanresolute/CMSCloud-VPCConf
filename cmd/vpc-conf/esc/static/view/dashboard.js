import {html, nothing, render} from '../lit-html/lit-html.js';
import {Breadcrumb} from './components/shared/breadcrumb.js'
import {Growl} from './components/shared/growl.js'
import {HasModal, MakesAuthenticatedAJAXRequests} from './mixins.js';

export function DashboardPage(info) {
	this._loginURL = info.ServerPrefix + 'oauth/callback';
	Object.assign(this, HasModal, MakesAuthenticatedAJAXRequests);

    this.init = async function(container) {
        render(
            html`
                <div id="background" class="hidden"></div>
                <div id="modal" class="hidden"></div>
                <div class="ds-l-container ds-u-padding--0">
                    <div class="ds-u-display--flex">
                        <div id="container">
                            <div id="dashboard"></div>
                        </div>
                    </div>
                </div>
            `, container);

        this._modal = document.getElementById('modal');
        this._background = document.getElementById('background');
        this._dashboard = document.getElementById('dashboard');

        Breadcrumb.set([{"name": "Dashboard", "link": "/provision"}]);
        this._loadDashboard();
        window.setInterval(() => this._loadDashboard(), 5000);
    }

    this._loadDashboard = async () => {
        let requests = [this._fetchJSON(info.ServerPrefix + 'dashboard.json'), this._fetchJSON('/health')];
        let responses;
		try {
			responses = await Promise.all(requests);
        } catch (err) {
			Growl.error('Error fetching dashboard data: ' + err);
			return;
		}

        if (responses.length != requests.length) {
            Growl.error('Unable to fetch dashboard data: expected ' + requests,length + ' responses, but got ' + responses.length);
			return;
        }

        const vpcRequests = responses[0].json.VPCRequests;
        const health = responses[1].json;

		render(
			html`
                <div id="dashboard">
                    <div class="ds-l-row">
                        <div class="ds-l-col--6">
                            <div class="section-header-secondary">Recent VPC Requests (Deprecated)</div>
                            <div class="section-body-bordered ds-u-padding--0">
                            ${vpcRequests.map((request) => {
                                return html`<div class="ds-l-row ds-u-margin-x--0 alternating-row">
                                                <div class="ds-l-col--3">
                                                    ${(new Date(request.AddedAt)).toLocaleString('en-US', { hour12: false, year: "2-digit", month: "2-digit", day: "2-digit", hour: "2-digit", minute: "2-digit" })}
                                                </div>
                                                <div class="ds-l-col--5">
                                                    <a href="${info.ServerPrefix}vpcreqs/${request.ID}">${request.RequestedConfig.VPCName}</a>
                                                </div>
                                                <div class="ds-l-col--2">
                                                    <a target="_blank" href="https://jiraent.cms.gov/browse/${request.JIRAIssue}">${request.JIRAIssue}</a>
                                                </div>
                                            </div>
                                `
                            })}

                            </div>
                        </div>
                        <div class="ds-l-col--6">
                            <div class="section-header-secondary">Statistics</div>
                            <div class="section-body-bordered ds-u-padding--0">
                                <div class="ds-l-row ds-u-margin-x--0 alternating-row">
                                    <div class="ds-l-col--3">Total Accounts</div>
                                    <div class="ds-l-col--3">${responses[0].json.TotalAccounts}</div>
                                </div>
                                <div class="ds-l-row ds-u-margin-x--0 alternating-row">
                                    <div class="ds-l-col--3">Total VPCs</div>
                                    <div class="ds-l-col--3">${responses[0].json.TotalVPCs}</div>
                                </div>
                                <div class="ds-l-row ds-u-margin-x--0 alternating-row">
                                    <div class="ds-l-col--3">Total VPC Requests</div>
                                    <div class="ds-l-col--3">${responses[0].json.TotalVPCRequests}</div>
                                </div>
                            </div>

                            <div class="section-header-secondary">Tasks</div>
                            <div class="section-body-bordered ds-u-padding--0">
                                <div class="ds-l-row ds-u-margin-x--0 alternating-row">
                                    <div class="ds-l-col--3">Max Workers</div>
                                    <div class="ds-l-col--3">${health.TaskStats.MaxWorkers}</div>
                                </div>
                                <div class="ds-l-row ds-u-margin-x--0 alternating-row">
                                    <div class="ds-l-col--3">Workers Allowed</div>
                                    <div class="ds-l-col--3">${health.TaskStats.AllWorkersAllowed ? 'All' : health.TaskStats.WorkersAllowed}</div>
                                </div>
                                <div class="ds-l-row ds-u-margin-x--0 alternating-row">
                                    <div class="ds-l-col--3">Working</div>
                                    <div class="ds-l-col--3">${health.TaskStats.NumInProgress}</div>
                                </div>
                                <div class="ds-l-row ds-u-margin-x--0 alternating-row">
                                    <div class="ds-l-col--3">Queued</div>
                                    <div class="ds-l-col--3">${health.TaskStats.NumQueued}</div>
                                </div>
                                <div class="ds-l-row ds-u-margin-x--0 alternating-row">
                                    <div class="ds-l-col--3">Reserved</div>
                                    <div class="ds-l-col--3">${health.TaskStats.NumTasksReserved}</div>
                                </div>
                                <div class="ds-l-row ds-u-margin-x--0 alternating-row ${health.TaskStats.OldestNotDoneAddedAt ? nothing : 'hidden'}">
                                    <div class="ds-l-col--3">Oldest Task</div>
                                    <div class="ds-l-col--3">${(new Date(health.TaskStats.OldestNotDoneAddedAt)).toLocaleString('en-US', { hour12: false, year: "2-digit", month: "2-digit", day: "2-digit", hour: "2-digit", minute: "2-digit" })}</div>
                                </div>
                            </div>


                            <div class="section-header-secondary">External Services</div>
                            <div class="section-body-bordered ds-u-padding--0">
                                <div class="ds-l-row ds-u-margin-x--0 alternating-row">
                                    <div class="ds-l-col--4">Update AWS Accounts</div>
                                    <div class="ds-l-col--2 ${health.UpdateAwsAccountsSynced ? 'success-text' : 'error-text'}"">${health.UpdateAwsAccountsSynced ? 'Synced' : 'Not Synced'}</div>
                                </div>
                                <div class="ds-l-row ds-u-margin-x--0 alternating-row">
                                    <div class="ds-l-col--4">Credentials Commercial</div>
                                    <div class="ds-l-col--2 ${health.CanConnect.Credentials.Commercial ? 'success-text' : 'error-text'}">${health.CanConnect.Credentials.Commercial ? 'Available' : 'Unreachable'}</div>
                                </div>
                                <div class="ds-l-row ds-u-margin-x--0 alternating-row">
                                    <div class="ds-l-col--4">Credentials GovCloud</div>
                                    <div class="ds-l-col--2 ${health.CanConnect.Credentials.GovCloud ? 'success-text' : 'error-text'}">${health.CanConnect.Credentials.GovCloud ? 'Available' : 'Unreachable'}</div>
                                </div>
                                <div class="ds-l-row ds-u-margin-x--0 alternating-row">
                                    <div class="ds-l-col--4">IP Control</div>
                                    <div class="ds-l-col--2 ${health.CanConnect.IPControl ? 'success-text' : 'error-text'}"">${health.CanConnect.IPControl ? 'Available' : 'Unreachable'}</div>
                                </div>
                                <div class="ds-l-row ds-u-margin-x--0 alternating-row">
                                    <div class="ds-l-col--4">Jira</div>
                                    <div class="ds-l-col--2 ${health.CanConnect.JIRA ? 'success-text' : 'error-text'}"">${health.CanConnect.JIRA ? 'Available' : 'Unreachable'}</div>
                                </div>
                                <div class="ds-l-row ds-u-margin-x--0 alternating-row">
                                    <div class="ds-l-col--4">Groot (CMS Net API)</div>
                                    <div class="ds-l-col--2 ${health.CanConnect.Groot ? 'success-text' : 'error-text'}"">${health.CanConnect.Groot ? 'Available' : 'Unreachable'}</div>
                                </div>
                                <div class="ds-l-row ds-u-margin-x--0 alternating-row">
                                    <div class="ds-l-col--4">Orchestration Engine</div>
                                    <div class="ds-l-col--2 ${health.CanConnect.Orchestration ? 'success-text' : 'error-text'}"">${health.CanConnect.Orchestration ? 'Available' : 'Unreachable'}</div>
                                </div>
                            </div>
                        </div>
                    </div>
                </div>
            `,
            this._dashboard);
    }
}
