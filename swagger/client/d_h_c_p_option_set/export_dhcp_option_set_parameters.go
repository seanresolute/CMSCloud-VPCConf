// Code generated by go-swagger; DO NOT EDIT.

package d_h_c_p_option_set

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

// NewExportDhcpOptionSetParams creates a new ExportDhcpOptionSetParams object
// with the default values initialized.
func NewExportDhcpOptionSetParams() *ExportDhcpOptionSetParams {
	var ()
	return &ExportDhcpOptionSetParams{

		timeout: cr.DefaultTimeout,
	}
}

// NewExportDhcpOptionSetParamsWithTimeout creates a new ExportDhcpOptionSetParams object
// with the default values initialized, and the ability to set a timeout on a request
func NewExportDhcpOptionSetParamsWithTimeout(timeout time.Duration) *ExportDhcpOptionSetParams {
	var ()
	return &ExportDhcpOptionSetParams{

		timeout: timeout,
	}
}

// NewExportDhcpOptionSetParamsWithContext creates a new ExportDhcpOptionSetParams object
// with the default values initialized, and the ability to set a context for a request
func NewExportDhcpOptionSetParamsWithContext(ctx context.Context) *ExportDhcpOptionSetParams {
	var ()
	return &ExportDhcpOptionSetParams{

		Context: ctx,
	}
}

// NewExportDhcpOptionSetParamsWithHTTPClient creates a new ExportDhcpOptionSetParams object
// with the default values initialized, and the ability to set a custom HTTPClient for a request
func NewExportDhcpOptionSetParamsWithHTTPClient(client *http.Client) *ExportDhcpOptionSetParams {
	var ()
	return &ExportDhcpOptionSetParams{
		HTTPClient: client,
	}
}

/*ExportDhcpOptionSetParams contains all the parameters to send to the API endpoint
for the export dhcp option set operation typically these are written to a http.Request
*/
type ExportDhcpOptionSetParams struct {

	/*Wscontext
	  The results from an initExportDhcpOptionSet operation

	*/
	Wscontext ExportDhcpOptionSetBody

	timeout    time.Duration
	Context    context.Context
	HTTPClient *http.Client
}

// WithTimeout adds the timeout to the export dhcp option set params
func (o *ExportDhcpOptionSetParams) WithTimeout(timeout time.Duration) *ExportDhcpOptionSetParams {
	o.SetTimeout(timeout)
	return o
}

// SetTimeout adds the timeout to the export dhcp option set params
func (o *ExportDhcpOptionSetParams) SetTimeout(timeout time.Duration) {
	o.timeout = timeout
}

// WithContext adds the context to the export dhcp option set params
func (o *ExportDhcpOptionSetParams) WithContext(ctx context.Context) *ExportDhcpOptionSetParams {
	o.SetContext(ctx)
	return o
}

// SetContext adds the context to the export dhcp option set params
func (o *ExportDhcpOptionSetParams) SetContext(ctx context.Context) {
	o.Context = ctx
}

// WithHTTPClient adds the HTTPClient to the export dhcp option set params
func (o *ExportDhcpOptionSetParams) WithHTTPClient(client *http.Client) *ExportDhcpOptionSetParams {
	o.SetHTTPClient(client)
	return o
}

// SetHTTPClient adds the HTTPClient to the export dhcp option set params
func (o *ExportDhcpOptionSetParams) SetHTTPClient(client *http.Client) {
	o.HTTPClient = client
}

// WithWscontext adds the wscontext to the export dhcp option set params
func (o *ExportDhcpOptionSetParams) WithWscontext(wscontext ExportDhcpOptionSetBody) *ExportDhcpOptionSetParams {
	o.SetWscontext(wscontext)
	return o
}

// SetWscontext adds the wscontext to the export dhcp option set params
func (o *ExportDhcpOptionSetParams) SetWscontext(wscontext ExportDhcpOptionSetBody) {
	o.Wscontext = wscontext
}

// WriteToRequest writes these params to a swagger request
func (o *ExportDhcpOptionSetParams) WriteToRequest(r runtime.ClientRequest, reg strfmt.Registry) error {

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
