import {html, render} from '../lit-html/lit-html.js';
import {Growl} from './components/shared/growl.js';
import {User} from './user.js';
import './components/task-log.js';
import './components/list-log-container.js';
import './components/dynamic-task-list.js';

/*
    Requires:
      this._modal: a div, initially class=hidden
        this._background: a div, initially class=hidden
*/
export var HasModal = {
    _showModal: function({className = '', canDismiss = true} = {}) {
        // If there was already a modal open then we are effectively closing it
        // by opening a new one.
        if (this._modalShown && this._onCloseModal) {
            this._onCloseModal();
            this._onCloseModal = null;
        }
        this._modalShown = true;
        this._modal.className = className;
        this._background.className = '';
        if (canDismiss) this._listenForClick();
    },

    _closeModal: function() {
        this._modal.className = 'hidden';
        this._background.className = 'hidden';
        render('', this._modal);
        if (this._onCloseModal) this._onCloseModal();
        this._onCloseModal = null;
        this._modalShown = false;
    },

    _listenForClick: function() {
        const closeModal = () => {
            this._closeModal();
            this._background.removeEventListener('click', closeModal);
        };
        this._background.addEventListener('click', closeModal);
    },
}

/*
    Requires:
      this._cancelTasksURL: URL like "/provision/task/cancel"

        MakesAuthenticatedAJAXRequests mixin
*/
export var CancelsTasks = {
    _cancelTasks: async function(taskIDs) {
        try {
            await this._fetchJSON(this._cancelTasksURL, {method: 'POST', body: JSON.stringify({TaskIDs: taskIDs})});
        } catch (err) {
            Growl.error('Error cancelling tasks: ' + err);
            return;
        }
    },

    _listenForCancelEvent: function(object) {
        object.addEventListener('cancel-click', e => {
            const taskIDs = e.detail.taskIDs;
            this._cancelTasks(taskIDs);
        });
    },
}


/*
    Requires:
      this._baseTaskURL: URL like "/provision/task/123456/"

        HasModal mixin
        CancelsTasks mixin
        MakesAuthenticatedAJAXRequests mixin
*/
export var DisplaysTasks = {
    _listenForShowTaskEvents: function(object) {
        object.addEventListener('show-task-click', e => {
            this._showTask(e.detail.logID);
        });
    },

    _showTask: function(logID) {
        this._showModal();
        render(
            html`
                <task-log 
                    baseTaskURL="${this._baseTaskURL}"
                    logID="${logID}"
                    .fetchJSON="${this._fetchJSON.bind(this)}"
                    >
                </task-log>
            `, 
            this._modal);  
    },

    _showOlderTasks: function(beforeID) {
        this._showModal();
        const taskList = html`
            <dynamic-task-list
                baseTaskURL="${this._baseTaskURL}"
                beforeID="${beforeID}"
                .fetchJSON="${this._fetchJSON.bind(this)}"
            >
            </dynamic-task-list> 
            `

        render(
            html`
                <list-log-container
                    class="rightPane"
                    baseTaskURL="${this._baseTaskURL}"
                    .taskList="${taskList}"
                    .fetchJSON="${this._fetchJSON.bind(this)}"
                >
                </list-log-container>
                
            `, 
            this._modal);  
    },
}


/*
    Requires:
        this._loginURL: URL of login handler

      HasModal mixin
*/
export var MakesAuthenticatedAJAXRequests = {
    _loginReject: function(err) {
        this._waitingForLogin.forEach(o => o.reject(err));
        this._waitingForLogin = [];
    },

    _loginResolve: function() {
        this._waitingForLogin.forEach(o => o.resolve());
        this._waitingForLogin = [];
    },

    _logIn: function() {
        this._waitingForLogin = this._waitingForLogin || [];
        return new Promise((resolve, reject) => {
            this._waitingForLogin.push({resolve: resolve, reject: reject});
            if (this._waitingForLogin.length > 1) {
                // Login modal already triggered.
                return;
            }

            this._showModal({className: 'login', canDismiss: false});

            render(
                html`
                <div class="modalContainer">
                    <div class="ds-u-fill--primary ds-u-color--white ds-u-overflow--hidden ds-u-padding--1">Session Expired</div>
                    <div class="modalBody">
                        <div id="sessionExpired">
                            <div class="center">
                                <button type="button" id="loginButton" class="ds-c-button ds-c-button--primary ds-u-margin-top--1"
                                        @click="${()=>{
                                            sessionExpired.classList.add('hidden');
                                            sessionRetry.classList.remove('hidden');
                                            window.open(`${this._loginURL}`, '_blank')
                                        } }">Log In</button>
                            </div>
                        </div>
                        <div id="sessionRetry" class="hidden">
                            <div>Once authentication has completed press the retry button to continue.</div>
                            <div class="center">
                                <button type="button" id="retryButton" @click="${()=>{this._retryRequests();}}" class="ds-c-button ds-c-button--primary ds-u-margin-top--1">Retry Request</button>
                            </div>
                        </div>
                    </div>
                </div>`,
                this._modal
            );

            const sessionExpired = document.getElementById('sessionExpired');
            const sessionRetry = document.getElementById('sessionRetry');
            this._retryRequests = () => {
                this._closeModal();
                this._loginResolve();
            }
        });
    },

    _fetchJSON: function(url, params) {
        params = params || {};
        params.credentials = 'same-origin';  // some older browsers may default to 'omit'
        const doRequest = (resolve, reject) => {
            var result = Object.create(null);
            fetch(url, params)
            .then((response) => {
                if (!response.ok) {
                    if (response.status == 401) {
                        this._logIn()
                        .then(() => doRequest(resolve, reject))
                        .catch((error) => reject('Error logging in: ' + error))
                        return;
                    }
                    response.text()
                    .then((text) => reject('Status ' + response.status + ': ' + text))
                    .catch(() => reject('Status ' + response.status))
                    return;
                }
                response.text()
                .then((text) => {
                    result.text = text;
                    try {
                        result.json = JSON.parse(text);
                        resolve(result);
                    } catch (err) {
                        reject(err);
                    }
                }).catch((err) => reject(err));
            }).catch((err) => reject(err));
        }
        return new Promise(doRequest);
    },
}

export var HasNewAdditionalSubnetsForm = {
    
    _updateRequests: function() {
        const subnetType = this._requestAdditionalSubnetsForm.subnetType.value;
        const isUnroutable = subnetType == null ? false : subnetType == "Unroutable";

        if (isUnroutable) {
            this._requestAdditionalSubnetsForm.subnetSize.classList.add('hidden');
            unroutableSubnetSize.classList.remove('hidden');
        } else {
            this._requestAdditionalSubnetsForm.subnetSize.classList.remove('hidden');
            unroutableSubnetSize.classList.add('hidden');
        }
    },

    initNewAddtionalSubnetsForm: function(container, regions, {
            Region,
            AccountID,
            VPCID,
            SubnetType,
            SubnetSize,
            GroupName,
            RequestID,
        }) {
        const subnetSizes = [20, 21, 22, 23, 24, 25, 26, 27, 28];
        const subnetTypes = ['Public', 'Private', 'Unroutable'];

        if (Region === undefined) {
            Growl.error("Region must be specified for additional subnets form");
            return;
        }
        if (SubnetType === undefined) {
            Growl.error("Subnet type must be specified for additional subnets form");
            return;
        }
        if (GroupName === undefined) {
            Growl.error("GroupName must be specified for additional subnets form");
            return;
        }
        if (SubnetType != "Unroutable" && subnetSizes.indexOf(SubnetSize) == -1) {
            Growl.error("Invalid subnet count for additional subnets form");
            return;
        }
        render(
            html`
            <form method="post" id="requestAdditionalSubnets">
                <table class="standard-table">
                    <thead>
                        <tr>
                            <th>
                                Subnet Type
                            </th>
                            <th>
                                Subnet Size
                            </th>
                            <th>
                                Group Name
                            </th>
                            <th>
                                Action
                            </th>
                        </tr>
                    </thead>
                    <tbody>
                        <tr>
                            <td>
                                <select id="subnetType" name="subnetType" class="ds-c-field input-medium">
                                    ${subnetTypes.map(type => html`
                                    <option value="${type}" ?selected=${type === SubnetType} disabled>${type}</option>
                                    `)}
                                </select>
                            </td>
                            <td>
                                <select id="subnetSize" name="zonedSubnetSize" class="ds-c-field input-medium">
                                    ${subnetSizes.map(size => html`
                                    <option value="${size}" ?selected="${size == SubnetSize}">/${size} (${Math.pow(2, 32 - size)} IPs per subnet)</option>
                                    `)}
                                </select>
                                <span id="unroutableSubnetSize" class="hidden">/${SubnetSize}</span>
                            </td>
                            <td>
                                <input id="subnetGroupName" name="subnetGroupName" value="${GroupName}" class="ds-c-field input-medium">
                            </td>
                            <td>
                                <input type="submit" value="Add Subnets" class="ds-c-button ds-c-button--primary">
                            </td>
                        </tr>
                    </tbody>
                </table>
            </form>
            `,
            container);

            this._requestAdditionalSubnetsForm = document.getElementById('requestAdditionalSubnets');
            this._updateRequests();
            this._requestAdditionalSubnetsForm.addEventListener('submit', (e) =>{
                e.preventDefault();
                const subnetSizeValue = (subnetType == "Unroutable") ? +this._requestAdditionalSubnetsForm.unroutableSubnetSize.text : +this._requestAdditionalSubnetsForm.subnetSize.value
                const config = {
                    AWSRegion: Region,
                    AccountID: AccountID,
                    VPCID: VPCID,
                    SubnetType: this._requestAdditionalSubnetsForm.subnetType.value,
                    SubnetSize: subnetSizeValue,
                    GroupName: this._requestAdditionalSubnetsForm.subnetGroupName.value,
                    RequestID: RequestID
                };
                this._requestAdditionalSubnetsForm.reset();
                this._provision(config);
                Growl.success("Add additional subnets task submitted");
            });
    }
}

export var HasNewVPCForm = {
    _getVPCName: function() {
        const region = this._createVPCForm.region.value;
        const shortRegion = {
            'us-east-1': 'east',
            'us-west-2': 'west',
            'us-gov-east-1': 'gov-east',
            'us-gov-west-1': 'gov-west',
        }[region] || region;
        return this._createVPCForm.name.value + "-" + shortRegion + '-' + this._createVPCForm.stack.value.toLowerCase();
    },

    _updateVPCNamePreview: function() {
        this._namePreview.innerText = this._getVPCName()
    },

    initNewVPCForm: function(container, regions, {
        Region,
        Stack,
        NamePrefix,
        NumPrivateSubnets,
        NumPublicSubnets,
        PrivateSize,
        PublicSize,
        IsDefaultDedicated,
        CanProvision,
        AddContainersSubnets,
        AddFirewall,
    }) {
        const stacks = ['sandbox', 'dev', 'test', 'impl', 'mgmt', 'nonprod', 'qa', 'prod'];
        const subnetSizes = [20, 21, 22, 23, 24, 25, 26, 27, 28];

        if (regions.indexOf(Region) == -1) {
            Growl.error("Invalid region for new VPC form");
            return;
        }
        if (stacks.indexOf(Stack) == -1) {
            Growl.error("Invalid stack for new VPC form");
            return;
        }
        if (NamePrefix === undefined) {
            Growl.error("Name prefix must be specified for new VPC form");
            return;
        }
        if (!(NumPrivateSubnets > 0 && +NumPublicSubnets > 0)) {
            Growl.error("Invalid subnet count for new VPC form");
            return;
        }
        if (subnetSizes.indexOf(PrivateSize) == -1) {
            Growl.error("Invalid private subnet size for new VPC form");
            return;
        }
        if (subnetSizes.indexOf(PublicSize) == -1) {
            Growl.error("Invalid public subnet size for new VPC form");
            return;
        }
        if (IsDefaultDedicated === undefined) {
            Growl.error("Dedicated (or not) must be specified for new VPC form");
            return;
        }

        render(
            html`
            <form method="post" id="createVPC" class="ds-l-container" style="margin-bottom: 10px;">
                <div class="ds-l-form-row">
                    <div class="ds-l-col--6">
                        <label class="ds-c-label">
                            Name Prefix
                            <input name="name" class="ds-c-field" size="20" maxlength="100" autocomplete="off" required ?disabled="${!CanProvision}" value="${NamePrefix}">
                        </label>
                    </div>

                    <div class="ds-l-col--3">
                        <label class="ds-c-label">
                            Region
                            <select name="region" class="ds-c-field" ?disabled="${!CanProvision}">
                                ${regions.map((region) => html`
                                <option value="${region}" ?selected="${region == Region}">
                                    ${region}
                                </option>
                                `)}
                            </select>
                        </label>
                    </div>
                    <div class="ds-l-col--3">
                        <label class="ds-c-label">
                            Stack
                            <select name="stack" class="ds-c-field" ?disabled="${!CanProvision}">
                                ${stacks.map(stack => html`
                                <option value="${stack}" ?selected="${stack == Stack}">${stack}</option>
                                `)}
                            </select>
                        </label>
                    </div>
                    </div>
                    
                <div class="ds-l-form-row">                  
                    <div class="ds-l-col--8">
                        <label class="ds-c-label">Name Preview</label>
                        <span id="namePreview" name="namePreview"></span>
                    </div>
                </div>

                <div class="ds-l-form-row">
                    <div class="ds-l-col--2">
                        <label class="ds-c-label" style="width: 90px">
                            Public AZs
                            <select name="publicAZs" class="ds-c-field" required ?disabled="${!CanProvision}">
                            ${[1,2,3,4].map((az) => {
                                return html`<option value="${az}" ?selected="${az == NumPublicSubnets}">${az}</option>`

                            })}
                            </select>
                        </label>
                    </div>
                    <div class="ds-l-col--4">
                        <label class="ds-c-label">
                            Public subnet size
                            <select name="publicAZSize" class="ds-c-field" ?disabled="${!CanProvision}">
                                ${subnetSizes.map(size => html`
                                    <option value="${size}" ?selected="${size == PublicSize}">/${size} (${Math.pow(2, 32 - size)} IPs per subnet)</option>
                                `)}
                            </select>
                        </label>
                    </div>
                </div>

                <div class="ds-l-form-row">
                    <div class="ds-l-col--2">
                        <label class="ds-c-label" style="width: 90px">
                            Private AZs
                            <select name="privateAZs" class="ds-c-field" required ?disabled="${!CanProvision}">
                                ${[1,2,3,4].map((az) => {
                                    return html`<option value="${az}" ?selected="${az == NumPrivateSubnets}">${az}</option>`

                                })}
                            </select>
                        </label>
                    </div>
                    <div class="ds-l-col--4">
                        <label class="ds-c-label">
                            Private subnet size
                            <select name="privateAZSize" class="ds-c-field" ?disabled="${!CanProvision}">
                                ${subnetSizes.map(size => html`
                                    <option value="${size}" ?selected="${size == PrivateSize}">/${size} (${Math.pow(2, 32 - size)} IPs per subnet)</option>
                                `)}
                            </select>
                        </label>
                    </div>
                </div>

                <div class="ds-l-form-row">
                    <div class="ds-l-col--2">
                        <label for="dedicated" class="ds-c-label">Tenancy</label>
                        <input type="checkbox" id="dedicated" name="dedicated" class="ds-c-choice ds-c-choice--small" value="1" ?checked="${IsDefaultDedicated}" ?disabled="${!CanProvision}">
                        <label for="dedicated">Dedicated</label>
                    </div>
                    <div class="ds-l-col--3">
                        <label for="containers" class="ds-c-label">Container Subnets</label>
                        <input type="checkbox" id="containers" name="containers" class="ds-c-choice ds-c-choice--small" value="1" ?checked="${AddContainersSubnets}" ?disabled="${!CanProvision}">
                        <label for="containers">Add (must be done manually)</label>
                    </div>
                    <div class="ds-l-col--4" id="addFirewall">
                        <label for="firewall" class="ds-c-label">Network Firewall</label>
                        <input type="checkbox" id="firewall" name="firewall" class="ds-c-choice ds-c-choice--small" value="1" ?checked="${AddFirewall}" ?disabled="${!CanProvision}">
                        <label for="firewall">Add</label>
                    </div>
                </div>
                <div class="ds-l-form-row ds-u-align-items--end">
                    <div class="ds-l-col--12" style="text-align: right">
                        <input type="submit" class="ds-c-button ds-c-button--primary" value="Create VPC" ?disabled="${!CanProvision}">
                    </div>
                </div>
            </form>`,
            container);
            this._submittedVPCNames = [];

            this._createVPCForm = document.getElementById('createVPC');
            this._namePreview = document.getElementById('namePreview');
            this._addFirewall = document.getElementById('addFirewall')
            
            this._createVPCForm.name.addEventListener('input', () => this._updateVPCNamePreview())
            this._createVPCForm.stack.addEventListener('change', () => this._updateVPCNamePreview());
            this._createVPCForm.region.addEventListener('change', () => {
                this._updateVPCNamePreview(); 
            });
            this._updateVPCNamePreview();
    
            this._createVPCForm.addEventListener('submit', (e) =>{
                e.preventDefault();
    
                if (this._submittedVPCNames.indexOf(this._getVPCName()) != -1) {
                    return;
                }
                this._submittedVPCNames.push(this._getVPCName());

                const config = {
                    AWSRegion: this._createVPCForm.region.value,
                    Stack: this._createVPCForm.stack.value,
                    VPCName: this._getVPCName(),
                    NumPrivateSubnets: +this._createVPCForm.privateAZs.value,
                    NumPublicSubnets: +this._createVPCForm.publicAZs.value,
                    PrivateSize: +this._createVPCForm.privateAZSize.value,
                    PublicSize: +this._createVPCForm.publicAZSize.value,
                    IsDefaultDedicated: !!this._createVPCForm.dedicated.checked,
                    AddContainersSubnets: !!this._createVPCForm.containers.checked,
                    AddFirewall: !!this._createVPCForm.firewall.checked,
                };
    
                this._createVPCForm.reset();
                this._provision(config);
            });
    },
}

export var OpensConsole = {
    _openConsole: function(serverPrefix, region, accountID) {
        const url = serverPrefix + 'accounts/' + accountID + '/console?region=' + region;
        window.open(url);
    }
}

export var GetsCredentials = {
    _getCredentials: async function(serverPrefix, region, accountID) {
        const url = serverPrefix + 'accounts/' + accountID + '/creds?region=' + region;
        let creds;
        try {
            creds = await this._fetchJSON(url);
        } catch (err) {
            Growl.error('Error getting credentials: ' + err);
            return;
        }
        this._showModal();
        render(
            html`
            <div class="modalContainer">
                <div class="modalTitle" style="text-align: left">Credentials for account ${accountID}</div>
                <div class="modalBody">
                    <span class="ds-h4">Environment variables</span> <a class="copyCredentialsIcon" @click="${() => navigator.clipboard.writeText(document.getElementById('aws-creds-env').innerText)}">⎘</a>
<pre style="margin: 10px; overflow-x: hidden;" id="aws-creds-env">export AWS_ACCESS_KEY_ID=${creds.json.AccessKeyID}
export AWS_SECRET_ACCESS_KEY=${creds.json.SecretAccessKey}
export AWS_SESSION_TOKEN=${creds.json.SessionToken}
</pre>
                    <span class="ds-h4">AccessKeyID</span> <a class="copyCredentialsIcon" @click="${() => navigator.clipboard.writeText(document.getElementById('aws-creds-access-key-id').innerText)}">⎘</a>
                        <pre style="margin: 10px; overflow-x: hidden;" id="aws-creds-access-key-id">${creds.json.AccessKeyID}</pre>
                    <span class="ds-h4">SecretAccessKey</span> <a class="copyCredentialsIcon" @click="${() => navigator.clipboard.writeText(document.getElementById('aws-creds-secret-access-key').innerText)}">⎘</a>
                        <pre style="margin: 10px; overflow-x: hidden;" id="aws-creds-secret-access-key">${creds.json.SecretAccessKey}</pre>
                    <span class="ds-h4">SessionToken</span> <a class="copyCredentialsIcon" @click="${() => navigator.clipboard.writeText(document.getElementById('aws-creds-session-token').innerText)}">⎘</a>
                        <pre style="margin: 10px; overflow-x: hidden;" id="aws-creds-session-token">${creds.json.SessionToken}</pre>
                </div>
            </div>
            `, 
            this._modal);  
    },
}

export var ShowsPrefixListEntries = {
    _showPLEntries: async function(serverPrefix, name, id, region) {
        this._showModal();
        render(
            html`
            <div class="modalContainer">
                <div class="modalTitle">Prefix list entries for ${name} (${id})</div>
                <div class="modalBody">
                    <div id="loading">Loading...</div>
                </div>
            </div>
            `, 
            this._modal); 

        const url = serverPrefix + `${region}/pl/${id}.json`
        let response;
        try {
            response = await this._fetchJSON(url);
        } catch (err) {
            Growl.error('Error getting prefix list details: ' + err);
            return;
        }
        const entries = response.json;

        render(
            html`
            <div class="modalContainer">
                <div class="modalTitle">Prefix list entries for ${name} (${id})</div>
                <div class="modalBody">
                    <ul class="plList">
                        ${entries.map(entry => {
                            return html`<li>${entry.Cidr} : ${entry.Description ? entry.Description : "[no description]"}</li>`
                        })}
                    </ul>
                </div>
            </div>
            `, 
            this._modal);
    }
}
