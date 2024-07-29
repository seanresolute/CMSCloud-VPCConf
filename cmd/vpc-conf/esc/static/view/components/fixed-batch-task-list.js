import { LitElement } from '../../lit-element/lit-element.js'
import { html } from '../../lit-html/lit-html.js'
import './batch-task-row.js';

class FixedBatchTaskList extends LitElement {
  static get properties() {
    return {
      tasks: {type: Object},
    }
  }

  render() {
    return html`
      <div class="customTable">
        <div class="customTableHead">
            <div class="customTableRow">
                <div class="customTableHeader">Type</div>
                <div class="customTableHeader">Added</div>
                <div class="customTableHeader">Queued</div>
                <div class="customTableHeader">In Progress</div>
                <div class="customTableHeader">Successful</div>
                <div class="customTableHeader">Failed</div>
                <div class="customTableHeader">Cancelled</div>
                <div class="customTableHeader">Action</div>
            </div>
        </div>
        <div class="customTableBody">
          ${this.tasks.map(task => {
            return html`
              <batch-task-row
                .task="${task}"
                class="customTableRow"
              >
              </batch-task-row>`
          })}
        </div>
      </div>
    `;
  }

  handleCancelTasksClick(taskIDs) {
    const cancelTasksEvent = new CustomEvent('cancel-click', { 
      detail: { taskIDs },
      bubbles: true,
    });
    this.dispatchEvent(cancelTasksEvent);
  }

  handleSubTasksClick(description, type, tasks) {
    const subTasksEvent = new CustomEvent('subtask-click', { 
      detail: { description, type, tasks },
      bubbles: true,
    });
    this.dispatchEvent(subTasksEvent);
  }

  createRenderRoot() {
    return this; // opt out of shadow DOM
  };
}

customElements.define('fixed-batch-task-list', FixedBatchTaskList);