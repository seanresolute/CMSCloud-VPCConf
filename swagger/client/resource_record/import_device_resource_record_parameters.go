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

// NewImportDeviceResourceRecordParams creates a new ImportDeviceResourceRecordParams object
// with the default values initialized.
func NewImportDeviceResourceRecordParams() *ImportDeviceResourceRecordParams {
	var ()
	return &ImportDeviceResourceRecordParams{

		timeout: cr.DefaultTimeout,
	}
}

// NewImportDeviceResourceRecordParamsWithTimeout creates a new ImportDeviceResourceRecordParams object
// with the default values initialized, and the ability to set a timeout on a request
func NewImportDeviceResourceRecordParamsWithTimeout(timeout time.Duration) *ImportDeviceResourceRecordParams {
	var ()
	return &ImportDeviceResourceRecordParams{

		timeout: timeout,
	}
}

// NewImportDeviceResourceRecordParamsWithContext creates a new ImportDeviceResourceRecordParams object
// with the default values initialized, and the ability to set a context for a request
func NewImportDeviceResourceRecordParamsWithContext(ctx context.Context) *ImportDeviceResourceRecordParams {
	var ()
	return &ImportDeviceResourceRecordParams{

		Context: ctx,
	}
}

// NewImportDeviceResourceRecordParamsWithHTTPClient creates a new ImportDeviceResourceRecordParams object
// with the default values initialized, and the ability to set a custom HTTPClient for a request
func NewImportDeviceResourceRecordParamsWithHTTPClient(client *http.Client) *ImportDeviceResourceRecordParams {
	var ()
	return &ImportDeviceResourceRecordParams{
		HTTPClient: client,
	}
}

/*ImportDeviceResourceRecordParams contains all the parameters to send to the API endpoint
for the import device resource record operation typically these are written to a http.Request
*/
type ImportDeviceResourceRecordParams struct {

	/*ImportParameters
	  The input describing the resource record. The following parameters are required: domain, hostname or ipAddress, owner, resourceRecType, data.The default date/time format for effectiveStart parameter is "MM/dd/yyyy hh:mm aa".Minutes if specified, must be in multiples of 5.

	*/
	ImportParameters ImportDeviceResourceRecordBody

	timeout    time.Duration
	Context    context.Context
	HTTPClient *http.Client
}

// WithTimeout adds the timeout to the import device resource record params
func (o *ImportDeviceResourceRecordParams) WithTimeout(timeout time.Duration) *ImportDeviceResourceRecordParams {
	o.SetTimeout(timeout)
	return o
}

// SetTimeout adds the timeout to the import device resource record params
func (o *ImportDeviceResourceRecordParams) SetTimeout(timeout time.Duration) {
	o.timeout = timeout
}

// WithContext adds the context to the import device resource record params
func (o *ImportDeviceResourceRecordParams) WithContext(ctx context.Context) *ImportDeviceResourceRecordParams {
	o.SetContext(ctx)
	return o
}

// SetContext adds the context to the import device resource record params
func (o *ImportDeviceResourceRecordParams) SetContext(ctx context.Context) {
	o.Context = ctx
}

// WithHTTPClient adds the HTTPClient to the import device resource record params
func (o *ImportDeviceResourceRecordParams) WithHTTPClient(client *http.Client) *ImportDeviceResourceRecordParams {
	o.SetHTTPClient(client)
	return o
}

// SetHTTPClient adds the HTTPClient to the import device resource record params
func (o *ImportDeviceResourceRecordParams) SetHTTPClient(client *http.Client) {
	o.HTTPClient = client
}

// WithImportParameters adds the importParameters to the import device resource record params
func (o *ImportDeviceResourceRecordParams) WithImportParameters(importParameters ImportDeviceResourceRecordBody) *ImportDeviceResourceRecordParams {
	o.SetImportParameters(importParameters)
	return o
}

// SetImportParameters adds the importParameters to the import device resource record params
func (o *ImportDeviceResourceRecordParams) SetImportParameters(importParameters ImportDeviceResourceRecordBody) {
	o.ImportParameters = importParameters
}

// WriteToRequest writes these params to a swagger request
func (o *ImportDeviceResourceRecordParams) WriteToRequest(r runtime.ClientRequest, reg strfmt.Registry) error {

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
