import { LitElement, html } from '../../lit-element/lit-element.js'
import { nothing } from '../../lit-html/lit-html.js';
import { Growl } from './shared/growl.js'
import { HasModal, MakesAuthenticatedAJAXRequests } from '../mixins.js';
import { VPCType } from '../vpctype.js'

export class SearchUI extends LitElement {

    constructor() {
        super();
        this.searchHandler = this.debounce(this.search.bind(this), 100);
        this.info = {};
        this.results = [];
        this.searching = false;
        this.searchingText = "Searching";
        this.elapsedTime = ""
        this.searchComplete = false;
        this.term = "";
        this.hasError = false;
        Object.assign(this, HasModal, MakesAuthenticatedAJAXRequests);
    }

    static get properties() {
        return {
            info: { type: Object },
            results: { type: Array },
            searching: { type: Boolean },
            searchingText: { type: String },
            elapsedTime: { type: String },
            searchComplete: { type: Boolean },
            hasError: { type: Boolean },
        }
    }

    connectedCallback() {
        super.connectedCallback();
        document.addEventListener('keyup', this.searchHandler);
    }

    disconnectedCallback() {
        document.removeEventListener('keyup', this.searchHandler);
        super.disconnectedCallback();
    }

    firstUpdated() {
        this._modal = document.getElementById('modal');
        this._background = document.getElementById('background');
        this._loginURL = this.info.ServerPrefix + 'oauth/callback';
    }

    render() {
        return html`
            <div id="background" class="hidden"></div>
            <div id="modal" class="hidden"></div>
            <div class="ds-l-container ds-u-padding--0">
                <div id="container">
                    <div class="ds-u-margin-y--1">
                        <span style="font-weight: 600">Search Criteria</span>
                        <ul class="ds-u-margin-y--0">
                            <li>Account ID, or name</li>
                            <li>Project name</li>
                            <li>VPC ID, name, subnet IP, or CIDR</li>
                        </ul>
                    </div>
                    <div id="search">
                        <input id="search-term" type="text" class="ds-c-field ds-u-display--inline-block" autocomplete="off" autofocus="true" />
                        <button class="ds-c-button" @click=${e => this.clearInput()}>Clear</button>
                    </div>
                    <div id="search-results" class="ds-l-container ds-u-padding-x--0">
                        <div class="ds-u-margin-y--2">
                        ${this.searchComplete && !this.hasError ?
                            html`Found ${this.results.length} match${this.results.length == 1 ? nothing : 'es'} for "<b>${this.term}</b>" in ${this.elapsedTime}`:
                            nothing
                        }
                        </div>
                        ${!this.searching && this.results.length ?
                            html`
                                <div id="search-results" class="ds-l-container ds-u-padding-x--0">
                                    <div class="section-header-secondary">
                                        <div class="ds-l-row">
                                            <div class="ds-l-col--4">Project</div>
                                            <div class="ds-l-col--4">Account</div>
                                            <div class="ds-l-col--4">VPCs</div>
                                        </div>
                                    </div>
                                    <div class="section-body-bordered">
                                    ${this.results.map(account => html`
                                        <div class="ds-l-row alternating-row" style="margin-left: -8px;margin-right: -8px">
                                            <div class="ds-l-col--4 new-window">
                                                ${this.highlightTerm(account.Project)}
                                            </div>
                                            <div class="ds-l-col--4 new-window">
                                                <a href="${account.URL}" title="Open in current tab">${this.highlightTerm(account.Name)}</a> &nbsp;<a href="${account.URL}" target="_blank" title="Open in new tab"></a>
                                            </div>
                                            <div class="ds-l-col--4 new-window">
                                            ${(account.VPCs || []).map(vpc => html`
                                                <div class="ds-l-row">
                                                    <div class="ds-l-col--8">
                                                    ${vpc.URL ?
                                                        html`<a href="${vpc.URL}" title="Open in current tab">${this.highlightTerm(vpc.Name)}</a>
                                                            <a href="${vpc.URL}" target="_blank" title="Open in new tab"></a>` :
                                                        html`${this.highlightTerm(vpc.Name)}`
                                                    }
                                                    </div>
                                                    <div class="ds-l-col--2">
                                                        ${VPCType.getStyled(vpc.VPCType)}
                                                    </div>
                                                    <div class="ds-l-col--2">
                                                        <a href="${vpc.AWSConsoleURL}" target="_blank" title="Open in new tab">AWS</a>
                                                    </div>
                                                </div>
                                            `)}
                                            </div>
                                        </div>
                                    `)}
                                    </div>
                                </div>
                            ` : nothing
                        }
                    </div>
                </div>
            </div>
        `;
    }

    clearInput(e) {
        const searchTerm = document.getElementById("search-term");
        searchTerm.value = "";
        searchTerm.focus();
        this.searchComplete = false;
    }

    debounce(callback, delay) {
        let timeout;
        return function() {
            clearTimeout(timeout);
            timeout = setTimeout(callback, delay);
        }
    }

    highlightTerm(text) {
        const regex = new RegExp(this.escapeRegExp(this.term), "i");
        const idx = text.search(regex);

        if (idx == -1) {
            return text;
        }

        const parts = text.split(regex)
        const highlighted = document.createElement('span');
        let pos = 0;

        parts.forEach((part, pIdx) => {
            highlighted.appendChild(document.createTextNode(part));
            pos += part.length;
            if (pIdx < parts.length - 1) {
                let highlightedTerm = document.createElement('span');
                highlightedTerm.className = 'search-term-highlight';
                highlightedTerm.appendChild(document.createTextNode(text.substring(pos, pos+this.term.length)))
                highlighted.appendChild(highlightedTerm);
                pos += this.term.length;
            }
        });

        return highlighted;
    }

    escapeRegExp(string) {
        return string.replace(/[.*+?^${}()|[\]\\]/g, '\\$&'); // $& means the whole matched string
    }

    async search(e) {
        const searchTerm = document.getElementById("search-term");
        const searchResults = document.getElementById("search-results");
        const input = searchTerm.value.trim();

        if (input === "" || input === this.term) {
            return
        }

        this.term = input;
        this.searching = true;
        this.searchComplete = false;
        this.results = [ ...[] ];
        this.hasError = false;

        let response;
        try {
            response = await this._fetchJSON('/provision/search/do', {method: 'POST', body: JSON.stringify({"SearchTerm": this.term })})
            const res = response.json;
            this.results = res.Results ? [ ...res.Results ] : [];
            this.elapsedTime = res.ElapsedTime;
        } catch (err) {
            Growl.warning('Search encountered an error: ' + err);
            this.hasError = true;
        }

        this.searching = false;
        this.searchComplete = true;
    }

    createRenderRoot() {
        return this;  // opt out of shadow DOM
    };
}
customElements.define('search-ui', SearchUI);
