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

// NewExportZoneResourceRecParams creates a new ExportZoneResourceRecParams object
// with the default values initialized.
func NewExportZoneResourceRecParams() *ExportZoneResourceRecParams {
	var ()
	return &ExportZoneResourceRecParams{

		timeout: cr.DefaultTimeout,
	}
}

// NewExportZoneResourceRecParamsWithTimeout creates a new ExportZoneResourceRecParams object
// with the default values initialized, and the ability to set a timeout on a request
func NewExportZoneResourceRecParamsWithTimeout(timeout time.Duration) *ExportZoneResourceRecParams {
	var ()
	return &ExportZoneResourceRecParams{

		timeout: timeout,
	}
}

// NewExportZoneResourceRecParamsWithContext creates a new ExportZoneResourceRecParams object
// with the default values initialized, and the ability to set a context for a request
func NewExportZoneResourceRecParamsWithContext(ctx context.Context) *ExportZoneResourceRecParams {
	var ()
	return &ExportZoneResourceRecParams{

		Context: ctx,
	}
}

// NewExportZoneResourceRecParamsWithHTTPClient creates a new ExportZoneResourceRecParams object
// with the default values initialized, and the ability to set a custom HTTPClient for a request
func NewExportZoneResourceRecParamsWithHTTPClient(client *http.Client) *ExportZoneResourceRecParams {
	var ()
	return &ExportZoneResourceRecParams{
		HTTPClient: client,
	}
}

/*ExportZoneResourceRecParams contains all the parameters to send to the API endpoint
for the export zone resource rec operation typically these are written to a http.Request
*/
type ExportZoneResourceRecParams struct {

	/*Wscontext
	  The results from an initExportZoneResourceRec operation

	*/
	Wscontext ExportZoneResourceRecBody

	timeout    time.Duration
	Context    context.Context
	HTTPClient *http.Client
}

// WithTimeout adds the timeout to the export zone resource rec params
func (o *ExportZoneResourceRecParams) WithTimeout(timeout time.Duration) *ExportZoneResourceRecParams {
	o.SetTimeout(timeout)
	return o
}

// SetTimeout adds the timeout to the export zone resource rec params
func (o *ExportZoneResourceRecParams) SetTimeout(timeout time.Duration) {
	o.timeout = timeout
}

// WithContext adds the context to the export zone resource rec params
func (o *ExportZoneResourceRecParams) WithContext(ctx context.Context) *ExportZoneResourceRecParams {
	o.SetContext(ctx)
	return o
}

// SetContext adds the context to the export zone resource rec params
func (o *ExportZoneResourceRecParams) SetContext(ctx context.Context) {
	o.Context = ctx
}

// WithHTTPClient adds the HTTPClient to the export zone resource rec params
func (o *ExportZoneResourceRecParams) WithHTTPClient(client *http.Client) *ExportZoneResourceRecParams {
	o.SetHTTPClient(client)
	return o
}

// SetHTTPClient adds the HTTPClient to the export zone resource rec params
func (o *ExportZoneResourceRecParams) SetHTTPClient(client *http.Client) {
	o.HTTPClient = client
}

// WithWscontext adds the wscontext to the export zone resource rec params
func (o *ExportZoneResourceRecParams) WithWscontext(wscontext ExportZoneResourceRecBody) *ExportZoneResourceRecParams {
	o.SetWscontext(wscontext)
	return o
}

// SetWscontext adds the wscontext to the export zone resource rec params
func (o *ExportZoneResourceRecParams) SetWscontext(wscontext ExportZoneResourceRecBody) {
	o.Wscontext = wscontext
}

// WriteToRequest writes these params to a swagger request
func (o *ExportZoneResourceRecParams) WriteToRequest(r runtime.ClientRequest, reg strfmt.Registry) error {

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
