import { LitElement } from '../../lit-element/lit-element.js';
import { html } from '../../lit-html/lit-html.js';
import './task-log.js';

class ListLogContainer extends LitElement {
  static get properties() {
    return {
      baseTaskURL: {type: String},
      taskList: {type: Object},
      fetchJSON: { type: Object },
    }
  }

  constructor() {
    super();
    this.logID = null;
    this.addEventListener('log-id-change', e => {
      this.logID = e.detail.newLogId;
      this.requestUpdate();
    })
  }
  
  render() {
    return html` 
      ${this.taskList}
      <task-log 
        baseTaskURL="${this.baseTaskURL}"
        logID="${this.logID}"
        .fetchJSON="${this.fetchJSON}"
        >
      </task-log>
    `;
  }

  createRenderRoot() {
    return this; // opt out of shadow DOM
  };
}

customElements.define('list-log-container', ListLogContainer);
