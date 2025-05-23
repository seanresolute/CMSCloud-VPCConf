// Code generated by go-swagger; DO NOT EDIT.

package task

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

// NewDiscoverNetElementParams creates a new DiscoverNetElementParams object
// with the default values initialized.
func NewDiscoverNetElementParams() *DiscoverNetElementParams {
	var ()
	return &DiscoverNetElementParams{

		timeout: cr.DefaultTimeout,
	}
}

// NewDiscoverNetElementParamsWithTimeout creates a new DiscoverNetElementParams object
// with the default values initialized, and the ability to set a timeout on a request
func NewDiscoverNetElementParamsWithTimeout(timeout time.Duration) *DiscoverNetElementParams {
	var ()
	return &DiscoverNetElementParams{

		timeout: timeout,
	}
}

// NewDiscoverNetElementParamsWithContext creates a new DiscoverNetElementParams object
// with the default values initialized, and the ability to set a context for a request
func NewDiscoverNetElementParamsWithContext(ctx context.Context) *DiscoverNetElementParams {
	var ()
	return &DiscoverNetElementParams{

		Context: ctx,
	}
}

// NewDiscoverNetElementParamsWithHTTPClient creates a new DiscoverNetElementParams object
// with the default values initialized, and the ability to set a custom HTTPClient for a request
func NewDiscoverNetElementParamsWithHTTPClient(client *http.Client) *DiscoverNetElementParams {
	var ()
	return &DiscoverNetElementParams{
		HTTPClient: client,
	}
}

/*DiscoverNetElementParams contains all the parameters to send to the API endpoint
for the discover net element operation typically these are written to a http.Request
*/
type DiscoverNetElementParams struct {

	/*TaskParameters
	  Specify either elementName or ipAddress. Use elementName to specify the name of the device to discover. Use ipAddress to specify the ipAddress or fully-qualified (FQDN) of the device to discover. Set priority to true to create a high priority task.

	*/
	TaskParameters DiscoverNetElementBody

	timeout    time.Duration
	Context    context.Context
	HTTPClient *http.Client
}

// WithTimeout adds the timeout to the discover net element params
func (o *DiscoverNetElementParams) WithTimeout(timeout time.Duration) *DiscoverNetElementParams {
	o.SetTimeout(timeout)
	return o
}

// SetTimeout adds the timeout to the discover net element params
func (o *DiscoverNetElementParams) SetTimeout(timeout time.Duration) {
	o.timeout = timeout
}

// WithContext adds the context to the discover net element params
func (o *DiscoverNetElementParams) WithContext(ctx context.Context) *DiscoverNetElementParams {
	o.SetContext(ctx)
	return o
}

// SetContext adds the context to the discover net element params
func (o *DiscoverNetElementParams) SetContext(ctx context.Context) {
	o.Context = ctx
}

// WithHTTPClient adds the HTTPClient to the discover net element params
func (o *DiscoverNetElementParams) WithHTTPClient(client *http.Client) *DiscoverNetElementParams {
	o.SetHTTPClient(client)
	return o
}

// SetHTTPClient adds the HTTPClient to the discover net element params
func (o *DiscoverNetElementParams) SetHTTPClient(client *http.Client) {
	o.HTTPClient = client
}

// WithTaskParameters adds the taskParameters to the discover net element params
func (o *DiscoverNetElementParams) WithTaskParameters(taskParameters DiscoverNetElementBody) *DiscoverNetElementParams {
	o.SetTaskParameters(taskParameters)
	return o
}

// SetTaskParameters adds the taskParameters to the discover net element params
func (o *DiscoverNetElementParams) SetTaskParameters(taskParameters DiscoverNetElementBody) {
	o.TaskParameters = taskParameters
}

// WriteToRequest writes these params to a swagger request
func (o *DiscoverNetElementParams) WriteToRequest(r runtime.ClientRequest, reg strfmt.Registry) error {

	if err := r.SetTimeout(o.timeout); err != nil {
		return err
	}
	var res []error

	if err := r.SetBodyParam(o.TaskParameters); err != nil {
		return err
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}
