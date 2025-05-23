// Code generated by go-swagger; DO NOT EDIT.

package resource_record

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

// NewEndExportDeviceResourceRecParams creates a new EndExportDeviceResourceRecParams object
// with the default values initialized.
func NewEndExportDeviceResourceRecParams() *EndExportDeviceResourceRecParams {
	var ()
	return &EndExportDeviceResourceRecParams{

		timeout: cr.DefaultTimeout,
	}
}

// NewEndExportDeviceResourceRecParamsWithTimeout creates a new EndExportDeviceResourceRecParams object
// with the default values initialized, and the ability to set a timeout on a request
func NewEndExportDeviceResourceRecParamsWithTimeout(timeout time.Duration) *EndExportDeviceResourceRecParams {
	var ()
	return &EndExportDeviceResourceRecParams{

		timeout: timeout,
	}
}

// NewEndExportDeviceResourceRecParamsWithContext creates a new EndExportDeviceResourceRecParams object
// with the default values initialized, and the ability to set a context for a request
func NewEndExportDeviceResourceRecParamsWithContext(ctx context.Context) *EndExportDeviceResourceRecParams {
	var ()
	return &EndExportDeviceResourceRecParams{

		Context: ctx,
	}
}

// NewEndExportDeviceResourceRecParamsWithHTTPClient creates a new EndExportDeviceResourceRecParams object
// with the default values initialized, and the ability to set a custom HTTPClient for a request
func NewEndExportDeviceResourceRecParamsWithHTTPClient(client *http.Client) *EndExportDeviceResourceRecParams {
	var ()
	return &EndExportDeviceResourceRecParams{
		HTTPClient: client,
	}
}

/*EndExportDeviceResourceRecParams contains all the parameters to send to the API endpoint
for the end export device resource rec operation typically these are written to a http.Request
*/
type EndExportDeviceResourceRecParams struct {

	/*Wscontext
	  The results from an exportDeviceResourceRec operation

	*/
	Wscontext EndExportDeviceResourceRecBody

	timeout    time.Duration
	Context    context.Context
	HTTPClient *http.Client
}

// WithTimeout adds the timeout to the end export device resource rec params
func (o *EndExportDeviceResourceRecParams) WithTimeout(timeout time.Duration) *EndExportDeviceResourceRecParams {
	o.SetTimeout(timeout)
	return o
}

// SetTimeout adds the timeout to the end export device resource rec params
func (o *EndExportDeviceResourceRecParams) SetTimeout(timeout time.Duration) {
	o.timeout = timeout
}

// WithContext adds the context to the end export device resource rec params
func (o *EndExportDeviceResourceRecParams) WithContext(ctx context.Context) *EndExportDeviceResourceRecParams {
	o.SetContext(ctx)
	return o
}

// SetContext adds the context to the end export device resource rec params
func (o *EndExportDeviceResourceRecParams) SetContext(ctx context.Context) {
	o.Context = ctx
}

// WithHTTPClient adds the HTTPClient to the end export device resource rec params
func (o *EndExportDeviceResourceRecParams) WithHTTPClient(client *http.Client) *EndExportDeviceResourceRecParams {
	o.SetHTTPClient(client)
	return o
}

// SetHTTPClient adds the HTTPClient to the end export device resource rec params
func (o *EndExportDeviceResourceRecParams) SetHTTPClient(client *http.Client) {
	o.HTTPClient = client
}

// WithWscontext adds the wscontext to the end export device resource rec params
func (o *EndExportDeviceResourceRecParams) WithWscontext(wscontext EndExportDeviceResourceRecBody) *EndExportDeviceResourceRecParams {
	o.SetWscontext(wscontext)
	return o
}

// SetWscontext adds the wscontext to the end export device resource rec params
func (o *EndExportDeviceResourceRecParams) SetWscontext(wscontext EndExportDeviceResourceRecBody) {
	o.Wscontext = wscontext
}

// WriteToRequest writes these params to a swagger request
func (o *EndExportDeviceResourceRecParams) WriteToRequest(r runtime.ClientRequest, reg strfmt.Registry) error {

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
