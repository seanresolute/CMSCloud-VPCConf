import { LitElement, css, html } from '../../../lit-element/lit-element.js'
import { repeat } from '../../../lit-html/directives/repeat.js'

export class Growl {
    static error(text) {
        const event = new CustomEvent('new-growl', { detail: {"text": text, "type": "error"} });
        window.dispatchEvent(event);
    }
    static info(text) {
        const event = new CustomEvent('new-growl', { detail: {"text": text, "type": "info"} });
        window.dispatchEvent(event);
    }
    static success(text) {
        const event = new CustomEvent('new-growl', { detail: {"text": text, "type": "success"} });
        window.dispatchEvent(event);
    }
    static warning(text) {
        const event = new CustomEvent('new-growl', { detail: {"text": text, "type": "warning"} });
        window.dispatchEvent(event);
    }
}
class GrowlComponent extends LitElement {

    static get properties() {
        return { 
            growls: {type: Array}
        };
    }

    constructor() {
        super();
        this.growls = [];
        this.counter = 0;
        this.closed = 0;
        this.removeHandler = this.remove.bind(this);
    }

    connectedCallback() {
        super.connectedCallback();
        window.addEventListener('new-growl', this.growl);
    }

    disconnectedCallback() {
        window.removeEventListener('new-growl', this.growl);
        super.disconnectedCallback();
    }

    render() {
        return html`
            <div id="growl-container">
                ${repeat(this.growls, (growl) => growl.id, (growl, _) => 
                    html`
                    <div class="growl-row" id="${growl.id}" @mouseover="${() => { this.focus(growl.id); clearTimeout(growl.timeout); } }}"> 
                        <div class="growl growl-${growl.type}">
                            ${growl.text}
                        </div>
                        <div class="growl-close growl-close-${growl.type}">
                            <a @click="${() => this.close(growl.id)}"><span class="growl-close-icon"></span></a>
                        </div>
                    </div>
                `)}
            </div>
        `;
    }

    add = (growl) => {
        this.growls = [ ...this.growls, growl ];
        requestAnimationFrame(() => requestAnimationFrame(() => {
            const rows = this.shadowRoot.querySelectorAll('.growl-row:not(.growl-new)'); // only grab rows without growl-new
            Array.from(rows).forEach((row) => {
                row.classList.add('growl-new');
            });
        }));
    }

    close = (id) => {
        let target = this.shadowRoot.querySelector("[id=" + id + "]");
        target.addEventListener("transitionend", this.removeHandler);
        target.classList.add('growl-expire');
        this.closed++;
    }

    focus = (id) => {
        const growl = this.shadowRoot.getElementById(id);
        const body = growl.children[0];
        const button = growl.children[1];

        if (body.classList.contains('growl-focus') || body.classList.contains('growl-error')) return;

        body.classList.add('growl-focus');
        button.classList.add('growl-close-focus');
    }

    growl = (e) => {
        const text = e.detail.text;
        if (!text) {
            console.warn("No text provided to growl");
            return;
        }
        if (this.growls.some((growl) => { return growl.text == text})) return; // de-dupe

        const type = e.detail.type ? e.detail.type : 'info';
        const id = 'growl-' + this.counter++;
        const timeout = (type != "error") ? setTimeout(this.close, 5000, id) : -1; // error growls do not autoclose

        this.add({ "id": id, "text": text, "type": type, "timeout": timeout });
    }

    remove = (e) => {
        this.growls = this.growls.filter((growl) => growl.id != e.target.id);
    }

    // embed the css to keep the component self contained - this component uses the shadow DOM
    static get styles() {
        return css`
        #growl-container {
            position: fixed;
            top: 90px;
            right: 10px;
            z-index: 10;
            display: flex;
            flex-direction: column;
            overflow-x: hidden;
        }

        .growl-row {
            display: table-row;
            margin-bottom: 10px;
            box-shadow: 2px 2px 5px #888888;
            opacity: 0;
            transition: transform 500ms, opacity 500ms;
            transform: translateX(101%);
        }

        .growl-new {
            opacity: 1;
            transform: translateX(0%);
        }

        .growl-expire {
            opacity: 0;
            transform: translateX(101%);
        }

        .growl {
            border-radius: 3px 0px 0px 3px;
            height: 60px;
            width: 400px;
            min-width: 400px;
            padding: 6px;
            margin-bottom: 10px;
            display: table-cell;
            vertical-align: middle;
        }
        .growl-close {
            border-radius: 0px 3px 3px 0px;
            transition: color .1s;
            display: table-cell;
            vertical-align: middle;
            width: 20px;
        }
        .growl-close-icon {
            background-image: url("data:image/svg+xml,%3Csvg width='72px' height='72px' viewBox='0 0 72 72' id='emoji' xmlns='http://www.w3.org/2000/svg'%3E%3Cg id='color'/%3E%3Cg id='hair'/%3E%3Cg id='skin'/%3E%3Cg id='skin-shadow'/%3E%3Cg id='line'%3E%3Cline x1='17.5' x2='54.5' y1='17.5' y2='54.5' fill='none' stroke='%23000000' stroke-linecap='round' stroke-linejoin='round' stroke-miterlimit='10' stroke-width='2'/%3E%3Cline x1='54.5' x2='17.5' y1='17.5' y2='54.5' fill='none' stroke='%23000000' stroke-linecap='round' stroke-linejoin='round' stroke-miterlimit='10' stroke-width='2'/%3E%3C/g%3E%3C/svg%3E");
            background-repeat: no-repeat;
            background-size: 100%;
            display: inline-flex;
            height: 20px;
            opacity: .6;
            width: 20px;
            cursor: pointer;
        }
        .growl-close-icon:hover {
            opacity: 1;
        }

        .growl-error {
            background-color: #F9DEDE;
            border-top: solid 1px #E31C3D;
            border-bottom: solid 1px #E31C3D;
            border-left: solid 10px #E31C3D;
        }
        .growl-close-error {
            background-color: #F9DEDE;
            border-top: solid 1px #E31C3D;
            border-bottom: solid 1px #E31C3D;
            border-right: solid 1px #E31C3D;
            padding-right: 6px;
        }

        .growl-focus {
            border-top: solid 1px #5B616B !important;
            border-bottom: solid 1px #5B616B !important;
            border-left: solid 10px #5B616B !important;
        }
        .growl-close-focus {
            border-top: solid 1px #5B616B !important;
            border-bottom: solid 1px #5B616B !important;
            border-right: solid 1px #5B616B !important;
            padding-right: 6px;
        }

        .growl-info {
            background-color: #E1F3F8;
            border-top: solid 1px #02BFE7;
            border-bottom: solid 1px #02BFE7;
            border-left: solid 10px #02BFE7;
        }
        .growl-close-info {
            background-color: #E1F3F8;
            border-top: solid 1px #02BFE7;
            border-bottom: solid 1px #02BFE7;
            border-right: solid 1px #02BFE7;
            padding-right: 6px;
        }

        .growl-success {
            background-color: #E7F4E4;
            border-top: solid 1px #2E8540;
            border-bottom: solid 1px #2E8540;
            border-left: solid 10px #2E8540;
        }
        .growl-close-success {
            background-color: #E7F4E4;
            border-top: solid 1px #2E8540;
            border-bottom: solid 1px #2E8540;
            border-right: solid 1px #2E8540;
            padding-right: 6px;
        }

        .growl-warning {
            background-color: #FFF1D2;
            border-top: solid 1px #FDB81E;
            border-bottom: solid 1px #FDB81E;
            border-left: solid 10px #FDB81E;
        }
        .growl-close-warning {
            background-color: #FFF1D2;
            border-top: solid 1px #FDB81E;
            border-bottom: solid 1px #FDB81E;
            border-right: solid 1px #FDB81E;
            padding-right: 6px;
        }       
        `;
    }
}
customElements.define('growl-component', GrowlComponent);
