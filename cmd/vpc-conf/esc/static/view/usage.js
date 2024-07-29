import {html, nothing, render} from '../lit-html/lit-html.js';
import {Breadcrumb} from './components/shared/breadcrumb.js'
import {Growl} from './components/shared/growl.js'
import {HasModal, MakesAuthenticatedAJAXRequests} from './mixins.js';
import './components/collapse-ui.js';

export function IPUsagePage(info) {
    this._loginURL = info.ServerPrefix + 'oauth/callback';
    Object.assign(this, HasModal, MakesAuthenticatedAJAXRequests);
    this._loading = false;
    this._usageInfo = null;

    this.init = async function(container) {        
        Breadcrumb.set([{name: "IP Usage"}]);
        render(
            html`
                <div id="background" class="hidden"></div>
                <div id="modal" class="hidden"></div>
                <div class="ds-l-container ds-u-padding--0">
                    <div class="ds-u-display--flex">
                        <div id="container">
                            <div class="ds-l-row">
                                <div class="ds-l-col--12">
                                    <div class="section-header ds-u-padding--1">CMS Cloud - Greenfield IP usage</div>
                                        <div class="ds-l-row">
                                            <div class="ds-l-col--12" > 
                                                <div class="ds-l-row ds-u-margin-x--0 alternating-row">
                                                    <div class="ds-l-col--1">
                                                        <button  @click="${e => {this.handleRefreshClick(e)}}"  style="margin-bottom: 5px" class="ds-c-button ds-c-button--medium">Refresh</button>
                                                    </div>
                                                    <div class="ds-l-col--9">
                                                        <div id="loader" class="hidden"></div>
                                                    </div>
                                                    <div class="ds-l-col--2">
                                                        <button id="button-expand-all" class="ds-c-button ds-c-button--small" @click="${e => {this.handleExpandAll(e, false)}}"  type="button">Expand All</button>
                                                        <button id="button-collapse-all" class="ds-c-button ds-c-button--small" @click="${e => {this.handleExpandAll(e, true)}}" type="button">Collapse All</button>
                                                    </div>
                                                </div>
                                            </div>
                                        </div>

                                        <div id="usage"></div>

                                    </div>
                                </div>
                            </div>
                        </div>
                    </div>
                </div>
            `, container
        );


        this._modal = document.getElementById('modal');
        this._background = document.getElementById('background');
        this._usage = document.getElementById('usage');
        this._loader = document.getElementById('loader');

        this._loadUsage();
        window.setInterval(() => this._loadUsage(), 5000);
    }

    this.handleExpandAll = function(e, collapsedValue) {
        [...this._usage.querySelectorAll("collapse-ui")].forEach(c => c.collapsed = collapsedValue)
    }

    this.handleRefreshClick = async function(e) {
        this._loader.classList.remove('hidden');
        const url = info.ServerPrefix + 'ipusage/refresh';
        let response;
        try {
            response = await this._fetchJSON(url, {method: 'POST'});
        } catch (err) {
            Growl.error('Error refreshing ip usage data: ' + err);
            this._loader.classList.add('hidden');
            return;
        }
        this._loader.classList.add('hidden');
        this._loadUsage();
    }

    this._loadUsage = async () => {
        const url = info.ServerPrefix + 'ipusage.json';
        let response;
        try {
            response = await this._fetchJSON(url);
        } catch (err) {
            Growl.error('Error fetching ip usage data: ' + err);
            return;
        }
        this._usageInfo  = response.json
        if (this._usageInfo){
            const environmentUsage = this._usageInfo.Data;
            const lastUpdated = new Date(this._usageInfo.LastUpdated);

            render(
                html`
                    <strong>Last Updated:</strong> ${lastUpdated.toDateString()} ${lastUpdated.toLocaleTimeString()}<br/>
                    ${environmentUsage.sort(
                        function(a,b) {
                            let gt = a.IPFreePercent - b.IPFreePercent;
                            if (gt != 0) {
                                return gt;
                            }
                            return a.IPFree - b.IPFree;
                        }
                    ).map(metrics => {
                    const textColor = (metrics.IPFreePercent < 0.2) ? "red" : "black";
                    return html`
                        <div class="ds-l-col--12">
                            <collapse-ui title="${metrics.Region} -  ${metrics.Zone}">
                                <div class="ds-l-row">
                                    <div class="ds-l-col--6" > 
                                        <div class="ds-l-row ds-u-margin-x--0 alternating-row">
                                            <div class="ds-l-col--5">Environment</div>
                                            <div class="ds-l-col--1"> ${metrics.Environment}</div>
                                        </div>
                                        <div class="ds-l-row ds-u-margin-x--0 alternating-row">
                                            <div class="ds-l-col--5">Total IPs</div>
                                            <div class="ds-l-col--1"> ${metrics.IPTotal}</div>
                                        </div>
                                        <div class="ds-l-row ds-u-margin-x--0 alternating-row">
                                            <div class="ds-l-col--5">Available IPs</div>
                                            <div class="ds-l-col--1"> ${metrics.IPFree}</div>
                                        </div>
                                        <div class="ds-l-row ds-u-margin-x--0 alternating-row">
                                            <div class="ds-l-col--5">Available IPs Percentage</div>
                                            <div class="ds-l-col--1" style="color:${textColor}"> ${Math.round(metrics.IPFreePercent * 100)}%</div>
                                        </div>
                                        <div class="ds-l-row ds-u-margin-x--0 alternating-row">
                                            <div class="ds-l-col--5 min-width-460-md">Largest Free Contiguous Block</div>
                                            <div class="ds-l-col--1"> ${metrics.LargestFreeContiguousBlock}</div>
                                        </div>
                                    </div>
                                    <div class="ds-l-col--6 ds-u-margin-x--0 "  >
                                        <div class="section-header-secondary">Allocated CIDR(s)</div>
                                        ${metrics.CIDRs.map(CIDR => {
                                            return html` 
                                                <div class="ds-l-row ds-u-margin-x--0 alternating-row">
                                                    <div class="ds-l-col--5">CIDR</div>
                                                    <div class="ds-l-col--1"> ${CIDR.CIDR}</div>
                                                </div>
                                                <div class="ds-l-row ds-u-margin-x--0 alternating-row">
                                                    <div class="ds-l-col--5">Available IPs</div>
                                                    <div class="ds-l-col--1"> ${CIDR.IPFree}</div>
                                                </div>
                                                <div class="ds-l-row ds-u-margin-x--0 alternating-row">
                                                    <div class="ds-l-col--5">Available IPs Percentage</div>
                                                    <div class="ds-l-col--1"> ${Math.round(CIDR.IPFreePercent * 100)}%</div>
                                                </div>
                                                <div class="ds-l-row ds-u-margin-x--0 alternating-row">
                                                    <div class="ds-l-col--5">Largest Free Contiguous Block</div>
                                                    <div class="ds-l-col--1"> ${CIDR.LargestFreeContiguousBlock}</div>
                                                </div>  
                                                <div class="divider div-transparent"></div>`
                                        })}    
                                    </div>
                            </collapse-ui>
                        </div>`
                    })}    
                `,this._usage);
        }
    }
}
