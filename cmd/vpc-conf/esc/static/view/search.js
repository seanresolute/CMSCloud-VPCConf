import {html, render} from '../lit-html/lit-html.js';
import {Breadcrumb} from './components/shared/breadcrumb.js'
import './components/search-ui.js'

export function SearchPage(info) {
    this.init = async function(container) {        
        Breadcrumb.set([{name: "Search"}]);
        render(
            html`<search-ui .info="${info}"></search-ui>`, container);
    }
}
