import { html } from '../../lit-html/lit-html.js'
import { LitElement } from '../../lit-element/lit-element.js'
import {Growl} from './shared/growl.js'
import './batch-task-row.js';

class DynamicBatchTaskList extends LitElement {
  static get properties() {
    return {
      baseTaskURL: { type: String },
      beforeID: { type: Number },
      fetchJSON: { type: Object },
    }
  }

  constructor() {
    super();
    this.tasks = [];
    this.nextBeforeID = null;
  }
  
  firstUpdated() {
    this.loadMore();
  }

  render() {
    return html`
    <div class="modalContainer">
      <div class="modalTitle">All Batch Tasks</div>
      <div class="modalBody">
        <div class="customTable">
          <div class="customTableHead">
              <div class="customTableRow">
                  <div class="customTableHeader">Type</div>
                  <div class="customTableHeader">Added</div>
                  <div class="customTableHeader">Queued</div>
                  <div class="customTableHeader">In progress</div>
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
        <div id="loading" class="hidden">Loading...</div>
        <button id="loadMore" class="ds-c-button ds-c-button-primary ds-c-button--small hidden" @click="${this.loadMore}">Load more tasks</button>
      </div>
    </div> 
    `;
  }

  async loadMore() {
    const loading = this.querySelector('#loading');
    const loadMoreButton = this.querySelector('#loadMore');
    loading.className = '';

    const url = this.baseTaskURL + '?beforeID=' + (this.nextBeforeID !== null ? this.nextBeforeID : this.beforeID);

    const newFetchEvent = new CustomEvent('new-fetch-request', { 
      bubbles: true,
    });
    this.dispatchEvent(newFetchEvent);

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

    loading.className = 'hidden';
    if (data.IsMoreTasks) {
      loadMoreButton.classList.remove('hidden');
      this.nextBeforeID = data.Tasks.map(t => t.ID).reduce((v, id) => Math.min(v, id));
    } else {
      loadMoreButton.classList.add('hidden');
    }
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

customElements.define('dynamic-batch-task-list', DynamicBatchTaskList);
