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

// NewExportResourceRecordRestoreListParams creates a new ExportResourceRecordRestoreListParams object
// with the default values initialized.
func NewExportResourceRecordRestoreListParams() *ExportResourceRecordRestoreListParams {
	var ()
	return &ExportResourceRecordRestoreListParams{

		timeout: cr.DefaultTimeout,
	}
}

// NewExportResourceRecordRestoreListParamsWithTimeout creates a new ExportResourceRecordRestoreListParams object
// with the default values initialized, and the ability to set a timeout on a request
func NewExportResourceRecordRestoreListParamsWithTimeout(timeout time.Duration) *ExportResourceRecordRestoreListParams {
	var ()
	return &ExportResourceRecordRestoreListParams{

		timeout: timeout,
	}
}

// NewExportResourceRecordRestoreListParamsWithContext creates a new ExportResourceRecordRestoreListParams object
// with the default values initialized, and the ability to set a context for a request
func NewExportResourceRecordRestoreListParamsWithContext(ctx context.Context) *ExportResourceRecordRestoreListParams {
	var ()
	return &ExportResourceRecordRestoreListParams{

		Context: ctx,
	}
}

// NewExportResourceRecordRestoreListParamsWithHTTPClient creates a new ExportResourceRecordRestoreListParams object
// with the default values initialized, and the ability to set a custom HTTPClient for a request
func NewExportResourceRecordRestoreListParamsWithHTTPClient(client *http.Client) *ExportResourceRecordRestoreListParams {
	var ()
	return &ExportResourceRecordRestoreListParams{
		HTTPClient: client,
	}
}

/*ExportResourceRecordRestoreListParams contains all the parameters to send to the API endpoint
for the export resource record restore list operation typically these are written to a http.Request
*/
type ExportResourceRecordRestoreListParams struct {

	/*Wscontext
	  The results from an initResourceRecordRestoreList operation

	*/
	Wscontext ExportResourceRecordRestoreListBody

	timeout    time.Duration
	Context    context.Context
	HTTPClient *http.Client
}

// WithTimeout adds the timeout to the export resource record restore list params
func (o *ExportResourceRecordRestoreListParams) WithTimeout(timeout time.Duration) *ExportResourceRecordRestoreListParams {
	o.SetTimeout(timeout)
	return o
}

// SetTimeout adds the timeout to the export resource record restore list params
func (o *ExportResourceRecordRestoreListParams) SetTimeout(timeout time.Duration) {
	o.timeout = timeout
}

// WithContext adds the context to the export resource record restore list params
func (o *ExportResourceRecordRestoreListParams) WithContext(ctx context.Context) *ExportResourceRecordRestoreListParams {
	o.SetContext(ctx)
	return o
}

// SetContext adds the context to the export resource record restore list params
func (o *ExportResourceRecordRestoreListParams) SetContext(ctx context.Context) {
	o.Context = ctx
}

// WithHTTPClient adds the HTTPClient to the export resource record restore list params
func (o *ExportResourceRecordRestoreListParams) WithHTTPClient(client *http.Client) *ExportResourceRecordRestoreListParams {
	o.SetHTTPClient(client)
	return o
}

// SetHTTPClient adds the HTTPClient to the export resource record restore list params
func (o *ExportResourceRecordRestoreListParams) SetHTTPClient(client *http.Client) {
	o.HTTPClient = client
}

// WithWscontext adds the wscontext to the export resource record restore list params
func (o *ExportResourceRecordRestoreListParams) WithWscontext(wscontext ExportResourceRecordRestoreListBody) *ExportResourceRecordRestoreListParams {
	o.SetWscontext(wscontext)
	return o
}

// SetWscontext adds the wscontext to the export resource record restore list params
func (o *ExportResourceRecordRestoreListParams) SetWscontext(wscontext ExportResourceRecordRestoreListBody) {
	o.Wscontext = wscontext
}

// WriteToRequest writes these params to a swagger request
func (o *ExportResourceRecordRestoreListParams) WriteToRequest(r runtime.ClientRequest, reg strfmt.Registry) error {

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
