// Code generated by go-swagger; DO NOT EDIT.

package network_service

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

// NewDeleteNetServiceParams creates a new DeleteNetServiceParams object
// with the default values initialized.
func NewDeleteNetServiceParams() *DeleteNetServiceParams {
	var ()
	return &DeleteNetServiceParams{

		timeout: cr.DefaultTimeout,
	}
}

// NewDeleteNetServiceParamsWithTimeout creates a new DeleteNetServiceParams object
// with the default values initialized, and the ability to set a timeout on a request
func NewDeleteNetServiceParamsWithTimeout(timeout time.Duration) *DeleteNetServiceParams {
	var ()
	return &DeleteNetServiceParams{

		timeout: timeout,
	}
}

// NewDeleteNetServiceParamsWithContext creates a new DeleteNetServiceParams object
// with the default values initialized, and the ability to set a context for a request
func NewDeleteNetServiceParamsWithContext(ctx context.Context) *DeleteNetServiceParams {
	var ()
	return &DeleteNetServiceParams{

		Context: ctx,
	}
}

// NewDeleteNetServiceParamsWithHTTPClient creates a new DeleteNetServiceParams object
// with the default values initialized, and the ability to set a custom HTTPClient for a request
func NewDeleteNetServiceParamsWithHTTPClient(client *http.Client) *DeleteNetServiceParams {
	var ()
	return &DeleteNetServiceParams{
		HTTPClient: client,
	}
}

/*DeleteNetServiceParams contains all the parameters to send to the API endpoint
for the delete net service operation typically these are written to a http.Request
*/
type DeleteNetServiceParams struct {

	/*DeleteParameters
	  The input describing the network service to be deleted. Specify name and type, if not DHCP.

	*/
	DeleteParameters DeleteNetServiceBody

	timeout    time.Duration
	Context    context.Context
	HTTPClient *http.Client
}

// WithTimeout adds the timeout to the delete net service params
func (o *DeleteNetServiceParams) WithTimeout(timeout time.Duration) *DeleteNetServiceParams {
	o.SetTimeout(timeout)
	return o
}

// SetTimeout adds the timeout to the delete net service params
func (o *DeleteNetServiceParams) SetTimeout(timeout time.Duration) {
	o.timeout = timeout
}

// WithContext adds the context to the delete net service params
func (o *DeleteNetServiceParams) WithContext(ctx context.Context) *DeleteNetServiceParams {
	o.SetContext(ctx)
	return o
}

// SetContext adds the context to the delete net service params
func (o *DeleteNetServiceParams) SetContext(ctx context.Context) {
	o.Context = ctx
}

// WithHTTPClient adds the HTTPClient to the delete net service params
func (o *DeleteNetServiceParams) WithHTTPClient(client *http.Client) *DeleteNetServiceParams {
	o.SetHTTPClient(client)
	return o
}

// SetHTTPClient adds the HTTPClient to the delete net service params
func (o *DeleteNetServiceParams) SetHTTPClient(client *http.Client) {
	o.HTTPClient = client
}

// WithDeleteParameters adds the deleteParameters to the delete net service params
func (o *DeleteNetServiceParams) WithDeleteParameters(deleteParameters DeleteNetServiceBody) *DeleteNetServiceParams {
	o.SetDeleteParameters(deleteParameters)
	return o
}

// SetDeleteParameters adds the deleteParameters to the delete net service params
func (o *DeleteNetServiceParams) SetDeleteParameters(deleteParameters DeleteNetServiceBody) {
	o.DeleteParameters = deleteParameters
}

// WriteToRequest writes these params to a swagger request
func (o *DeleteNetServiceParams) WriteToRequest(r runtime.ClientRequest, reg strfmt.Registry) error {

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
