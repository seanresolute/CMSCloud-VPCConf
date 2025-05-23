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

// NewDhcpConfigurationAllFilesByTagParams creates a new DhcpConfigurationAllFilesByTagParams object
// with the default values initialized.
func NewDhcpConfigurationAllFilesByTagParams() *DhcpConfigurationAllFilesByTagParams {
	var ()
	return &DhcpConfigurationAllFilesByTagParams{

		timeout: cr.DefaultTimeout,
	}
}

// NewDhcpConfigurationAllFilesByTagParamsWithTimeout creates a new DhcpConfigurationAllFilesByTagParams object
// with the default values initialized, and the ability to set a timeout on a request
func NewDhcpConfigurationAllFilesByTagParamsWithTimeout(timeout time.Duration) *DhcpConfigurationAllFilesByTagParams {
	var ()
	return &DhcpConfigurationAllFilesByTagParams{

		timeout: timeout,
	}
}

// NewDhcpConfigurationAllFilesByTagParamsWithContext creates a new DhcpConfigurationAllFilesByTagParams object
// with the default values initialized, and the ability to set a context for a request
func NewDhcpConfigurationAllFilesByTagParamsWithContext(ctx context.Context) *DhcpConfigurationAllFilesByTagParams {
	var ()
	return &DhcpConfigurationAllFilesByTagParams{

		Context: ctx,
	}
}

// NewDhcpConfigurationAllFilesByTagParamsWithHTTPClient creates a new DhcpConfigurationAllFilesByTagParams object
// with the default values initialized, and the ability to set a custom HTTPClient for a request
func NewDhcpConfigurationAllFilesByTagParamsWithHTTPClient(client *http.Client) *DhcpConfigurationAllFilesByTagParams {
	var ()
	return &DhcpConfigurationAllFilesByTagParams{
		HTTPClient: client,
	}
}

/*DhcpConfigurationAllFilesByTagParams contains all the parameters to send to the API endpoint
for the dhcp configuration all files by tag operation typically these are written to a http.Request
*/
type DhcpConfigurationAllFilesByTagParams struct {

	/*TaskParameters
	  A tag must be specified. stopOnError: Specify whether to stop if an error is encountered or ignore the error and continue; pushOnlyChanges: Specify whether to push the files even if there have been no configuration changes; updateFailovers: Specify whether the push should also update failover servers. ignoreDdnsError: If stopOnError is true, specifiy if Dynamic DNS errors and warnings should be ignored. Set priority to true to create a high priority task.creatediff: Set to true to create a file of differences from the last known good configuration

	*/
	TaskParameters DhcpConfigurationAllFilesByTagBody

	timeout    time.Duration
	Context    context.Context
	HTTPClient *http.Client
}

// WithTimeout adds the timeout to the dhcp configuration all files by tag params
func (o *DhcpConfigurationAllFilesByTagParams) WithTimeout(timeout time.Duration) *DhcpConfigurationAllFilesByTagParams {
	o.SetTimeout(timeout)
	return o
}

// SetTimeout adds the timeout to the dhcp configuration all files by tag params
func (o *DhcpConfigurationAllFilesByTagParams) SetTimeout(timeout time.Duration) {
	o.timeout = timeout
}

// WithContext adds the context to the dhcp configuration all files by tag params
func (o *DhcpConfigurationAllFilesByTagParams) WithContext(ctx context.Context) *DhcpConfigurationAllFilesByTagParams {
	o.SetContext(ctx)
	return o
}

// SetContext adds the context to the dhcp configuration all files by tag params
func (o *DhcpConfigurationAllFilesByTagParams) SetContext(ctx context.Context) {
	o.Context = ctx
}

// WithHTTPClient adds the HTTPClient to the dhcp configuration all files by tag params
func (o *DhcpConfigurationAllFilesByTagParams) WithHTTPClient(client *http.Client) *DhcpConfigurationAllFilesByTagParams {
	o.SetHTTPClient(client)
	return o
}

// SetHTTPClient adds the HTTPClient to the dhcp configuration all files by tag params
func (o *DhcpConfigurationAllFilesByTagParams) SetHTTPClient(client *http.Client) {
	o.HTTPClient = client
}

// WithTaskParameters adds the taskParameters to the dhcp configuration all files by tag params
func (o *DhcpConfigurationAllFilesByTagParams) WithTaskParameters(taskParameters DhcpConfigurationAllFilesByTagBody) *DhcpConfigurationAllFilesByTagParams {
	o.SetTaskParameters(taskParameters)
	return o
}

// SetTaskParameters adds the taskParameters to the dhcp configuration all files by tag params
func (o *DhcpConfigurationAllFilesByTagParams) SetTaskParameters(taskParameters DhcpConfigurationAllFilesByTagBody) {
	o.TaskParameters = taskParameters
}

// WriteToRequest writes these params to a swagger request
func (o *DhcpConfigurationAllFilesByTagParams) WriteToRequest(r runtime.ClientRequest, reg strfmt.Registry) error {

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
