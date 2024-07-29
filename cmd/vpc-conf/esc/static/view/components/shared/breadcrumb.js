import { LitElement, css, html } from '../../../lit-element/lit-element.js'
import { User } from '../../user.js'

export class Breadcrumb {
    static set(crumbs) {
        const event = new CustomEvent('breadcrumb-update', { detail: crumbs });
        window.dispatchEvent(event);
    }
}

class BreadcrumbTrail extends LitElement {

    static get properties() {
        return { 
            breadcrumbs: {type: Array},
            root: {type: Object}
        };
    }

    constructor() {
        super();
        this.breadcrumbs = [];
    }

    connectedCallback() {
        super.connectedCallback();
        window.addEventListener('breadcrumb-update', this.updateCrumbs);
        window.addEventListener('user-ready', this.updateCrumbs);
    }

    disconnectedCallback() {
        window.removeEventListener('user-ready', this.updateCrumbs);
        window.removeEventListener('breadcrumb-update', this.updateCrumbs);
        super.disconnectedCallback();
    }

    updateCrumbs = (e) => {
        let crumbs;

        if (!e.detail) {
            crumbs = localStorage.getItem('breadcrumbs')
            if (crumbs.length) {
                crumbs = JSON.parse(crumbs);
            }
        } else if (e.detail[0].name != this.root.name) {
            crumbs = e.detail;
            localStorage.setItem('breadcrumbs', JSON.stringify(crumbs));
        }

        this.breadcrumbs = crumbs ? [ this.root, ...crumbs] : [{name: this.root.name}];
    }

    render() {
        return html`
            <div id="breadcrumb" style="display: ${User.name() == undefined ? "none" : "block"}">
                ${this.breadcrumbs.map((crumb) => {
                    return crumb.link ? html`<a href="${crumb.link}"}">${crumb.name}</a> <span class="separator">‣</span>` : crumb.name; 
                })}
            </div>
        `;
    }

    static get styles() {
        return css`
        #breadcrumb {
            display: flex;
            width: 100%;
            min-width: 544px;
            max-width: 1600px;
            margin-left: auto;
            margin-right: auto;
            overflow: hidden;
            padding: 8px;
            font-weight: bold;
        }

        #breadcrumb > a {
            color: #205493;
        }

        .separator {
            margin: 0px 8px;
            font-weight: 100;
        }
        `        
    }
}
customElements.define('breadcrumb-trail', BreadcrumbTrail);