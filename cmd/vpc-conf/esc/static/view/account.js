import {html, nothing, render} from '../lit-html/lit-html.js';
import {Breadcrumb} from './components/shared/breadcrumb.js'
import {Growl} from './components/shared/growl.js'
import {User} from './user.js'
import './components/fixed-task-list.js';
import './components/tab-ui.js';
import './components/label-ui.js';
import {VPCType} from './vpctype.js'

import {DisplaysTasks, CancelsTasks, HasModal, MakesAuthenticatedAJAXRequests, HasNewVPCForm, GetsCredentials, OpensConsole} from './mixins.js';

export function AccountPage(info) {
	this._cancelTasksURL = info.ServerPrefix + 'task/cancel';
	this._baseTaskURL = info.ServerPrefix + 'task/' + info.AccountID + '/';
	this._loginURL = info.ServerPrefix + 'oauth/callback';
	Object.assign(this, DisplaysTasks, CancelsTasks, HasModal, MakesAuthenticatedAJAXRequests, HasNewVPCForm, GetsCredentials, OpensConsole);

	let accountInfo = null;
	let loadingAccountInfo = false;

	this._setBreadcrumbs = (account) => {
		Breadcrumb.set([{name: "Accounts", link: "/provision/accounts"},
						{name: account.ProjectName + " - " + account.AccountName + " - " + account.AccountID}]);
	}

	this._loadAccountInfo = async function() {
		if (loadingAccountInfo) return;  // Only one at a time please
		loadingAccountInfo = true;
		const url = info.ServerPrefix + 'accounts/' + info.AccountID + '.json';
		let response;
		try {
			response = await this._fetchJSON(url);
		} catch (err) {
			Growl.error('Error getting account info: ' + err);
			return;
		}
		if (response.text == accountInfo) {
			// No change to table.
			loadingAccountInfo = false;
			return;
		}
		accountInfo = response.text;
		loadingAccountInfo = false;
		this._setBreadcrumbs(response.json)
		this._renderAccountInfo(response.json);
	}

	this._provision = async function(config) {
		config.AccountID = info.AccountID;
		let response;
		try {
			response = await this._fetchJSON(info.ServerPrefix + config.AWSRegion + '/vpc/', {method: 'POST', body: JSON.stringify(config)});
		} catch (err) {
			Growl.warning('Error creating new VPC: ' + err);
			return;
		}
		Growl.success("Create VPC task submitted");
		this._showTask(response.json.TaskID);
	}

	this._unsetResourceShare = async function(region, tgw, button) {
		button.disabled = true;
		const url = info.ServerPrefix + region + '/rs/' + info.AccountID + '/' + tgw.TransitGatewayID + '/';
		try {
			await this._fetchJSON(url, {method: 'DELETE'});
		} catch (err) {
			Growl.error('Error unsetting resource share: ' + err);
			button.disabled = false;
			return;
		}
		await this._loadAccountInfo();
		button.disabled = false;
	}

	this._setResourceShare = async function(region, tgw, button) {
		const resourceShareID = prompt('Enter the resource share ID for ' + tgw.TransitGatewayName + ' (' + tgw.TransitGatewayID + ') in ' + region);
		if (!resourceShareID) return;
		if (resourceShareID.substring(0, 3) != 'rs-') {
			alert('Please enter a resource share ID beginning with "rs-"');
			return;
		}
		button.disabled = true;
		const url = info.ServerPrefix + region + '/rs/' + info.AccountID + '/' + tgw.TransitGatewayID + '/';
		const formData = new FormData();
		formData.append('ResourceShareID', resourceShareID)
		let response;
		try {
			response = await this._fetchJSON(url, {method: 'POST', body: formData});
		} catch (err) {
			Growl.error('Error setting resource share: ' + err);
			button.disabled = false;
			return;
		}
		await this._loadAccountInfo();
		button.disabled = false;
	}

	this._getWarnings = function(vpc) {
		var warnings = [];
		if (vpc.IsMissing) warnings.push('Missing in AWS');

		return warnings.join(", ");
	}

	let newVPCFormInited = false;
	this._renderAccountInfo = function(info) {
		info.Tasks = info.Tasks || [];
		const view = this;
		let noVPCs = true;
		info.Regions = info.Regions || [];
		info.Regions.forEach((region) => {
			region.VPCs = (region.VPCs || []).filter((vpc) => !vpc.IsDefault);
			region.VPCs.sort((v1, v2) => {
				if (v1.Name && !v2.Name) return -1;
				if (v2.Name && !v1.Name) return 1;
				if (v1.Name < v2.Name) return -1;
				if (v2.Name < v1.Name) return 1;
				return v1.VPCID < v2.VPCID ? -1 : 1;
			})
			region.VPCs.forEach(vpc => vpc.Issues = vpc.Issues || []);
			region.TransitGateways = (region.TransitGateways || []);
			region.TransitGateways.sort((t1, t2) => {
				if (t1.TransitGatewayName && !t2.TransitGatewayName) return -1;
				if (t2.TransitGatewayName && !t1.TransitGatewayName) return 1;
				if (t1.TransitGatewayName < t2.TransitGatewayName) return -1;
				if (t2.TransitGatewayName < t1.TransitGatewayName) return 1;
				return t1.TransitGatewayID < t2.TransitGatewayID ? -1 : 1;
			})
			noVPCs = noVPCs && region.VPCs.length == 0;
		});
		info.Regions.sort((region1, region2) => {
			if (region1.VPCs.length != region2.VPCs.length) {
				return region2.VPCs.length - region1.VPCs.length;
			}
			return region1.Name < region2.Name ? -1 : 1;
		});

		if (!newVPCFormInited) {
			newVPCFormInited = true;
			this.initNewVPCForm(
				this._newVPCContainer,
				info.Regions.map(region => region.Name),
				{
					Region: info.DefaultRegion,
					Stack: 'test',
					NamePrefix: '',
					NumPrivateSubnets: 3,
					NumPublicSubnets: 3,
					PrivateSize: 27,
					PublicSize: 27,
					IsDefaultDedicated: false,
					CanProvision: User.isAdmin(),
					AddContainersSubnets: false,
					AddFirewall: false,
				});
		}

		render(
			html`
				${info.Regions.map((region) => 
					region.VPCs.length
					? html`
						<div class="section-header">${region.Name} <button class="ds-c-button ds-c-button--inverse ds-c-button--small" style="margin-left: 10px" title="Open AWS console in the ${region.Name} region" @click="${() => this._openConsole(info.ServerPrefix, region.Name, info.AccountID)}">AWS Console</button></div>		
						<table class="standard-table" style="border: solid 1px #112E51; margin-bottom: 10px; width: 100%">
							<thead>
								<tr><th>VPC Name</th><th>VPC ID</th><th>Type</th>${User.isAdmin() ? html`<th>Actions</th>` : nothing}<th>Warnings</th></tr>
							</thead>
							<tbody>
							${region.VPCs.map((vpc) => html`
								<tr>
									<td nowrap class="${vpc.Issues.length ? (vpc.Issues.filter(i=>!i.IsFixable).length ? 'unfixable' : 'fixable') : nothing}">
									${vpc.IsAutomated && !vpc.IsMissing
										? html`<a href="${info.ServerPrefix}/accounts/${info.AccountID}/vpc/${region.Name}/${vpc.VPCID}">${vpc.Name}</a>`
										: vpc.Name}
									</td>
									<td nowrap>
									${vpc.IsAutomated && !vpc.IsMissing
										? html`<a href="${info.ServerPrefix}/accounts/${info.AccountID}/vpc/${region.Name}/${vpc.VPCID}">${vpc.VPCID}</a>`
										: vpc.VPCID}
									</td>
									<td>${VPCType.getStyled(vpc.VPCType)}</td>
									${User.isAdmin() ? html`
									<td nowrap>
										<button @click="${() => view._importVPC(region.Name, vpc.VPCID)}" class="ds-c-button ds-c-button--hover ds-c-button--primary ds-c-button--small ${vpc.IsAutomated || vpc.IsException ? 'ds-c-button--disabled' : nothing}">Import</button>
										<button @click="${() => view._importVPC(region.Name, vpc.VPCID, true)}" class="ds-c-button ds-c-button--hover ds-c-button--primary ds-c-button--small ${vpc.IsAutomated || vpc.IsException ? 'ds-c-button--disabled' : nothing}">Import Legacy</button>
										<button @click="${() => view._establishExceptionVPC(region.Name, vpc.VPCID)}" class="ds-c-button ds-c-button--hover ds-c-button--primary ds-c-button--small ${vpc.IsAutomated || vpc.IsException ? 'ds-c-button--disabled' : nothing}">Establish Exception</button>
										<button @click="${() => view._renameVPC(region.Name, vpc.VPCID, vpc.Stack, vpc.Name)}" class="ds-c-button ds-c-button--hover ds-c-button--primary ds-c-button--small ${vpc.IsAutomated ? nothing : 'ds-c-button--disabled' }">Rename</button>
										<button @click="${() => view._deleteVPC(region.Name, vpc.VPCID, vpc.Name)}" class="ds-c-button ds-c-button--hover ds-c-button--primary ds-c-button--small ${vpc.IsAutomated ? nothing : 'ds-c-button--disabled'}">Delete</button>
										<button @click="${() => view._unimportVPC(region.Name, info.AccountID, vpc.VPCID, vpc.Name, vpc.VPCType)}" class="ds-c-button ds-c-button--hover ds-c-button--primary ds-c-button--small ${vpc.IsAutomated ? nothing : 'ds-c-button--disabled'}">Unimport</button>
										<button @click="${() => view._removeExceptionVPC(region.Name, info.AccountID, vpc.VPCID, vpc.Name)}" class="ds-c-button ds-c-button--hover ds-c-button--primary ds-c-button--small ${vpc.IsException ? nothing : 'ds-c-button--disabled'}">Remove Exception</button>
									</td>
									` : nothing}
									<td>${this._getWarnings(vpc)}</td>
								</tr>
							`)}
							</tbody>
						</table>
					`
					: nothing)}`,
			this._vpcs);

		render(
			html`
				${info.Regions.map((region) => 
					region.TransitGateways.length
					? html`
						<div class="section-header">${region.Name}</div>
						<table class="standard-table" style="border: solid 1px #112E51; margin-bottom: 10px; width: 100%">
							<thead>
								<tr><th>Name</th><th>ID</th><th>Resource Share Name</th><th>Resource Share ID</th>${User.isAdmin() ? html`<th>Actions</th>` : nothing}</tr>
							</thead>
							<tbody>
							${region.TransitGateways.map((tgw) => html`
								<tr>
								<td>${tgw.TransitGatewayName}</td>
								<td>${tgw.TransitGatewayID}</td>
								<td>${tgw.ResourceShareName}</td>
								<td>${tgw.ResourceShareID}</td>
								${User.isAdmin() ? html`
								<td>
									<button class="ds-c-button ds-c-button--hover ds-c-button--primary ds-c-button--small" @click="${(e) => view._setResourceShare(region.Name, tgw, e.target)}">Set Resource Share</button>
									<button class="ds-c-button ds-c-button--hover ds-c-button--primary ds-c-button--small ${tgw.ResourceShareID ? nothing : 'ds-c-button--disabled'}" @click="${(e) => view._unsetResourceShare(region.Name, tgw, e.target)}">Unset Resource Share</button>
								</td>
								` : nothing}
								</tr>
							`)}
							</tbody>
						</table>
					`
					: html`<div class="section-header-secondary ds-u-margin-y--1">No Transit Gateways owned by this account in ${region.Name}</div>`)}`,
			this._transitGateways);

		render(
			html`
				<fixed-task-list .tasks="${info.Tasks}"></fixed-task-list>
				`,
			this._tasks
		);

		const labelConfig = { "page": "account", "AccountID": info.AccountID,  "ServerPrefix": info.ServerPrefix }
		render(
			html`<label-ui .info="${labelConfig}" .fetchJSON="${this._fetchJSON.bind(this)}"></label-ui>`,
			this._labels
		);

		this._showOlderTasksButton.addEventListener('click', (e) => {
			e.preventDefault();
			this._showOlderTasks(this._oldestTaskID);
		});

		if (info.IsMoreTasks) {
			this._showOlderTasksButton.classList.remove('ds-c-button--disabled');
			this._oldestTaskID = info.Tasks.map(t => t.ID).reduce((v, id) => Math.min(v, id))
		} else {
			this._showOlderTasksButton.classList.add('ds-c-button--disabled');
		}
	}

	this.init = function(container) {
		document.title += ' - Account ' + info.AccountID;
		const tabs = [{"name": "VPCs", "id": "vpcs", "adminOnly": false},
					  {"name": "Transit Gateways", "id": "transitGateways", "adminOnly": false},
					  {"name": "Create VPC", "id": "newVPC", "adminOnly": true },
					  {"name": "Task Logs", "id": "tasksContainer", "adminOnly": false},
					  {"name": "Labels", "id": "labelsContainer", "adminOnly": false}];
		render(
			html`
				<div id="background" class="hidden"></div>
				<div id="modal" class="hidden"></div>
				<div class="ds-l-container ds-u-padding--0">
					<div id="container">
						<div style="margin: 6px 0px;">
							<button class="ds-c-button ds-c-button--primary ds-c-button--small" @click="${() => this._getCredentials(info.ServerPrefix, info.Region, info.AccountID)}">Get Account Credentials</button>
						</div>
						<tab-ui .tabs="${tabs}"></tab-ui>
							<div id="vpcs" class="tab-content">Loading...</div>

							<div id="transitGateways" class="tab-content">Loading...</div>

							<div id="newVPC" class="tab-content" style="width: 800px">
								<div class="section-header">VPC Form</div>
								<div id="newVPCContainer" style="border: solid 1px #0071BC">Loading...</div>
							</div>

							<div id="tasksContainer" class="tab-content ds-l-row">
								<div class="ds-l-col--7">
									<div id="tasks"></div>
									<div class="ds-u-margin-y--1" style="text-align: right">
										<button id="showOlderTasks" class="ds-c-button ds-c-button--primary ds-c-button--disabled">Show Older Tasks</button>
									</div>
								</div>
							</div>

							<div id="labelsContainer" class="tab-content">
								<div class="ds-l-col--8 ds-u-padding--0">
									<div id="labels">
									</div>
								</div>
							</div>

						</div>
					</div>
				</div>`,
			container);
		this._background = document.getElementById('background');
		this._modal = document.getElementById('modal');
		this._vpcs = document.getElementById('vpcs');
		this._transitGateways = document.getElementById('transitGateways');
		this._tasks = document.getElementById('tasks');
		this._labels = document.getElementById('labels');
		this._showOlderTasksButton = document.getElementById('showOlderTasks');
		this._newVPCContainer = document.getElementById('newVPCContainer');

		this._oldestTaskID = null;

		this._listenForCancelEvent(container);
		this._listenForShowTaskEvents(container);

		this._showOlderTasksButton.addEventListener('click', (e) => {
			e.preventDefault();
			this._showOlderTasks(this._oldestTaskID);
		});

		this._loadAccountInfo();
		window.setInterval(() => this._loadAccountInfo(), 3000);
	}

	this._deleteVPC = async function(region, vpcID, name) {
		const confirmed = prompt('Are you sure you want to delete VPC ' + name + '? To confirm, type "' + name + '" below.');
		if (confirmed !== name && confirmed !== '"' + name + '"') {
			// Cancelled
			return;
		}
		let response;
		try {
			response = await this._fetchJSON(info.ServerPrefix + region + "/vpc/" + info.AccountID + "/" + vpcID, {method: 'DELETE'});
		} catch (err) {
			alert('Error deleting VPC: ' + err);
			return;
		}
		this._showTask(response.json.TaskID);
	}

	this._importVPC = async function(region, vpcID, isLegacy) {
		let response;
		try {
			response = await this._fetchJSON(info.ServerPrefix + region + "/vpc/" + info.AccountID + "/" + vpcID + '/import?legacy=' + (isLegacy ? '1' : '0'), {method: 'POST'});
		} catch (err) {
			alert('Error importing VPC: ' + err);
			return;
		}
		this._showTask(response.json.TaskID);
	}

	this._establishExceptionVPC = async function(region, vpcID) {
		let response;
		try {
			response = await this._fetchJSON(info.ServerPrefix + region + "/vpc/" + info.AccountID + "/" + vpcID + '/except', {method: 'POST'});
		} catch (err) {
			Growl.error('Error establishing VPC as exception: ' + err);
			return;
		}
		this._showTask(response.json.TaskID);
	}

	this._getRegionShortName = function(region) {
		const shortRegion = {
			'us-east-1': 'east',
			'us-west-2': 'west',
			'us-gov-east-1': 'gov-east',
			'us-gov-west-1': 'gov-west',
		}[region] || region;
		return shortRegion;
	}

	this._renameVPC = async function(region, vpcID, stack, name) {
		const newVPCprefix = prompt('Please a new name prefix (will have region and stack appended)');
		if (newVPCprefix === "") {
			// Assume cancelled, no name
			return;
		}
		const newVPCname = newVPCprefix + "-" + this._getRegionShortName(region) + "-" + stack;
		const confirmed = prompt('Are you sure you want to rename VPC ' + name + ' to ' + newVPCname + '? To confirm, retype the new name "' + newVPCname + '" below.');
		if (confirmed !== newVPCname && confirmed !== '"' + newVPCname + '"') {
			// Cancelled
			return;
		}
		let response;
		try {
			response = await this._fetchJSON(info.ServerPrefix + region + "/vpc/" + info.AccountID + "/" + vpcID + '/renameVPC', {method: 'POST', body: '"' + newVPCname + '"'});
		} catch (err) {
			alert('Error renaming VPC: ' + err);
			return;
		}
		this._showTask(response.json.TaskID);
	}

	this._unimportVPC = async function(region, accountID, vpcID, name, vpcType) {
		let response;
		try {
			response = await this._fetchJSON(info.ServerPrefix + 'accounts/' + accountID + '/vpc/' + region + '/' + vpcID + '.json?dbOnly');
		} catch (err) {
			Growl.error('Error getting VPC info: ' + err);
			return;
		}

		let message = '';

		if (VPCType.isFirewallMigrationType(vpcType)) {
			message += "This VPC is in the middle of a firewall migration and may not re-import correctly. ";
		} 

		const config = response.json.Config;
		const managedResources = [];
		Object.keys(config).forEach(key => {
			if (config[key] !== null && config[key].length > 0) { 
				if (key === 'ManagedResolverRuleSetIDs') {
					managedResources.push('Resolver Rules');
				} else if (key === 'SecurityGroupSetIDs') {
					managedResources.push('Security Groups')
				} else if (key === 'PeeringConnections') {
					managedResources.push('Peering Connections')
				}
			}
		});
		if (managedResources.length > 0) {
			message += `The following resources currently managed by VPC Conf are not re-importable: ${managedResources.join(', ')}. `;
		}

		const confirmed = prompt(`Are you sure you want to unimport VPC ${name}? ${message}To confirm, type "${name}" below.`);
		if (confirmed !== name && confirmed !== '"' + name + '"') {
			// Cancelled
			return;
		}
		try {
			response = await this._fetchJSON(info.ServerPrefix + region + "/vpc/" + info.AccountID + "/" + vpcID + '/unimport', {method: 'POST'});
		} catch (err) {
			Growl.error('Error unimporting VPC: ' + err);
			return;
		}
		this._showTask(response.json.TaskID);
	}

	this._removeExceptionVPC = async function(region, accountID, vpcID, name) {
		const confirmed = prompt(`Are you sure you want to remove VPC ${name} as an exception VPC? This will leave the VPC unmanaged by any tool. To confirm, type "${name}" below.`);
		if (confirmed !== name && confirmed !== '"' + name + '"') {
			// Cancelled
			return;
		}
		let response;
		try {
			response = await this._fetchJSON(info.ServerPrefix + region + "/vpc/" + accountID + "/" + vpcID + '/unimport', {method: 'POST'});
		} catch (err) {
			Growl.error('Error removing VPC as exception: ' + err);
			return;
		}
		this._showTask(response.json.TaskID);
	}

	this._hasMissingVPC = function(region) {
		return region.VPCs.some(vpc => vpc.IsMissing)
	}

	this._hasExceptionVPC = function(region) {
		return region.VPCs.some(vpc => vpc.IsException)
	}
}
