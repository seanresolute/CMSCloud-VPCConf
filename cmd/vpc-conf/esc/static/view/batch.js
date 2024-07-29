import { html, nothing, render } from '../lit-html/lit-html.js';
import { Breadcrumb } from './components/shared/breadcrumb.js'
import { Growl } from './components/shared/growl.js';
import { User } from './user.js'
import { VPCType } from './vpctype.js'
import './components/fixed-batch-task-list.js';
import './components/dynamic-batch-task-list.js';
import './components/fixed-subtask-list.js';
import './components/list-log-container.js';

import { DisplaysTasks, CancelsTasks, HasModal, MakesAuthenticatedAJAXRequests } from './mixins.js';

export function BatchTasksPage(info) {
    this._loginURL = info.ServerPrefix + 'oauth/callback';
    this._baseTaskURL = info.ServerPrefix + 'task/ignored/'
    this._cancelTasksURL = info.ServerPrefix + 'task/cancel'
    Object.assign(this, DisplaysTasks, CancelsTasks, HasModal, MakesAuthenticatedAJAXRequests);

    const labelFilterWith = 'with';
    const labelFilterWithout = 'without';
    const labelFilterAny = 'any';
    this._filterType = labelFilterAny;
    let loadingTasks = false;

    this._loadTasks = async function () {
        if (loadingTasks) return;  // Only one at a time please
        loadingTasks = true;
        const taskURL = info.ServerPrefix + 'batch/task/';
        let response;
        try {
            response = await this._fetchJSON(taskURL);
        } catch (err) {
            Growl.error('Error fetching account info: ' + err);
            return;
        }
        loadingTasks = false;
        this._renderTasks(response.json);
    }

    this._renderTasks = function (info) {
        const tasks = info.Tasks || [];

        if (info.IsMoreTasks) {
            this._showOlderTasksButton.classList.remove('ds-c-button--disabled');
            this._oldestTaskID = info.Tasks.map(t => t.ID).reduce((v, id) => Math.min(v, id))
        } else {
            this._showOlderTasksButton.classList.add('ds-c-button--disabled');
        }

        render(
            html`
                <fixed-batch-task-list .tasks="${tasks}">
                </fixed-batch-task-list>
            `,
            this._tasks);
    }

    this._showSubTasks = function (description, type, tasks) {
        this._showModal();

        const title = `${description}: ${type} tasks`;
        const taskList = html`
            <fixed-subtask-list
                class="leftPane"
                .tasks="${tasks}"
                serverPrefix="${info.ServerPrefix}"
                selectedTaskID="${tasks[0].ID}"
                title="${title}"
            >
            </fixed-subtask-list>
        `
        render(
            html`
                <list-log-container
                    baseTaskURL="${this._baseTaskURL}"
                    .taskList="${taskList}"
                    .fetchJSON="${this._fetchJSON.bind(this)}"
                >
                </list-log-container>
            `,
            this._modal);
    }

    this._showOlderTasks = function (beforeID) {
        this._showModal();
        render(
            html`
                <dynamic-batch-task-list 
                    baseTaskURL="${info.ServerPrefix + 'batch/task/'}"
                    beforeID="${beforeID}"
                    .fetchJSON="${this._fetchJSON.bind(this)}"
                >
                </dynamic-batch-task-list>
            `,
            this._modal);
    }

    this._filterByLabels = function (type) {
        this._filterType = type;
        if (type == labelFilterAny) {
            for (const cb of this._labelFilter) {
                cb.disabled = true;
            }
        } else if (User.isAdmin()) {
            for (const cb of this._labelFilter) {
                cb.disabled = false;
            }
        }
    }

    this.init = async function (container) {
        Breadcrumb.set([{ name: "Batch Tasks" }]);
        render(
            html`
                <div id="background" class="hidden"></div>
                <div id="modal" class="hidden"></div>
                <div class="ds-l-container ds-u-padding--0">
                    <div id="container">
                        <form id="batchForm"></form>
                    </div>
                </div>`,
            container)
        this._modal = document.getElementById('modal');
        this._background = document.getElementById('background');
        const form = document.getElementById('batchForm');

        const vpcsURL = info.ServerPrefix + 'batch/vpcs.json';
        const sgsURL = info.ServerPrefix + 'sgs.json';
        const allLabelsURL = info.ServerPrefix + 'labels.json';
        const vpcLabelsURL = info.ServerPrefix + 'batch/labels.json';
        const mrrsURL = info.ServerPrefix + 'mrrs.json';

        let responses;
        try {
            responses = await Promise.all([this._fetchJSON(vpcsURL), this._fetchJSON(sgsURL), this._fetchJSON(allLabelsURL), this._fetchJSON(vpcLabelsURL), this._fetchJSON(mrrsURL)]);
        } catch (err) {
            Growl.error('Error fetching VPCs: ' + err);
            return;
        }
        let vpcs = responses[0].json.VPCs;
        const regions = responses[0].json.Regions;
        const vpcTypes = responses[0].json.VPCTypes.sort((a, b) => a.Name > b.Name ? 1 : -1);
        const sgss = responses[1].json;
        const allLabels = responses[2].json;
        const labels = responses[3].json;
        const mrrs = responses[4].json.sort((a, b) => {
            if (a.Region > b.Region) return 1;
            if (a.Region < b.Region) return -1;
            if (a.Name > b.Name) return 1;
            return -1;
        });
        let lookupTable = {};

        vpcs.forEach((vpc, idx) => {
            vpc.checked = false;
            lookupTable[vpc.Name] = idx;
        });

        render(
            html`
                <div class="ds-l-row ds-u-margin-y--2">
                    <div class="ds-l-col--6">
                        <div class="section-header">Tasks</div>
                            <div class="section-header-secondary">
                                Mode
                            </div>
                            <input type="radio" id="verify" checked name="mode" value="verify" class="ds-c-choice ds-c-choice--small" ?disabled="${!User.isAdmin()}"><label for="verify">Verify state</label>
                            <input type="radio" id="repair" name="mode" value="repair" class="ds-c-choice ds-c-choice--small" ?disabled="${!User.isAdmin()}"><label for="repair">Sync state, repair tags, apply config</label>
                            <input type="radio" id="syncroutes" checked name="mode" value="syncroutes" class="ds-c-choice ds-c-choice--small" ?disabled="${!User.isAdmin()}"><label for="syncroutes">Resync AWS Routes into State</label>

                            <div class="section-header-secondary">
                                Configuration Types
                            </div>
                            <div @change="${(e) => this._updateVerifyAllCheckbox(e)}">
                                <input type="checkbox" name="verifyAll" id="verifyAll" value="1" class="ds-c-choice ds-c-choice--small" ?disabled="${!User.isAdmin()}">
                                <label for="verifyAll" class="ds-c-label">All</label>

                                <input type="checkbox" name="verifyNetworking" id="verifyNetworking" value="1" class="ds-c-choice ds-c-choice--small" ?disabled="${!User.isAdmin()}">
                                <label for="verifyNetworking" class="ds-c-label">Networking</label>

                                <input type="checkbox" name="verifyLogging" id="verifyLogging" value="1" class="ds-c-choice ds-c-choice--small" ?disabled="${!User.isAdmin()}">
                                <label for="verifyLogging" class="ds-c-label">Logging</label>

                                <input type="checkbox" name="verifyResolverRules" id="verifyResolverRules" value="1" class="ds-c-choice ds-c-choice--small" ?disabled="${!User.isAdmin()}">
                                <label for="verifyResolverRules" class="ds-c-label">Resolver Rules</label>

                                <input type="checkbox" name="verifySecurityGroups" id="verifySecurityGroups" value="1" class="ds-c-choice ds-c-choice--small" ?disabled="${!User.isAdmin()}">
                                <label for="verifySecurityGroups" class="ds-c-label">Security Groups</label>

                                <input type="checkbox" name="verifyCIDRs" id="verifyCIDRs" value="1" class="ds-c-choice ds-c-choice--small" ?disabled="${!User.isAdmin()}">
                                <label for="verifyCIDRs" class="ds-c-label">CIDRs</label>

                                <input type="checkbox" name="verifyCMSNet" id="verifyCMSNet" value="1" class="ds-c-choice ds-c-choice--small" ?disabled="${!User.isAdmin()}">
                                <label for="verifyCMSNet" class="ds-c-label">CMSNet</label>
                            </div>
                        <div class="section-header">VPC Filters</div>
                            <div class="section-header-secondary">Regions</div>
                                ${regions.map(region => html`
                                <input id="${region}" name="region" value="${region}" type="checkbox" class="ds-c-choice ds-c-choice--small" ?disabled="${!User.isAdmin()}"><label for="${region}">${region}</label>
                                `)}
                            <div class="section-header-secondary">VPC Types</div>
                                ${vpcTypes.map(vpcType => html`
                                <input id="${vpcType.Name}" name="vpcType" value="${vpcType.ID}" type="checkbox" class="ds-c-choice ds-c-choice--small" ?disabled="${!User.isAdmin() || !vpcType.IsVerifiable}"><label for="${vpcType.Name}">${VPCType.getStyled(vpcType.ID)}</label>
                                `)}
                            <div class="section-header-secondary">Stacks</div>
                            <input id="sandbox" name="stack" value="sandbox" type="checkbox" class="ds-c-choice ds-c-choice--small" ?disabled="${!User.isAdmin()}"><label for="sandbox">Sandbox</label>
                            <input id="dev" name="stack" value="dev" type="checkbox" class="ds-c-choice ds-c-choice--small" ?disabled="${!User.isAdmin()}"><label for="dev">Dev</label>
                            <input id="test" name="stack" value="test" type="checkbox" class="ds-c-choice ds-c-choice--small" ?disabled="${!User.isAdmin()}"><label for="test">Test</label>
                            <input id="impl" name="stack" value="impl" type="checkbox" class="ds-c-choice ds-c-choice--small" ?disabled="${!User.isAdmin()}"><label for="impl">Impl</label>
                            
                            <input id="qa" name="stack" value="qa" type="checkbox" class="ds-c-choice ds-c-choice--small" ?disabled="${!User.isAdmin()}"><label for="qa">QA</label>
                            <input id="nonprod" name="stack" value="nonprod" type="checkbox" class="ds-c-choice ds-c-choice--small" ?disabled="${!User.isAdmin()}"><label for="nonprod">Non Prod</label>
                            <input id="mgmt" name="stack" value="mgmt" type="checkbox" class="ds-c-choice ds-c-choice--small" ?disabled="${!User.isAdmin()}"><label for="mgmt">Mgmt</label>
                            
                            <input id="prod" name="stack" value="prod" type="checkbox" class="ds-c-choice ds-c-choice--small" ?disabled="${!User.isAdmin()}"><label for="prod">Prod</label>
                            <div class="section-header-secondary"> 
                                <table id="LabelFilter">
                                    <tr>
                                        <td nowrap>Labels: </td>
                                        <td nowrap><input type="radio" id="labels-${labelFilterAny}" name="labelFilter" value="${labelFilterAny}" @click="${(e) => { this._filterByLabels(labelFilterAny); }}"  checked class="ds-c-choice ds-c-choice--small" ?disabled="${!User.isAdmin()}"><label for="labels-${labelFilterAny}"> Any </label></td>
                                        <td nowrap><input type="radio" id="labels-${labelFilterWith}" name="labelFilter" value="${labelFilterWith}" @click="${(e) => { this._filterByLabels(labelFilterWith); }}" class="ds-c-choice ds-c-choice--small" ?disabled="${!User.isAdmin()}"><label for="labels-${labelFilterWith}"> With </label></td>
                                        <td nowrap><input type="radio" id="labels-${labelFilterWithout}" name="labelFilter" value="${labelFilterWithout}" @click="${(e) => { this._filterByLabels(labelFilterWithout); }}" class="ds-c-choice ds-c-choice--small" ?disabled="${!User.isAdmin()}"><label for="labels-${labelFilterWithout}"> Without </label></td>
                                    </tr>
                                </table>
                            </div>
                            <div style="overflow-y:auto;max-height:200px;">
                                    ${allLabels.map(label => html`
                                    <input id="label-${label.Name}" name="label" value="${label.Name}" type="checkbox" class="ds-c-choice ds-c-choice--small" disabled="true"><label for="label-${label.Name}">${label.Name}</label>
                                    `)}
                                </div>
                            </div>              
                    <div class="ds-l-col--6">
                        <div class="section-header">Configuration Updates</div>
                            <div class="section-header-secondary">Security Groups</div>
                            <table id="securityGroupChanges">
                                ${sgss.map(sgs => html`
                                <tr>
                                    <td nowrap>${sgs.Name}</td>
                                    <td nowrap><input type="radio" id="sgs${sgs.ID}-add" name="sgs${sgs.ID}" value="add" class="ds-c-choice ds-c-choice--small" ?disabled="${!User.isAdmin()}"><label for="sgs${sgs.ID}-add">Add to all</label></td>
                                    <td nowrap><input type="radio" id="sgs${sgs.ID}-remove" name="sgs${sgs.ID}" value="remove" class="ds-c-choice ds-c-choice--small" ?disabled="${!User.isAdmin()}"><label for="sgs${sgs.ID}-remove">Remove from all</label></td>
                                    <td nowrap><input type="radio" id="sgs${sgs.ID}-leave" name="sgs${sgs.ID}" value="" checked class="ds-c-choice ds-c-choice--small" ?disabled="${!User.isAdmin()}"><label for="sgs${sgs.ID}-leave">Leave as-is</label></td>
                                </tr>
                                `)}
                            </table>
                            <div class="section-header-secondary">Resolver Rules</div>
                            <table id="resolverRuleChanges">
                                ${mrrs.map(mrr => html`
                                <tr>
                                    <td nowrap>${mrr.Name}</td>
                                    <td nowrap><input type="radio" id="mrr${mrr.ID}-add" name="mrr${mrr.ID}" value="add" class="ds-c-choice ds-c-choice--small" ?disabled="${!User.isAdmin()}"><label for="mrr${mrr.ID}-add">Add to all</label></td>
                                    <td nowrap><input type="radio" id="mrr${mrr.ID}-remove" name="mrr${mrr.ID}" value="remove" class="ds-c-choice ds-c-choice--small" ?disabled="${!User.isAdmin()}"><label for="mrr${mrr.ID}-remove">Remove from all</label></td>
                                    <td nowrap><input type="radio" id="mrr${mrr.ID}-leave" name="mrr${mrr.ID}" value="" checked class="ds-c-choice ds-c-choice--small" ?disabled="${!User.isAdmin()}"><label for="mrr${mrr.ID}-leave">Leave as-is</label></td>
                                </tr>
                                `)}
                            </table>
                    </div> 
                </div>
                <div class="ds-l-row ds-u-margin-y--2">
                    <div class="ds-l-col--6">
                        
                        <div class="section-header">
                            VPCs Affected (<span id="numVPCs"></span>)
                        </div>
                        <div class="section-header-secondary ds-l-container">
                            <div class="ds-l-row">
                                <div class="ds-l-col--4" style="margin-left: 4px;">
                                    <input type="checkbox" id="selectAllCheckbox" @click="${(e) => toggleAllVPCSelections(e.target.checked)}" value="1" class="ds-c-choice ds-c-choice--small" /><label for="selectAllCheckbox">Select All</label>
                                </div>
                                <div class="ds-l-col--5 ds-u-margin-left--auto">
                                    <input id="vpcNameFilter" type="text" class="ds-c-field ds-c-field--medium ds-u-display--inline-block" placeholder="Filter by IDs or Names" title="Separate multiple values with a space" autocomplete="off" />
                                    <button id="clearVPCNameFilter" type="button" class="ds-c-button ds-c-button--inverse" @click="${() => clearVPCNameFilterAction()}">Clear</button>
                                </div>
                            </div>
                        </div>
                        
                        <div id="vpcTable" style="max-height: 600px; overflow-y: auto"></div>
                        <button id="submitButton" type="submit" class="ds-c-button ds-c-button--primary" style="float: right;margin-top: 10px">Schedule Tasks</button>
                        <div id="selectionHint" class="ds-u-float--right ds-u-margin-y--2 ds-u-margin-x--2"></div>
                    </div>

                    <div class="ds-l-col--6">
                        <div class="section-header">Scheduled Task Status</div>
                        <div id="tasks"></div>
                        <button id="showOlderTasks" class="ds-c-button ds-c-button--primary ds-c-button--disabled" style="float: right;margin-top: 10px">Show Older Tasks</button>
                    </div>
                </div>
                `,
            form);

        this._tasks = document.getElementById('tasks');
        this._showOlderTasksButton = document.getElementById('showOlderTasks');
        this._labelFilter = document.querySelectorAll('input[name=label]');

        const repair = document.getElementById('repair');
        const verify = document.getElementById('verify');
        const syncroutes = document.getElementById('syncroutes');
        const submit = document.getElementById('submitButton');
        const vpcNameFilter = document.getElementById('vpcNameFilter');
        const numVPCs = document.getElementById('numVPCs');
        const selectAllCheckbox = document.getElementById('selectAllCheckbox');
        const selectionHint = document.getElementById("selectionHint");

        const verifyCheckboxNames = [
            'verifyNetworking',
            'verifyLogging',
            'verifyResolverRules',
            'verifySecurityGroups',
            'verifyCIDRs',
            'verifyCMSNet',
        ]

        this._updateVerifyAllCheckbox = function (e) {
            if (e.target.name == 'verifyAll') {
                verifyCheckboxNames.forEach(name => {
                    form[name].checked = !!e.target.checked;
                });
            } else {
                form.verifyAll.checked = verifyCheckboxNames.every(name => !!form[name].checked);
            }
        }

        const getSGIDs = function (nodeList) {
            return Array.from(nodeList).map(radio => +radio.name.replace(/^sgs/, ''));
        }

        const getMRRIDs = function (nodeList) {
            return Array.from(nodeList).map(radio => +radio.name.replace(/^mrr/, ''));
        }

        const enableDisableVPCListInputs = (bool) => {
            selectAllCheckbox.disabled = bool;
            vpcNameFilter.disabled = bool;
        }

        const getSelectedVPCs = () => {
            const checkedVPCNames = Array.from(document.querySelectorAll('input[name^=vpc-filter]:checked')).map(checked => checked.id)
            return applyToVPCs.filter(vpc => checkedVPCNames.includes(vpc.Name));
        }

        let applyToVPCs = [];

        const hasSelectedVPCs = () => {
            return getSelectedVPCs().length > 0;
        }
        const hasSelectedVerifyTypes = () => {
            return verifyCheckboxNames.some(name => !!form[name].checked);
        }
        const isVerifyMode = () => {
            return repair.checked || verify.checked;
        }


        const vpcTable = document.getElementById('vpcTable');
        const showVPCs = (vpcs) => {
            render(
                vpcs.length ? html`
                        <table id="vpc-sortable-table" class="standard-table big-table">
                        <tbody>
                        ${vpcs.map(v => html`
                            <tr class="${v.checked || doesVPCIDOrNameContain(v.ID, v.Name) ? nothing : 'hidden'}">
                                <td>
                                    <input type="checkbox" id="${v.Name}" name="vpc-filter" class="ds-c-choice ds-c-choice--small" @click="${(e) => toggleVPCselection(e, v)}" />
                                    <label for="${v.Name}">${v.Name}</label>
                                </td>
                                <td>
                                    ${v.ID}
                                </td>
                            </tr>
                        `)}
                        </tbody>
                    </table>
                `
                    : nothing,
                vpcTable
            )
            render(html`${document.querySelectorAll('input[name=vpc-filter]:checked').length + ' / ' + applyToVPCs.length}`, numVPCs);

            enableDisableVPCListInputs(!vpcs.length > 0);
        }
        showVPCs(applyToVPCs);

        const updateHintAndSubmitButton = () => {
            let hints = [];

            if (User.isAdmin()) {
                if (!hasSelectedVPCs()) {
                    hints.push("VPC");
                }
                if (isVerifyMode()) {
                    if (!hasSelectedVerifyTypes()) {
                        hints.push("Configuration Type");
                    }
                }
            } else {
                hints.push("Authorization");
            }

            const hasHints = hints.length > 0;

            selectionHint.innerText = hasHints ? "Missing " + hints.join(" and ") : '';
            submit.classList.toggle('ds-c-button--disabled', hasHints)
            selectionHint.classList.toggle('fixable', hasHints)
        }
        updateHintAndSubmitButton();

        const toggleVPCselection = (e, vpc) => {
            vpc.checked = e.target.checked;
            selectAllCheckbox.checked = false;
            showVPCs(applyToVPCs);
        }

        const toggleAllVPCSelections = (bool) => {
            // only select rows that are not being filtered
            const checkboxes = document.querySelectorAll('tr:not(.hidden) input[name=vpc-filter]');
            Array.from(checkboxes).map(chk => {
                chk.checked = bool;
                const idx = lookupTable[chk.id]; // chk.id is the vpc.Name
                vpcs[idx].checked = bool;
            });
            showVPCs(applyToVPCs);
        }

        const clearVPCNameFilterAction = () => {
            vpcNameFilter.value = '';
            const event = document.createEvent('Event');
            event.initEvent('input', true, false);
            vpcNameFilter.dispatchEvent(event);
        }

        const doesVPCIDOrNameContain = (id, name) => {
            const searchInput = vpcNameFilter.value.split(" ").filter((e) => { return e != ""; });
            if (searchInput.length == 0) return true;

            let rv = false;
            for (let i = 0; i < searchInput.length; i++) {
                let search = searchInput[i].toLowerCase();
                if (id.toLowerCase().indexOf(search) > -1 || name.toLowerCase().indexOf(search) > -1) {
                    rv = true;
                    break;
                }
            }

            return rv;
        }

        const debounce = (callback, delay) => {
            let timeout;
            return function () {
                clearTimeout(timeout);
                timeout = setTimeout(callback, delay);
            }
        }

        form.addEventListener('input', debounce(() => {
            // update the enabled/disabled VPC types based on the task mode
            const inRepairMode = form.repair.checked;
            vpcTypes.forEach(vpcType => {
                let typeIsEnabled = inRepairMode ? vpcType.IsRepairable : vpcType.IsVerifiable;
                if (typeIsEnabled) {
                    document.getElementById(vpcType.Name).disabled = false;
                } else {
                    document.getElementById(vpcType.Name).disabled = true;
                    document.getElementById(vpcType.Name).checked = false;
                }
            })

            const stackFormValues = [...form.stack.values()];
            const regionFormValues = [...form.region.values()];
            const vpcTypeFormValues = [...form.vpcType.values()];

            applyToVPCs = vpcs.filter(vpc => stackFormValues.some(cb => (cb.checked && cb.value == vpc.Stack)))
                .filter(vpc => regionFormValues.some(cb => (cb.checked && cb.value == vpc.Region)))
                .filter(vpc => vpcTypeFormValues.some(cb => (cb.checked && cb.value == vpc.State.VPCType)));

            const labelCheckedBoxes = [...document.querySelectorAll('input[name=label]:checked')];
            if (this._filterType == labelFilterWith) {
                const filteredLabels = labels.filter(label => labelCheckedBoxes.some(cb => cb.value == label.Label));
                applyToVPCs = applyToVPCs.filter(vpc => filteredLabels.some(label => (label.ID == vpc.ID && label.Region == vpc.Region)));
            } else if (this._filterType == labelFilterWithout) {
                const filteredLabels = labels.filter(label => labelCheckedBoxes.some(cb => cb.value == label.Label));
                applyToVPCs = applyToVPCs.filter(vpc => !filteredLabels.some(label => (label.ID == vpc.ID && label.Region == vpc.Region)));
            }
            showVPCs(applyToVPCs);
            updateHintAndSubmitButton();
        }, 50));

        form.addEventListener('submit', async (e) => {
            e.preventDefault();

            const addGroups = getSGIDs(document.querySelectorAll('input[name^=sgs][value=add]:checked'));
            const removeGroups = getSGIDs(document.querySelectorAll('input[name^=sgs][value=remove]:checked'));

            const addResolverRules = getMRRIDs(document.querySelectorAll('input[name^=mrr][value=add]:checked'));
            const removeResolverRules = getMRRIDs(document.querySelectorAll('input[name^=mrr][value=remove]:checked'));

            if (!applyToVPCs.length) {
                alert('No VPCs are selected to update');
                return;
            }

            const taskType = ( function() {
                if (repair.checked) {
                    return 2;
                }
                if (verify.checked) {
                    return 16;
                }
                if (syncroutes.checked) {
                    return 64;
                }
            })()

            submit.classList.add('ds-c-button--disabled');

            const verifySpec = Object.create(null);
            verifyCheckboxNames.forEach(name => {
                verifySpec[name[0].toUpperCase() + name.slice(1)] = !!form[name].checked;
            });

            const selectedVPCs = getSelectedVPCs();

            const url = info.ServerPrefix + 'batch'
            let response;
            const req = {
                VPCs: selectedVPCs,
                TaskTypes: taskType,
                VerifySpec: verifySpec,
                AddSecurityGroupSets: addGroups,
                RemoveSecurityGroupSets: removeGroups,
                AddResolverRuleSets: addResolverRules,
                RemoveResolverRuleSets: removeResolverRules,
            };
            try {
                response = await this._fetchJSON(url, { method: 'POST', body: JSON.stringify(req) });
            } catch (err) {
                Growl.error('Error saving: ' + err);
                submit.disabled = false;
                return;
            }
            submit.disabled = false;
            Growl.success('Tasks successfully scheduled!');
        });

        this._oldestTaskID = null;

        this._listenForCancelEvent(container);

        container.addEventListener('subtask-click', (e) => {
            this._showSubTasks(e.detail.description, e.detail.type, e.detail.tasks);
        });

        this._showOlderTasksButton.addEventListener('click', (e) => {
            e.preventDefault();
            this._showOlderTasks(this._oldestTaskID);
        });

        this._loadTasks();
        window.setInterval(() => this._loadTasks(), 3000);
    }
}
