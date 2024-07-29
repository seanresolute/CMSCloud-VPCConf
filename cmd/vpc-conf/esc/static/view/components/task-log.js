import { LitElement } from '../../lit-element/lit-element.js';
import { html } from '../../lit-html/lit-html.js';
import {Growl} from './shared/growl.js'

class TaskLog extends LitElement {
  static get properties() {
    return {
      baseTaskURL: { type: String },
      logID: { type: Number },
      fetchJSON: { type: Object },
    }
  }

  constructor() {
    super();
    this.refetchLogsTimeoutID = null;
    this.shouldChangeScroll = null;
    this.task = null;
  }

  updated(changedProperties) {
    changedProperties.forEach((oldValue, propName) => {
      if (propName === "logID" && oldValue !== this.logID && this.logID) {
        this.task = null;
        if (this.refetchLogsTimeoutID) {
          window.clearTimeout(this.refetchLogsTimeoutID);
        };
        this.fetchTaskLogs(this.logID)
      }
    });
    this.changeScroll();
  }

  disconnectedCallback() {
    window.clearTimeout(this.refetchLogsTimeoutID);
  }

  render() { 
    this.checkScroll(); 
    return html` 
      <div>
      ${this.task ?
        html`
        <div class="modalTitle">${this.task.Description} - ${this.task.Status}</div>
        <div class="modalBody">
            <table class="standard-table task-log-table">
                <tbody>
                ${this.task.Log.map((entry) => (
                    html`<tr><td style="width: 250px;">${(new Date(entry.Time)).toUTCString()}</td><td>${entry.Message}</td></tr>`
                ))}
                ${this.inProgress() ? "" : html`<tr><td colspan=2><b>Task ${this.task.Status}</b></td></tr>`}
                </tbody>
            </table>
        </div>
        ` :
        ""
      } 
      </div>
    `;
  }

  async fetchTaskLogs(id) {
    const clearErrorEvent = new CustomEvent('new-fetch-request', { 
      bubbles: true,
    });
    this.dispatchEvent(clearErrorEvent);
    
    const url = this.baseTaskURL + id + '.json';     
    let task;
    try {
      const response = await this.fetchJSON(url);
      task = response.json;
    } catch (err) {
      Growl.error('Error fetching: ' + err);
      return;
    }
    task.Status = [
      "Queued",
      "In progress",
      "Successful",
      "Failed",
      "Cancelled",
    ][task.Status];
    task.Log = task.Log || [];
    this.task = task;
    this.requestUpdate(); 

    if (this.inProgress()) {
      this.refetchLogsTimeoutID = setTimeout(() => this.fetchTaskLogs(id), 1000);
    };
  };

  inProgress() {
    return this.task.Status == "In progress" || this.task.Status == "Queued";
  }

  checkScroll() {
    // Only scroll to the bottom if the user hasn't manually scrolled up.
    this.shouldChangeScroll = true;
    if (this.querySelector('.modalBody')) {
      const modalBody = this.querySelector('.modalBody');
      if (modalBody.scrollHeight - modalBody.clientHeight != modalBody.scrollTop) {
        this.shouldChangeScroll = false;
      };
    };
  }

  changeScroll() {
    const modalBody = this.querySelector('.modalBody');
    if (modalBody && this.shouldChangeScroll) {
        modalBody.scrollTop = this.querySelector('table').clientHeight;
    };
  }

  createRenderRoot() {
    return this;  // opt out of shadow DOM
  };
}

customElements.define('task-log', TaskLog);
