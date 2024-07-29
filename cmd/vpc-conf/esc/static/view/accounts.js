import {html, render} from '../lit-html/lit-html.js';
import './components/shared/selectable-account-list.js';

import {HasModal, MakesAuthenticatedAJAXRequests} from './mixins.js';
import {Breadcrumb} from './components/shared/breadcrumb.js'

export function AccountsPage(info) {
	this._loginURL = info.ServerPrefix + 'oauth/callback';
	Object.assign(this, HasModal, MakesAuthenticatedAJAXRequests);

	this.init = async function(container) {
		Breadcrumb.set([{name: "Accounts"}]);
		render(
			html`
				<div id="background" class="hidden"></div>
				<div id="modal" class="hidden"></div>
				<div class="ds-l-container ds-u-padding--0">
					<div class="ds-u-display--flex">
						<div id="container">
							<div id="accounts"></div>
						</div>
					</div>
				</div>`,
			container)
		this._accounts = document.getElementById('accounts');
		this._modal = document.getElementById('modal');
		this._background = document.getElementById('background');

		container.addEventListener('account-selected', (e) => {
			const account = e.detail.account;
			window.location.href = `${info.ServerPrefix}accounts/${account.ID}`;
		});
		
		let accounts;
		try {
			const response = await this._fetchJSON(info.ServerPrefix + 'accounts/accounts.json');
			accounts = response.json;
		} catch (err) {
			alert('Error fetching account IDs: ' + err);
			return;
		}
		accounts = accounts || [];

		render(
			html`<selectable-account-list 
				.accounts="${accounts}"
				?renderLinks="${true}"
				serverPrefix="${info.ServerPrefix}"
			>
			</selectable-account-list>`,
			this._accounts
		);
	}
}
