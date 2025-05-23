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

// NewExportAdminParams creates a new ExportAdminParams object
// with the default values initialized.
func NewExportAdminParams() *ExportAdminParams {
	var ()
	return &ExportAdminParams{

		timeout: cr.DefaultTimeout,
	}
}

// NewExportAdminParamsWithTimeout creates a new ExportAdminParams object
// with the default values initialized, and the ability to set a timeout on a request
func NewExportAdminParamsWithTimeout(timeout time.Duration) *ExportAdminParams {
	var ()
	return &ExportAdminParams{

		timeout: timeout,
	}
}

// NewExportAdminParamsWithContext creates a new ExportAdminParams object
// with the default values initialized, and the ability to set a context for a request
func NewExportAdminParamsWithContext(ctx context.Context) *ExportAdminParams {
	var ()
	return &ExportAdminParams{

		Context: ctx,
	}
}

// NewExportAdminParamsWithHTTPClient creates a new ExportAdminParams object
// with the default values initialized, and the ability to set a custom HTTPClient for a request
func NewExportAdminParamsWithHTTPClient(client *http.Client) *ExportAdminParams {
	var ()
	return &ExportAdminParams{
		HTTPClient: client,
	}
}

/*ExportAdminParams contains all the parameters to send to the API endpoint
for the export admin operation typically these are written to a http.Request
*/
type ExportAdminParams struct {

	/*Wscontext
	  The results from an initExportAdmin operation

	*/
	Wscontext ExportAdminBody

	timeout    time.Duration
	Context    context.Context
	HTTPClient *http.Client
}

// WithTimeout adds the timeout to the export admin params
func (o *ExportAdminParams) WithTimeout(timeout time.Duration) *ExportAdminParams {
	o.SetTimeout(timeout)
	return o
}

// SetTimeout adds the timeout to the export admin params
func (o *ExportAdminParams) SetTimeout(timeout time.Duration) {
	o.timeout = timeout
}

// WithContext adds the context to the export admin params
func (o *ExportAdminParams) WithContext(ctx context.Context) *ExportAdminParams {
	o.SetContext(ctx)
	return o
}

// SetContext adds the context to the export admin params
func (o *ExportAdminParams) SetContext(ctx context.Context) {
	o.Context = ctx
}

// WithHTTPClient adds the HTTPClient to the export admin params
func (o *ExportAdminParams) WithHTTPClient(client *http.Client) *ExportAdminParams {
	o.SetHTTPClient(client)
	return o
}

// SetHTTPClient adds the HTTPClient to the export admin params
func (o *ExportAdminParams) SetHTTPClient(client *http.Client) {
	o.HTTPClient = client
}

// WithWscontext adds the wscontext to the export admin params
func (o *ExportAdminParams) WithWscontext(wscontext ExportAdminBody) *ExportAdminParams {
	o.SetWscontext(wscontext)
	return o
}

// SetWscontext adds the wscontext to the export admin params
func (o *ExportAdminParams) SetWscontext(wscontext ExportAdminBody) {
	o.Wscontext = wscontext
}

// WriteToRequest writes these params to a swagger request
func (o *ExportAdminParams) WriteToRequest(r runtime.ClientRequest, reg strfmt.Registry) error {

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
