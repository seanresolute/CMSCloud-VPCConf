import { LitElement } from '../../lit-element/lit-element.js'
import { html, nothing } from '../../lit-html/lit-html.js'

class FixedSubtaskList extends LitElement {
  static get properties() {
    return {
      tasks: { type: Object },
      serverPrefix: { type: String },
      selectedTaskID: { type: Number },
      title: { type: String },
    }
  }

  updated(changedProperties) {
    changedProperties.forEach((_, propName) => {
      if (propName === "selectedTaskID" && this.selectedTaskID) {
        const logIDChangeEvent = new CustomEvent('log-id-change', { 
          detail: { newLogId: this.selectedTaskID },
          bubbles: true,
        });
        this.dispatchEvent(logIDChangeEvent);
      }
    });
  }

  render() {
    return html`
    <div class="modalContainer">
      <div class="modalTitle">${this.title}</div>
      <div class="modalBody">
        <table class="selectableTable">
          <thead>
              <tr><th>Account/VPC</th><th>Task</th><th>Status</th></tr>
          </thead>
          <tbody>
          ${this.tasks.map(task => html`
            <tr 
              class="${this.selectedTaskID === task.ID ? "selectedRow" : nothing}"
              @click="${(e) => this.selectedTaskID = task.ID }"
            >
                <td>
                    ${task.VPCID === null
                      ? html`<a href="${this.serverPrefix}accounts/${task.AccountID}">${task.AccountID}</a>`
                      : html`<a href="${this.serverPrefix}/accounts/${task.AccountID}/vpc/${task.VPCRegion}/${task.VPCID}">${task.VPCID}</a>`
                    }
                </td>
                <td>${task.Description}</td>
                <td>${task.Status}</td>
            </tr>
          `)}
          </tbody>
        </table>
      </div>
    </div>
    `;
  }

  createRenderRoot() {
    return this; // opt out of shadow DOM;
  };
}

customElements.define('fixed-subtask-list', FixedSubtaskList);
