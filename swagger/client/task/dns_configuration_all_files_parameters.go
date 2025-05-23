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

// NewDNSConfigurationAllFilesParams creates a new DNSConfigurationAllFilesParams object
// with the default values initialized.
func NewDNSConfigurationAllFilesParams() *DNSConfigurationAllFilesParams {
	var ()
	return &DNSConfigurationAllFilesParams{

		timeout: cr.DefaultTimeout,
	}
}

// NewDNSConfigurationAllFilesParamsWithTimeout creates a new DNSConfigurationAllFilesParams object
// with the default values initialized, and the ability to set a timeout on a request
func NewDNSConfigurationAllFilesParamsWithTimeout(timeout time.Duration) *DNSConfigurationAllFilesParams {
	var ()
	return &DNSConfigurationAllFilesParams{

		timeout: timeout,
	}
}

// NewDNSConfigurationAllFilesParamsWithContext creates a new DNSConfigurationAllFilesParams object
// with the default values initialized, and the ability to set a context for a request
func NewDNSConfigurationAllFilesParamsWithContext(ctx context.Context) *DNSConfigurationAllFilesParams {
	var ()
	return &DNSConfigurationAllFilesParams{

		Context: ctx,
	}
}

// NewDNSConfigurationAllFilesParamsWithHTTPClient creates a new DNSConfigurationAllFilesParams object
// with the default values initialized, and the ability to set a custom HTTPClient for a request
func NewDNSConfigurationAllFilesParamsWithHTTPClient(client *http.Client) *DNSConfigurationAllFilesParams {
	var ()
	return &DNSConfigurationAllFilesParams{
		HTTPClient: client,
	}
}

/*DNSConfigurationAllFilesParams contains all the parameters to send to the API endpoint
for the dns configuration all files operation typically these are written to a http.Request
*/
type DNSConfigurationAllFilesParams struct {

	/*TaskParameters
	  One of name or ip must be specified. name: Net service Name;   ip: Net service IP Address; abortfailedcheck: Set to true if the push should halt if either check configuration or check zones fails; checkconf: Set to true if the config file checker should be run; Set priority to true to create a high priority task; creatediff: Set to true to create a file of differences from the last known good configuration.

	*/
	TaskParameters DNSConfigurationAllFilesBody

	timeout    time.Duration
	Context    context.Context
	HTTPClient *http.Client
}

// WithTimeout adds the timeout to the dns configuration all files params
func (o *DNSConfigurationAllFilesParams) WithTimeout(timeout time.Duration) *DNSConfigurationAllFilesParams {
	o.SetTimeout(timeout)
	return o
}

// SetTimeout adds the timeout to the dns configuration all files params
func (o *DNSConfigurationAllFilesParams) SetTimeout(timeout time.Duration) {
	o.timeout = timeout
}

// WithContext adds the context to the dns configuration all files params
func (o *DNSConfigurationAllFilesParams) WithContext(ctx context.Context) *DNSConfigurationAllFilesParams {
	o.SetContext(ctx)
	return o
}

// SetContext adds the context to the dns configuration all files params
func (o *DNSConfigurationAllFilesParams) SetContext(ctx context.Context) {
	o.Context = ctx
}

// WithHTTPClient adds the HTTPClient to the dns configuration all files params
func (o *DNSConfigurationAllFilesParams) WithHTTPClient(client *http.Client) *DNSConfigurationAllFilesParams {
	o.SetHTTPClient(client)
	return o
}

// SetHTTPClient adds the HTTPClient to the dns configuration all files params
func (o *DNSConfigurationAllFilesParams) SetHTTPClient(client *http.Client) {
	o.HTTPClient = client
}

// WithTaskParameters adds the taskParameters to the dns configuration all files params
func (o *DNSConfigurationAllFilesParams) WithTaskParameters(taskParameters DNSConfigurationAllFilesBody) *DNSConfigurationAllFilesParams {
	o.SetTaskParameters(taskParameters)
	return o
}

// SetTaskParameters adds the taskParameters to the dns configuration all files params
func (o *DNSConfigurationAllFilesParams) SetTaskParameters(taskParameters DNSConfigurationAllFilesBody) {
	o.TaskParameters = taskParameters
}

// WriteToRequest writes these params to a swagger request
func (o *DNSConfigurationAllFilesParams) WriteToRequest(r runtime.ClientRequest, reg strfmt.Registry) error {

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
