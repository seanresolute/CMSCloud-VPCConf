import { LitElement, html } from '../../lit-element/lit-element.js';
import { nothing } from '../../lit-html/lit-html.js';
import {Growl} from '../components/shared/growl.js';
import {User} from '../user.js'

/*
USAGE

The configuration is an array of objects with a name, id, and adminOnly fields: 
    `name`      is the text shown on the tab in the UI and used for the location hash
    `id`        is the id of the content (typically a div) to show
    `adminOnly` is a boolean to indicate if access to the tab and its content is for administrators only

In this example the second tab will only be shown to admin accounts.

    var tabs = [ 
                 { name: "One", "id": "one-content", "adminOnly": false},
                 { name: "Two", "id": "two-content", "adminOnly": true}
               ]

	<tab-ui .tabs="${tabs}"></tab-ui>
	<div id="one-content" class="tab-content">Loading...</div>
	<div id="two-content" class="tab-content">Static content.</div>

When a tab is selected it adds its name as a URL hash.

The tabs can be set to sticky so they stay on screen when scrolling by
adding 'sticky' to the tab-ui element.

<tab-ui .tabs="${tabs}" sticky></tab-ui>
*/
class TabUI extends LitElement {
    static get properties() {
        return {
            tabs: { type: Array },
            sticky: { type: Boolean },
        };
    }
    constructor() {
        super();
        this.updateUIHandler = this.updateUI.bind(this);
    }

    connectedCallback() {
        super.connectedCallback();
        window.addEventListener('hashchange', this.updateUIHandler);
    }

    disconnectedCallback() {
        window.removeEventListener('hashchange', this.updateUIHandler);
        super.disconnectedCallback();
    }

    firstUpdated() {
        if (this.tabs.length == 0) return;

        if (location.hash.length == 0) {
            history.replaceState(null, document.title, location.pathname + '#' + this.tabs[0].name);
        }
        this.updateUI();
    }

    switchTab(targetTab) {
        location.hash = targetTab;
    }

    updateUI() {
        const targetTab = decodeURIComponent(location.hash.substring(1));

        // edge case if only the # is left after a manual edit of the URL
        if (!targetTab.length) { 
            location.hash = this.tabs[0].name; // abort this update and trigger a new event
            return;
        }

        if (this.tabs.findIndex((tab, idx, array) => { return tab.name == targetTab}) == -1) {
            Growl.error('Invalid tab name: "' + targetTab + '"');
            return;
        }

        this.tabs.map(tab => {
            const button = document.querySelector('[name="' + tab.name + '"]');
            const content = document.getElementById(tab.id);
            if (tab.name == targetTab) {
                button.classList.add('active');
                content.style.display = "block";
            } else {
                button.classList.remove('active');
                content.style.display = "none";
            }
        });
    }

    render(){
        return html`<div class="tab ${this.sticky ? 'tab-sticky' : ''}">
            ${this.tabs.map(tab => html`<button name="${tab.name}" class="${tab.adminOnly && !User.isAdmin() ? 'hidden' : nothing}" @click="${() => this.switchTab(tab.name)}">${tab.name}</button>`)}
        </div>`;
    }

    createRenderRoot() {
        return this;  // opt out of shadow DOM
    };
}
customElements.define('tab-ui', TabUI);