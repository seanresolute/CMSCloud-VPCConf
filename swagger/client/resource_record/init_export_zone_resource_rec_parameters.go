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

// NewInitExportZoneResourceRecParams creates a new InitExportZoneResourceRecParams object
// with the default values initialized.
func NewInitExportZoneResourceRecParams() *InitExportZoneResourceRecParams {
	var ()
	return &InitExportZoneResourceRecParams{

		timeout: cr.DefaultTimeout,
	}
}

// NewInitExportZoneResourceRecParamsWithTimeout creates a new InitExportZoneResourceRecParams object
// with the default values initialized, and the ability to set a timeout on a request
func NewInitExportZoneResourceRecParamsWithTimeout(timeout time.Duration) *InitExportZoneResourceRecParams {
	var ()
	return &InitExportZoneResourceRecParams{

		timeout: timeout,
	}
}

// NewInitExportZoneResourceRecParamsWithContext creates a new InitExportZoneResourceRecParams object
// with the default values initialized, and the ability to set a context for a request
func NewInitExportZoneResourceRecParamsWithContext(ctx context.Context) *InitExportZoneResourceRecParams {
	var ()
	return &InitExportZoneResourceRecParams{

		Context: ctx,
	}
}

// NewInitExportZoneResourceRecParamsWithHTTPClient creates a new InitExportZoneResourceRecParams object
// with the default values initialized, and the ability to set a custom HTTPClient for a request
func NewInitExportZoneResourceRecParamsWithHTTPClient(client *http.Client) *InitExportZoneResourceRecParams {
	var ()
	return &InitExportZoneResourceRecParams{
		HTTPClient: client,
	}
}

/*InitExportZoneResourceRecParams contains all the parameters to send to the API endpoint
for the init export zone resource rec operation typically these are written to a http.Request
*/
type InitExportZoneResourceRecParams struct {

	/*ExportParameters
	  The query string syntax and supported selectors are defined in the IPControl CLI and API Guide. Use pageSize to specify the number of results to be returned. Use firstResultPos to indicate where the set of results should begin. Results begin at position 0. There are no options defined for this operation.

	*/
	ExportParameters InitExportZoneResourceRecBody

	timeout    time.Duration
	Context    context.Context
	HTTPClient *http.Client
}

// WithTimeout adds the timeout to the init export zone resource rec params
func (o *InitExportZoneResourceRecParams) WithTimeout(timeout time.Duration) *InitExportZoneResourceRecParams {
	o.SetTimeout(timeout)
	return o
}

// SetTimeout adds the timeout to the init export zone resource rec params
func (o *InitExportZoneResourceRecParams) SetTimeout(timeout time.Duration) {
	o.timeout = timeout
}

// WithContext adds the context to the init export zone resource rec params
func (o *InitExportZoneResourceRecParams) WithContext(ctx context.Context) *InitExportZoneResourceRecParams {
	o.SetContext(ctx)
	return o
}

// SetContext adds the context to the init export zone resource rec params
func (o *InitExportZoneResourceRecParams) SetContext(ctx context.Context) {
	o.Context = ctx
}

// WithHTTPClient adds the HTTPClient to the init export zone resource rec params
func (o *InitExportZoneResourceRecParams) WithHTTPClient(client *http.Client) *InitExportZoneResourceRecParams {
	o.SetHTTPClient(client)
	return o
}

// SetHTTPClient adds the HTTPClient to the init export zone resource rec params
func (o *InitExportZoneResourceRecParams) SetHTTPClient(client *http.Client) {
	o.HTTPClient = client
}

// WithExportParameters adds the exportParameters to the init export zone resource rec params
func (o *InitExportZoneResourceRecParams) WithExportParameters(exportParameters InitExportZoneResourceRecBody) *InitExportZoneResourceRecParams {
	o.SetExportParameters(exportParameters)
	return o
}

// SetExportParameters adds the exportParameters to the init export zone resource rec params
func (o *InitExportZoneResourceRecParams) SetExportParameters(exportParameters InitExportZoneResourceRecBody) {
	o.ExportParameters = exportParameters
}

// WriteToRequest writes these params to a swagger request
func (o *InitExportZoneResourceRecParams) WriteToRequest(r runtime.ClientRequest, reg strfmt.Registry) error {

	if err := r.SetTimeout(o.timeout); err != nil {
		return err
	}
	var res []error

	if err := r.SetBodyParam(o.ExportParameters); err != nil {
		return err
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}
