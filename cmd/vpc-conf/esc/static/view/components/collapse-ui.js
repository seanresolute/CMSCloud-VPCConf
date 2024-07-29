import { LitElement, html } from '../../lit-element/lit-element.js';

class CollapseUI extends LitElement {
    static get properties() {
        return {
            collapsed: {type: Boolean},
            title: {type: String}
        };
    }

    render() {
        return html`            
            <div style="background-color: #112E51;color: #fff;font-size: 18px;padding: 4px;"> 
                <button id="button-expand" style="border: 1px solid #fff; background-color: #fff; cursor: pointer; margin-bottom:3px;" @click="${e => {this.toggleExpand(e)}}"  type="button" ?hidden="${!this.collapsed}">Expand</button>
                <button id="button-collapse" style="border: 1px solid #fff; background-color: #fff; cursor: pointer; margin-bottom:3px;" @click="${e => {this.toggleCollapse(e)}}" type="button" ?hidden="${this.collapsed}">Collapse</button> 
                ${this.title}
            </div>

            <span role="region" ?hidden="${this.collapsed}"> 
                <slot></slot>
            </span>

            <hr/>
        `;
    }

    constructor() {
        super();
        this.collapsed = true;
    }

    toggleExpand(e) {
        this.collapsed = false;
    }

    toggleCollapse(e){
        this.collapsed = true;
    }
   
}
customElements.define('collapse-ui', CollapseUI);