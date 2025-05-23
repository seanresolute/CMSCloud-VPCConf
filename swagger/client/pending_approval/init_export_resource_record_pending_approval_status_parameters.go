// Code generated by go-swagger; DO NOT EDIT.

package pending_approval

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

// NewInitExportResourceRecordPendingApprovalStatusParams creates a new InitExportResourceRecordPendingApprovalStatusParams object
// with the default values initialized.
func NewInitExportResourceRecordPendingApprovalStatusParams() *InitExportResourceRecordPendingApprovalStatusParams {
	var ()
	return &InitExportResourceRecordPendingApprovalStatusParams{

		timeout: cr.DefaultTimeout,
	}
}

// NewInitExportResourceRecordPendingApprovalStatusParamsWithTimeout creates a new InitExportResourceRecordPendingApprovalStatusParams object
// with the default values initialized, and the ability to set a timeout on a request
func NewInitExportResourceRecordPendingApprovalStatusParamsWithTimeout(timeout time.Duration) *InitExportResourceRecordPendingApprovalStatusParams {
	var ()
	return &InitExportResourceRecordPendingApprovalStatusParams{

		timeout: timeout,
	}
}

// NewInitExportResourceRecordPendingApprovalStatusParamsWithContext creates a new InitExportResourceRecordPendingApprovalStatusParams object
// with the default values initialized, and the ability to set a context for a request
func NewInitExportResourceRecordPendingApprovalStatusParamsWithContext(ctx context.Context) *InitExportResourceRecordPendingApprovalStatusParams {
	var ()
	return &InitExportResourceRecordPendingApprovalStatusParams{

		Context: ctx,
	}
}

// NewInitExportResourceRecordPendingApprovalStatusParamsWithHTTPClient creates a new InitExportResourceRecordPendingApprovalStatusParams object
// with the default values initialized, and the ability to set a custom HTTPClient for a request
func NewInitExportResourceRecordPendingApprovalStatusParamsWithHTTPClient(client *http.Client) *InitExportResourceRecordPendingApprovalStatusParams {
	var ()
	return &InitExportResourceRecordPendingApprovalStatusParams{
		HTTPClient: client,
	}
}

/*InitExportResourceRecordPendingApprovalStatusParams contains all the parameters to send to the API endpoint
for the init export resource record pending approval status operation typically these are written to a http.Request
*/
type InitExportResourceRecordPendingApprovalStatusParams struct {

	/*ExportParameters
	  The query string syntax and supported selectors are defined in the IPControl CLI and API Guide. Use pageSize to specify the number of results to be returned. Use firstResultPos to indicate where the set of results should begin. Results begin at position 0.

	*/
	ExportParameters InitExportResourceRecordPendingApprovalStatusBody

	timeout    time.Duration
	Context    context.Context
	HTTPClient *http.Client
}

// WithTimeout adds the timeout to the init export resource record pending approval status params
func (o *InitExportResourceRecordPendingApprovalStatusParams) WithTimeout(timeout time.Duration) *InitExportResourceRecordPendingApprovalStatusParams {
	o.SetTimeout(timeout)
	return o
}

// SetTimeout adds the timeout to the init export resource record pending approval status params
func (o *InitExportResourceRecordPendingApprovalStatusParams) SetTimeout(timeout time.Duration) {
	o.timeout = timeout
}

// WithContext adds the context to the init export resource record pending approval status params
func (o *InitExportResourceRecordPendingApprovalStatusParams) WithContext(ctx context.Context) *InitExportResourceRecordPendingApprovalStatusParams {
	o.SetContext(ctx)
	return o
}

// SetContext adds the context to the init export resource record pending approval status params
func (o *InitExportResourceRecordPendingApprovalStatusParams) SetContext(ctx context.Context) {
	o.Context = ctx
}

// WithHTTPClient adds the HTTPClient to the init export resource record pending approval status params
func (o *InitExportResourceRecordPendingApprovalStatusParams) WithHTTPClient(client *http.Client) *InitExportResourceRecordPendingApprovalStatusParams {
	o.SetHTTPClient(client)
	return o
}

// SetHTTPClient adds the HTTPClient to the init export resource record pending approval status params
func (o *InitExportResourceRecordPendingApprovalStatusParams) SetHTTPClient(client *http.Client) {
	o.HTTPClient = client
}

// WithExportParameters adds the exportParameters to the init export resource record pending approval status params
func (o *InitExportResourceRecordPendingApprovalStatusParams) WithExportParameters(exportParameters InitExportResourceRecordPendingApprovalStatusBody) *InitExportResourceRecordPendingApprovalStatusParams {
	o.SetExportParameters(exportParameters)
	return o
}

// SetExportParameters adds the exportParameters to the init export resource record pending approval status params
func (o *InitExportResourceRecordPendingApprovalStatusParams) SetExportParameters(exportParameters InitExportResourceRecordPendingApprovalStatusBody) {
	o.ExportParameters = exportParameters
}

// WriteToRequest writes these params to a swagger request
func (o *InitExportResourceRecordPendingApprovalStatusParams) WriteToRequest(r runtime.ClientRequest, reg strfmt.Registry) error {

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
