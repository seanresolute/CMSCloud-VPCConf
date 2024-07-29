import { LitElement, html } from './lit-element/lit-element.js';
import { nothing } from './lit-html/lit-html.js';
import { User } from './view/user.js';

class VPCConfNavigation extends LitElement {
    constructor() {
        super();
        this.name = "";
        this.isAdmin = false;
        this._handleRetrieveName();
        this.user = new User();
    }

    static get properties() {
        return {
            name: { type: String },
            isAdmin: { type: Boolean },
        };
    }

    connectedCallback() {
        super.connectedCallback();
        window.addEventListener('user-ready', this._handleRetrieveName);
    }

    disconnectedCallback() {
        window.removeEventListener('user-ready', this._handleRetrieveName);
        super.disconnectedCallback();
    }

    _handleRetrieveName = () => {
        this.name = User.name();
        this.isAdmin = User.isAdmin();
    }

    logOut = (e) => {
        User.clearDetails();
    }

    render() {
        return html`
        <div id="nav-container" class="ds-l-container ds-u-padding--0">	
            <div class="ds-l-row">
                <div id="nav" class="ds-l-col--10 ds-u-display--flex ds-u-justify-content--start ds-u-flex-wrap--nowrap ds-u-align-items--center">
                    <a href="/provision" style="text-decoration: none"><span id="vpc-conf" class="ds-u-padding-x--1">VPC Conf</span></a>
                    <ul class="ds-u-color--white ${this.name == undefined ? 'invisible' : nothing}">
                        <li><a id="new-request" href="/provision/accounts">Accounts</a></li>
                        <li><a id="vpc-requests" href="/provision/vpcreqs">VPC Requests</a></li>
                        <li><a id="batch-tasks" href="/provision/batch">Batch Tasks</a></li>
                        <li><a id="ip-usage" href="/provision/usage">IP Usage</a></li>
                        <li class="dropdown">
                            <a id="attachment-templates" href="javascript:void(0)" class="dropdown-btn">Templates</a>
                            <div class="dropdown-content">
                                <a id="resolver-rules" href="/provision/mrrs">Resolver Rules</a>
                                <a id="security-groups" href="/provision/sgs">Security Groups</a>
                                <a id="transit-gateways" href="/provision/mtgas">Transit Gateways</a>
                            </div>
                        </li>
                        <li><a id="search" href="/provision/search">Search</a></li>
                    </ul>
                </div>
                <div id="nav" class="ds-l-col--2 ds-u-display--flex ds-u-justify-content--end ds-u-align-items--center">
                    <ul class="ds-u-color--white">
                        <li class="dropdown">
                            <a id="username" href="javascript:void(0)" class="dropdown-btn"><span id="statusIcon" class="adminIcon ${this.isAdmin ? nothing : 'hidden'}" title="Administrator Access"></span>${this.name}</a>
                            <div class="dropdown-content">
                                <a id="logOut" @click="${this.logOut}" href="/provision/logout">Log Out</a>
                            </div>
                        </li>
                    </ul>
                </div>
            </div>
        </div>
        `;
    }

    createRenderRoot() {
        return this;
    };
}

customElements.define('vpc-conf-navigation', VPCConfNavigation);