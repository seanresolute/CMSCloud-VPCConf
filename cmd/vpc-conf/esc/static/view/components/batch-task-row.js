import { html } from '../../lit-html/lit-html.js'
import { LitElement } from '../../lit-element/lit-element.js'

class BatchTaskRow extends LitElement {
  static get properties() {
    return {
      task: { type: Object },
    }
  }

  render() {
    this.computeStats();
    return html`
      <div class="customTableData" style="padding-right:5px">${this.task.Description}</div>
      <div class="customTableData" style="padding-right:5px">${(new Date(this.task.AddedAt).toLocaleString('en-US', { hour12: false }))}</div>
      <div class="customTableData center">
          ${this.task.QueuedTasks.length
              ? html`<a href="#" @click="${(e) => {e.preventDefault(); this.handleSubTasksClick(this.task.Description, "Queued", this.task.QueuedTasks);}}">${this.task.QueuedTasks.length}</a>`
          : '0'}
      </div>
      <div class="customTableData center">
          ${this.task.InProgressTasks.length
              ? html`<a href="#" @click="${(e) => {e.preventDefault(); this.handleSubTasksClick(this.task.Description, "In Progress", this.task.InProgressTasks);}}">${this.task.InProgressTasks.length}</a>`
          : '0'}
      </div>
      <div class="customTableData center">
          ${this.task.SuccessfulTasks.length
              ? html`<a href="#" @click="${(e) => {e.preventDefault(); this.handleSubTasksClick(this.task.Description, "Successful", this.task.SuccessfulTasks);}}">${this.task.SuccessfulTasks.length}</a>`
          : '0'}
      </div>
      <div class="customTableData center">
          ${this.task.FailedTasks.length
              ? html`<a href="#" @click="${(e) => {e.preventDefault(); this.handleSubTasksClick(this.task.Description, "Failed", this.task.FailedTasks);}}">${this.task.FailedTasks.length}</a>`
          : '0'}
      </div>
      <div class="customTableData center">
          ${this.task.CancelledTasks.length
              ? html`<a href="#" @click="${(e) => {e.preventDefault(); this.handleSubTasksClick(this.task.Description, "Cancelled", this.task.CancelledTasks);}}">${this.task.CancelledTasks.length}</a>`
          : '0'}
      </div>
      <div class="customTableData">
         <button type="button" ?disabled=${this.task.QueuedTasks.length == 0} @click="${() => this.handleCancelTasksClick(this.task.QueuedTasks.map(t => t.ID))}">Cancel Queued</button>
      </div>
    `;
  }

  computeStats() {
    this.task.Tasks = this.task.Tasks || [];
    this.task.QueuedTasks = this.task.Tasks.filter(t => t.Status == "Queued");
    this.task.InProgressTasks = this.task.Tasks.filter(t => t.Status == "In progress");
    this.task.SuccessfulTasks = this.task.Tasks.filter(t => t.Status == "Successful");
    this.task.FailedTasks = this.task.Tasks.filter(t => t.Status == "Failed");
    this.task.CancelledTasks = this.task.Tasks.filter(t => t.Status == "Cancelled");
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

customElements.define('batch-task-row', BatchTaskRow);