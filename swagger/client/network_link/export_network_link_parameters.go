// Code generated by go-swagger; DO NOT EDIT.

package network_link

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

// NewExportNetworkLinkParams creates a new ExportNetworkLinkParams object
// with the default values initialized.
func NewExportNetworkLinkParams() *ExportNetworkLinkParams {
	var ()
	return &ExportNetworkLinkParams{

		timeout: cr.DefaultTimeout,
	}
}

// NewExportNetworkLinkParamsWithTimeout creates a new ExportNetworkLinkParams object
// with the default values initialized, and the ability to set a timeout on a request
func NewExportNetworkLinkParamsWithTimeout(timeout time.Duration) *ExportNetworkLinkParams {
	var ()
	return &ExportNetworkLinkParams{

		timeout: timeout,
	}
}

// NewExportNetworkLinkParamsWithContext creates a new ExportNetworkLinkParams object
// with the default values initialized, and the ability to set a context for a request
func NewExportNetworkLinkParamsWithContext(ctx context.Context) *ExportNetworkLinkParams {
	var ()
	return &ExportNetworkLinkParams{

		Context: ctx,
	}
}

// NewExportNetworkLinkParamsWithHTTPClient creates a new ExportNetworkLinkParams object
// with the default values initialized, and the ability to set a custom HTTPClient for a request
func NewExportNetworkLinkParamsWithHTTPClient(client *http.Client) *ExportNetworkLinkParams {
	var ()
	return &ExportNetworkLinkParams{
		HTTPClient: client,
	}
}

/*ExportNetworkLinkParams contains all the parameters to send to the API endpoint
for the export network link operation typically these are written to a http.Request
*/
type ExportNetworkLinkParams struct {

	/*Wscontext
	  The results from an initExportNetworkLink operation

	*/
	Wscontext ExportNetworkLinkBody

	timeout    time.Duration
	Context    context.Context
	HTTPClient *http.Client
}

// WithTimeout adds the timeout to the export network link params
func (o *ExportNetworkLinkParams) WithTimeout(timeout time.Duration) *ExportNetworkLinkParams {
	o.SetTimeout(timeout)
	return o
}

// SetTimeout adds the timeout to the export network link params
func (o *ExportNetworkLinkParams) SetTimeout(timeout time.Duration) {
	o.timeout = timeout
}

// WithContext adds the context to the export network link params
func (o *ExportNetworkLinkParams) WithContext(ctx context.Context) *ExportNetworkLinkParams {
	o.SetContext(ctx)
	return o
}

// SetContext adds the context to the export network link params
func (o *ExportNetworkLinkParams) SetContext(ctx context.Context) {
	o.Context = ctx
}

// WithHTTPClient adds the HTTPClient to the export network link params
func (o *ExportNetworkLinkParams) WithHTTPClient(client *http.Client) *ExportNetworkLinkParams {
	o.SetHTTPClient(client)
	return o
}

// SetHTTPClient adds the HTTPClient to the export network link params
func (o *ExportNetworkLinkParams) SetHTTPClient(client *http.Client) {
	o.HTTPClient = client
}

// WithWscontext adds the wscontext to the export network link params
func (o *ExportNetworkLinkParams) WithWscontext(wscontext ExportNetworkLinkBody) *ExportNetworkLinkParams {
	o.SetWscontext(wscontext)
	return o
}

// SetWscontext adds the wscontext to the export network link params
func (o *ExportNetworkLinkParams) SetWscontext(wscontext ExportNetworkLinkBody) {
	o.Wscontext = wscontext
}

// WriteToRequest writes these params to a swagger request
func (o *ExportNetworkLinkParams) WriteToRequest(r runtime.ClientRequest, reg strfmt.Registry) error {

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
