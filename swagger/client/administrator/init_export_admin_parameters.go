// Code generated by go-swagger; DO NOT EDIT.

package administrator

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

// NewInitExportAdminParams creates a new InitExportAdminParams object
// with the default values initialized.
func NewInitExportAdminParams() *InitExportAdminParams {
	var ()
	return &InitExportAdminParams{

		timeout: cr.DefaultTimeout,
	}
}

// NewInitExportAdminParamsWithTimeout creates a new InitExportAdminParams object
// with the default values initialized, and the ability to set a timeout on a request
func NewInitExportAdminParamsWithTimeout(timeout time.Duration) *InitExportAdminParams {
	var ()
	return &InitExportAdminParams{

		timeout: timeout,
	}
}

// NewInitExportAdminParamsWithContext creates a new InitExportAdminParams object
// with the default values initialized, and the ability to set a context for a request
func NewInitExportAdminParamsWithContext(ctx context.Context) *InitExportAdminParams {
	var ()
	return &InitExportAdminParams{

		Context: ctx,
	}
}

// NewInitExportAdminParamsWithHTTPClient creates a new InitExportAdminParams object
// with the default values initialized, and the ability to set a custom HTTPClient for a request
func NewInitExportAdminParamsWithHTTPClient(client *http.Client) *InitExportAdminParams {
	var ()
	return &InitExportAdminParams{
		HTTPClient: client,
	}
}

/*InitExportAdminParams contains all the parameters to send to the API endpoint
for the init export admin operation typically these are written to a http.Request
*/
type InitExportAdminParams struct {

	/*ExportParameters
	  The query string syntax and supported selectors are defined in the IPControl CLI and API Guide. Use pageSize to specify the number of results to be returned. Use firstResultPos to indicate where the set of results should begin. Results begin at position 0.

	*/
	ExportParameters InitExportAdminBody

	timeout    time.Duration
	Context    context.Context
	HTTPClient *http.Client
}

// WithTimeout adds the timeout to the init export admin params
func (o *InitExportAdminParams) WithTimeout(timeout time.Duration) *InitExportAdminParams {
	o.SetTimeout(timeout)
	return o
}

// SetTimeout adds the timeout to the init export admin params
func (o *InitExportAdminParams) SetTimeout(timeout time.Duration) {
	o.timeout = timeout
}

// WithContext adds the context to the init export admin params
func (o *InitExportAdminParams) WithContext(ctx context.Context) *InitExportAdminParams {
	o.SetContext(ctx)
	return o
}

// SetContext adds the context to the init export admin params
func (o *InitExportAdminParams) SetContext(ctx context.Context) {
	o.Context = ctx
}

// WithHTTPClient adds the HTTPClient to the init export admin params
func (o *InitExportAdminParams) WithHTTPClient(client *http.Client) *InitExportAdminParams {
	o.SetHTTPClient(client)
	return o
}

// SetHTTPClient adds the HTTPClient to the init export admin params
func (o *InitExportAdminParams) SetHTTPClient(client *http.Client) {
	o.HTTPClient = client
}

// WithExportParameters adds the exportParameters to the init export admin params
func (o *InitExportAdminParams) WithExportParameters(exportParameters InitExportAdminBody) *InitExportAdminParams {
	o.SetExportParameters(exportParameters)
	return o
}

// SetExportParameters adds the exportParameters to the init export admin params
func (o *InitExportAdminParams) SetExportParameters(exportParameters InitExportAdminBody) {
	o.ExportParameters = exportParameters
}

// WriteToRequest writes these params to a swagger request
func (o *InitExportAdminParams) WriteToRequest(r runtime.ClientRequest, reg strfmt.Registry) error {

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
