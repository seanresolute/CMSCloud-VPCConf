// Code generated by go-swagger; DO NOT EDIT.

package zone

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

// NewImportDNSZoneParams creates a new ImportDNSZoneParams object
// with the default values initialized.
func NewImportDNSZoneParams() *ImportDNSZoneParams {
	var ()
	return &ImportDNSZoneParams{

		timeout: cr.DefaultTimeout,
	}
}

// NewImportDNSZoneParamsWithTimeout creates a new ImportDNSZoneParams object
// with the default values initialized, and the ability to set a timeout on a request
func NewImportDNSZoneParamsWithTimeout(timeout time.Duration) *ImportDNSZoneParams {
	var ()
	return &ImportDNSZoneParams{

		timeout: timeout,
	}
}

// NewImportDNSZoneParamsWithContext creates a new ImportDNSZoneParams object
// with the default values initialized, and the ability to set a context for a request
func NewImportDNSZoneParamsWithContext(ctx context.Context) *ImportDNSZoneParams {
	var ()
	return &ImportDNSZoneParams{

		Context: ctx,
	}
}

// NewImportDNSZoneParamsWithHTTPClient creates a new ImportDNSZoneParams object
// with the default values initialized, and the ability to set a custom HTTPClient for a request
func NewImportDNSZoneParamsWithHTTPClient(client *http.Client) *ImportDNSZoneParams {
	var ()
	return &ImportDNSZoneParams{
		HTTPClient: client,
	}
}

/*ImportDNSZoneParams contains all the parameters to send to the API endpoint
for the import Dns zone operation typically these are written to a http.Request
*/
type ImportDNSZoneParams struct {

	/*ImportParameters
	  The input describing the DNS zone. domainName and zoneType are required. When dynamicZone is true, one of allowUpdateACL or updatePolicy must be specified. masters is required for slave and stub zones. Either server or galaxyName must be specified.

	*/
	ImportParameters ImportDNSZoneBody

	timeout    time.Duration
	Context    context.Context
	HTTPClient *http.Client
}

// WithTimeout adds the timeout to the import Dns zone params
func (o *ImportDNSZoneParams) WithTimeout(timeout time.Duration) *ImportDNSZoneParams {
	o.SetTimeout(timeout)
	return o
}

// SetTimeout adds the timeout to the import Dns zone params
func (o *ImportDNSZoneParams) SetTimeout(timeout time.Duration) {
	o.timeout = timeout
}

// WithContext adds the context to the import Dns zone params
func (o *ImportDNSZoneParams) WithContext(ctx context.Context) *ImportDNSZoneParams {
	o.SetContext(ctx)
	return o
}

// SetContext adds the context to the import Dns zone params
func (o *ImportDNSZoneParams) SetContext(ctx context.Context) {
	o.Context = ctx
}

// WithHTTPClient adds the HTTPClient to the import Dns zone params
func (o *ImportDNSZoneParams) WithHTTPClient(client *http.Client) *ImportDNSZoneParams {
	o.SetHTTPClient(client)
	return o
}

// SetHTTPClient adds the HTTPClient to the import Dns zone params
func (o *ImportDNSZoneParams) SetHTTPClient(client *http.Client) {
	o.HTTPClient = client
}

// WithImportParameters adds the importParameters to the import Dns zone params
func (o *ImportDNSZoneParams) WithImportParameters(importParameters ImportDNSZoneBody) *ImportDNSZoneParams {
	o.SetImportParameters(importParameters)
	return o
}

// SetImportParameters adds the importParameters to the import Dns zone params
func (o *ImportDNSZoneParams) SetImportParameters(importParameters ImportDNSZoneBody) {
	o.ImportParameters = importParameters
}

// WriteToRequest writes these params to a swagger request
func (o *ImportDNSZoneParams) WriteToRequest(r runtime.ClientRequest, reg strfmt.Registry) error {

	if err := r.SetTimeout(o.timeout); err != nil {
		return err
	}
	var res []error

	if err := r.SetBodyParam(o.ImportParameters); err != nil {
		return err
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}
