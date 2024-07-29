import {html, nothing, render} from '../lit-html/lit-html.js';
import {Breadcrumb} from './components/shared/breadcrumb.js'
import {Growl} from './components/shared/growl.js';
import {Shared} from './shared.js'
import {User} from './user.js'
import {HasModal, MakesAuthenticatedAJAXRequests, ShowsPrefixListEntries } from './mixins.js';

export function SecurityGroupSetsPage(info) {
    this._loginURL = info.ServerPrefix + 'oauth/callback';
    Object.assign(this, HasModal, MakesAuthenticatedAJAXRequests, ShowsPrefixListEntries);

    const regionsCommercial = ["us-east-1", "us-west-2"]
    const regionsGovCloud = ["us-gov-west-1"]
    
    this._edit = function(sgs) {
        this._sgss.forEach(sgs => sgs.editing = false);
        if (sgs) {
            sgs.editing = true;
            this._handleRegionLockOnPL(sgs)
            sgs.beforeEdits = JSON.parse(JSON.stringify(sgs));  // make a copy
        }
        this._render();
        if (sgs) document.querySelector('.sgs.editing').name.focus();
    }

    this._delete = async function(sgs, f) {
        if (!confirm('Are you sure you want to delete "' + sgs.Name + '"?')) return;
        const url = info.ServerPrefix + 'sgs/' + sgs.ID;
        let response;
        try {
            response = await this._fetchJSON(url, {method: 'DELETE'});
        } catch (err) {
            Growl.error('Error deleting: ' + err);
            return;
        }
        this._sgss.splice(this._sgss.indexOf(sgs), 1);
        this._render();
    }

    this._save = async function(sgs, f) {
        sgs.Name = f.name.value;
        sgs.IsDefault = f.isDefault.checked;
        sgs.IsGovCloud = f.isGovCloud.checked;
        sgs.Region = f.region.value;
        // Must loop before filtering so indexes line up
        sgs.Groups.forEach((group, gIdx) => {
            if (group._deleted) return;
            group.Name = f['group' + gIdx + 'Name'].value;
            group.Description = f['group' + gIdx + 'Description'].value;
            group.Rules = group.Rules || [];
            // Must loop before filtering so indexes line up
            group.Rules.forEach((rule, rIdx) => {
                if (rule._deleted) return;
                rule.Description = f['group' + gIdx + 'Rule' + rIdx + 'Description'].value;
                rule.IsEgress = f['group' + gIdx + 'Rule' + rIdx + 'IsEgress'].value == "1";
                rule.Protocol = f['group' + gIdx + 'Rule' + rIdx + 'Protocol'].value;
                rule.FromPort = +f['group' + gIdx + 'Rule' + rIdx + 'FromPort'].value;
                rule.ToPort = +f['group' + gIdx + 'Rule' + rIdx + 'ToPort'].value;
                rule.Source = f['group' + gIdx + 'Rule' + rIdx + 'Source'].value;
            });
            group.Rules = group.Rules.filter(r => !r._deleted);
        })
        sgs.Groups = sgs.Groups.filter(g => !g._deleted);
        Array.from(f.querySelectorAll("button")).forEach(b => b.disabled = true);
        Array.from(f.querySelectorAll("select")).forEach(b => b.disabled = true);
        Array.from(f.querySelectorAll("input")).forEach(b => b.disabled = true);
        const url = info.ServerPrefix + 'sgs/' + (sgs.ID || '')
        let response;
        try {
            response = await this._fetchJSON(url, {method: ('ID' in sgs) ? 'PATCH' : 'POST', body: JSON.stringify(sgs)});
        } catch (err) {
            Growl.error('Error saving: ' + err);
            Array.from(f.querySelectorAll("button")).forEach(b => b.disabled = false);
            Array.from(f.querySelectorAll("input")).forEach(b => b.disabled = false);
            return;
        }
        // Fill in any IDs from response.
        Object.assign(sgs, response.json);
        Array.from(f.querySelectorAll("button")).forEach(b => b.disabled = false);
        Array.from(f.querySelectorAll("select")).forEach(b => b.disabled = false);
        Array.from(f.querySelectorAll("input")).forEach(b => b.disabled = false);
        sgs.editing = false;
        this._render();
    }

    this._new = function() {
        this._sgss.splice(0, 0, {
            Name: "",
            Groups: [],
            editing: true,
        });
        this._edit(this._sgss[0]);
    }

    this._updateRegion = function(sgs, target) {
        sgs.IsGovCloud = !!target.checked
        if (sgs.IsGovCloud) {
            this._loadPLs(regionsGovCloud[0])
        } else {
            this._loadPLs(regionsCommercial[0])
        }
        this._render();
    }

    this._addGroup = function(sgs) {
        sgs.Groups = sgs.Groups || [];
        sgs.Groups.push({
            Name: "",
            Description: "",
            Rules: [],
        });
        this._render();
        document.querySelector('.sgs.editing')['group' + (sgs.Groups.length - 1) + 'Name'].focus();
    }

    this._deleteGroup = function(group) {
        group._deleted = true;
        this._render();
    }

    this._addRule = function(group) {
        group.Rules = group.Rules || [];
        group.Rules.push({
            Description: '',
            IsEgress: false,
            Protocol: '',
            FromPort: '',
            ToPort: '',
            Source: '',
        });
        this._render();
    }

    this._deleteRule = function(rule) {
        rule._deleted = true;
        this._render();
    }

    this._cancel = function(sgs) {
        if (!('ID' in sgs)) {
            this._sgss.splice(this._sgss.indexOf(sgs), 1);
        } else {
            Object.assign(sgs, sgs.beforeEdits);  // revert to previous version
        }
        this._edit(null);
    }

    this._applyShortcut = function(select, rule) {
        if (!select.value) return;
        const data = JSON.parse(select.value);
        Object.assign(rule, data);
        this._render();
        select.selectedIndex = 0;
    }

    this._showSelectPL = function(selectRule, region) {
        const currentSgs = this._sgss.filter(sgs => sgs.editing)[0];
        currentSgs.Groups.forEach(group => {
            (group.Rules || []).forEach(unselectRule => unselectRule._selectingPL = false);
        });
        selectRule._selectingPL = true;
        this._loadPLs(region);
        this._render();
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

    this._handleSelectPL = function(sgs, rule, id, f, gIdx, rIdx) {
        f.querySelector(`[name=group${gIdx}Rule${rIdx}Source`).value = id;
        sgs.RegionLocked = true
        rule._selectingPL = false;
        this._render();
    }

    this._handleCancelPL = function(rule) {
        rule._selectingPL = false;
        this._render();
    }

    this._handlePLClick = function(e, name, id, region) {
        e.preventDefault();
        this._showPLEntries(info.ServerPrefix, name, id, region);
    }

    this._handleRegionLockOnPL = function(sgs) {
        sgs.RegionLocked = false
        sgs.Groups.forEach(group => {
            group.Rules?.forEach(rule => {
                if (rule.Source.includes('pl-')) {
                    sgs.RegionLocked = true;
                }
            })
        })
    }

    this._render = function() {

        render(
            html`
                ${this._sgss.some(sgs => sgs.editing) || !User.isAdmin()
                    ? nothing
                    : html`<button type="button" @click="${() => this._new()}" class="ds-c-button ds-c-button--primary ds-c-button--small">New Security Group Set</button>`}
                ${this._sgss.map(sgs => (
                    sgs.editing
                    ? html`
                        <form class="sgs editing" onsubmit="return false">
                        <div class="section-header ds-u-padding--1">
                            <div class="ds-l-row ds-u-align-items--end">
                                <div class="ds-l-col--4">
                                    <label class="ds-c-label ds-u-margin--0">
                                        Set Name
                                        <input id="name" name="name" value="${sgs.Name}" placeholder="Name" class="ds-c-field ds-u-display--inline-block">
                                    </label>
                                </div>
                                <div class="ds-l-col--4">
                                    <input type="checkbox" id="isGovCloud" name="isGovCloud" class="ds-c-choice ds-c-choice--small" ?disabled="${((sgs.InUseVPCs && sgs.InUseVPCs.length) || sgs.RegionLocked)}" @click="${(e) => this._updateRegion(sgs, e.target.closest('form').isGovCloud)}" ?checked=${sgs.IsGovCloud}>
                                    <label for="isGovCloud" class="ds-c-label">Gov Cloud <img src="/static/images/govcloud.png" style="vertical-align: bottom; height: 20px;margin-left: 6px;"></label>
                                    <input type="checkbox" id="isDefault" name="isDefault" class="ds-c-choice ds-c-choice--small" ?checked=${sgs.IsDefault}>
                                    <label for="isDefault" class="ds-c-label"><span class="tooltip" data-tooltip="Add to all new VPCs">Default</span></label>
                                </div>
                                <div class="ds-l-col--2">
                                    <label for="region" class="ds-c-label ds-u-margin--0">Region</label>
                                    <select id="region" name="region" class="ds-c-field select-medium" ?disabled="${((sgs.InUseVPCs && sgs.InUseVPCs.length) || sgs.RegionLocked)}" @change="${e => this._loadPLs(e.target.closest('form').region.value)}">
                                        ${sgs.IsGovCloud 
                                            ? html`${regionsGovCloud.map(region => html`<option value="${region}" ?selected=${sgs.Region == region}>${region}</option>`)}`
                                            : html`
                                                ${regionsCommercial.map(region => html`<option value="${region}" ?selected=${sgs.Region == region}>${region}</option>`)}
                                            `}
                                    </select>
                                </div>
                                <div class="ds-l-col--4">
                                    <span class="ds-u-float--right">
                                        <button type="button" @click="${(e) => this._save(sgs, e.target.closest('form'))}" class="ds-c-button ds-c-button--inverse">Save</button>
                                        <button type="button" @click="${() => this._cancel(sgs)}" class="ds-c-button ds-c-button--inverse">Cancel</button>
                                    </span>
                                </div>
                            </div>
                        </div>
                        ${(sgs.InUseVPCs && sgs.InUseVPCs.length)
                            ? html`
                                <div class="edit-warning">
                                    <b>Warning:</b> This security group set is in use. Groups may already be attached to network interfaces. Avoid deleting groups or changing their meaning.
                                </div>`
                            : nothing}
                        ${(sgs.Groups || []).map((group, gIdx) => group._deleted ? '' : html`
                        <div class="ds-l-container ds-u-padding-x--2 ds-u-padding-y--0">
                            <div class="ds-l-row section-header-secondary ds-u-align-items--end">
                                <div class="ds-l-col--2">
                                    <label class="ds-c-label ds-u-margin--0">
                                        Group Template Name
                                        <input name="group${gIdx}Name" value="${group.Name}" class="ds-c-field input-medium">
                                    </label>
                                </div>
                                <div class="ds-l-col--4">                                
                                    <label class="ds-c-label ds-u-margin--0">
                                        Group Template Description
                                        <input name="group${gIdx}Description" value="${group.Description}" class="ds-c-field">
                                    </label>
                                </div>
                                <div class="ds-l-col--6">
                                    <button type="button" @click="${() => this._deleteGroup(group)}" class="ds-c-button ds-c-button--inverse ds-u-margin-y--1 ds-u-float--right">Delete Group Template From Set</button>
                                </div>
                            </div>

                            ${(group.Rules || []).map((rule, rIdx) => rule._deleted ? nothing : html`
                                <div class="ds-l-row alternating-row">
                                    <div class="ds-l-col--2">
                                        <label class="ds-c-label ds-u-margin--0">
                                            Rule Description
                                            <input name="group${gIdx}Rule${rIdx}Description" value="${rule.Description}" class="ds-c-field input-medium">
                                        </label>
                                    </div>
                                    <div class="ds-l-col--1">
                                        <label class="ds-c-label ds-u-margin--0">
                                            Direction
                                            <select name="group${gIdx}Rule${rIdx}IsEgress" class="ds-c-field select-small">
                                                <option value="0" ?selected=${!rule.IsEgress}>ingress</option>
                                                <option value="1" ?selected=${rule.IsEgress}>egress</option>
                                            </select>
                                        </label>
                                    </div>
                                    <div class="ds-l-col--1">
                                        <label class="ds-c-label ds-u-margin--0">
                                            Shortcuts
                                            <select @change="${(e) => this._applyShortcut(e.target, rule)}" class="ds-c-field select-medium">
                                                <option>- Select -</option>
                                                <option value='{"Protocol":"-1","FromPort":0,"ToPort":0}'>All Traffic</option>
                                                <option value='{"Protocol":"tcp","FromPort":0,"ToPort":65535}'>All TCP</option>
                                                <option value='{"Protocol":"udp","FromPort":0,"ToPort":65535}'>All UDP</option>
                                                <option value='{"Protocol":"icmp","FromPort":-1,"ToPort":-1}'>All ICMP</option>
                                            </select>
                                        </label>
                                    </div>
                                    <div class="ds-l-col--1">
                                        <label for="group${gIdx}Rule${rIdx}Protocol" class="ds-c-label ds-u-margin--0">Protocol</label>
                                        <input name="group${gIdx}Rule${rIdx}Protocol" value="${rule.Protocol}" class="ds-c-field input-small">
                                    </div>
                                    <div class="ds-l-col--2">
                                        <label for="group${gIdx}Rule${rIdx}FromPort" class="ds-c-label ds-u-margin--0">Port Range</label>
                                        <input name="group${gIdx}Rule${rIdx}FromPort" value="${rule.FromPort}" class="ds-c-field input-small ds-u-display--inline-block"> - 
                                        <input name="group${gIdx}Rule${rIdx}ToPort" value="${rule.ToPort}" class="ds-c-field input-small ds-u-display--inline-block">
                                    </div>
                                    <div class="ds-l-col--5">
                                        <label for="group${gIdx}Rule${rIdx}Source" class="ds-c-label ds-u-margin--0">Source</label>
                                        <input name="group${gIdx}Rule${rIdx}Source" value="${rule.Source}" class="ds-c-field input-medium ds-u-display--inline-block">
                                        <button type="button" @click="${() => this._showSelectPL(rule, document.getElementById("region").value)}" class="ds-c-button ds-c-button--primary ds-u-display--inline-block">Select Prefix List</button>
                                        <button type="button" @click="${() => this._deleteRule(rule)}" class="ds-c-button ds-c-button--primary ds-u-display--inline-block">Delete Rule</button>
                                    </div>
                                </div>
                                ${rule._selectingPL
                                    ? html`
                                    <div class="ds-u-margin--2"> 
                                        <div class="ds-u-margin-y--2">
                                        ${this._pls === null 
                                            ? "Loading..."
                                            : html`${this._pls.map(pl => {
                                                    return html`
                                                    <input id="prefixList${pl.ID}" type="radio" name="prefixList" value="${pl.ID}" class="ds-c-choice ds-c-choice--small">
                                                    <label for="prefixList${pl.ID}">
                                                        <a class="ds-c-link" href="" @click="${e => this._handlePLClick(e, pl.Name, pl.ID, e.target.closest('form').plRegion.value)}"> ${pl.Name} (${pl.ID})</a>
                                                    </label>
                                                    `
                                                })
                                            }`
                                        }
                                        </div>
                                        <button type="button" @click="${(e) => this._handleSelectPL(sgs, rule, e.target.closest('form').prefixList.value, e.target.closest('form'), gIdx, rIdx)}" class="ds-c-button ds-c-button--primary">Select</button>
                                        <button type="button" @click="${(e) => this._handleCancelPL(rule)}" class="ds-c-button ds-c-button--primary">Cancel</button>
                                    </div>
                                    `
                                    : nothing
                                }
                            `)}
                            <button type="button" @click="${() => this._addRule(group)}" class="ds-c-button ds-c-button--primary ds-u-margin-y--2">Add Rule</button>
                        </div>
                        `)}
                        <button type="button" @click="${() => this._addGroup(sgs)}" class="ds-c-button ds-c-button--primary ds-u-margin--2">Add Security Group Template</button>
                        </form>
                    `
                    : html`
                        <div class="ds-u-margin-y--1 ds-u-padding--0">
                            <div class="section-header ds-u-padding--1">
                                ${sgs.Name}
                                ${sgs.IsDefault ? html`<span class="ds-c-badge badge--info-inverted tooltip" data-tooltip="Add to all new VPCs">Default</span>` : nothing}
                                ${"Region: " + sgs.Region}
                                ${this._sgss.some(sgs => sgs.editing) ? nothing : html`
                                    <span class="ds-u-float--right">
                                        <button type="button" @click="${() => this._edit(sgs)}" class="ds-c-button ds-c-button--inverse ds-c-button--small" ?disabled="${!User.isAdmin()}">Edit</button>
                                        <button @click="${() => this._delete(sgs)}" class="ds-c-button ds-c-button--inverse ds-c-button--small" ?disabled="${(sgs.InUseVPCs && sgs.InUseVPCs.length) || !User.isAdmin()}">Delete</button>
                                    </span>
                                `}
                            </div>
                            ${(sgs.Groups || []).map(group => html`
                            <div>
                                <div class="section-header-secondary ds-u-padding--1">${group.Name} - ${group.Description}</div>
                                <table class="standard-table">
                                    <thead>
                                        <tr>
                                            <th>Rule</th>
                                            <th>Direction</th>
                                            <th>Protocol</th>
                                            <th>Port range</th>
                                            <th>Source</th>
                                        </tr>
                                    </thead>
                                    <tbody>
                                    ${(group.Rules || []).map(rule => html`
                                        <tr>
                                            <td>${rule.Description}</td>
                                            <td>${rule.IsEgress ? 'egress' : 'ingress'}</td>
                                            <td>${rule.Protocol}</td>
                                            <td>${rule.FromPort == rule.ToPort ? rule.FromPort : html`${rule.FromPort}–${rule.ToPort}`}</td>
                                            <td>${rule.Source}</td>
                                        </tr>
                                    `)}
                                    </tbody>
                                </table>
                            </div>
                            `)}
                        <div class="associated-with ds-u-padding--1" style="font-size: 18px;">
                            Associated with ${
                                (sgs.InUseVPCs && sgs.InUseVPCs.length)
                                ? html`
                                    <a href="#" @click="${(e) => {e.preventDefault(); document.getElementById(sgs.ID + "VPCs").classList.toggle('hidden')}}">${sgs.InUseVPCs.length == 1 ? '1 VPC' : sgs.InUseVPCs.length + ' VPCs'}</a>
                                    <div id="${sgs.ID}VPCs" class="hidden">
                                        ${sgs.InUseVPCs.map(accountVPC => html`
                                        <div class="alternating-row">
                                            <a href="${info.ServerPrefix + Shared.InUseVPCToURL(accountVPC)}">${accountVPC.replace(/\//g, ' / ')}</a>
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
            this._sgssContainer);
    }

    this.init = async function(container) {
       Breadcrumb.set([{name: "Security Groups"}]);
        render(
            html`
                <div id="background" class="hidden"></div>
                <div id="modal" class="hidden"></div>
                <div class="ds-l-container ds-u-margin-y--1">
                    <div id="container">
                        <div id="sgs">
                        </div>
                    </div>
                </div>`,
            container)
        this._sgssContainer = document.getElementById('sgs');
        this._modal = document.getElementById('modal');
        this._background = document.getElementById('background');
        
        // const accountsURL = info.ServerPrefix + 'accounts.json';
        const sgsURL = info.ServerPrefix + 'sgs.json';
        let responses;
        try {
            responses = await Promise.all([this._fetchJSON(sgsURL)/*, this._fetchJSON(accountsURL)*/]);
        } catch (err) {
            Growl.error('Error fetching security group sets: ' + err);
            return;
        }
        this._sgss = responses[0].json || [];
        this._sgss.sort((a1, a2) => {
            if (a1.Name == a2.Name) return (a1.ID < a2.ID ? -1 : 1)
            return a1.Name < a2.Name ? -1 : 1;
        });
        this._render();
    }
}
