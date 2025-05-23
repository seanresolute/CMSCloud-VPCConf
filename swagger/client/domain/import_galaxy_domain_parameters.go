// Code generated by go-swagger; DO NOT EDIT.

package domain

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

// NewImportGalaxyDomainParams creates a new ImportGalaxyDomainParams object
// with the default values initialized.
func NewImportGalaxyDomainParams() *ImportGalaxyDomainParams {
	var ()
	return &ImportGalaxyDomainParams{

		timeout: cr.DefaultTimeout,
	}
}

// NewImportGalaxyDomainParamsWithTimeout creates a new ImportGalaxyDomainParams object
// with the default values initialized, and the ability to set a timeout on a request
func NewImportGalaxyDomainParamsWithTimeout(timeout time.Duration) *ImportGalaxyDomainParams {
	var ()
	return &ImportGalaxyDomainParams{

		timeout: timeout,
	}
}

// NewImportGalaxyDomainParamsWithContext creates a new ImportGalaxyDomainParams object
// with the default values initialized, and the ability to set a context for a request
func NewImportGalaxyDomainParamsWithContext(ctx context.Context) *ImportGalaxyDomainParams {
	var ()
	return &ImportGalaxyDomainParams{

		Context: ctx,
	}
}

// NewImportGalaxyDomainParamsWithHTTPClient creates a new ImportGalaxyDomainParams object
// with the default values initialized, and the ability to set a custom HTTPClient for a request
func NewImportGalaxyDomainParamsWithHTTPClient(client *http.Client) *ImportGalaxyDomainParams {
	var ()
	return &ImportGalaxyDomainParams{
		HTTPClient: client,
	}
}

/*ImportGalaxyDomainParams contains all the parameters to send to the API endpoint
for the import galaxy domain operation typically these are written to a http.Request
*/
type ImportGalaxyDomainParams struct {

	/*ImportParameters
	  The input describing the domain. The following parameters are required: domainName, galaxyName.

	*/
	ImportParameters ImportGalaxyDomainBody

	timeout    time.Duration
	Context    context.Context
	HTTPClient *http.Client
}

// WithTimeout adds the timeout to the import galaxy domain params
func (o *ImportGalaxyDomainParams) WithTimeout(timeout time.Duration) *ImportGalaxyDomainParams {
	o.SetTimeout(timeout)
	return o
}

// SetTimeout adds the timeout to the import galaxy domain params
func (o *ImportGalaxyDomainParams) SetTimeout(timeout time.Duration) {
	o.timeout = timeout
}

// WithContext adds the context to the import galaxy domain params
func (o *ImportGalaxyDomainParams) WithContext(ctx context.Context) *ImportGalaxyDomainParams {
	o.SetContext(ctx)
	return o
}

// SetContext adds the context to the import galaxy domain params
func (o *ImportGalaxyDomainParams) SetContext(ctx context.Context) {
	o.Context = ctx
}

// WithHTTPClient adds the HTTPClient to the import galaxy domain params
func (o *ImportGalaxyDomainParams) WithHTTPClient(client *http.Client) *ImportGalaxyDomainParams {
	o.SetHTTPClient(client)
	return o
}

// SetHTTPClient adds the HTTPClient to the import galaxy domain params
func (o *ImportGalaxyDomainParams) SetHTTPClient(client *http.Client) {
	o.HTTPClient = client
}

// WithImportParameters adds the importParameters to the import galaxy domain params
func (o *ImportGalaxyDomainParams) WithImportParameters(importParameters ImportGalaxyDomainBody) *ImportGalaxyDomainParams {
	o.SetImportParameters(importParameters)
	return o
}

// SetImportParameters adds the importParameters to the import galaxy domain params
func (o *ImportGalaxyDomainParams) SetImportParameters(importParameters ImportGalaxyDomainBody) {
	o.ImportParameters = importParameters
}

// WriteToRequest writes these params to a swagger request
func (o *ImportGalaxyDomainParams) WriteToRequest(r runtime.ClientRequest, reg strfmt.Registry) error {

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
