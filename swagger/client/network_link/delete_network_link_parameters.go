// Code generated by go-swagger; DO NOT EDIT.

package network_link

// This file was generated by the swagger tool.
// Editing this file might prove futile when you re-run the swagger generate command

import (
	"context"
	"net/http"
	"time"

	"github.com/go-openapi/errors"
	"github.com/go-openapi/runtime"
	cr "github.com/go-openapi/runtime/client"

	strfmt "github.com/go-openapi/strfmt"
)

// NewDeleteNetworkLinkParams creates a new DeleteNetworkLinkParams object
// with the default values initialized.
func NewDeleteNetworkLinkParams() *DeleteNetworkLinkParams {
	var ()
	return &DeleteNetworkLinkParams{

		timeout: cr.DefaultTimeout,
	}
}

// NewDeleteNetworkLinkParamsWithTimeout creates a new DeleteNetworkLinkParams object
// with the default values initialized, and the ability to set a timeout on a request
func NewDeleteNetworkLinkParamsWithTimeout(timeout time.Duration) *DeleteNetworkLinkParams {
	var ()
	return &DeleteNetworkLinkParams{

		timeout: timeout,
	}
}

// NewDeleteNetworkLinkParamsWithContext creates a new DeleteNetworkLinkParams object
// with the default values initialized, and the ability to set a context for a request
func NewDeleteNetworkLinkParamsWithContext(ctx context.Context) *DeleteNetworkLinkParams {
	var ()
	return &DeleteNetworkLinkParams{

		Context: ctx,
	}
}

// NewDeleteNetworkLinkParamsWithHTTPClient creates a new DeleteNetworkLinkParams object
// with the default values initialized, and the ability to set a custom HTTPClient for a request
func NewDeleteNetworkLinkParamsWithHTTPClient(client *http.Client) *DeleteNetworkLinkParams {
	var ()
	return &DeleteNetworkLinkParams{
		HTTPClient: client,
	}
}

/*DeleteNetworkLinkParams contains all the parameters to send to the API endpoint
for the delete network link operation typically these are written to a http.Request
*/
type DeleteNetworkLinkParams struct {

	/*DeleteParameters
	  The input describing the network link to be deleted. Specify the name.

	*/
	DeleteParameters DeleteNetworkLinkBody

	timeout    time.Duration
	Context    context.Context
	HTTPClient *http.Client
}

// WithTimeout adds the timeout to the delete network link params
func (o *DeleteNetworkLinkParams) WithTimeout(timeout time.Duration) *DeleteNetworkLinkParams {
	o.SetTimeout(timeout)
	return o
}

// SetTimeout adds the timeout to the delete network link params
func (o *DeleteNetworkLinkParams) SetTimeout(timeout time.Duration) {
	o.timeout = timeout
}

// WithContext adds the context to the delete network link params
func (o *DeleteNetworkLinkParams) WithContext(ctx context.Context) *DeleteNetworkLinkParams {
	o.SetContext(ctx)
	return o
}

// SetContext adds the context to the delete network link params
func (o *DeleteNetworkLinkParams) SetContext(ctx context.Context) {
	o.Context = ctx
}

// WithHTTPClient adds the HTTPClient to the delete network link params
func (o *DeleteNetworkLinkParams) WithHTTPClient(client *http.Client) *DeleteNetworkLinkParams {
	o.SetHTTPClient(client)
	return o
}

// SetHTTPClient adds the HTTPClient to the delete network link params
func (o *DeleteNetworkLinkParams) SetHTTPClient(client *http.Client) {
	o.HTTPClient = client
}

// WithDeleteParameters adds the deleteParameters to the delete network link params
func (o *DeleteNetworkLinkParams) WithDeleteParameters(deleteParameters DeleteNetworkLinkBody) *DeleteNetworkLinkParams {
	o.SetDeleteParameters(deleteParameters)
	return o
}

// SetDeleteParameters adds the deleteParameters to the delete network link params
func (o *DeleteNetworkLinkParams) SetDeleteParameters(deleteParameters DeleteNetworkLinkBody) {
	o.DeleteParameters = deleteParameters
}

// WriteToRequest writes these params to a swagger request
func (o *DeleteNetworkLinkParams) WriteToRequest(r runtime.ClientRequest, reg strfmt.Registry) error {

	if err := r.SetTimeout(o.timeout); err != nil {
		return err
	}
	var res []error

	if err := r.SetBodyParam(o.DeleteParameters); err != nil {
		return err
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}
