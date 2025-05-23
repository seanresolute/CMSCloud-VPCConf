// Code generated by go-swagger; DO NOT EDIT.

package address_pool

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

// NewEndExportAddressPoolParams creates a new EndExportAddressPoolParams object
// with the default values initialized.
func NewEndExportAddressPoolParams() *EndExportAddressPoolParams {
	var ()
	return &EndExportAddressPoolParams{

		timeout: cr.DefaultTimeout,
	}
}

// NewEndExportAddressPoolParamsWithTimeout creates a new EndExportAddressPoolParams object
// with the default values initialized, and the ability to set a timeout on a request
func NewEndExportAddressPoolParamsWithTimeout(timeout time.Duration) *EndExportAddressPoolParams {
	var ()
	return &EndExportAddressPoolParams{

		timeout: timeout,
	}
}

// NewEndExportAddressPoolParamsWithContext creates a new EndExportAddressPoolParams object
// with the default values initialized, and the ability to set a context for a request
func NewEndExportAddressPoolParamsWithContext(ctx context.Context) *EndExportAddressPoolParams {
	var ()
	return &EndExportAddressPoolParams{

		Context: ctx,
	}
}

// NewEndExportAddressPoolParamsWithHTTPClient creates a new EndExportAddressPoolParams object
// with the default values initialized, and the ability to set a custom HTTPClient for a request
func NewEndExportAddressPoolParamsWithHTTPClient(client *http.Client) *EndExportAddressPoolParams {
	var ()
	return &EndExportAddressPoolParams{
		HTTPClient: client,
	}
}

/*EndExportAddressPoolParams contains all the parameters to send to the API endpoint
for the end export address pool operation typically these are written to a http.Request
*/
type EndExportAddressPoolParams struct {

	/*Wscontext
	  The results from an exportAddressPool operation

	*/
	Wscontext EndExportAddressPoolBody

	timeout    time.Duration
	Context    context.Context
	HTTPClient *http.Client
}

// WithTimeout adds the timeout to the end export address pool params
func (o *EndExportAddressPoolParams) WithTimeout(timeout time.Duration) *EndExportAddressPoolParams {
	o.SetTimeout(timeout)
	return o
}

// SetTimeout adds the timeout to the end export address pool params
func (o *EndExportAddressPoolParams) SetTimeout(timeout time.Duration) {
	o.timeout = timeout
}

// WithContext adds the context to the end export address pool params
func (o *EndExportAddressPoolParams) WithContext(ctx context.Context) *EndExportAddressPoolParams {
	o.SetContext(ctx)
	return o
}

// SetContext adds the context to the end export address pool params
func (o *EndExportAddressPoolParams) SetContext(ctx context.Context) {
	o.Context = ctx
}

// WithHTTPClient adds the HTTPClient to the end export address pool params
func (o *EndExportAddressPoolParams) WithHTTPClient(client *http.Client) *EndExportAddressPoolParams {
	o.SetHTTPClient(client)
	return o
}

// SetHTTPClient adds the HTTPClient to the end export address pool params
func (o *EndExportAddressPoolParams) SetHTTPClient(client *http.Client) {
	o.HTTPClient = client
}

// WithWscontext adds the wscontext to the end export address pool params
func (o *EndExportAddressPoolParams) WithWscontext(wscontext EndExportAddressPoolBody) *EndExportAddressPoolParams {
	o.SetWscontext(wscontext)
	return o
}

// SetWscontext adds the wscontext to the end export address pool params
func (o *EndExportAddressPoolParams) SetWscontext(wscontext EndExportAddressPoolBody) {
	o.Wscontext = wscontext
}

// WriteToRequest writes these params to a swagger request
func (o *EndExportAddressPoolParams) WriteToRequest(r runtime.ClientRequest, reg strfmt.Registry) error {

	if err := r.SetTimeout(o.timeout); err != nil {
		return err
	}
	var res []error

	if err := r.SetBodyParam(o.Wscontext); err != nil {
		return err
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}
