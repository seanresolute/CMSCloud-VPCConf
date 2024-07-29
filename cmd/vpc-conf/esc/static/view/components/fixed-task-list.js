import { LitElement } from '../../lit-element/lit-element.js'
import { html } from '../../lit-html/lit-html.js'
import { User } from '../user.js'

class FixedTaskList extends LitElement {
  static get properties() {
    return {
      tasks: {type: Object},
    }
  }

  render() {
    return html`
      <table class="standard-table">
        <thead>
            <tr><th>Task</th><th>Status</th><th>Actions</th></tr>
        </thead>
        <tbody>
          ${this.tasks.map(task => html`
            <tr>
              <td>${task.Description}</td>
              <td>${task.Status}</td>
              <td>
                <button type="button" class="ds-c-button ds-c-button--primary ds-c-button--small ${!User.isAdmin() || task.Status != 'Queued' ? 'ds-c-button--disabled' : ''}" @click="${() => this.handleCancelTaskClick([task.ID])}">Cancel Queued</button>
                <button class="ds-c-button ds-c-button--primary ds-c-button--small" @click="${() => this.handleShowTaskClick(task.ID)}">Show Log</button>
              </td>
            </tr>
          `)}
          </tbody>
      </table>
    `;
  }

  handleShowTaskClick(logID) {
    const logIDChangeEvent = new CustomEvent('show-task-click', { 
      detail: { logID },
      bubbles: true,
    });
    this.dispatchEvent(logIDChangeEvent);
  }

  handleCancelTaskClick(taskIDs) {
    const cancelTasksEvent = new CustomEvent('cancel-click', { 
      detail: { taskIDs },
      bubbles: true,
    });
    this.dispatchEvent(cancelTasksEvent);
  }

  createRenderRoot() {
    return this; // opt out of shadow DOM;
  };
}

customElements.define('fixed-task-list', FixedTaskList);