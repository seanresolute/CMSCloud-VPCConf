import { LitElement, html } from '../../lit-element/lit-element.js';
import {Growl} from '../components/shared/growl.js';

class LabelUI extends LitElement {

  constructor() {
    super();
    this.allLabels = [];
    this.labelsInfo = [];
  }

  static get properties() {
    return {
      fetchJSON: { type: Object },
    }
  }

  firstUpdated() {
    const newFetchEvent = new CustomEvent('new-fetch-request', { 
      bubbles: true,
    });
    this.dispatchEvent(newFetchEvent);
    this.updateLabels();
  }

  render() {
    return html`
      <label for="label-input-container">Type label and press ENTER or select from the list below: </label>
      <div id="input-container" class="input-container ds-l-col--4 ds-u-padding--0">
        <input id="labelsInput" list="all-labels" name="label-input-container" class="input-container" />
      </div>
      <div id="label-container" class="label-container ds-l-col--12 ds-u-padding--0">
        ${this.labelsInfo.map(label => html`
        <div class="label"><span>${label.Name}<div class="close" @click="${() => this.deleteLabel(label.Name) }" data-id="${label.Name}"></div></span></div>
        `)}
      </div>

      <datalist id="all-labels">
        ${this.allLabels.map(label => html`
        <option value="${label.Name}">
        `)}
      </datalist>
    `;
  }

  async updateLabels() {
    try {
      let allLabelsURL = this.info.ServerPrefix + 'labels.json'; 
      let pageLabelsURL = '';
      if (this.info.page == 'vpc') {
        pageLabelsURL = this.info.ServerPrefix + 'labels/' + this.info.Region + '/' + this.info.VPCID;
      } else {
        pageLabelsURL = this.info.ServerPrefix + 'labels/' + this.info.AccountID;
      }
      let response = await Promise.all([this.fetchJSON(allLabelsURL), this.fetchJSON(pageLabelsURL)]);
      this.allLabels = response[0].json;
      this.labelsInfo = response[1].json;
    } catch (err) {
      Growl.error('Error getting labels: ' + err);
    }

    this._labelsInput = document.getElementById('labelsInput');
    this._labelsInput.addEventListener('keyup', (e) => {
      this.checkLabels(e, false);
    });

    self=this;
    this._labelsInput.addEventListener('input', (e) => {
      const dataList = document.getElementById('all-labels').childNodes;
      for (const entry of dataList) {
        if (entry.value == self._labelsInput.value) {
          self.checkLabels(e, true);
          break;
        }
      }
    });

    this.requestUpdate(); 
  }

  checkLabels(event, optionSelected) {
    const keyPressedIsEnter = event.key === 'Enter';
    if (keyPressedIsEnter || optionSelected) {
      let label = this._labelsInput.value.toLowerCase().replace(/[^a-z0-9 ]/g, "").trim();
      if(label != "" && this.labelsInfo.filter(l => l.Name == label).length == 0) {
        this.setLabel(label);
        this._labelsInput.value = '';
        this._labelsInput.blur();
      } else if (this.labelsInfo.filter(l => l.Name == label).length > 0) {
        Growl.warning('Label "' + label + '" already exists');
      }
    }
  }

  async setLabel(label) {
    if (label != '') {
        let url = ''
        if (this.info.page == 'vpc') {
          url = this.info.ServerPrefix + 'labels/' + this.info.Region + '/' + this.info.VPCID + '/' + label;
        } else { 
          url = this.info.ServerPrefix + 'labels/' + this.info.AccountID + '/' + label;
        }
        try {
          await this.fetchJSON(url, {method: 'POST'});
        } catch (err) {
          Growl.warning('Error setting label: ' + err );
        }
        this.updateLabels()
    }
  }

  async deleteLabel(label) {
    if (label != '') {
      let url = '';
      if (this.info.page == 'vpc') {
        url = this.info.ServerPrefix + 'labels/' + this.info.Region + '/' + this.info.VPCID + '/' + label;
      } else {
        url = this.info.ServerPrefix + 'labels/' + this.info.AccountID + '/' + label;
      }
      try {
        await this.fetchJSON(url, {method: 'DELETE'});
      } catch (err) {
        Growl.warning('Error deleting label: ' + err );
      }
      this.updateLabels()
    }
  }

  createRenderRoot() {
    return this; // opt out of shadow DOM
  };
}

customElements.define('label-ui', LabelUI);
