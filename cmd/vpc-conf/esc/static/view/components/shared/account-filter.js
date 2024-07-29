import { LitElement, html } from '../../../lit-element/lit-element.js';

class AccountFilter extends LitElement {
  static get properties() {
    return {
      accounts: { type: Object },
    };
  }
  
  connectedCallback() {
    super.connectedCallback();
    this.filteredAccounts = this.accounts;
  }
  
  firstUpdated() {
    this.querySelector('form').addEventListener('submit', (e) => e.preventDefault());
    const accountFilterInput = this.querySelector('input');
    accountFilterInput.focus();
  }

  render() {
    return html`
      <form>
        <div class="ds-u-clearfix">
          <input type="text" style="margin: -1px 0px" class="ds-c-field ds-u-display--inline-block" @input=${e => this.handleFilterText(e)} placeholder="Filter by Project, Account Name or ID" title="Filter by">
          <button type="button" style="margin-left: 4px" class="ds-c-button ds-c-button--medium" @click=${e => this.handleClearFilter(e)}>Clear</button>
        </div>
      </form>
    `;
  }

  handleFilterText(e) {
    const fields = ["ProjectName", "Name", "ID"]
    const filterText = e.target.value.toLowerCase();

    this.filteredAccounts = this.accounts.filter(account => {
      return fields.some(field => {
        return account[field].toLowerCase().indexOf(filterText) > -1;
      })
    });
    this.fireFilterChangeEvent();
  }

  handleClearFilter() {
    this.querySelector('input').value = '';
    this.querySelector('input').focus();
    this.filteredAccounts = this.accounts;
    this.fireFilterChangeEvent();
  }

  fireFilterChangeEvent() {
    const filterChangeEvent = new CustomEvent('filter-change', { 
      detail: { filteredAccounts: this.filteredAccounts },
      bubbles: true,
    });
    this.dispatchEvent(filterChangeEvent);
  }

  createRenderRoot() {
    return this; // opt out of shadow DOM
  };
}

customElements.define('account-filter', AccountFilter);
