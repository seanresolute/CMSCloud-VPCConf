// Code generated by go-swagger; DO NOT EDIT.

package child_block

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

// NewImportChildBlockParams creates a new ImportChildBlockParams object
// with the default values initialized.
func NewImportChildBlockParams() *ImportChildBlockParams {
	var ()
	return &ImportChildBlockParams{

		timeout: cr.DefaultTimeout,
	}
}

// NewImportChildBlockParamsWithTimeout creates a new ImportChildBlockParams object
// with the default values initialized, and the ability to set a timeout on a request
func NewImportChildBlockParamsWithTimeout(timeout time.Duration) *ImportChildBlockParams {
	var ()
	return &ImportChildBlockParams{

		timeout: timeout,
	}
}

// NewImportChildBlockParamsWithContext creates a new ImportChildBlockParams object
// with the default values initialized, and the ability to set a context for a request
func NewImportChildBlockParamsWithContext(ctx context.Context) *ImportChildBlockParams {
	var ()
	return &ImportChildBlockParams{

		Context: ctx,
	}
}

// NewImportChildBlockParamsWithHTTPClient creates a new ImportChildBlockParams object
// with the default values initialized, and the ability to set a custom HTTPClient for a request
func NewImportChildBlockParamsWithHTTPClient(client *http.Client) *ImportChildBlockParams {
	var ()
	return &ImportChildBlockParams{
		HTTPClient: client,
	}
}

/*ImportChildBlockParams contains all the parameters to send to the API endpoint
for the import child block operation typically these are written to a http.Request
*/
type ImportChildBlockParams struct {

	/*ImportParametersInpBlockPolicy
	  The input describing the child block. The following parameters are required: blockSize, blockStatus, container, interfaceName for devicecontainers only, userDefinedFields defined as required fields. No policy parameters are required. For userDefinedFields, specify comma-separated name=value pairs, where each pair is enclosed in double quotes.

	*/
	ImportParametersInpBlockPolicy ImportChildBlockBody

	timeout    time.Duration
	Context    context.Context
	HTTPClient *http.Client
}

// WithTimeout adds the timeout to the import child block params
func (o *ImportChildBlockParams) WithTimeout(timeout time.Duration) *ImportChildBlockParams {
	o.SetTimeout(timeout)
	return o
}

// SetTimeout adds the timeout to the import child block params
func (o *ImportChildBlockParams) SetTimeout(timeout time.Duration) {
	o.timeout = timeout
}

// WithContext adds the context to the import child block params
func (o *ImportChildBlockParams) WithContext(ctx context.Context) *ImportChildBlockParams {
	o.SetContext(ctx)
	return o
}

// SetContext adds the context to the import child block params
func (o *ImportChildBlockParams) SetContext(ctx context.Context) {
	o.Context = ctx
}

// WithHTTPClient adds the HTTPClient to the import child block params
func (o *ImportChildBlockParams) WithHTTPClient(client *http.Client) *ImportChildBlockParams {
	o.SetHTTPClient(client)
	return o
}

// SetHTTPClient adds the HTTPClient to the import child block params
func (o *ImportChildBlockParams) SetHTTPClient(client *http.Client) {
	o.HTTPClient = client
}

// WithImportParametersInpBlockPolicy adds the importParametersInpBlockPolicy to the import child block params
func (o *ImportChildBlockParams) WithImportParametersInpBlockPolicy(importParametersInpBlockPolicy ImportChildBlockBody) *ImportChildBlockParams {
	o.SetImportParametersInpBlockPolicy(importParametersInpBlockPolicy)
	return o
}

// SetImportParametersInpBlockPolicy adds the importParametersInpBlockPolicy to the import child block params
func (o *ImportChildBlockParams) SetImportParametersInpBlockPolicy(importParametersInpBlockPolicy ImportChildBlockBody) {
	o.ImportParametersInpBlockPolicy = importParametersInpBlockPolicy
}

// WriteToRequest writes these params to a swagger request
func (o *ImportChildBlockParams) WriteToRequest(r runtime.ClientRequest, reg strfmt.Registry) error {

	if err := r.SetTimeout(o.timeout); err != nil {
		return err
	}
	var res []error

	if err := r.SetBodyParam(o.ImportParametersInpBlockPolicy); err != nil {
		return err
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}
