import {html, nothing, render} from '../lit-html/lit-html.js';
import {HasModal, MakesAuthenticatedAJAXRequests} from './mixins.js';
import {Breadcrumb} from './components/shared/breadcrumb.js'
import {Growl} from './components/shared/growl.js';
import {Shared} from './shared.js'
import {User} from './user.js'
import './components/shared/account-select.js';

export function ManagedResolverRulesPage(info) {
    this._loginURL = info.ServerPrefix + 'oauth/callback';
    Object.assign(this, HasModal, MakesAuthenticatedAJAXRequests);
    const rsToBeCreated = "to be created when needed";
    
    this._loadAccountInfo = async function() {
        const accountsURL = info.ServerPrefix + 'accounts/accounts.json';
        let responses;
        try {
            responses = await Promise.all([this._fetchJSON(accountsURL)]);
        } catch (err) {
            Growl.error('Error fetching account info: ' + err);
            return;
        }
        this._accounts = responses[0].json || [];
    }

    this._loadRulesInfo = async function() {
        const rrsURL = info.ServerPrefix + 'mrrs.json';
        let responses;
        try {
            responses = await Promise.all([this._fetchJSON(rrsURL)]);
        } catch (err) {
            Growl.error('Error fetching resolver rulesets: ' + err);
            return;
        }
        this._rrss = responses[0].json || [];
        this._rrss.sort((a1, a2) => {
            if (a1.Name == a2.Name) return (a1.ID < a2.ID ? -1 : 1)
            return a1.Name < a2.Name ? -1 : 1;
        });

        this._render();
    }

    this._edit = function(rrs) {
        this._rrss.forEach(rrs => rrs.editing = false);
        if (rrs) {
            rrs.editing = true;
            rrs.beforeEdits = JSON.parse(JSON.stringify(rrs));  // make a copy
        }
        this._render();
        if (rrs) document.querySelector('.rrs.editing').name.focus();
    }

    this._delete = async function(rrs, f) {
        if (!confirm('Are you sure you want to delete "' + rrs.Name + '"?')) return;
        const url = info.ServerPrefix + 'mrrs/' + rrs.ID;
        let response;
        try {
            response = await this._fetchJSON(url, {method: 'DELETE'});
        } catch (err) {
            Growl.error('Error deleting: ' + err);
            return;
        }
        this._rrss.splice(this._rrss.indexOf(rrs), 1);
        this._render();
    }

    this._save = async function(rrs, f) {
        if (f.accountID.value == 0) {
            Growl.error('Error saving: please choose an account');
            return;
        }
        let newRRS = {
            Name: f.name.value,
            Region: f.region.value,
            IsDefault: f.isDefault.checked,
            AccountID: f.accountID.value,
            ResourceShareID: f.share.value == rsToBeCreated ? "" : f.share.value,
            Rules: Array.from(rrs.Rules || []).map((rule, rIdx) => {
                if (rule._deleted) {
                    rule.Delete = true;
                } else {
                    rule.Description = f['rule' + rIdx + 'Name'].value;
                    rule.AWSID = f['rule' + rIdx + 'ID'].value;
                }
                return rule;
            })
        }
        // Must loop before filtering so indexes line up
        newRRS.Rules = newRRS.Rules.filter(g => !g._deleted);
        Array.from(f.querySelectorAll("button")).forEach(b => b.disabled = true);
        Array.from(f.querySelectorAll("select")).forEach(b => b.disabled = true);
        Array.from(f.querySelectorAll("input")).forEach(b => b.disabled = true);
        const url = info.ServerPrefix + 'mrrs/' + (rrs.ID || '')
        let response;
        try {
            response = await this._fetchJSON(url, {method: ('ID' in rrs) ? 'PATCH' : 'POST', body: JSON.stringify(newRRS)});
        } catch (err) {
            Growl.error('Error saving: ' + err);
            Array.from(f.querySelectorAll("button")).forEach(b => b.disabled = false);
            Array.from(f.querySelectorAll("input")).forEach(b => b.disabled = false);
            return;
        }
        // Fill in any IDs from response.
        Object.assign(rrs, response.json);
        Array.from(f.querySelectorAll("button")).forEach(b => b.disabled = false);
        Array.from(f.querySelectorAll("select")).forEach(b => b.disabled = false);
        Array.from(f.querySelectorAll("input")).forEach(b => b.disabled = false);
        rrs.editing = false;
        this._render();
    }

    this._addRuleset = function(rrs) {
        this._rrss.splice(0, 0, {
            Name: "",
            ResourceShareID: "",
            Rules: [],
        });
        this._edit(this._rrss[0]);
    }

    this._deleteRuleset = function(ruleset) {
        ruleset._deleted = true;
        this._render();
    }

    this._addRule = function(ruleset) {
        ruleset.Rules = ruleset.Rules || [];
        ruleset.Rules.push({
            ID: 0,
            AWSID: 'rslvr-rr-',
            Description: '',
        });
        this._render();
    }

    this._deleteRule = function(rule) {
        rule._deleted = true;
        this._render();
    }

    this._cancel = function(rrs) {
        if (!('ID' in rrs)) {
            this._rrss.splice(this._rrss.indexOf(rrs), 1);
        } else {
            Object.assign(rrs, rrs.beforeEdits);  // revert to previous version
        }
        this._edit(null);
    }

    this._generateAccountInfo = function(rrs) {
        if (rrs.AccountID === undefined) {
            return "--Account--";
        } else {
            const account = this._accounts.find(account => account.ID === rrs.AccountID);
            return `${account.ProjectName} | ${account.Name} | ${account.ID}`;
        }
    }

    this._render = function() {
        render(
            html`
                ${this._rrss.some(rrs => rrs.editing) || !User.isAdmin()
                    ? ''
                    : html`<button type="button" @click="${() => this._addRuleset()}" class="ds-c-button ds-c-button--primary ds-c-button--small">New Resolver Ruleset</button>`}
                ${this._rrss.map(rrs => (
                    rrs.editing
                    ? html`
                        <form class="rrs editing" onsubmit="return false" @account-selected="${(e) => e.currentTarget.accountID.value = e.detail.account.ID}">
                        <div class="section-header ds-u-padding--1">
                            <div class="ds-l-row ds-u-align-items--end">
                                <div class="ds-l-col--4">
                                    <label class="ds-c-label ds-u-margin--0">
                                        Resolver Ruleset Name
                                        <input name="name" value="${rrs.Name}" class="ds-c-field ds-u-display--inline-block">
                                    </label>
                                </div>
                                <div class="ds-l-col--4">
                                    <input type="checkbox" id="isDefault" name="isDefault" class="ds-c-choice ds-c-choice--small" ?checked=${rrs.IsDefault}>
                                    <label for="isDefault" class="ds-c-label"><span class="tooltip" data-tooltip="Add to all new VPCs in region">Default</span></label>
                                </div>
                                <div class="ds-l-col--4">
                                    <span class="ds-u-float--right">
                                        <button type="button" @click="${(e) => this._save(rrs, e.target.closest('form'))}" class="ds-c-button ds-c-button--inverse">Save</button>
                                        <button type="button" @click="${() => this._cancel(rrs)}" class="ds-c-button ds-c-button--inverse">Cancel</button>
                                    </span>
                                </div>
                            </div>
                        </div>
                        ${(rrs.InUseVPCs && rrs.InUseVPCs.length)
                            ? html`
                                <div class="edit-warning">
                                    <b>Warning:</b> This resolver rule set is in use.
                                </div>`
                            : ''}
                            <div>
                            <table class="rules">
                                <thead>
                                    <tr>
                                        <th style="text-align: right">Account</th>
                                        <td style="text-align: left">
                                            <account-select 
                                                accountInfo="${this._generateAccountInfo(rrs)}"
                                                .accounts="${this._accounts}"
                                            >
                                            </account-select>
                                            <input name="accountID" type="hidden" value="${rrs.AccountID || 0}"> 
                                        </td>
                                    </tr>
                                    <tr>
                                        <th style="text-align: right">Region</th>
                                        <th style="text-align: left">
                                            <select name="region" class="ds-c-field">
                                                <option value="us-east-1">us-east-1</option>
                                                <option value="us-west-2">us-west-2</option>
                                                <option value="us-gov-west-1">us-gov-west-1</option>
                                            </select>
                                        </th>
                                    </tr>
                                    <tr>
                                        <th style="text-align: right">Share</th>
                                        <th style="text-align: left">
                                            <input size="36" class="ds-c-field" maxlength="36" name="share" value="${rrs.ResourceShareID}" placeholder="Share ID">
                                        </th>
                                    </tr>
                                </thead>
                            </table>

                            <div class="section-header-secondary">Rules</div>
                            <div class="ds-u-padding--1">
                                ${(rrs.Rules || []).map((rule, rIdx) => rule._deleted ? '' : html`                                   
                                <div>
                                    <input name="rule${rIdx}ID" value="${rule.AWSID}" placeholder="Rule ID" class="ds-c-field ds-u-display--inline-block">
                                    <input name="rule${rIdx}Name" value="${rule.Description}" placeholder="Rule Name" class="ds-c-field ds-u-display--inline-block">
                                    <button type="button" @click="${() => this._deleteRule(rule)}" class="ds-c-button ds-c-button--primary">Delete</button>
                                </div>
                                `)}
                                <button type="button" @click="${() => this._addRule(rrs)}" class="ds-c-button ds-c-button--primary">Add Rule</button>
                            </div>
                        </div>
                    `
                    : html`
                    <div class="ds-u-margin-y--1 ds-u-padding--0">
                        <div class="section-header ds-u-padding--1">
                            ${rrs.Name} (${rrs.AccountID}/${rrs.Region}) [Share ID: ${rrs.ResourceShareID == "" ? rsToBeCreated : rrs.ResourceShareID}]
                            ${rrs.IsDefault ? html`<span class="ds-c-badge badge--info-inverted tooltip" data-tooltip="Add to all new VPCs in region">Default</span>` : nothing}
                            <span class="ds-u-float--right">
                                ${this._rrss.some(rrs => rrs.editing) ? nothing : html`<button type="button" class="ds-c-button ds-c-button--inverse ds-c-button--small" @click="${() => this._edit(rrs)}" ?disabled="${!User.isAdmin()}">Edit</button>
                                <button type="button" class="ds-c-button ds-c-button--inverse ds-c-button--small" @click="${() => this._delete(rrs)}" ?disabled="${(rrs.InUseVPCs && rrs.InUseVPCs.length) || !User.isAdmin()}">Delete</button>`}
                            </span>
                        </div>
                        <div class="ds-u-padding--0 ds-u-margin--0">
                            <table class="standard-table ds-u-margin--0">
                                <thead>
                                    <tr>
                                        <th style="text-align: left">Rules</th>
                                    </tr>
                                </thead>
                                <tbody>
                            ${(rrs.Rules || []).map((rule, rIdx) => rule._deleted ? '' : html`
                                <tr>
                                    <td>
                                        ${rule.AWSID} (${rule.Description})
                                    </td>
                                </tr>
                            `)}
                            </table>
                        </div>
                        <div class="associated-with">
                            Associated with ${
                                (rrs.InUseVPCs && rrs.InUseVPCs.length)
                                ? html`
                                    <a href="#" @click="${(e) => {e.preventDefault(); document.getElementById(rrs.ID + "VPCs").classList.toggle('hidden')}}">${rrs.InUseVPCs.length == 1 ? '1 VPC' : rrs.InUseVPCs.length + ' VPCs'}</a>
                                    <div id="${rrs.ID}VPCs" class="hidden">
                                        ${rrs.InUseVPCs.map(accountVPC => html`
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
            this._rrssContainer);
    }

    this.init = async function(container) {
        Breadcrumb.set([{name: "Resolver Rules"}]);
        render(
            html`
                <div id="background" class="hidden"></div>
                <div id="modal" class="hidden"></div>
                <div class="ds-l-container ds-u-margin-y--1">
                    <div id="container">
                        <div id="rrs"></div>
                    </div>
                </div>`,
            container)
        this._rrssContainer = document.getElementById('rrs');
        this._modal = document.getElementById('modal');
        this._background = document.getElementById('background');
        
        this._loadAccountInfo();
        window.setInterval(() => this._loadAccountInfo(), 3000);

        this._loadRulesInfo();
    }
}