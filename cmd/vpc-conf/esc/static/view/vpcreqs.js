import {html, render, nothing} from '../lit-html/lit-html.js';
import {HasModal, MakesAuthenticatedAJAXRequests, HasNewVPCForm, HasNewAdditionalSubnetsForm, DisplaysTasks} from './mixins.js';
import {Breadcrumb} from './components/shared/breadcrumb.js'
import {Growl} from './components/shared/growl.js';
import { User } from './user.js';

export function VPCRequestPage(info) {
    this._loginURL = info.ServerPrefix + 'oauth/callback';
    this._baseTaskURL = info.ServerPrefix + 'task/'
    Object.assign(this, HasModal, MakesAuthenticatedAJAXRequests, HasNewVPCForm, HasNewAdditionalSubnetsForm, DisplaysTasks);
    this._pageURL = info.ServerPrefix + 'vpcreqs';

    let requests = null;
    let loadingRequests = false;

    this._loadRequests = async function() {
        if (loadingRequests) return;  // Only one at a time please
        loadingRequests = true;
        const url = info.ServerPrefix + 'vpcreqs.json';
        let response;
        try {
            response = await this._fetchJSON(url);
        } catch (err) {
            Growl.error('Error setting resource share: ' + err);
            return;
        }
        if (response.text == requests) {
            // No change to table.
            loadingRequests = false;
            return;
        }
        requests = response.text;
        loadingRequests = false;
        this._renderRequests(response.json);

        if (info.RequestID) {
            for (const req of (response.json.Requests || [])) {
                if (req.ID === +info.RequestID) {
                    this._showRequest(req, true);
                    break;
                }
            }
        }
        info.RequestID = null;
    }

    this._describeRequestStatus = (status => {
        const descriptions = [
            "Submitted",
            "Cancelled",
            "Rejected",
            "Approved",
            "In Progress",
            "Done",
        ];
        if (status < descriptions.length) {
            return descriptions[status];
        }
        return "Unknown";
    })

    this._describeTasks = (req => {
        const describeStatus = status => (
            [
                "Queued",
                "In progress",
                "Successful",
                "Failed",
            ][status] || "Unknown"
        );

        const describeTask = ((task, n) =>
            html`<span class="tooltip provisioning-task provisioning-task-${describeStatus(task.Status).replace(/ /g, '-')}" data-tooltip="${task.Description}: ${describeStatus(task.Status)}" @click="${() => this._showTask(task.ID)}">${n}</span>`
        )

        const firstTask = {
            Description: 'Create VPC',
            ID: req.TaskID,
            Status: req.TaskStatus,
        };
        return html`
            ${describeTask(firstTask, 1)}
            ${(req.DependentTasks || []).map((t, idx) => html`${describeTask(t, idx+2)} `)}
        `
    })

    this._renderRequests = function(data) {
        requests = data.Requests || [];
        this._regions = data.Regions;
        const view = this;

        const describeJiraIssue = (req => {
            if (req.HasJIRAErrors) {
                return html`<a @click=${e => this._showJiraErrorModal(req)} href="#" class="ds-c-link">Retrying...</a>`
            } else if (req.JIRAIssue) {
                return html`<a target="_blank" href="https://jiraent.cms.gov/browse/${req.JIRAIssue}" class="ds-c-link">${req.JIRAIssue}</a>`
            }
            return 'Creating...'
        })

        const describeRequestType = (type => {
            const descriptions = [
                "New VPC",
                "Additional Subnets",
            ];
            if (type < descriptions.length) {
                return descriptions[type];
            }
            return "Unknown";
        })

        const requestStatusApproved = 3;
        const taskStatusFailed = 3;

        render(
            html`
            <table class="standard-table" style="border-top: 0">
                <thead>
                <tr>
                    <th>Date</td>
                    <th>Jira Issue</th>
                    <th>Account</th>
                    <th>VPC Name</th>
                    <th>Region</th>
                    <th>Requester</th>
                    <th>Request Type</th>
                    <th>Status</th>
                    <th></th>
                </tr>
                </thead>
                <tbody>
                ${requests.map(req => html`
                <tr>
                    <td>${(new Date(req.AddedAt)).toDateString()}</td>
                    <td>${describeJiraIssue(req)}</td>
                    <td><a href="${info.ServerPrefix}accounts/${req.AccountID}">${req.AccountID}</a></td>
                    ${req.ProvisionedVPC
                        ? html`<td><a href="${info.ServerPrefix}accounts/${req.AccountID}/vpc/${req.ProvisionedVPC.Region}/${req.ProvisionedVPC.ID}">${req.RequestedConfig.VPCName}</a></td>`
                        : (req.RequestedConfig.VPCID
                            ? html`<td><a href="${info.ServerPrefix}accounts/${req.AccountID}/vpc/${req.RequestedConfig.AWSRegion}/${req.RequestedConfig.VPCID}">${req.RequestedConfig.VPCName}</a></td>`
                            : html`<td>${req.RequestedConfig.VPCName}</td>`)}
                    <td>${req.RequestedConfig.AWSRegion}</td>
                    <td>${req.RequesterName} (${req.RequesterUID})</td>
                    <td>${describeRequestType(req.RequestType)}</td>
                    <td>${
                        req.TaskID === null
                        ? this._describeRequestStatus(req.Status)
                        : this._describeTasks(req)
                    }</td>
                    <td><button class="ds-c-button ds-c-button--primary ds-c-button--small" type="button" @click="${() => view._showRequest(req)}">Show Request</button></td>
                </tr>
                `)}
                </tbody>
            </table>`,
            this._requestsContainer
        );
    }

    this._showRequest = function(req, isOnPageLoad) {
        this._activeRequest = req;
        this._showModal({className: 'provision-panel'});
        if (!isOnPageLoad) {
            history.pushState('req-' + req.ID, document.title, this._pageURL + '/' + req.ID);
        }
        this._onCloseModal = () => {
            history.pushState('reqs', document.title, this._pageURL);
        }
        render(
            html`
                <div class="section-header">VPC Request Details</div>
                <table id="vpcRequestDetails" class="standard-table">
                    <tbody>
                        <tr>
                            <th>Request ID</th>
                            <td>${req.ID}</td>
                        </tr>
                        <tr>
                            <th>Requester</th>
                            <td>${req.RequesterName} (${req.RequesterUID}) ${req.RequesterEmail}</td>
                        </tr>
                        <tr>
                            <th>Project</th>
                            <td>${req.ProjectName}</td>
                        </tr>
                        <tr>
                            <th>Account</th>
                            <td>${req.AccountID} ${req.AccountName}</td>
                        </tr>
                        <tr>
                            <th>Jira Issue</th>
                            <td><a target="_blank" href="https://jiraent.cms.gov/browse/${req.JIRAIssue}">${req.JIRAIssue}</a> - ${this._describeRequestStatus(req.Status)}</td>
                        </tr>
                    </tbody>
                </table>
                <div class="section-header-secondary-primary">VPC Parameters</div>
                <div id="requestContainer"></div>
            `,
            this._modal);

        if (req.RequestType == 1) {
            this.initNewAddtionalSubnetsForm(
                document.getElementById('requestContainer'),
                this._regions,
                {
                    Region: req.RequestedConfig.AWSRegion,
                    AccountID: req.RequestedConfig.AccountID,
                    VPCID: req.RequestedConfig.VPCID,
                    SubnetType: req.RequestedConfig.SubnetType,
                    SubnetSize: req.RequestedConfig.SubnetSize,
                    GroupName: req.RequestedConfig.GroupName,
                    RequestID: req.ID,
                },
            )
        } else {
            const vpcName = req.RequestedConfig.VPCName.replace(/^(.+?)(-(gov-)?(east|west))?-(sandbox|dev|test|impl|prod|nonprod|qa|mgmt)$/, '$1');
            this.initNewVPCForm(
                document.getElementById('requestContainer'),
                this._regions,
                {
                    Region: req.RequestedConfig.AWSRegion,
                    Stack: req.RequestedConfig.Stack,
                    NamePrefix: vpcName,
                    NumPrivateSubnets: req.RequestedConfig.NumPrivateSubnets,
                    NumPublicSubnets: req.RequestedConfig.NumPrivateSubnets,
                    PrivateSize: req.RequestedConfig.PrivateSize,
                    PublicSize: req.RequestedConfig.PublicSize,
                    IsDefaultDedicated: req.RequestedConfig.IsDefaultDedicated,
                    CanProvision: this._describeRequestStatus(req.Status) == "Approved" && User.isAdmin(),
                    AddContainersSubnets: req.RequestedConfig.AddContainersSubnets,
                    AddFirewall: req.RequestedConfig.AddFirewall,
                },
            )
        }
    }

    this.uninit = function(container) {
        window.clearInterval(this._loadRequestsIntervalID);
    }

    this.init = function(container) {
        Breadcrumb.set([{name: "VPC Requests"}]);
        render(
            html`
                <div id="background"></div>
                <div id="modal"></div>
                <div class="ds-l-container ds-u-padding-y--0 ds-u-padding-x--0 ds-u-margin-y--0">
                    <div id="requests"></div>
                </div>`,
            container);
        this._background = document.getElementById('background');
        this._modal = document.getElementById('modal');
        // Explicitly hide these in case we are re-rendering because of navigation.
        this._background.className = 'hidden';
        this._modal.className = 'hidden';
        this._requestsContainer = document.getElementById('requests');
        this._provisionPanel = document.getElementById('provision-panel');

        this._loadRequests();
        this._loadRequestsIntervalID = window.setInterval(() => this._loadRequests(), 3000);
    }

    this._provision = async function(config) {
        let response;
        try {
            response = await this._fetchJSON(info.ServerPrefix + 'vpcreq/' + this._activeRequest.ID + '/provision', {method: 'POST', body: JSON.stringify(config)});
        } catch (err) {
            Growl.error('Error submitting task: ' + err);
            return;
        }

        this._closeModal();
    }

    this._showJiraErrorModal = async function(vpcRequest) {
        const url = info.ServerPrefix + "vpcreqs/" + vpcRequest.ID + "/jiraErrors"
        let response;
        try {
            response = await this._fetchJSON(url);
        } catch (err) {
            Growl.error('Error fetching Jira errors: ' + err);
            return;
        }

        this._showModal()
        render(html`<div class="modalContainer">
                        <div class="modalTitle" style="text-align: left">VPC Name: ${vpcRequest.RequestedConfig.VPCName}</div>
                        <div class="modalBody">
                            <table class="standard-table">
                                <thead>
                                    <tr>
                                        <th>Date</th>
                                        <th>Retries</th>
                                        <th>Message</th>
                                    </tr>
                                <thead>
                                <tbody>
                                    ${response.json.map(jiraError => html`
                                    <tr>
                                        <td id="date">${(new Date(jiraError.added_at)).toLocaleString('en-US', { hour12: false })}</td>
                                        <td>${jiraError.retry_attempts}</td>
                                        <td>${jiraError.message}</td>
                                    </tr>
                                    `)}
                                </tbody>
                            </table>
                        </div>
                    </div>`, this._modal);
    }
}
