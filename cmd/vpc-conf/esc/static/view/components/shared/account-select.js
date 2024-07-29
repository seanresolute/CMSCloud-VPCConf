import { LitElement, html } from '../../../lit-element/lit-element.js';
import './selectable-account-list.js';

class AccountSelect extends LitElement {
  static get properties() {
    return {
      accountInfo: { type: String },
      accounts: { type: Object },
    };
  }

  constructor() {
    super();
    this.accountInfo = "";
    this.showModal = false;
  }

  connectedCallback() {
    super.connectedCallback()
    this._background = document.getElementById('background');
    this.addEventListener('account-selected', this.handleAccountSelected);
  }

  disconnectedCallback() {
    this.removeEventListener('account-selected', this.handleAccountSelected);
    super.disconnectedCallback();
  }

  render() {
    return html`
    <div class="grid">
      <div class="row">
      <input id="selectedAccount" value="${this.accountInfo.trim()}" disabled /> <button type="button" @click=${e => {this.handleSelectAccountClick(e)}} class="ds-c-button ds-c-button--secondary">Select Account</button>
      </div>
    </div>
      ${this.showModal
        ? html`
        <div id="accountSelectModal">
          <div class="modalContainer">
            <div class="modalTitle">Select an account</div>
            <div class="modalBody">
              <selectable-account-list .accounts="${this.accounts}"></selectable-account-list> 
            </div>
          </div> 
        </div>`
        : ""
      }
    `;
  }

  handleAccountSelected(e) {
    this.handleCloseModal();
    const account = e.detail.account;
		this.accountInfo = `${account.ProjectName} | ${account.Name} | ${account.ID}`;
    let accountEvent = new CustomEvent("account-event", { bubbles: true, detail: account});
    this.dispatchEvent(accountEvent);
  }

  handleSelectAccountClick() {
    this.showModal = true;
    this._background.className = "";
    this.boundCloseHandler = this.handleCloseModal.bind(this);
    this._background.addEventListener('click', this.boundCloseHandler);
    this.requestUpdate();
  }

  handleCloseModal() {
    this.showModal = false;
    this._background.className = "hidden";
    this._background.removeEventListener('click', this.boundCloseHandler);
    this.requestUpdate();
  }

  createRenderRoot() {
    return this; // opt out of shadow DOM
  };
}
customElements.define('account-select', AccountSelect);

