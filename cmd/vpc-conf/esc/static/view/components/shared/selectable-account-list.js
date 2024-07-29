import { LitElement, html } from '../../../lit-element/lit-element.js';
import './account-filter.js';

class SelectableAccountList extends LitElement {
  static get properties() {
    return {
      accounts: { type: Object },
      renderLinks: { type: Boolean },
      serverPrefix: { type: String },
      sortProperty: { type: String },
      secondarySortProperty: { type: String },
      sortDirection: { type: Number },
    };
  }

  constructor() {
    super();
    this.filteredAccounts = null;
    this.addEventListener('filter-change', e => {
      this.filteredAccounts = e.detail.filteredAccounts;
      this.requestUpdate();
    });
    this.sortProperty = "ProjectName";
    this.secondarySortProperty = "ID";
    this.sortDirection = 1;
  }
  
  connectedCallback() {
    super.connectedCallback();
    this.filteredAccounts = this.accounts;
  }
  
  firstUpdated() {
    this.setHeaderListenersByProperty("ProjectName", "ID");
    this.setHeaderListenersByProperty("Name", "ID");
    this.setHeaderListenersByProperty("ID", "Name");
    this.setActiveHeaderClass();
  }

  render() {
    this.sortAccountsByProperty(this.sortProperty, this.secondarySortProperty, this.sortDirection);
    return html`
      <account-filter .accounts="${this.accounts}"></account-filter>
      <table id="accountSelectTable">
        <thead class="ds-u-fill--primary">
          <tr id="accountSelectHeader">
            <th colspan="2" data-property-name="ProjectName">Project</th>
            <th data-property-name="Name">Account Name</th>
            <th data-property-name="ID">Account ID</th>
          </tr>
        </thead>
        <tbody>
          ${this.filteredAccounts.map((account) => html`
            <tr class="accountSelectRow" @click="${() => this.handleAccountClick(account)}">
              <td>${account.ProjectName}</td>
              <td style="padding:0; padding-left: 5px;">${account.IsGovCloud ? html`<img src="/static/images/govcloud.png" height=20>`:''}</td>
              ${this.renderLinks ? 
                html`
                  <td><a href="${this.serverPrefix}accounts/${account.ID}" @click="${(e) => this.handleLinkClick(e, account.ID)}">${account.Name}</a></td>
                  <td><a href="${this.serverPrefix}accounts/${account.ID}" @click="${(e) => this.handleLinkClick(e, account.ID)}">${account.ID}</a></td>
                ` :
                html`
                  <td>${account.Name}</td>
                  <td>${account.ID}</td>
                `
              } 
            </tr>
          `)}
        </tbody>
      </table>  
    `;
  }

  handleAccountClick(account) {
    const accountSelectedEvent = new CustomEvent('account-selected', { 
      detail: { account },
      bubbles: true,
    });
    this.dispatchEvent(accountSelectedEvent);
  }

  handleLinkClick(e) {
    // Let the default link click handler handle the click
    // if shift/control/alt/command is pressed. Otherwise
    // let it bubble to a row click.
    if (e.metaKey || e.altKey || e.shiftKey || e.ctrlKey) {
      e.stopPropagation();
    } else {
      e.preventDefault();
    }
  }

  sortAccountsByProperty(property, secondaryProperty, direction) {
    this.filteredAccounts.sort((a1, a2) => {
      if (a1[property] == a2[property]) return (a1[secondaryProperty] < a2[secondaryProperty] ? -direction : direction)
      return a1[property] < a2[property] ? -direction : direction;
    });
  }

  setActiveHeaderClass() {
    const currentHeader = document.querySelector(`th[data-property-name=${this.sortProperty}`);
    currentHeader.className = this.sortDirection === 1 ? 'asc' : 'desc';
  }

  setHeaderListenersByProperty(property, secondaryProperty) {
    const currentHeader = document.querySelector(`th[data-property-name=${property}`);
    const currentProperty = currentHeader.dataset.propertyName;

    currentHeader.addEventListener('click', () => {
      this.querySelectorAll('th').forEach(header => { header.className = '' });
      
      if (this.sortProperty !== currentProperty) {
        currentHeader.className = 'asc';
        this.sortDirection = 1;
      } else {
        if (this.sortDirection === 1) {
          currentHeader.className = 'desc';
          this.sortDirection = -1;
        } else if (this.sortDirection === -1) {
          currentHeader.className = 'asc';
          this.sortDirection = 1;
        }; 
      }

      this.sortProperty = property;
      this.secondarySortProperty = secondaryProperty;
    });
  };

  createRenderRoot() {
    return this; // opt out of shadow DOM
  };
}

customElements.define('selectable-account-list', SelectableAccountList);
