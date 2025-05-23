// Code generated by go-swagger; DO NOT EDIT.

package container

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

// NewGetContainerParentHierarchyParams creates a new GetContainerParentHierarchyParams object
// with the default values initialized.
func NewGetContainerParentHierarchyParams() *GetContainerParentHierarchyParams {
	var ()
	return &GetContainerParentHierarchyParams{

		timeout: cr.DefaultTimeout,
	}
}

// NewGetContainerParentHierarchyParamsWithTimeout creates a new GetContainerParentHierarchyParams object
// with the default values initialized, and the ability to set a timeout on a request
func NewGetContainerParentHierarchyParamsWithTimeout(timeout time.Duration) *GetContainerParentHierarchyParams {
	var ()
	return &GetContainerParentHierarchyParams{

		timeout: timeout,
	}
}

// NewGetContainerParentHierarchyParamsWithContext creates a new GetContainerParentHierarchyParams object
// with the default values initialized, and the ability to set a context for a request
func NewGetContainerParentHierarchyParamsWithContext(ctx context.Context) *GetContainerParentHierarchyParams {
	var ()
	return &GetContainerParentHierarchyParams{

		Context: ctx,
	}
}

// NewGetContainerParentHierarchyParamsWithHTTPClient creates a new GetContainerParentHierarchyParams object
// with the default values initialized, and the ability to set a custom HTTPClient for a request
func NewGetContainerParentHierarchyParamsWithHTTPClient(client *http.Client) *GetContainerParentHierarchyParams {
	var ()
	return &GetContainerParentHierarchyParams{
		HTTPClient: client,
	}
}

/*GetContainerParentHierarchyParams contains all the parameters to send to the API endpoint
for the get container parent hierarchy operation typically these are written to a http.Request
*/
type GetContainerParentHierarchyParams struct {

	/*ContainerName
	  The name of the container

	*/
	ContainerName string

	timeout    time.Duration
	Context    context.Context
	HTTPClient *http.Client
}

// WithTimeout adds the timeout to the get container parent hierarchy params
func (o *GetContainerParentHierarchyParams) WithTimeout(timeout time.Duration) *GetContainerParentHierarchyParams {
	o.SetTimeout(timeout)
	return o
}

// SetTimeout adds the timeout to the get container parent hierarchy params
func (o *GetContainerParentHierarchyParams) SetTimeout(timeout time.Duration) {
	o.timeout = timeout
}

// WithContext adds the context to the get container parent hierarchy params
func (o *GetContainerParentHierarchyParams) WithContext(ctx context.Context) *GetContainerParentHierarchyParams {
	o.SetContext(ctx)
	return o
}

// SetContext adds the context to the get container parent hierarchy params
func (o *GetContainerParentHierarchyParams) SetContext(ctx context.Context) {
	o.Context = ctx
}

// WithHTTPClient adds the HTTPClient to the get container parent hierarchy params
func (o *GetContainerParentHierarchyParams) WithHTTPClient(client *http.Client) *GetContainerParentHierarchyParams {
	o.SetHTTPClient(client)
	return o
}

// SetHTTPClient adds the HTTPClient to the get container parent hierarchy params
func (o *GetContainerParentHierarchyParams) SetHTTPClient(client *http.Client) {
	o.HTTPClient = client
}

// WithContainerName adds the containerName to the get container parent hierarchy params
func (o *GetContainerParentHierarchyParams) WithContainerName(containerName string) *GetContainerParentHierarchyParams {
	o.SetContainerName(containerName)
	return o
}

// SetContainerName adds the containerName to the get container parent hierarchy params
func (o *GetContainerParentHierarchyParams) SetContainerName(containerName string) {
	o.ContainerName = containerName
}

// WriteToRequest writes these params to a swagger request
func (o *GetContainerParentHierarchyParams) WriteToRequest(r runtime.ClientRequest, reg strfmt.Registry) error {

	if err := r.SetTimeout(o.timeout); err != nil {
		return err
	}
	var res []error

	// query param containerName
	qrContainerName := o.ContainerName
	qContainerName := qrContainerName
	if qContainerName != "" {
		if err := r.SetQueryParam("containerName", qContainerName); err != nil {
			return err
		}
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}
