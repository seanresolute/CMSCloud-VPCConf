import {html, nothing, render} from '../lit-html/lit-html.js';
import {DisplaysTasks, CancelsTasks, HasModal, MakesAuthenticatedAJAXRequests, GetsCredentials} from './mixins.js';
import {Breadcrumb} from './components/shared/breadcrumb.js'
import {Growl} from './components/shared/growl.js'
import {User} from './user.js'
import {VPCType} from './vpctype.js'
import './components/shared/account-select.js';
import './components/label-ui.js';

export function VPCPage(info) {
	this._loginURL = info.ServerPrefix + 'oauth/callback';
	this._cancelTasksURL = info.ServerPrefix + 'task/cancel';
	this._baseTaskURL = info.ServerPrefix + 'task/' + info.AccountID + '/' + info.VPCID + '/'
	Object.assign(this, DisplaysTasks, CancelsTasks, HasModal, MakesAuthenticatedAJAXRequests, GetsCredentials);
	

	this.canAddFirewall = (info) => {
		return User.isAdmin() && VPCType.canAddFirewall(info.VPCType) && info.CustomPublicRoutes === "";
	}

	this.canRemoveFirewall = (info) => {
		return User.isAdmin() && VPCType.canRemoveFirewall(info.VPCType) && info.CustomPublicRoutes === "";
	}

	this._disableIfMigrating = (vpcType) => {
		const elements = document.querySelectorAll(".disableIfMigrating");
		if (VPCType.isFirewallMigrationType(vpcType)) {
			elements.forEach(el => el.disabled = true);
		} else {
			elements.forEach(el => el.disabled = false);
		}
	}

	this._sendFirewallRequest = async (e) => {
		e.preventDefault();

		const action = e.target.value;
		let prompt = `Are you sure you want to ${action} Network Firewall? All other actions will be disabled until the process is completed.`
		const confirmed = confirm(prompt);
		if (!confirmed) return;

		const config = {
			AddNetworkFirewall: action === "add",
		};

		let response;
		try {
			response = await this._fetchJSON(info.ServerPrefix + info.Region + '/vpc/' + info.AccountID + '/' + info.VPCID + '/networkFirewall', {method: 'POST', body: JSON.stringify(config)});
		} catch (err) {
			alert('Error adding/removing network firewall : ' + err);
			return;
		}
		this._showTask(response.json.TaskID);
	}
	
	this._setBreadcrumbs = (vpc) => {
		Breadcrumb.set([{name: "Accounts", link: "/provision/accounts"},
						{name: vpc.ProjectName + " - " + vpc.AccountName + " - " + vpc.AccountID, link: "/provision/accounts/" + vpc.AccountID},
						{name: vpc.Name + ' - ' + vpc.VPCID}]);
	}
	
	let vpcInfo = null;
	let loadingVPCInfo = false;

	this._disableUI = function() {
		this._background.classList.remove('hidden');
	}

	this._enableUI = function() {
		this._background.classList.add('hidden');
	}

	this._loadVPCs = async function(accountID) {
		render(
			'',
			this._updateNetworking.newConnectionOtherVPCID)
		this._updateNetworking.newConnectionOtherVPCID.disabled = true;
		this._updateNetworking.addPeeringConnection.disabled = true;
		this._peerAccountID = accountID;
		const url = info.ServerPrefix + 'accounts/' + accountID + '.json';
		let response;
		try {
			response = await this._fetchJSON(url);
		} catch (err) {
			Growl.error('Error fetching VPCs: ' + err);
			return;
		}
		if (this._peerAccountID != accountID) return;  // was called again while loading
		const vpcs = (response.json.Regions || []).map(region => (region.VPCs || []).map(vpc => Object.assign(vpc, {Region: region.Name}))).flat().filter(v => v.IsAutomated && (v.Region != info.Region || v.VPCID != info.VPCID));
		vpcs.sort((v1, v2) => {
			if (v1.Name && !v2.Name) return -1;
			if (v2.Name && !v1.Name) return 1;
			if (v1.Name < v2.Name) return -1;
			if (v2.Name < v1.Name) return 1;
			return v1.VPCID < v2.VPCID ? -1 : 1;
		})
		render(
			html`
				<option>--- VPC ---</option>
				${vpcs.map(vpc =>
					html`
					<option value="${vpc.Region}|${vpc.VPCID}">${vpc.Name} | ${vpc.VPCID} (${vpc.Region})</option>
					`
				)}`,
			this._updateNetworking.newConnectionOtherVPCID)
		this._updateNetworking.newConnectionOtherVPCID.selectedIndex = 0;
		this._updateNetworking.newConnectionOtherVPCID.disabled = false;
	}

	this._loadOtherVPCInfo = async function(regionAndID) {
		this._updateNetworking.addPeeringConnection.disabled = true;
		if (regionAndID[0] == '-') return; // --- VPC ---
		const [region, vpcID] = regionAndID.split('|');
		const accountID = this._peerAccountID
		this._peerRegionAndID = regionAndID;
		const otherVPCSubnets = this._updateNetworking.querySelector('#otherVPCSubnets');
		const url = info.ServerPrefix + 'accounts/' + accountID + '/vpc/' + region + '/' + vpcID + '.json?dbOnly';
		let response;
		try {
			response = await this._fetchJSON(url);
		} catch (err) {
			Growl.error('Error fetching VPCs: ' + err);
			return;
		}
		if (this._peerAccountID != accountID) return;  // was called again while loading
		if (this._peerRegionAndID != regionAndID) return;  // was called again while loading
		const subnetGroups = (response.json.SubnetGroups || []).sort((g1, g2) => {
			if (g1.Name == g2.Name) return (g1.SubnetType < g2.SubnetType ? -1 : 1)
			return g1.Name < g2.Name ? -1 : 1;
		});
		render(
			html`
				${subnetGroups.some(group => group.SubnetType == "Private") 
					? html`
						<input type="checkbox" id="pcxOtherVPCConnectPrivate" name="pcxOtherVPCConnectPrivate" class="ds-c-choice ds-c-choice--small">
						<label for="pcxOtherVPCConnectPrivate" class="ds-c-label">Private</label>
					`
					: nothing
				}
				${this._validPeeringGroups(subnetGroups).map((group, idx) => html`
					<input type="checkbox" name="pcxOtherVPCConnectSubnetGroups" id="pcxOtherVPCConnectSubnetGroup-${idx}" value="${group.Name}" class="ds-c-choice ds-c-choice--small">
					<label for="pcxOtherVPCConnectSubnetGroup-${idx}" class="ds-c-label">${group.Name}</label>
				`)}
			`,
			otherVPCSubnets);
		
		if (this._hasValidPeeringGroups(subnetGroups)) { this._updateNetworking.addPeeringConnection.disabled = false };
	}
	
	this._showAddNewPeeringConnection = function() {
		document.getElementById('newPeeringConnectionLink').hidden = false;
		document.getElementById('newPeeringConnection').className = '';
		this._updateNetworking.newConnectionOtherVPCID.selectedIndex = 0;
		this._updateNetworking.newConnectionOtherVPCID.disabled = true;
		this._updateNetworking.addPeeringConnection.disabled = true;
		Array.from(this._updateNetworking.querySelectorAll('[name="pcxConnectSubnetGroups"]')).forEach(cb => cb.checked = false);
		this._updateNetworking.pcxConnectPrivate.checked = false;
		this._clearAccountSelect();
	}
	
	this._hideAddNewPeeringConnection = function() {
		document.getElementById('newPeeringConnectionLink').hidden = true;
		document.getElementById('newPeeringConnection').className = 'hidden';
		this._clearAccountSelect();
	}

	this._loadVPCInfo = async function() {
		if (loadingVPCInfo) return;  // Only one at a time please
		loadingVPCInfo = true;
		const vpcURL = info.ServerPrefix + 'accounts/' + info.AccountID + '/vpc/' + info.Region + '/' + info.VPCID + '.json';
		const mtgaURL = info.ServerPrefix + 'mtgas.json';
		const sgsURL = info.ServerPrefix + 'sgs.json';
		const mrrsURL = info.ServerPrefix + 'mrrs.json';
		const accountsURL = info.ServerPrefix + 'accounts/accounts.json';
		let responses;
		try {
			responses = await Promise.all([this._fetchJSON(vpcURL), this._fetchJSON(mtgaURL), this._fetchJSON(accountsURL), this._fetchJSON(sgsURL), this._fetchJSON(mrrsURL)]);
		} catch (err) {
			Growl.error('Error fetching account info: ' + err);
			return;
		}
		const vpcResponse = responses[0];
		const mtgasResponse = responses[1];
		const sgsResponse = responses[3];
		const mrrsResponse = responses[4];
		this._accounts = responses[2].json || [];
		if (vpcResponse.text == vpcInfo) {
			// No change to table.
			loadingVPCInfo = false;
			return;
		}
		vpcInfo = vpcResponse.text;
		loadingVPCInfo = false;
		this._renderVPCInfo(vpcResponse.json, mtgasResponse.json, sgsResponse.json, mrrsResponse.json, info.Region, info.ServerPrefix, info.AccountID, info.VPCID);
	}

	const verifyCheckboxNames = [
		'verifyNetworking',
		'verifyLogging',
		'verifyResolverRules',
		'verifySecurityGroups',
		'verifyCIDRs',
		'verifyCMSNet',
	];

	this._repair = async function() {
		let response;
		const body = Object.create(null);
		verifyCheckboxNames.forEach(name => {
			body[name[0].toUpperCase() + name.slice(1)] = !!this._verifyForm[name].checked;
		});
		try {
			response = await this._fetchJSON(info.ServerPrefix + info.Region + "/vpc/" + info.AccountID + "/" + info.VPCID + '/repair', {method: 'POST', body: JSON.stringify(body)});
		} catch (err) {
			alert('Error repairing VPC: ' + err);
			return;
		}
		this._showTask(response.json.TaskID);
	}

	this._verify = async function() {
		let response;
		const body = Object.create(null);
		verifyCheckboxNames.forEach(name => {
			body[name[0].toUpperCase() + name.slice(1)] = !!this._verifyForm[name].checked;
		});

		try {
			response = await this._fetchJSON(info.ServerPrefix + info.Region + "/vpc/" + info.AccountID + "/" + info.VPCID + '/verify', {method: 'POST', body: JSON.stringify(body)});
		} catch (err) {
			alert('Error verifying VPC: ' + err);
			return;
		}
		this._showTask(response.json.TaskID);
	}

	this._syncRoutes = async function() {
		let response;
		try {
			response = await this._fetchJSON(info.ServerPrefix + info.Region + "/vpc/" + info.AccountID + "/" + info.VPCID + '/syncRouteState', {method: 'POST'});
		} catch (err) {
			alert('Error syncing route state for VPC: ' + err);
			return;
		}
		this._showTask(response.json.TaskID);
	}

	this._verificationOptionsChanged = function(e) {
		if (e.target.name == 'verifyAll') {
			verifyCheckboxNames.forEach(name => {
				this._verifyForm[name].checked = !!e.target.checked;
			});
		} else {
			this._verifyForm.verifyAll.checked = verifyCheckboxNames.every(name => !!this._verifyForm[name].checked);
		}
		[...this._verifyForm.querySelectorAll('button')].forEach(btn => {
			btn.disabled = !User.isAdmin() || verifyCheckboxNames.every(name => !this._verifyForm[name].checked);
		})
	}

	this._addAvailabilityZone = async function(e) {
		let response;
		const availabilityZoneSelector = this._availabilityZones.querySelector('#availabilityZoneSelector');
		const body = Object.create(null);
		body.AZName = availabilityZoneSelector.value
		try {			
			response = await this._fetchJSON(info.ServerPrefix + info.Region + "/vpc/" + info.AccountID + "/" + info.VPCID + '/addAvailabilityZone', {method: 'POST', body: JSON.stringify(body)});
		} catch (err) {
			alert('Error verifying VPC: ' + err);
			return;
		}
		this._showTask(response.json.TaskID);
	}

	this._removeAvailabilityZone = async function(e) {
		let response;
		const availabilityZoneSelector = this._availabilityZones.querySelector('#availabilityZoneSelector');
		const confirmed = prompt('Are you sure you want to delete AZ ' + availabilityZoneSelector.value + '? To confirm, type "' + availabilityZoneSelector.value + '" below.');
		if (confirmed !== availabilityZoneSelector.value && confirmed !== '"' + availabilityZoneSelector.value + '"') {
			// Cancelled
			return;
		}
		const body = Object.create(null);
		body.AZName = availabilityZoneSelector.value
		try {			
			response = await this._fetchJSON(info.ServerPrefix + info.Region + "/vpc/" + info.AccountID + "/" + info.VPCID + '/removeAvailabilityZone', {method: 'POST', body: JSON.stringify(body)});
		} catch (err) {
			alert('Error verifying VPC: ' + err);
			return;
		}
		this._showTask(response.json.TaskID);
	}

	this._disconnectZonedSubnets = async function(groupName, cidr) {
		const confirmed = confirm('Are you sure you want to disconnect subnet group "' + groupName + '" from CMSNet CIDR ' + cidr + '?');
		if (!confirmed) return;

		const config = {
			GroupName: groupName,
			DestinationCIDR: cidr,
		};

		let response;
		this._disableUI();
		try {
			response = await this._fetchJSON(info.ServerPrefix + info.Region + '/vpc/' + info.AccountID + '/' + info.VPCID + '/disconnectCMSNet', {method: 'POST', body: JSON.stringify(config)});
		} catch (err) {
			alert('Error disconnecting subnet group: ' + err);
			this._enableUI();
			return;
		}
		alert('Disconnect request submitted! See subnet table for status details.');
		this._enableUI();
	}

	this._deleteCMSNetNAT = async function(nat, reserve) {
		let prompt = 'Are you sure you want to remove the NAT\n  ' + nat.InsideNetwork + ' to ' + nat.OutsideNetwork;
		if (reserve) {
			prompt = 'Are you sure you want to remove the NAT\n  ' + nat.InsideNetwork + ' to ' + nat.OutsideNetwork + ' \nand reserve public IP ' + nat.OutsideNetwork + ' for a future NAT?'
		}
		const confirmed = confirm(prompt);
		if (!confirmed) return;

		var params = Object.assign(Object.create(null), nat);
		params.KeepIPReserved = !!reserve;
		this._disableUI();
		try {
			await this._fetchJSON(info.ServerPrefix + info.Region + '/vpc/' + info.AccountID + '/' + info.VPCID + '/deleteCMSNetNAT', {method: 'POST', body: JSON.stringify(params)});
		} catch (err) {
			alert('Error deleting NAT: ' + err);
			this._enableUI();
			return;
		}
		let msg = 'Delete request submitted! See subnet table for status details.';
		if (reserve) {
			msg += ' You should add a new NAT with the reserved IP as soon as possible after the delete succeeds.'
		}
		alert(msg);
		this._enableUI();
	}

	this._clearAccountSelect = function() {
		document.querySelector("account-select").accountInfo = "--Account--";
	}

	this._addPeeringConnection = async function() {
		const [otherVPCRegion, otherVPCID] = this._peerRegionAndID.split('|');
		const pc = {
			Keep: true,
			OtherVPCID: otherVPCID,
			OtherVPCAccountID: this._peerAccountID,
			OtherVPCRegion: otherVPCRegion,
			IsRequester: true,
			ConnectPrivate: !!this._updateNetworking.pcxConnectPrivate.checked,
			OtherVPCConnectPrivate: this._updateNetworking.pcxOtherVPCConnectPrivate && this._updateNetworking.pcxOtherVPCConnectPrivate.checked,
			ConnectSubnetGroups: Array.from(this._updateNetworking.querySelectorAll('[name="pcxConnectSubnetGroups"]')).filter(c => c.checked).map(c => c.value),
			OtherVPCConnectSubnetGroups: Array.from(this._updateNetworking.querySelectorAll('[name="pcxOtherVPCConnectSubnetGroups"]')).filter(c => c.checked).map(c => c.value),
		}
		if (!pc.ConnectPrivate && !pc.ConnectSubnetGroups.length) {
			alert('You must select some subnets to connect from each VPC');
			return;
		}
		if (!pc.OtherVPCConnectPrivate && !pc.OtherVPCConnectSubnetGroups.length) {
			alert('You must select some subnets to connect from each VPC');
			return;
		}
		this._peeringConnections.push(pc);
		this._renderPeeringConnections();
		this._hideAddNewPeeringConnection();
	}

	this._fillInVPCInfo = async function(peeringConnection) {
		const url = info.ServerPrefix + 'accounts/' + peeringConnection.OtherVPCAccountID + '/vpc/' + peeringConnection.OtherVPCRegion + '/' + peeringConnection.OtherVPCID + '.json?dbOnly';
		let response;
		try {
			response = await this._fetchJSON(url);
		} catch (err) {
			Growl.error('Error fetching VPC info: ' + err);
			return;
		}
		peeringConnection.OtherVPCName = response.json.Name;
	}

	this._renderPeeringConnections = async function() {
		function getSubnetGroupString(groups, connectPrivate) {
			const sn = (groups || []).slice();
			if (connectPrivate) sn.unshift('Private');
			return sn.join(', ')
		}

		await Promise.all(this._peeringConnections.map(pc => this._fillInVPCInfo(pc)));

		render(
			html`
			<table>
				${this._peeringConnections.length
					? html`<thead>
							<th></th>
							<th></th>
							<th></th>
							<th></th>
							<th style="text-align: center">Requester?</th>
							<th style="text-align: center">Keep?</th>
						</thead>`
					: ''}
				<tbody>
					${this._peeringConnections.map(pc => html`
						<tr>
							<td>${getSubnetGroupString(pc.ConnectSubnetGroups, pc.ConnectPrivate)}</td>
							<td>⇄</td>
							<td>${getSubnetGroupString(pc.OtherVPCConnectSubnetGroups, pc.OtherVPCConnectPrivate)}</td>
							<td><a href="${info.ServerPrefix + 'accounts/' + pc.OtherVPCAccountID + '/vpc/' + pc.OtherVPCRegion + '/' + pc.OtherVPCID}" target="_blank">${pc.OtherVPCName}</a></td>
							<td>${pc.IsRequester ? 'Yes' : 'No'}</td>
							<td class="disableIfMigrating" style="text-align: center"><input type="checkbox" ?checked=${pc.Keep} @change="${(e) => pc.Keep = !!e.target.checked}" ?disabled="${!User.isAdmin()}"></td>
						</tr>
					`)}
				</tbody>
			</table>
			<table>
				<tr>
					<td colspan="5">
						<button id="newPeeringConnectionLink" @click="${(e) => {e.preventDefault(); this._showAddNewPeeringConnection();}}" class="ds-c-button ds-c-button--primary disableIfMigrating" ?disabled="${!User.isAdmin()}">Add New</button>
						<div id="newPeeringConnection" class="hidden">
							<div id="accountSelectContainer" style="margin-top: -4px; margin-bottom: 5px">
								<account-select .accounts="${this._accounts}" @account-selected="${(e) => this._loadVPCs(e.detail.account.ID)}"></account-select>
							</div>
							<select name="newConnectionOtherVPCID" @change="${(e) => {this._loadOtherVPCInfo(e.target.value)}}" disabled class="ds-c-field">
								<option>--- VPC ---</option>
							</select>
							<table>
								<tr class="ds-u-valign--top">
									<td>
										${this._subnetGroups.some(group => group.SubnetType == "Private") 
											? html`
												<input type="checkbox" id="pcxConnectPrivate" name="pcxConnectPrivate" class="ds-c-choice ds-c-choice--small">
												<label for="pcxConnectPrivate" class="ds-c-label">Private</label>
											`
											: nothing
										}
										${this._validPeeringGroups(this._subnetGroups).map((group, idx) => html`
											<input type="checkbox" id="pcxConnectSubnetGroup-${idx}" name="pcxConnectSubnetGroups" value="${group.Name}" class="ds-c-choice ds-c-choice--small">
											<label for="pcxConnectSubnetGroup-${idx}" class="ds-c-label">${group.Name}</label>
										`)}
									</td>
									<td class="ds-u-valign--middle">⇄</td>
									<td>
										<div id="otherVPCSubnets"></div>
									</td>
								</tr>
							</table>
							<input type="button" value="Cancel" @click="${() => this._hideAddNewPeeringConnection()}" class="ds-c-button ds-c-button--primary disableIfMigrating" ?disabled="${!User.isAdmin()}">
							<input name="addPeeringConnection" type="button" value="Add" @click="${() => this._addPeeringConnection()}" class="ds-c-button ds-c-button--primary disableIfMigrating" ?disabled="${!User.isAdmin()}">
						</div>
					</td>
				</tr>
			</table>
			`,
			this._peeringConnectionsContainer
		);
	}

	this._toggleUseReservedIP = function(useReservedIP) {
		const container = document.getElementById('reservedPublicIPInput');
		if (useReservedIP) {
			container.disabled = false;
		} else {
			container.value = '';
			container.disabled = true;
		}
	}

	this._updateZonedSubnetGroupName = function() {
		const type = this._addZonedSubnets.zonedSubnetType.value.toLowerCase();
		this._addZonedSubnets.zonedSubnetGroupName.value = `${type}${this._suffixForSubnetGroup(type)}`;
	}

	this._suffixForSubnetGroup = function(groupName) {
		let maxNumber = 0;
		let found = false;
		let suffix = "";

		this._subnetGroups.forEach(group => {
			if (group.Name.startsWith(groupName)) {
				found = true;
				const regex = new RegExp(`${groupName}-?(\\d+)`);
				const match = group.Name.match(regex);
				if (match !== null) {
					if (match[1] !== undefined) {
						const currentNumber = +match[1];
						if (currentNumber > maxNumber) {
							maxNumber = currentNumber;
						}
					}
				}
			}
		})

		if (found) {
			suffix = `-${maxNumber + 1}`;
		} 
		return suffix;
	}

	// Private subnets are valid for peering but are handled separately
	this._validPeeringGroups = function(groups) {
		return groups.filter(group => group.SubnetType !== "Public" && group.SubnetType !== "Private" && group.SubnetType !== "Unroutable" && group.SubnetType !== "Firewall");
	}

	this._hasValidPeeringGroups = function(groups) {
		return groups.filter(group => group.SubnetType !== "Public" && group.SubnetType !== "Unroutable" && group.SubnetType !== "Firewall").length > 0;
	}

	this._validCMSNetConnectGroups = function(groups) {
		return groups.filter(group => group.SubnetType !== "Transitive" && group.SubnetType !== "Public" && group.SubnetType !== "Private" && group.SubnetType !== "Unroutable" && group.SubnetType !== "Firewall");
	}

	this._validRemoveSubnetGroups = function(info) {
		const groupNamesInPrimaryCIDR = info.Subnets
		.filter(subnet => subnet.InPrimaryCIDR)
		.map(subnet => subnet.GroupName);
		return info.SubnetGroups.filter(group => !groupNamesInPrimaryCIDR.includes(group.Name) && group.SubnetType != "Firewall");
	}

	this.init = function(container) {
		document.title += ' - VPC ' + info.VPCID;
		const view = this;

		var tabs = [{ "name": "Networking", "id": "networking-tab", "adminOnly": false},
					{ "name": "Zoned Subnets", "id": "zoned-subnets-tab", "adminOnly": false},
					{"name": "Security Groups", "id": "security-groups-tab", "adminOnly": false },
					{"name": "Resolver Rules", "id": "resolver-rules-tab", "adminOnly": false},
					{"name": "Network Firewall", "id": "network-firewall-tab", "adminOnly": false},
					{"name": "Task Logs", "id": "task-logs-tab", "adminOnly": false},
					{"name": "Labels", "id": "labels-tab", "adminOnly": false}]

		render(
			html`
				<div id="background" class="hidden"></div>
				<div id="modal" class="hidden"></div>
				<div class="ds-l-container ds-u-padding--0">
					<div id="container">
						<div class="ds-u-margin-y--1">
							<a class="ds-c-button ds-c-button--primary ds-c-button--small" target="_blank" href="${info.ServerPrefix}accounts/${info.AccountID}/console?region=${info.Region}&vpc=${info.VPCID}">Log in to Console</a>
							<a class="ds-c-button ds-c-button--primary ds-c-button--small" href="javascript:void(0)" @click="${() => view._getCredentials(info.ServerPrefix, info.Region, info.AccountID)}">Get credentials</a>
							<div id="vpcType" style="float: right"></div>
						</div>
						<tab-ui .tabs="${tabs}" sticky></tab-ui>

						<div id="networking-tab" class="tab-content">
							<div id="primaryCIDR" class="ds-u-margin-y--0 ds-h4"></div>
							<div id="secondaryCIDRs" class="ds-u-margin-y--0 ds-h4"></div>
							<table id="subnets" class="standard-table">
							</table>
	
							<div id="issues" class="hidden">
								<div class="section-header-secondary">Issues</div>
								<div id="issuesList"></div>
							</div>

							<div class="section-header-secondary">Availability Zones</div>
							<div class="section-body-bordered">
								<form id="availabilityZones" @submit="${(e)=>e.preventDefault()}">
									<select id="availabilityZoneSelector" name="availabilityZone" class="ds-c-field input-medium" ?disabled="${!User.isAdmin()}">
									</select>

									<button @click="${() => view._addAvailabilityZone()}" class="ds-c-button ds-c-button--primary" ?disabled="${!User.isAdmin()}">Add AZ</button>
								</form>
							</div>

							<div class="ds-u-margin-y--2">
								<div class="section-header-secondary">Verify/Repair</div>
								<div class="section-body-bordered">
									<form id="verify" @submit="${(e)=>e.preventDefault()}" @change="${(e)=>view._verificationOptionsChanged(e)}">
										<input type="checkbox" name="verifyAll" id="verifyAll" value="1" class="ds-c-choice ds-c-choice--small disableIfMigrating" ?disabled="${!User.isAdmin()}">
										<label for="verifyAll" class="ds-c-label">All</label>

										<input type="checkbox" name="verifyNetworking" id="verifyNetworking" value="1" class="ds-c-choice ds-c-choice--small disableIfMigrating" ?disabled="${!User.isAdmin()}">
										<label for="verifyNetworking" class="ds-c-label">Networking</label>

										<input type="checkbox" name="verifyLogging" id="verifyLogging" value="1" class="ds-c-choice ds-c-choice--small disableIfMigrating" ?disabled="${!User.isAdmin()}">
										<label for="verifyLogging" class="ds-c-label">Logging</label>

										<input type="checkbox" name="verifyResolverRules" id="verifyResolverRules" value="1" class="ds-c-choice ds-c-choice--small disableIfMigrating" ?disabled="${!User.isAdmin()}">
										<label for="verifyResolverRules" class="ds-c-label">Resolver Rules</label>

										<input type="checkbox" name="verifySecurityGroups" id="verifySecurityGroups" value="1" class="ds-c-choice ds-c-choice--small disableIfMigrating" ?disabled="${!User.isAdmin()}">
										<label for="verifySecurityGroups" class="ds-c-label">Security Groups</label>

										<input type="checkbox" name="verifyCIDRs" id="verifyCIDRs" value="1" class="ds-c-choice ds-c-choice--small disableIfMigrating" ?disabled="${!User.isAdmin()}">
										<label for="verifyCIDRs" class="ds-c-label">CIDRs</label>

										<input type="checkbox" name="verifyCMSNet" id="verifyCMSNet" value="1" class="ds-c-choice ds-c-choice--small disableIfMigrating" ?disabled="${!User.isAdmin()}">
										<label for="verifyCMSNet" class="ds-c-label">CMSNet</label>

										<button @click="${() => view._verify()}" class="ds-c-button ds-c-button--primary disableIfMigrating" disabled>Verify state</button>
										<button @click="${() => view._repair()}" class="ds-c-button ds-c-button--primary disableIfMigrating" disabled>Sync state, repair tags, apply config</button>
									</form>
								</div>
							</div>

							<div class="ds-u-margin-y--2">
								<div class="section-header-secondary">Routes</div>
								<div class="section-body-bordered">
									<button @click="${() => view._syncRoutes()}" class="ds-c-button ds-c-button--primary disableIfMigrating" disabled>Re-Import Routes</button>
								</div>
							</div>

							<div class="ds-u-margin-y--1">
								<form method="post" id="updateNetworking" class="hidden">
								</form>
							</div>
						</div>

						<div id="zoned-subnets-tab" class="tab-content">
							<form method="post" id="addZonedSubnets" class="hidden">
								<table class="standard-table">
								<thead>
									<tr>
										<th>
											Zone Type
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
											<select id="zonedSubnetType" name="zonedSubnetType" class="ds-c-field input-medium disableIfMigrating" ?disabled="${!User.isAdmin()}">
												<option value="App">App</option>
												<option value="Data">Data</option>
												<option value="Web">Web</option>
												<option value="Transport">Transport</option>
												<option value="Management">Management</option>
												<option value="Security">Security</option>
												<option value="Shared">Shared</option>
												<option value="Shared-OC">Shared-OC</option>
												<option value="Private">Private</option>
												<option value="Public">Public</option>
												<option value="Unroutable">Unroutable</option>
											</select>
										</td>
										<td>
											<select id="zonedSubnetSize" name="zonedSubnetSize" class="ds-c-field input-medium disableIfMigrating" ?disabled="${!User.isAdmin()}">
												<option value="20">/20 (4,096 IPs per AZ)</option>
												<option value="21">/21 (2,048 IPs per AZ)</option>
												<option value="22">/22 (1,024 IPs per AZ)</option>
												<option value="23">/23 (512 IPs per AZ)</option>
												<option value="24">/24 (256 IPs per AZ)</option>
												<option value="25">/25 (128 IPs per AZ)</option>
												<option value="26">/26 (64 IPs per AZ)</option>
												<option value="27">/27 (32 IPs per AZ)</option>
												<option value="28" selected>/28 (16 IPs per AZ)</option>
											</select>
											<span id="unroutableSubnetSize" class="hidden"></span>
										</td>
										<td>
											<input id="zonedSubnetGroupName" name="zonedSubnetGroupName" class="ds-c-field input-medium disableIfMigrating" ?disabled="${!User.isAdmin()}">
										</td>
										<td>
											<input type="submit" value="Add" class="ds-c-button ds-c-button--primary disableIfMigrating" ?disabled="${!User.isAdmin()}">
										</td>
									</tr>
								</tbody>
							</table>
							</form>

							<div id="cmsnetAPIDown" class="hidden">
								<h3>Connect zoned subnet group to CMS</h3>
								<p class="cmsnet-error">CMSNet connectivity functionality is temporarily disabled because of an issue with the CMSNet API: <span id="cmsnetError"></span></p>
							</div>

							<div id="cmsnetUnsupported" class="hidden">
								<div class="section-header">Connect zoned subnet group to CMS</div>
								CMSNet connections are not supported in this region.</p>
							</div>

							<form method="post" id="connectZonedSubnets" class="hidden">
								<div class="ds-l-row ds-u-margin-x--0 ds-u-margin-y--1">
									<div class="ds-l-col--7 ds-u-padding-x--0">
										<div class="section-header-secondary">Connect zoned subnet group to CMS</div>
										<div class="section-body-bordered">
											<div class="ds-l-row ds-u-align-items--end">
												<div class="ds-l-col--4">
													<label for="connectGroupName" class="ds-c-label ds-u-margin--0">Zone</label>
													<select id="connectGroupName" name="groupName" class="ds-c-field input-medium disableIfMigrating" ?disabled="${!User.isAdmin()}"></select>
												</div>
												<div class="ds-l-col--6">
													<label for="cidr" class="ds-c-label ds-u-margin--0">CMSNet CIDR (must be in same network zone)</label>
													<input type="text" name="cidr" id="cidr" placeholder="10.x.x.x/x" class="ds-c-field input-medium disableIfMigrating" ?disabled="${!User.isAdmin()}">
												</div>
												<div class="ds-l-col--2">
													<input type="submit" name="connect" value="Connect" class="ds-c-button ds-c-button--primary disableIfMigrating" style="margin-bottom: 5px;" ?disabled="${!User.isAdmin()}">
												</div>
											</div>
										</div>
									</div>
								</div>
							</form>

							<form method="post" id="removeZonedSubnets" class="hidden">
								<div class="ds-l-row ds-u-margin-x--0 ds-u-margin-y--1">
									<div class="ds-l-col--3 ds-u-padding-x--0">
										<div class="section-header-secondary">Delete zoned subnet group</div>
										<div class="section-body-bordered">
											<div class="ds-l-row ds-u-align-items--end">
												<div class="ds-l-col--8">
													<label for="removeGroupName" class="ds-c-label ds-u-margin--0">Zone</label>
													<select id="removeGroupName" name="groupName" class="ds-c-field input-medium disableIfMigrating" ?disabled="${!User.isAdmin()}"></select>
												</div>
												<div class="ds-l-col--4">
													<input type="submit" value="Delete" class="ds-c-button ds-c-button--primary disableIfMigrating" style="margin-bottom: 5px;" ?disabled="${!User.isAdmin()}">
												</div>
											</div>
										</div>
									</div>
								</div>
							</form>

							<form method="post" id="addCMSNetNAT" class="hidden">
								<div class="ds-l-row ds-u-margin-x--0 ds-u-margin-y--1">
									<div class="ds-l-col--7 ds-u-padding-x--0">
										<div class="section-header-secondary">Add CMSNet NAT</div>
										<div class="section-body-bordered">
											<div class="ds-l-row ds-u-align-items--end">
												<div class="ds-l-col--4">
													<label for="privateIP" class="ds-c-label ds-u-margin--0">Private IP</label>
													<input name="privateIP" required placeholder="10.x.x.x" class="ds-c-field input-medium disableIfMigrating" ?disabled="${!User.isAdmin()}">
												</div>
												<div class="ds-l-col--3">
													<input type="checkbox" id="reservedIP" @change="${(e) => view._toggleUseReservedIP(e.target.checked)}" class="ds-c-choice ds-c-choice--small disableIfMigrating" ?disabled="${!User.isAdmin()}">
													<label for="reservedIP" class="ds-c-label">Use reserved public IP</label>
												</div>
												<div class="ds-l-col--3">
													<label for="reservedPublicIPInput" class="ds-c-label ds-u-margin--0">Reserved Public IP</label>
													<input id="reservedPublicIPInput" name="publicIP" placeholder="x.x.x.x" class="ds-c-field" disabled>
												</div>
												<div class="ds-l-col--1">
													<input type="submit" name="add" value="Add" class="ds-c-button ds-c-button--primary disableIfMigrating" style="margin-bottom: 5px;" ?disabled="${!User.isAdmin()}">
												</div>
											</div>
										</div>
									</div>
								</div>
							</form>
						</div>
						
						<div id="security-groups-tab" class="tab-content">
							<form method="post" id="updateSecurityGroups" class="hidden">
							</form>
						</div>
						
						<div id="resolver-rules-tab" class="tab-content">
							<form method="post" id="updateResolverRules" class="hidden">
							</form>
						</div>

						<div id="network-firewall-tab" class="tab-content">
							<div class="ds-l-col--8 ds-u-padding--0" id="networkFirewall">
							</div>
						</div>

						<div id="task-logs-tab" class="tab-content ds-l-row ds-u-margin--0">
							<div class="ds-l-col--8 ds-u-padding--0">
								<div id="tasks"></div>
								<div class="ds-u-margin-y--1" style="text-align: right">
									<button id="showOlderTasks" class="ds-c-button ds-c-button--primary ds-c-button--disabled">Show Older Tasks</button>
								</div>
							</div>
						</div>

						<div id="labels-tab" class="tab-content">
							<div class="ds-l-col--8 ds-u-padding--0">
								<div id="labels">
								</div>
							</div>
						</div>

					</div>
				</div>
			`,
			container)

		this._modal = document.getElementById('modal');
		this._background = document.getElementById('background');
		this._subnets = document.getElementById('subnets');
		this._tasks = document.getElementById('tasks');
		this._labels = document.getElementById('labels');
		this._issues = document.getElementById('issues');
		this._issuesList = document.getElementById('issuesList');
		this._showOlderTasksButton = document.getElementById('showOlderTasks');
		this._primaryCIDR = document.getElementById('primaryCIDR');
		this._secondaryCIDRs = document.getElementById('secondaryCIDRs');
		this._availabilityZones = document.getElementById('availabilityZones');
		this._updateNetworking = document.getElementById('updateNetworking');
		this._updateSecurityGroups = document.getElementById('updateSecurityGroups');
		this._updateResolverRules = document.getElementById('updateResolverRules');
		this._addZonedSubnets = document.getElementById('addZonedSubnets');
		this._removeZonedSubnets = document.getElementById('removeZonedSubnets');
		this._addCMSNetNAT = document.getElementById('addCMSNetNAT');
		this._connectZonedSubnets = document.getElementById('connectZonedSubnets');
		this._cmsnetAPIDown = document.getElementById('cmsnetAPIDown');
		this._cmsnetUnsupported = document.getElementById('cmsnetUnsupported');
		this._cmsnetError = document.getElementById('cmsnetError');
		this._unroutableSubnetSize = document.getElementById('unroutableSubnetSize');
		this._verifyForm = document.getElementById('verify');
		this._vpcType = document.getElementById('vpcType');
		this._networkFirewall = document.getElementById('networkFirewall');

		this._listenForCancelEvent(container);
		this._listenForShowTaskEvents(container);

		this._serverPrefix = info.ServerPrefix;

		this._updateNetworking.addEventListener('submit', async (e) =>{
			e.preventDefault();

			const peeringConnections = this._peeringConnections.filter(pc => pc.Keep);
			for (const idx1 in peeringConnections) {
				const pc1 = peeringConnections[idx1];
				for (let idx2 = 0; idx2 < idx1; idx2++) {
					const pc2 = peeringConnections[idx2];
					if (pc1.OtherVPCRegion == pc2.OtherVPCRegion && pc1.OtherVPCID == pc2.OtherVPCID) {
						alert('You have two peering connections to ' + pc1.OtherVPCName + ' (' + pc1.OtherVPCID + ') configured. Please configure a single connection for all the subnets you want connected and uncheck all other connections to that VPC.');
						return;
					}
				}
			}

			const config = {
				ConnectPublic: this._updateNetworking.connectPublic && !!this._updateNetworking.connectPublic.checked,
				ConnectPrivate: this._updateNetworking.connectPrivate && !!this._updateNetworking.connectPrivate.checked,
				ManagedTransitGatewayAttachmentIDs: Array.from(this._updateNetworking.querySelectorAll("[name=mtga]")).filter(inp => inp.checked).map(inp => +inp.value),
				PeeringConnections: peeringConnections,
			};

			let response;
			try {
				response = await this._fetchJSON(info.ServerPrefix + info.Region + '/vpc/' + info.AccountID + '/' + info.VPCID + '/network', {method: 'POST', body: JSON.stringify(config)});
			} catch (err) {
				alert('Error updating networking: ' + err);
				return;
			}
			this._showTask(response.json.TaskID);
		});

		this._updateSecurityGroups.addEventListener('submit', async (e) =>{
			e.preventDefault();

			const config = {
				SecurityGroupSetIDs: Array.from(document.querySelectorAll('[name=sgs]')).filter(cb => cb.checked).map(cb => +cb.value),
			};

			let response;
			try {
				response = await this._fetchJSON(info.ServerPrefix + info.Region + '/vpc/' + info.AccountID + '/' + info.VPCID + '/sgs', {method: 'POST', body: JSON.stringify(config)});
			} catch (err) {
				alert('Error updating security groups: ' + err);
				return;
			}
			this._showTask(response.json.TaskID);
		});

		this._addZonedSubnets.zonedSubnetType.addEventListener('change', () => {
			this._updateZonedSubnetGroupName();			
			const isUnroutable = this._addZonedSubnets == null ? false : this._addZonedSubnets.zonedSubnetType.value == "Unroutable";
			if (isUnroutable) {
				this._addZonedSubnets.zonedSubnetSize.classList.add('hidden');
				let subnetSize = 16;
				if (this._numberOfAZs > 4) {
					subnetSize += 3
				} else if (this._numberOfAZs > 2) {
					subnetSize += 2
				} else if (this._numberOfAZs > 1) {
					subnetSize += 1
				}

				this._unroutableSubnetSize.innerText = `/${subnetSize} (${Math.pow(2, 32 - subnetSize)} IPs per AZ - Total = ${this._numberOfAZs * (Math.pow(2, 32 - subnetSize))}) IPs)`;
				this._unroutableSubnetSize.classList.remove('hidden');
			} else {
				this._addZonedSubnets.zonedSubnetSize.classList.remove('hidden');
				this._unroutableSubnetSize.classList.add('hidden');
			}
		});

		this._addZonedSubnets.addEventListener('submit', async (e) =>{
			e.preventDefault();

			const config = {
				SubnetType: this._addZonedSubnets.zonedSubnetType.value,
				SubnetSize: +this._addZonedSubnets.zonedSubnetSize.value,
				GroupName: this._addZonedSubnets.zonedSubnetGroupName.value,
			};
			let response;
			try {
				response = await this._fetchJSON(info.ServerPrefix + info.Region + '/vpc/' + info.AccountID + '/' + info.VPCID + '/addZonedSubnets', {method: 'POST', body: JSON.stringify(config)});
			} catch (err) {
				alert('Error adding a zoned subnet: ' + err);
				return;
			}
			this._showTask(response.json.TaskID);
		});

		this._removeZonedSubnets.addEventListener('submit', async (e) => {
			e.preventDefault();
			
			const groupName = this._removeZonedSubnets.groupName.value;
			const confirmed = prompt('Are you sure you want to delete subnet group ' + groupName + '? To confirm, type "' + groupName + '" below.');
			if (confirmed !== groupName && confirmed !== '"' + groupName + '"') {
				// Cancelled
				return;
			}

			const config = {
				GroupName: groupName,
				SubnetType: this._getSubnetType(groupName),
			};

			let response;
			try {
				response = await this._fetchJSON(info.ServerPrefix + info.Region + '/vpc/' + info.AccountID + '/' + info.VPCID + '/removeZonedSubnets', {method: 'POST', body: JSON.stringify(config)});
			} catch (err) {
				alert('Error removing zoned subnet group: ' + err);
				return;
			}
			this._showTask(response.json.TaskID);
		});

		this._connectZonedSubnets.addEventListener('submit', async (e) =>{
			e.preventDefault();

			this._connectZonedSubnets.connect.disabled = true;

			const config = {
				GroupName: this._connectZonedSubnets.groupName.value,
				DestinationCIDR: this._connectZonedSubnets.cidr.value,
			};

			let response;
			this._disableUI();
			try {
				response = await this._fetchJSON(info.ServerPrefix + info.Region + '/vpc/' + info.AccountID + '/' + info.VPCID + '/connectCMSNet', {method: 'POST', body: JSON.stringify(config)});
			} catch (err) {
				alert('Error connecting subnet group: ' + err);
				this._connectZonedSubnets.connect.disabled = false;
				this._enableUI();
				return;
			}
			alert('Request submitted! See subnet table for status details.');
			this._connectZonedSubnets.connect.disabled = false;
			this._connectZonedSubnets.reset();
			this._enableUI();
		});

		this._addCMSNetNAT.addEventListener('submit', async (e) => {
			e.preventDefault();

			this._addCMSNetNAT.add.disabled = true;

			const config = {
				PrivateIP: this._addCMSNetNAT.privateIP.value,
				PublicIP: this._addCMSNetNAT.publicIP.value,
			};

			let response;
			this._disableUI();
			try {
				response = await this._fetchJSON(info.ServerPrefix + info.Region + '/vpc/' + info.AccountID + '/' + info.VPCID + '/addCMSNetNAT', {method: 'POST', body: JSON.stringify(config)});
			} catch (err) {
				alert('Error connecting subnet group: ' + err);
				this._addCMSNetNAT.add.disabled = false;
				this._enableUI();
				return;
			}
			alert('Request submitted! See subnet table for status details.');
			this._addCMSNetNAT.add.disabled = false;
			this._addCMSNetNAT.reset();
			this._toggleUseReservedIP(false);
			this._enableUI();
		});

		this._updateResolverRules.addEventListener('submit', async (e) =>{
			e.preventDefault();

			const config = {
				ManagedResolverRuleSetIDs: Array.from(this._updateResolverRules.querySelectorAll("[name=mrr]")).filter(inp => inp.checked).map(inp => +inp.value),
			};

			let response;
			try {
				response = await this._fetchJSON(info.ServerPrefix + info.Region + '/vpc/' + info.AccountID + '/' + info.VPCID + '/resolverRules', {method: 'POST', body: JSON.stringify(config)});
			} catch (err) {
				alert('Error updating resolver rules: ' + err);
				return;
			}
			this._showTask(response.json.TaskID);
		});

		this._oldestTaskID = null;
		this._showOlderTasksButton.addEventListener('click', (e) => {
			e.preventDefault();
			this._showOlderTasks(this._oldestTaskID);
		});

		this._loadVPCInfo();

		window.setInterval(() => this._loadVPCInfo(), 3000);
	}

	this._getSubnetType = (groupName) => {
		for (let i=0; i<this._subnetGroups.length; i++) {
			if (this._subnetGroups[i].Name == groupName) return this._subnetGroups[i].SubnetType;
		}
	}

	this._byCIDR = function(conn1, conn2) {
		let [ip1] = conn1.CIDR.split('/')
		let [ip2] = conn2.CIDR.split('/')
		let [a1, b1, c1, d1] = ip1.split('.').map(x => +x)
		let [a2, b2, c2, d2] = ip2.split('.').map(x => +x)
		return a1 * (1 << 24) + b1 * (1 << 16) + c1 * (1 << 8) + d1 - (a2 * (1 << 24) + b2 * (1 << 16) + c2 * (1 << 8) + d2);
	}

	this._byInsideNetwork = function(nat1, nat2) {
		let [ip1] = nat1.InsideNetwork.split('/')
		let [ip2] = nat2.InsideNetwork.split('/')
		let [a1, b1, c1, d1] = ip1.split('.').map(x => +x)
		let [a2, b2, c2, d2] = ip2.split('.').map(x => +x)
		return a1 * (1 << 24) + b1 * (1 << 16) + c1 * (1 << 8) + d1 - (a2 * (1 << 24) + b2 * (1 << 16) + c2 * (1 << 8) + d2);
	}

	this._mtgasByTGID = function(mtgas, info) {
		return mtgas
			.filter(mtga => (info.Subnets || []).some(subnet => (subnet.ConnectedManagedTransitGatewayAttachments || []).indexOf(mtga.ID) != -1))
			.map(mtga => {
				 	const mtgaByTGID = {
						 Names: [mtga.Name], 
						 IDs:[mtga.ID],
						 TransitGatewayID: mtga.TransitGatewayID, 
					 };
					return mtgaByTGID;
				})
			.reduce((prev, curr) => {
					const match = prev.find(tga => tga.TransitGatewayID == curr.TransitGatewayID); 					
					if (match) {
						match.Names = [...match.Names, curr.Names];
						match.IDs = [...match.IDs, ...curr.IDs];
					} else {
						prev = [...prev, curr];
					}
					return prev
				}, [])
			.map(mtga => {
					mtga.Name = mtga.Names.join("/");
					return mtga;
				});
	}

	let firstRender = true;
	this._renderVPCInfo = function(info, mtgas, sgss, mrrs, region, serverPrefix, accountID, vpcID) {
		const allMTGAs = mtgas.filter(mtga => mtga.Region == region);
		const allMRRs = mrrs.filter(mrr => mrr.IsGovCloud == info.IsGovCloud).filter(mrr => mrr.AccountID != 0);
		const allSGSs = sgss.filter(sgs => sgs.Region == region) || [];
		const mtgasByTGID = this._mtgasByTGID(mtgas, info);
		const availabilityZoneSelector = this._availabilityZones.querySelector('#availabilityZoneSelector');
		info.Tasks = info.Tasks || [];
		info.Subnets = info.Subnets || [];
		info.SubnetGroups = (info.SubnetGroups || []).sort((g1, g2) => {
			if (g1.Name == g2.Name) return (g1.SubnetType < g2.SubnetType ? -1 : 1)
			return g1.Name < g2.Name ? -1 : 1;
		});
		this._subnetGroups = info.SubnetGroups;
		info.Subnets.sort((s1, s2) => {
			if (s1.IsManaged != s2.IsManaged) {
				return s1.IsManaged ? -1 : 1;
			}
			if (s1.Type != s2.Type) {
				return s1.Type < s2.Type ? -1 : 1;
			}
			if (s1.Name != s2.Name) {
				return s1.Name < s2.Name ? -1 : 1;
			}
			return s1.SubnetID < s2.SubnetID ? -1 : 1;
		});
		const view = this;
		if (firstRender) {
			this._setBreadcrumbs(info);
			this._updateZonedSubnetGroupName();
			firstRender = false;
			this._peeringConnections = [];
			render(
				html`
					${info.IsLegacy
						? nothing
						: html`
							<div class="section-header-secondary">Internet Connections</div>
							<div class="section-body-bordered">
								<input ?checked="${info.Config.ConnectPublic}" type="checkbox" name="connectPublic" id="connectPublic" value="1" class="ds-c-choice ds-c-choice--small disableIfMigrating" ?disabled="${!User.isAdmin()}">
								<label for="connectPublic" class="ds-c-label">Public Subnets</label>

								<input ?checked="${info.Config.ConnectPrivate}" type="checkbox" name="connectPrivate" id="connectPrivate" value="1" class="ds-c-choice ds-c-choice--small disableIfMigrating" ?disabled="${!User.isAdmin()}">
								<label for="connectPrivate" class="ds-c-label">Private Subnets</label>
							</div>
					`}
					<div class="section-header-secondary">Transit Gateways</div>
					<div class="section-body-bordered">
					${allMTGAs.map(mtga => html`
						<input type="checkbox" id="mtga${mtga.ID}" name="mtga" value="${mtga.ID}" ?checked="${info.Config.ManagedTransitGatewayAttachmentIDs.indexOf(mtga.ID) != -1}" class="ds-c-choice ds-c-choice--small disableIfMigrating" ?disabled="${!User.isAdmin()}">
						<label for="mtga${mtga.ID}" class="ds-c-label">${mtga.Name}</label>
					`)}
					</div>


					<div class="section-header-secondary">Peering Connections</div>
					<div class="section-body-bordered">
						<div id="peeringConnections"></div>
					</div>

				<input type="submit" value="Update" class="ds-c-button ds-c-button--primary disableIfMigrating" ?disabled="${!User.isAdmin()}">
				`,
				this._updateNetworking
			)
			if (!info.IsLegacy) {
				render(
					html`
					${Object.entries(info.AvailabilityZones).map(([name, id]) => html`
						<option value="${name}">${id} (${name})</option>
					`)}
					`,
					availabilityZoneSelector
				);
				render(
					html`
					${allSGSs.map(sgs => html`
						<input type="checkbox" id="sgs${sgs.ID}" name="sgs" value="${sgs.ID}" ?checked="${(info.Config.SecurityGroupSetIDs || []).indexOf(sgs.ID) != -1}" class="ds-c-choice ds-c-choice--small disableIfMigrating" ?disabled="${!User.isAdmin()}">
						<label for="sgs${sgs.ID}" class="ds-c-label">${sgs.Name}</label>
					`)}
					<div style="margin-top: 10px"><input type="submit" value="Update" class="ds-c-button ds-c-button--primary disableIfMigrating" ?disabled="${!User.isAdmin()}"></div>`,
					this._updateSecurityGroups
				);
			}
			render(
				html`
				${allMRRs.map(mrr => html`
					<input id="rr${mrr.ID}" type="checkbox" name="mrr" value="${mrr.ID}" ?checked="${(info.Config.ManagedResolverRuleSetIDs || []).indexOf(mrr.ID) != -1}" class="ds-c-choice ds-c-choice--small disableIfMigrating" ?disabled="${!User.isAdmin()}">
					<label for="rr${mrr.ID}" class="ds-c-label">${mrr.Name}</label>
				`)}
				<div style="margin-top: 10px"><input type="submit" value="Update" class="ds-c-button ds-c-button--primary disableIfMigrating" ?disabled="${!User.isAdmin()}"></div>`,
				this._updateResolverRules
			);
			this._peeringConnectionsContainer = document.getElementById('peeringConnections');
		}
		(info.Config.PeeringConnections || []).forEach(pc => {
			let alreadyAdded = false;
			for (var pc2 of this._peeringConnections) {
				if (pc.OtherVPCID == pc2.OtherVPCID && pc.OtherVPCRegion == pc2.OtherVPCRegion && pc.IsRequester == pc2.IsRequester) {
					alreadyAdded = true;
					break;
				}
			}
			if (!alreadyAdded) {
				this._peeringConnections.push(pc);
				pc.Keep = true;
			}
		})

		render(
			html`
				${info.IsLegacy
					? html`
					<div class="ds-c-alert ds-c-alert--hide-icon">
						<div class="ds-c-alert__body">
								<p>Network Firewall is not supported for Legacy VPCs</p>
						</div>
					</div>	
					`
					: html`
					<div class="ds-l-form-row">
						<div class="ds-l-col--auto">
							<div style="margin-top: 10px">
								<button type="button" value="add" @click="${(e) => this._sendFirewallRequest(e)}" class="ds-c-button ds-c-button--primary" ?disabled="${ !this.canAddFirewall(info) }">Add Network Firewall</button>
							</div>
						</div>
						<div class="ds-l-col--auto">
							<div style="margin-top: 10px">
								<button type="button" value="remove" @click="${(e) => this._sendFirewallRequest(e)}" class="ds-c-button ds-c-button--primary" ?disabled="${ !this.canRemoveFirewall(info) }">Remove Network Firewall</button>
							</div>
						</div>
					</div>
					${info.CustomPublicRoutes !== "" 
						? html`
							<div class="ds-c-alert ds-c-alert--warn" style="margin-top: 10px">
								<div class="ds-c-alert__body">
									<h2 class="ds-c-alert__heading">Not Eligible</h2>
									<p class="ds-c-alert__text">This VPC is not eligible for adding/removing Network Firewall. The following routes on public route tables are not managed by VPC Conf and would be destroyed in the process. These routes must be manually removed before adding/removing the firewall and re-added after the process is complete.</p>
									<p class="ds-c-alert__text" style="white-space: pre; margin-top: 10px">${info.CustomPublicRoutes}</p>
								</div>
							</div>
						`
						: nothing
					}
					`
				}
			`,
			this._networkFirewall
		)

		// Show "update networking" and "add zoned subnets" form.
		this._updateNetworking.className = '';
		this._updateSecurityGroups.className = '';
		this._addZonedSubnets.className = '';
		this._updateResolverRules.className = '';

		if (info.IsLegacy) {
			this._updateSecurityGroups.innerText = this._addZonedSubnets.innerText = 'Not available for Legacy VPCs';
		}

		let numberPrivatePublicSubnets = 0;

		info.Subnets.forEach(subnet => {
			subnet.Issues = info.Issues.filter(i => (i.AffectedSubnetIDs || []).indexOf(subnet.SubnetID) != -1);
			subnet.HasFixableIssues = subnet.Issues.filter(i => i.IsFixable).length > 0;
			subnet.HasUnfixableIssues = subnet.Issues.filter(i => !i.IsFixable).length > 0;
			if (subnet.Type === "Public" || subnet.Type == "Private") numberPrivatePublicSubnets++;
		})

		this._numberOfAZs = info.Subnets.length / info.SubnetGroups.length;

		render(
			html`Primary CIDR: ${info.PrimaryCIDR} <a class="copyRedactedCIDRsIcon" @click="${() => this._copyRedactedCIDRsToClipboard([info.PrimaryCIDR])}">⎘</a>`,
			this._primaryCIDR
		);
		
		render(
			html`
			${info.SecondaryCIDRs.length
				? html`Secondary CIDR${info.SecondaryCIDRs.length > 1 ? "s" : ""}: ${info.SecondaryCIDRs.join(', ')} <a class="copyRedactedCIDRsIcon" @click="${() => this._copyRedactedCIDRsToClipboard(info.SecondaryCIDRs)}">⎘</a>`
				: nothing
			}
			`,
			this._secondaryCIDRs
		);

		render(VPCType.getBadge(info.VPCType), this._vpcType);

		render(
			html`
				<thead>
					<tr style="background-color: #112E51;text-align: left">
						<th>Subnet</th>
						<th>Use</th>
						${info.IsLegacy
						? nothing
						: html`
						<th>Connected to Internet</th>
						${mtgasByTGID.map(mtga => html`<th title="Transit Gateway">${mtga.Name}</th>`)}
						${info.CMSNetSupported
						? html`<th>CMSNet connections</th><th>CMSNet NATs</th>`
						: nothing}
						`}
						<th>Issues</th>
					</tr>
				</thead>
				<tbody>
				${info.Subnets.map((subnet) => html`
					<tr>
						<td nowrap class="${subnet.HasUnfixableIssues ? 'unfixable' : (subnet.HasFixableIssues ? 'fixable' : '')}">
							${subnet.Name}<br/>
							${subnet.SubnetID}<br/>
							${subnet.CIDR}<br/>
						</td>
						${subnet.IsManaged
							? html`
								<td>${subnet.Type}</td>
								${info.IsLegacy
								? ''
								: html`
									<td>${subnet.IsConnectedToInternet ? "Yes" : "No"}</td>
									${mtgasByTGID.map(mtga=>html`
										<td>
											${mtga.IDs.some(id => (subnet.ConnectedManagedTransitGatewayAttachments || []).indexOf(id) == -1) ? "No" : "Yes"}
										</td>`)}
									${info.CMSNetSupported
									? html`
										<td>
											${info.CMSNetError
											? html`<div class="cmsnet-error">Error: ${info.CMSNetError}</div>`
											: html`
											<ul class="cmsnetConnections">
												${(subnet.CMSNetConnections || []).sort(this._byCIDR).map((conn) => html`
													<li>
														${conn.CIDR}<br>
														<b>${conn.Status}</b>
														${conn.LastMessage
															? html`
															<span class="tooltip" data-tooltip="${conn.LastMessage}">ⓘ</span>
															`
															: nothing}
														${conn.Status == 'Connected'
															? html`
															<br><button @click="${(e) => {e.preventDefault(); view._disconnectZonedSubnets(subnet.GroupName, conn.CIDR)}}" class="ds-c-button ds-c-button--primary ds-c-button--small disableIfMigrating" ?disabled="${!User.isAdmin()}">Remove</button>
															`
															: nothing}
													</li>
												`)}
											</ul>
											`}
										</td>
										<td>
											${info.CMSNetError
											? ''
											: html`
											<ul class="cmsnetConnections">
												${(subnet.CMSNetNATs || []).sort(this._byInsideNetwork).map((nat) => html`
													<li>
														${nat.InsideNetwork} to ${nat.OutsideNetwork || "[TBD]"}<br>
														<b>${nat.Status}</b>
														${nat.LastMessage
															? html`
															<span class="tooltip" data-tooltip="${nat.LastMessage}">ⓘ</span>
															`
															: nothing}
														${nat.Status == 'Connected'
															? html`
															<br>
															<button @click="${(e) => {e.preventDefault(); view._deleteCMSNetNAT(nat)}}" class="ds-c-button ds-c-button--primary ds-c-button--small disableIfMigrating" ?disabled="${!User.isAdmin()}">Remove</button>
															<button @click="${(e) => {e.preventDefault(); view._deleteCMSNetNAT(nat, true)}}" class="ds-c-button ds-c-button--primary ds-c-button--small disableIfMigrating" ?disabled="${!User.isAdmin()}">Move</button>
															`
															: nothing}
													</li>
												`)}
											</ul>
											`}
										</td>`
									: nothing}
								`}
								<td>
									<ul class="issues">
										${subnet.Issues.map((issue) => html`
											<li>
												${issue.Description}
											</li>
										`)}
									</ul>
								</td>`
							: html`<td colspan="6"></td>`
						}
					</tr>
				`)}
				</tbody>`,
			this._subnets);

		const labelConfig = { "page": "vpc", "Region": region, "VPCID": vpcID, "ServerPrefix": serverPrefix }
		render(
			html`<label-ui .info="${labelConfig}" .fetchJSON="${this._fetchJSON.bind(this)}"></label-ui>`,
			this._labels
		);

		
		render(
			html`
				<fixed-task-list .tasks="${info.Tasks}">
				</fixed-task-list> 
				`,
			this._tasks);

		if (info.IsMoreTasks) {
			this._showOlderTasksButton.classList.remove('ds-c-button--disabled');
			this._oldestTaskID = info.Tasks.map(t => t.ID).reduce((v, id) => Math.min(v, id))
		} else {
			this._showOlderTasksButton.classList.add('ds-c-button--disabled');
		}

		if (info.Issues.length) {
			const fixable = info.Issues.filter(i => i.IsFixable);
			const unfixable = info.Issues.filter(i => !i.IsFixable);
			const isTasksInProgress = info.Tasks.reduce((yes, task) => yes || task.Status == "Queued" || task.Status == "In progress", false);
			render(
				html`
				<ul class="issues">
					${fixable.map((issue) => html`
						<li class="fixable">
							${issue.Description}
						</li>
					`)}
					${unfixable.map((issue) => html`
						<li class="unfixable">
							${issue.Description}
						</li>
					`)}
				</ul>
				<div class="${isTasksInProgress ? "warning" : "hidden"}">There are currently unfinished tasks which may affect these issues</div>
				`,
				this._issuesList)
			this._issues.className = '';
		} else {
			this._issues.className = 'hidden';
		}

		this._subnetGroups = info.SubnetGroups;

		if (!info.IsLegacy) {
			const validConnectGroups = this._validCMSNetConnectGroups(info.SubnetGroups)
			this._addCMSNetNAT.className = validConnectGroups.length ? '' : 'hidden';

			const validRemoveGroups = this._validRemoveSubnetGroups(info)

			const oldRemoveVal = this._removeZonedSubnets.groupName.value;
			render(
				html`
				${validRemoveGroups.map(group => html`
					<option value="${group.Name}">${group.Name}</option>
				`)}
				`,
				this._removeZonedSubnets.groupName
			)
			this._removeZonedSubnets.className = validRemoveGroups.length ? '' : 'hidden';
			if (oldRemoveVal) {
				this._removeZonedSubnets.groupName.value = oldRemoveVal;
			}

			if (!info.CMSNetSupported) {
				this._cmsnetUnsupported.className = '';
				this._cmsnetAPIDown.className = 'hidden';
				this._connectZonedSubnets.className = 'hidden';
				this._addCMSNetNAT.className = 'hidden';
			} else if (info.CMSNetError) {
				this._cmsnetError.innerText = info.CMSNetError;
				this._cmsnetAPIDown.className = '';
				this._connectZonedSubnets.className = 'hidden';
				this._cmsnetUnsupported.className = 'hidden';
				this._addCMSNetNAT.className = 'hidden';
			} else {
				this._cmsnetAPIDown.className = 'hidden';
				this._cmsnetUnsupported.className = 'hidden';
				const oldConnectVal = this._connectZonedSubnets.groupName.value;
				
				render(
					html`
						${validConnectGroups.map(group => html`<option value="${group.Name}">${group.Name}</option>`)}
					`,
					this._connectZonedSubnets.groupName
				)
				this._connectZonedSubnets.className = validConnectGroups.length ? '' : 'hidden';
				if (oldConnectVal) {
					this._connectZonedSubnets.groupName.value = oldConnectVal;
				}
			}
		}
		if (this._hasValidPeeringGroups(info.SubnetGroups)) { this._renderPeeringConnections() };
		this._disableIfMigrating(info.VPCType);
	}

	this._copyRedactedCIDRsToClipboard = (cidrs) => {
		const regex = /^\d{1,3}\.\d{1,3}/g;
		const redacted = cidrs.map((cidr) => { return cidr.replace(regex, 'x.x'); })
		const plural = cidrs.length > 1; 
		const pluralized = plural ? 'VPC CIDRs: ' : 'VPC CIDR: ';
		navigator.clipboard.writeText(pluralized + redacted.join(", "))
		Growl.info(`Redacted CIDR${plural ? "s" : ""} copied to clipboard!`);
	}
}
