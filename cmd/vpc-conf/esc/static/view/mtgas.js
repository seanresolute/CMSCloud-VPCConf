import {html, nothing, render} from '../lit-html/lit-html.js';
import {HasModal, MakesAuthenticatedAJAXRequests, ShowsPrefixListEntries} from './mixins.js';
import {Breadcrumb} from './components/shared/breadcrumb.js'
import {Growl} from './components/shared/growl.js';
import {Shared} from './shared.js'
import {User} from './user.js'
import './components/shared/account-select.js';

export function ManagedTransitGatewayAttachmentsPage(info) {
    this._loginURL = info.ServerPrefix + 'oauth/callback';
    Object.assign(this, HasModal, MakesAuthenticatedAJAXRequests, ShowsPrefixListEntries);
    
    const regionsCommercial = ["us-east-1", "us-west-2"]
    const regionsGovCloud = ["us-gov-west-1"]

    this._edit = function(mtga) {
        this._pls = [];
        this._mtgas.forEach(mtga => mtga.editing = false);
        if (mtga) {
            mtga.editing = true;
            mtga.beforeEdits = JSON.parse(JSON.stringify(mtga));  // make a copy
        }
        if (mtga != null) {
            if (mtga.IsGovCloud) {
                this._loadPLs(regionsGovCloud[0])
            } else {
                this._loadPLs(regionsCommercial[0])
            }
        }
        this._render();
    }

    this._routeInput = function(current) {
        return 
    }

    this._isPrefixListID = function(route) {
        return route.startsWith('pl-');
    }

    this._addRoute = function(mtga) {
        mtga.Routes.push('');
        this._render();
    }

    this._delete = async function(mtga, f) {
        if (!confirm('Are you sure you want to delete "' + mtga.Name + '"?')) return;
        const url = info.ServerPrefix + 'mtgas/' + mtga.ID;
        let response;
        try {
            response = await this._fetchJSON(url, {method: 'DELETE'});
        } catch (err) {
            Growl.error('Error deleting: ' + err);
            return;
        }
        this._mtgas.splice(this._mtgas.indexOf(mtga), 1);
        this._render();
    }

    this._save = async function(mtga, f) {
        const prefixLists = Array.from(f.querySelectorAll("[name=prefixList]")).filter(c => c.checked).map(c => c.value);
        const CIDRS = Array.from(f.querySelectorAll("[name=route]")).map(i => '' + i.value).filter(r => !!r.trim());
        const plIDsInCIDRS = CIDRS.some(cidr => this._isPrefixListID(cidr));
        if (plIDsInCIDRS) {
            alert("Prefix list IDs cannot be entered manually and must be configured by selecting from any existing options.");
            return
        }
        const newMTGA = {
            Name: f.name.value,
            TransitGatewayID: f.transitGatewayID.value,
            Region: f.region.value,
            IsGovCloud: f.isGovCloud.checked,
            Routes: [...new Set([...prefixLists, ...CIDRS])],
            SubnetTypes: Array.from(f.querySelectorAll("[name=subnetType]")).filter(c => c.checked).map(c => c.value),
            IsDefault: f.isDefault.checked,
        }
        Array.from(f.querySelectorAll("button")).forEach(b => b.disabled = true);
        Array.from(f.querySelectorAll("input")).forEach(b => b.disabled = true);
        const url = info.ServerPrefix + 'mtgas/' + (mtga.ID || '')
        let response;
        try {
            response = await this._fetchJSON(url, {method: ('ID' in mtga) ? 'PATCH' : 'POST', body: JSON.stringify(newMTGA)});
        } catch (err) {
            Growl.error('Error saving: ' + err);
            Array.from(f.querySelectorAll("button")).forEach(b => b.disabled = false);
            Array.from(f.querySelectorAll("input")).forEach(b => b.disabled = false);
            return;
        }
        if (!('ID' in mtga)) {
            // Fill in ID from response so editing later will work.
            mtga.ID = response.json;
        }
        Array.from(f.querySelectorAll("button")).forEach(b => b.disabled = false);
        Array.from(f.querySelectorAll("input")).forEach(b => b.disabled = false);
        mtga.editing = false;
        Object.assign(mtga, newMTGA);
        this._render();
    }

    this._new = function() {
        this._mtgas.splice(0, 0, {
            Name: "",
            TransitGatewayID: "",
            Region: "",
            Routes: [''],
            editing: true,
            SubnetTypes: ['Private','App','Data','Web','Transport','Security','Management','Shared','Shared-OC','Transitive'],
        });
        this._loadPLs(regionsCommercial[0])
        this._edit(this._mtgas[0]);
    }

    this._cancel = function(mtga) {
        if (!('ID' in mtga)) {
            // Remove new MTGA created one from list.
            this._mtgas.splice(this._mtgas.indexOf(mtga), 1);
        } else {
            Object.assign(mtga, mtga.beforeEdits);  // revert to previous version
        }
        this._edit(null);
    }

    this._copy = function(target, sourceID) {
        const source = this._mtgas.filter(m => m.ID == sourceID)[0];
        target.Routes = source.Routes;
        target.TransitGatewayID = source.TransitGatewayID;
        target.Region = source.Region;
        this._render();
    }

    this._updateRegion = function(mtga, target) {
        mtga.IsGovCloud = !!target.checked
        if (mtga.IsGovCloud) {
            this._loadPLs(regionsGovCloud[0])
        } else {
            this._loadPLs(regionsCommercial[0])
        }
        this._render();
    }

    this._loadVPCs = async function(accountID, f) {
        this._selectedAccountID = accountID;
        render(
            '',
            f.importFromVPC)
        f.importFromVPC.disabled = true;
        f.importFromTransitGatewayAttachment.selectedIndex = 0;
        f.doImport.selectedIndex = 0;
        f.doImport.disabled = true;
        const url = info.ServerPrefix + 'accounts/' + accountID + '.json';
        let response;
        try {
            response = await this._fetchJSON(url);
        } catch (err) {
            Growl.error('Error fetching VPCs: ' + err);
            return;
        }
        if (this._selectedAccountID != accountID) return;  // was called again while loading
        response.json.Regions.forEach(region => { region.VPCs = region.VPCs || [] });
        this._vpcs = response.json.Regions.map(region => region.VPCs.map(vpc => Object.assign(vpc, {Region: region.Name}))).flat();
        this._vpcs.sort((v1, v2) => {
            if (v1.Name && !v2.Name) return -1;
            if (v2.Name && !v1.Name) return 1;
            if (v1.Name < v2.Name) return -1;
            if (v2.Name < v1.Name) return 1;
            return v1.VPCID < v2.VPCID ? -1 : 1;
        })
        render(
            html`
                <option>--- VPC ---</option>
                ${this._vpcs.map(vpc =>
                    html`
                    <option value="${vpc.VPCID}">${vpc.Name} | ${vpc.VPCID}</option>
                    `
                )}`,
            f.importFromVPC)
        f.importFromVPC.disabled = false;
    }

    this._loadTGAs = async function(vpcIndex, f) {
        const vpc = this._vpcs[vpcIndex];
        const vpcID = vpc.VPCID;
        this._selectedVPCID = vpcID;
        render(
            '',
            f.importFromTransitGatewayAttachment)
        f.importFromTransitGatewayAttachment.disabled = true;
        const region = vpc.Region;
        const url = info.ServerPrefix + region + '/vpc/' + this._selectedAccountID + '/' + vpcID + '/tgas.json';
        let response;
        try {
            response = await this._fetchJSON(url);
        } catch (err) {
            Growl.error('Error fetching Transit Gateway Attachments: ' + err);
            return;
        }
        if (this._selectedVPCID != vpcID) return;  // was called again while loading
        this._tgas = response.json;
        if (this._tgas && this._tgas[0]) this._tgas[0].Region = region;
        render(
            html`
                <option>--- Transit Gateway Attachment ---</option>
                ${this._tgas.map(tga =>
                    html`
                    <option value="${tga.TransitGatewayID}">${tga.Name} | ${tga.TransitGatewayID}</option>
                    `
                )}`,
            f.importFromTransitGatewayAttachment)
        f.importFromTransitGatewayAttachment.disabled = false;
    }

    this._import = function(target, tgaIndex) {
        const tga = this._tgas[tgaIndex];
        target.Routes = tga.Routes;
        target.TransitGatewayID = tga.TransitGatewayID;
        target.Region = tga.Region;
        this._loadPLs(tga.Region);
    }

    this._loadPLs = async function(region) {
        this._pls = null;
        this._render();

        const plsURL = info.ServerPrefix + `${region}/pl.json`
        let response;
        try {
            response = await this._fetchJSON(plsURL);
        } catch (err) {
            Growl.error('Error fetching managed prefix lists: ' + err);
            return;
        }
        this._pls = response.json;

        this._render();
    }

    this._handlePLClick = function(e, name, id, region) {
        e.preventDefault();
        this._showPLEntries(info.ServerPrefix, name, id, region);
    }

    this._render = function() {
        const allSubnetTypes = [
            'Private',
            'Public',
            'App',
            'Data',
            'Web',
            'Transport',
            'Security',
            'Management',
            'Shared',
            'Shared-OC',
            'Transitive',
        ];
        render(
            html`
                ${this._mtgas.some(mtga => mtga.editing) || !User.isAdmin()
                    ? nothing
                    : html`<button @click="${() => this._new()}" class="ds-c-button ds-c-button--primary ds-c-button--small">New Transit Gateway Template</button>`}
                ${this._mtgas.map(mtga => (
                    mtga.editing
                    ? html`
                        <form id="mtgaForm" class="mtga editing" onsubmit="return false" @account-selected="${(e) => this._loadVPCs(e.detail.account.ID, e.currentTarget)}">
                            <div class="section-header ds-u-padding--1">
                                <div class="ds-l-row ds-u-align-items--end">
                                    <div class="ds-l-col--2">
                                        <label class="ds-c-label ds-u-margin--0">
                                            Template Name
                                            <input id="name" name="name" value="${mtga.Name}" class="ds-c-field input-medium">
                                        </label>
                                    </div>
                                    <div class="ds-l-col--6">
                                        <input type="checkbox" id="isGovCloud" name="isGovCloud" class="ds-c-choice ds-c-choice--small" ?disabled="${(mtga.InUseVPCs && mtga.InUseVPCs.length)}" @click="${(e) => this._updateRegion(mtga, e.target.closest('form').isGovCloud)}" ?checked=${mtga.IsGovCloud}>
                                        <label for="isGovCloud" class="ds-c-label">Gov Cloud <img src="/static/images/govcloud.png" style="vertical-align: bottom; height: 20px;margin-left: 6px;"></label>
                                        <input type="checkbox" id="isDefault" name="isDefault" class="ds-c-choice ds-c-choice--small" ?checked=${mtga.IsDefault}>
                                        <label for="isDefault" class="ds-c-label"><span class="tooltip" data-tooltip="Add to all new VPCs in region">Default</span></label>
                                    </div>
                                    <div class="ds-l-col--4">
                                        <span class="ds-u-float--right">
                                            <button type="button" @click="${(e) => this._save(mtga, e.target.closest('form'))}" class="ds-c-button ds-c-button--inverse">Save</button>
                                            <button type="button" @click="${() => this._cancel(mtga)}" class="ds-c-button ds-c-button--inverse">Cancel</button>
                                        </span>
                                    </div>
                                </div>
                            </div>
                            <div class="section-header-secondary">Create New Template</div>
                            <div class="ds-l-row ds-u-padding--1">
                                <div class="ds-l-col--2">
                                <label for="transitGatewayID" class="ds-c-label ds-u-margin--0">Transit Gateway ID</label>
                                    <input id="transitGatewayID" name="transitGatewayID" class="ds-c-field input-medium" value="${mtga.TransitGatewayID}">
                                </div>
                                <div class="ds-l-col--2">
                                    <label for="region" class="ds-c-label ds-u-margin--0">Region</label>
                                    <select id="region" name="region" class="ds-c-field select-medium" ?disabled="${(mtga.InUseVPCs && mtga.InUseVPCs.length)}" @change="${e => this._loadPLs(e.target.closest('form').region.value)}">
                                        ${mtga.IsGovCloud 
                                            ? html`${regionsGovCloud.map(region => html`<option value="${region}" ?selected=${mtga.Region == region}>${region}</option>`)}`
                                            : html`
                                                ${regionsCommercial.map(region => html`<option value="${region}" ?selected=${mtga.Region == region}>${region}</option>`)}
                                            `}
                                    </select>
                                </div>
                            </div>
                            <div class="ds-l-row ds-u-padding--1">
                                <div class="ds-l-col--auto">
                                    <span class="psuedo-ds-c-label">Connected Subnets</span>
                                    ${allSubnetTypes.map(st =>
                                        html`
                                        <input type="checkbox" id="${st}" name="subnetType" value="${st}" ?checked="${mtga.SubnetTypes.indexOf(st) != -1}" class="ds-c-choice ds-c-choice--small">
                                        <label for="${st}" class="ds-c-label">${st}</label>
                                        `
                                    )}
                                </div>
                            </div>

                            <div class="ds-l-row ds-u-padding--1">
                                <div class="ds-l-col--auto">
                                    <span class="psuedo-ds-c-label">Prefix Lists</span>
                                    ${this._pls === null 
                                        ? "Loading..."
                                        : html`${this._pls.map(pl => {
                                                return html`
                                                    <input type="checkbox" id="prefixList${pl.ID}" name="prefixList" value="${pl.ID}" ?checked="${mtga.Routes.indexOf(pl.ID) != -1}" class="ds-c-choice ds-c-choice--small">
                                                    <label for="prefixList${pl.ID}" class="ds-c-label ds-u-margin--0">
                                                        <a class="plLink" href="" @click="${e => this._handlePLClick(e, pl.Name, pl.ID, e.target.closest('form').region.value)}"> ${pl.Name} (${pl.ID})</a>
                                                    </label>
                                                                    
                                                `
                                            })
                                        }`
                                    }
                                </div>
                            </div>

                            <div class="ds-l-row ds-u-padding--1">
                                <div class="ds-l-col--auto">
                                    <label for="route" class="ds-c-label ds-u-margin--0">Routes<label>
                                    ${mtga.Routes.map(route => html`
                                        ${this._isPrefixListID(route) ? nothing : html`<span><input id="route" name="route" value="${route}" placeholder="New route" class="ds-c-field input-medium"></span>`}   
                                    `)}
                                </div>
                                <div class="ds-l-col--auto">
                                    <button class="ds-c-button ds-c-button--primary" style="margin-top: 26px;" @click=${() => this._addRoute(mtga)}>Add Route</button><br>
                                </div>
                            </div>
                            <!-- COPY FROM TEMPLATE -->
                            <div class="section-header-secondary">Copy Template</div>
                            <div class="ds-l-row ds-u-padding--1">
                                <div class="ds-l-col--12">
                                    <select name="copyFrom" class="ds-c-field ds-u-display--inline-block">
                                        ${this._mtgas.filter(other => other.ID != mtga.ID).map(other => html`
                                            <option value="${other.ID}">${other.Name + (other.IsGovCloud ? ' (GovCloud)' : '')}</option>
                                        `)}
                                    </select> <button type="button" @click=${(e) => this._copy(mtga, e.target.closest('form').copyFrom.value)} class="ds-c-button ds-button--primary">Copy</button>
                                </div>
                            </div>
                            <!-- IMPORT FROM AWS RESOURCE -->
                            <div class="section-header-secondary">Import From AWS Resource</div>
                            <div class="ds-l-row ds-u-padding--1">
                                <div class="ds-l-col--12">
                                    <div class="importFromAWS">
                                        <div id="accountSelectContainer">
                                            <account-select .accounts="${this._accounts}"></account-select>
                                        </div>
                                        <select name="importFromVPC" disabled @change="${(e) => {this._loadTGAs(e.target.selectedIndex - 1, e.target.closest('form'))}}" class="ds-c-field">
                                        </select>
                                        <select name="importFromTransitGatewayAttachment" disabled @change="${(e) => e.target.closest('form').doImport.disabled = e.target.selectedIndex == 0}" class="ds-c-field">
                                        </select>
                                        <button name="doImport" disabled type="button" @click=${(e) => this._import(mtga, e.target.closest('form').importFromTransitGatewayAttachment.selectedIndex - 1)} class="ds-c-button ds-c-button--primary">Import</button>
                                    </div>
                                </div>
                            </div>
                        </form>
                    `
                    : html`
                        <div class="ds-u-margin-y--1 ds-u-padding--0">
                            <div class="section-header ds-u-padding--1">
                                ${mtga.Name}
                                ${mtga.IsGovCloud ? html`<img src="/static/images/govcloud.png" style="height: 20px; position:relative; top: 2px">` : nothing}
                                ${mtga.IsDefault ? html`<span class="ds-c-badge badge--info-inverted tooltip" data-tooltip="Add to all new VPCs in region">Default</span>` : nothing}
                                ${this._mtgas.some(mtga => mtga.editing) ? nothing : html`
                                    <span class="ds-u-float--right">
                                        <button @click="${() => this._edit(mtga)}" class="ds-c-button ds-c-button--inverse ds-c-button--small" ?disabled="${!User.isAdmin()}">Edit</button>
                                        <button @click="${() => this._delete(mtga)}" class="ds-c-button ds-c-button--inverse ds-c-button--small" ?disabled="${(mtga.InUseVPCs && mtga.InUseVPCs.length) || !User.isAdmin()}">Delete</button>
                                    </span>
                                `}
                            </div>
                            <div class="section-header-secondary">${mtga.TransitGatewayID} - ${mtga.Region}</div>
                            <div class="mtga-body">
                                <div class="subnet-types">
                                    Connected Subnets: ${mtga.SubnetTypes.join(', ')}
                                </div>
                                <div>
                                    ${mtga.Routes.map(route => html`<div class="alternating-row">${route}</div>`)}
                                </div>
                            </div>
                            <div class="associated-with">
                                Associated with ${
                                    (mtga.InUseVPCs && mtga.InUseVPCs.length)
                                    ? html`
                                    <a href="#" @click="${(e) => {e.preventDefault(); document.getElementById(mtga.ID + "VPCs").classList.toggle('hidden')}}" class="ds-c-link">${mtga.InUseVPCs.length == 1 ? '1 VPC' : mtga.InUseVPCs.length + ' VPCs'}</a>
                                    <div id="${mtga.ID}VPCs" class="hidden">
                                        ${mtga.InUseVPCs.map(accountVPC => html`
                                        <div class="alternating-row">
                                            <a href="${info.ServerPrefix + Shared.InUseVPCToURL(accountVPC)}" class="ds-c-link">${accountVPC.replace(/\//g, ' / ')}</a>
                                        </div>
                                        `)}
                                    </div>
                                    `
                                    : '0 VPCs'}
                            </div>
                        </div>
                    `
                ))}
            `,
            this._mtgasContainer);
    }

    this.init = async function(container) {
        Breadcrumb.set([{name: "Transit Gateways"}]);
        render(
            html`
                <div id="background" class="hidden"></div>
                <div id="modal" class="hidden"></div>
                <div class="ds-l-container ds-u-margin-y--1">
                    <div id="container">
                        <div id="mtgas">
                        </div>
                    </div>
                </div>`,
            container)
        this._mtgasContainer = document.getElementById('mtgas');
        this._modal = document.getElementById('modal');
        this._background = document.getElementById('background');
        const accountsURL = info.ServerPrefix + 'accounts/accounts.json';
        const mtgasURL = info.ServerPrefix + 'mtgas.json';
        let responses;
        try {
            responses = await Promise.all([this._fetchJSON(accountsURL), this._fetchJSON(mtgasURL)]);
        } catch (err) {
            Growl.error('Error fetching managed transit gateway attachments: ' + err);
            return;
        }
        this._accounts = responses[0].json || [];
        this._mtgas = responses[1].json;
        this._mtgas.sort((m1, m2) => {
            if (m1.Name == m2.Name) return (m1.ID < m2.ID ? -1 : 1)
            return m1.Name < m2.Name ? -1 : 1; 
        })

        this._render();
    }
}
