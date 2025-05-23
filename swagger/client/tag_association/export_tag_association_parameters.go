// Code generated by go-swagger; DO NOT EDIT.

package tag_association

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

// NewExportTagAssociationParams creates a new ExportTagAssociationParams object
// with the default values initialized.
func NewExportTagAssociationParams() *ExportTagAssociationParams {
	var ()
	return &ExportTagAssociationParams{

		timeout: cr.DefaultTimeout,
	}
}

// NewExportTagAssociationParamsWithTimeout creates a new ExportTagAssociationParams object
// with the default values initialized, and the ability to set a timeout on a request
func NewExportTagAssociationParamsWithTimeout(timeout time.Duration) *ExportTagAssociationParams {
	var ()
	return &ExportTagAssociationParams{

		timeout: timeout,
	}
}

// NewExportTagAssociationParamsWithContext creates a new ExportTagAssociationParams object
// with the default values initialized, and the ability to set a context for a request
func NewExportTagAssociationParamsWithContext(ctx context.Context) *ExportTagAssociationParams {
	var ()
	return &ExportTagAssociationParams{

		Context: ctx,
	}
}

// NewExportTagAssociationParamsWithHTTPClient creates a new ExportTagAssociationParams object
// with the default values initialized, and the ability to set a custom HTTPClient for a request
func NewExportTagAssociationParamsWithHTTPClient(client *http.Client) *ExportTagAssociationParams {
	var ()
	return &ExportTagAssociationParams{
		HTTPClient: client,
	}
}

/*ExportTagAssociationParams contains all the parameters to send to the API endpoint
for the export tag association operation typically these are written to a http.Request
*/
type ExportTagAssociationParams struct {

	/*Wscontext
	  The results from the operation

	*/
	Wscontext ExportTagAssociationBody

	timeout    time.Duration
	Context    context.Context
	HTTPClient *http.Client
}

// WithTimeout adds the timeout to the export tag association params
func (o *ExportTagAssociationParams) WithTimeout(timeout time.Duration) *ExportTagAssociationParams {
	o.SetTimeout(timeout)
	return o
}

// SetTimeout adds the timeout to the export tag association params
func (o *ExportTagAssociationParams) SetTimeout(timeout time.Duration) {
	o.timeout = timeout
}

// WithContext adds the context to the export tag association params
func (o *ExportTagAssociationParams) WithContext(ctx context.Context) *ExportTagAssociationParams {
	o.SetContext(ctx)
	return o
}

// SetContext adds the context to the export tag association params
func (o *ExportTagAssociationParams) SetContext(ctx context.Context) {
	o.Context = ctx
}

// WithHTTPClient adds the HTTPClient to the export tag association params
func (o *ExportTagAssociationParams) WithHTTPClient(client *http.Client) *ExportTagAssociationParams {
	o.SetHTTPClient(client)
	return o
}

// SetHTTPClient adds the HTTPClient to the export tag association params
func (o *ExportTagAssociationParams) SetHTTPClient(client *http.Client) {
	o.HTTPClient = client
}

// WithWscontext adds the wscontext to the export tag association params
func (o *ExportTagAssociationParams) WithWscontext(wscontext ExportTagAssociationBody) *ExportTagAssociationParams {
	o.SetWscontext(wscontext)
	return o
}

// SetWscontext adds the wscontext to the export tag association params
func (o *ExportTagAssociationParams) SetWscontext(wscontext ExportTagAssociationBody) {
	o.Wscontext = wscontext
}

// WriteToRequest writes these params to a swagger request
func (o *ExportTagAssociationParams) WriteToRequest(r runtime.ClientRequest, reg strfmt.Registry) error {

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
