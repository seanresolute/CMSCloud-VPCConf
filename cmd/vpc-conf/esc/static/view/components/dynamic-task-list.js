import { html, nothing } from '../../lit-html/lit-html.js'
import { LitElement } from '../../lit-element/lit-element.js'
import {Growl} from './shared/growl.js'

class DynamicTaskList extends LitElement {
  static get properties() {
    return {
      baseTaskURL: { type: String },
      beforeID: { type: Number },
      fetchJSON: { type: Object },
      selectedTaskID: { type: Number },
    }
  }

  constructor() {
    super();
    this.tasks = [];
    this.selectedTaskID = null;
    this.nextBeforeID = null;
  }

  async firstUpdated() {
    await this.loadMore();
    this.selectedTaskID = this.tasks[0].ID;
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
      <div class="modalTitle">All Tasks</div>
      <div class="modalBody" style="padding: 0px">        
        <table class="standard-table task-log-table selectableTable">
          <thead>
              <tr><th>Task</th><th>Status</th></tr>
          </thead>
          <tbody>
            ${this.tasks.map(task => html`
              <tr 
                class="${this.selectedTaskID === task.ID ? "selectedRow" : nothing}"
                @click="${(e) => this.selectedTaskID = task.ID}"
              >
                  <td>${task.Description}</td>
                  <td>${task.Status}</td>
                      ${task.Status == 'Queued'
                      ? html`<button ds-c-button ds-c-button--hover ds-c-button--primary ds-c-button--small type="button" @click="${() => this.handleCancelTaskClick([task.ID])}">Cancel</button>`
                      : nothing}
                  </td>
              </tr>
            `)}
            </tbody>
        </table>
      <div id="loading" class="hidden">Loading...</div>
        <button id="loadMore" class="ds-c-button ds-c-button--hover ds-c-button--primary ds-c-button--small hidden ds-u-float--right ds-u-margin--1" @click="${this.loadMore}">Load more tasks</button>
      </div>
    </div>
    `;
  }
  
  async loadMore() {
    const loading = this.querySelector('#loading');
    const loadMoreButton = this.querySelector('#loadMore');
    loading.className = '';

    const url = this.baseTaskURL + '?beforeID=' + (this.nextBeforeID !== null ? this.nextBeforeID : this.beforeID);

    const clearErrorEvent = new CustomEvent('new-fetch-request', { 
      bubbles: true,
    });
    this.dispatchEvent(clearErrorEvent);

    let data;
    try {
      const response = await this.fetchJSON(url);
      data = response.json;  
    } catch (err) {
      Growl.error('Error fetching: ' + err);
      return;
    }  
    this.tasks = [...this.tasks, ...data.Tasks];
    this.requestUpdate();
    loading.classList.add('hidden');
  
    if (data.IsMoreTasks) {
      loadMoreButton.classList.remove('hidden');
      this.nextBeforeID = data.Tasks.map(t => t.ID).reduce((v, id) => Math.min(v, id));
    } else {
      loadMoreButton.classList.add('hidden');
    }
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

customElements.define('dynamic-task-list', DynamicTaskList);