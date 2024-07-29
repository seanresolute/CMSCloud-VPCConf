# Frontend Architecture

The UI code is written almost entirely in pure Javascript. We use [a very small third-party library](https://lit-html.polymer-project.org/) that leverages Javascript [template literals](https://developer.mozilla.org/en-US/docs/Web/JavaScript/Reference/Template_literals) to implement easy and clean HTML templates in Javascript, and a [simple base class](https://lit-element.polymer-project.org/) for creating reusable custom [web components](https://developer.mozilla.org/en-US/docs/Web/Web_Components).

## Routing

The design is not technically a single-page application, but the server actually responds to almost every (browser-visible) URL with the same effectively static content (`index.tpl`) and then a client-side router (`router.js`) loads the script (from the `view` directory) that controls what the user actually sees. This could be converted to a single-page app with minimal effort.

## Views

Each page has a script in `static/views` that controls its operation. To be clear, this script controls all the rendering and activity on that page, so it is more than what would be called a "view" in most [architectural patterns](https://en.wikipedia.org/wiki/Architectural_pattern) that use that word.

Typically views perform authenticated AJAX requests to get the user-specific data required to render the current page, often repeating them to update the data "live." (We currently do this via polling, not via any of the more sophisticated "pushing" methods.) Then they make further AJAX requests in response to user actions and continue to update the page. The templating library we use can efficiently re-render the same template with new data, so that is our typical approach to updating the page: just update the underlying data (or receive updated data from the server) and re-render the part that we want to reflect the changes.

Note that we generally don't use the generated HTML as a source of any data (except for user input); if we need to update the data client-side after the initial rendering then we save the data as a Javascript variable/property and reuse it later.

## Mixins

`mixins.js` contains common functionality that is used across multiple views. We structure them as mixins/traits so that they can refer to each other and so that each view can choose which functionality it needs.

## Authentication

Because only the AJAX endpoints require authentication, it is handled by a mixin (`MakesAuthenticatedAJAXRequests`) which provides a wrapper for [`fetch`](https://developer.mozilla.org/en-US/docs/Web/API/Fetch_API). The wrapper automatically throws up a login box if receives a 401 response from the server, logs the user in via another AJAX request, and then repeats the previously attempted request after successful login. This allows views to use the wrapper as a drop-in replacement for `fetch` that abstracts away authentication.

## Async style

We generally do network I/O with [`async` functions](https://developer.mozilla.org/en-US/docs/Web/JavaScript/Reference/Statements/async_function) that `await` Promises from other `async` functions, because it makes the code easier to follow and we rarely need to do anything with Promises other than wait for them to resolve.
