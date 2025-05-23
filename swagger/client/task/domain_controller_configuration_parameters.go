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

// NewDomainControllerConfigurationParams creates a new DomainControllerConfigurationParams object
// with the default values initialized.
func NewDomainControllerConfigurationParams() *DomainControllerConfigurationParams {
	var ()
	return &DomainControllerConfigurationParams{

		timeout: cr.DefaultTimeout,
	}
}

// NewDomainControllerConfigurationParamsWithTimeout creates a new DomainControllerConfigurationParams object
// with the default values initialized, and the ability to set a timeout on a request
func NewDomainControllerConfigurationParamsWithTimeout(timeout time.Duration) *DomainControllerConfigurationParams {
	var ()
	return &DomainControllerConfigurationParams{

		timeout: timeout,
	}
}

// NewDomainControllerConfigurationParamsWithContext creates a new DomainControllerConfigurationParams object
// with the default values initialized, and the ability to set a context for a request
func NewDomainControllerConfigurationParamsWithContext(ctx context.Context) *DomainControllerConfigurationParams {
	var ()
	return &DomainControllerConfigurationParams{

		Context: ctx,
	}
}

// NewDomainControllerConfigurationParamsWithHTTPClient creates a new DomainControllerConfigurationParams object
// with the default values initialized, and the ability to set a custom HTTPClient for a request
func NewDomainControllerConfigurationParamsWithHTTPClient(client *http.Client) *DomainControllerConfigurationParams {
	var ()
	return &DomainControllerConfigurationParams{
		HTTPClient: client,
	}
}

/*DomainControllerConfigurationParams contains all the parameters to send to the API endpoint
for the domain controller configuration operation typically these are written to a http.Request
*/
type DomainControllerConfigurationParams struct {

	/*TaskParameters
	  One of name or ip must be specified; dcName: Domain Controller Name.  ipAddress: IP Address of domain controller; preview: True to create a push preview; priority: True to create a high priority task.

	*/
	TaskParameters DomainControllerConfigurationBody

	timeout    time.Duration
	Context    context.Context
	HTTPClient *http.Client
}

// WithTimeout adds the timeout to the domain controller configuration params
func (o *DomainControllerConfigurationParams) WithTimeout(timeout time.Duration) *DomainControllerConfigurationParams {
	o.SetTimeout(timeout)
	return o
}

// SetTimeout adds the timeout to the domain controller configuration params
func (o *DomainControllerConfigurationParams) SetTimeout(timeout time.Duration) {
	o.timeout = timeout
}

// WithContext adds the context to the domain controller configuration params
func (o *DomainControllerConfigurationParams) WithContext(ctx context.Context) *DomainControllerConfigurationParams {
	o.SetContext(ctx)
	return o
}

// SetContext adds the context to the domain controller configuration params
func (o *DomainControllerConfigurationParams) SetContext(ctx context.Context) {
	o.Context = ctx
}

// WithHTTPClient adds the HTTPClient to the domain controller configuration params
func (o *DomainControllerConfigurationParams) WithHTTPClient(client *http.Client) *DomainControllerConfigurationParams {
	o.SetHTTPClient(client)
	return o
}

// SetHTTPClient adds the HTTPClient to the domain controller configuration params
func (o *DomainControllerConfigurationParams) SetHTTPClient(client *http.Client) {
	o.HTTPClient = client
}

// WithTaskParameters adds the taskParameters to the domain controller configuration params
func (o *DomainControllerConfigurationParams) WithTaskParameters(taskParameters DomainControllerConfigurationBody) *DomainControllerConfigurationParams {
	o.SetTaskParameters(taskParameters)
	return o
}

// SetTaskParameters adds the taskParameters to the domain controller configuration params
func (o *DomainControllerConfigurationParams) SetTaskParameters(taskParameters DomainControllerConfigurationBody) {
	o.TaskParameters = taskParameters
}

// WriteToRequest writes these params to a swagger request
func (o *DomainControllerConfigurationParams) WriteToRequest(r runtime.ClientRequest, reg strfmt.Registry) error {

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
