// Code generated by go-swagger; DO NOT EDIT.

package block

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

// NewGetDeployedBlockByNameCalculateStatsParams creates a new GetDeployedBlockByNameCalculateStatsParams object
// with the default values initialized.
func NewGetDeployedBlockByNameCalculateStatsParams() *GetDeployedBlockByNameCalculateStatsParams {
	var ()
	return &GetDeployedBlockByNameCalculateStatsParams{

		timeout: cr.DefaultTimeout,
	}
}

// NewGetDeployedBlockByNameCalculateStatsParamsWithTimeout creates a new GetDeployedBlockByNameCalculateStatsParams object
// with the default values initialized, and the ability to set a timeout on a request
func NewGetDeployedBlockByNameCalculateStatsParamsWithTimeout(timeout time.Duration) *GetDeployedBlockByNameCalculateStatsParams {
	var ()
	return &GetDeployedBlockByNameCalculateStatsParams{

		timeout: timeout,
	}
}

// NewGetDeployedBlockByNameCalculateStatsParamsWithContext creates a new GetDeployedBlockByNameCalculateStatsParams object
// with the default values initialized, and the ability to set a context for a request
func NewGetDeployedBlockByNameCalculateStatsParamsWithContext(ctx context.Context) *GetDeployedBlockByNameCalculateStatsParams {
	var ()
	return &GetDeployedBlockByNameCalculateStatsParams{

		Context: ctx,
	}
}

// NewGetDeployedBlockByNameCalculateStatsParamsWithHTTPClient creates a new GetDeployedBlockByNameCalculateStatsParams object
// with the default values initialized, and the ability to set a custom HTTPClient for a request
func NewGetDeployedBlockByNameCalculateStatsParamsWithHTTPClient(client *http.Client) *GetDeployedBlockByNameCalculateStatsParams {
	var ()
	return &GetDeployedBlockByNameCalculateStatsParams{
		HTTPClient: client,
	}
}

/*GetDeployedBlockByNameCalculateStatsParams contains all the parameters to send to the API endpoint
for the get deployed block by name calculate stats operation typically these are written to a http.Request
*/
type GetDeployedBlockByNameCalculateStatsParams struct {

	/*Container
	  The container holding the block. Required only if block name is not unique.

	*/
	Container *string
	/*Name
	  The Name of the block, e.g. 10.0.0.0/24

	*/
	Name string

	timeout    time.Duration
	Context    context.Context
	HTTPClient *http.Client
}

// WithTimeout adds the timeout to the get deployed block by name calculate stats params
func (o *GetDeployedBlockByNameCalculateStatsParams) WithTimeout(timeout time.Duration) *GetDeployedBlockByNameCalculateStatsParams {
	o.SetTimeout(timeout)
	return o
}

// SetTimeout adds the timeout to the get deployed block by name calculate stats params
func (o *GetDeployedBlockByNameCalculateStatsParams) SetTimeout(timeout time.Duration) {
	o.timeout = timeout
}

// WithContext adds the context to the get deployed block by name calculate stats params
func (o *GetDeployedBlockByNameCalculateStatsParams) WithContext(ctx context.Context) *GetDeployedBlockByNameCalculateStatsParams {
	o.SetContext(ctx)
	return o
}

// SetContext adds the context to the get deployed block by name calculate stats params
func (o *GetDeployedBlockByNameCalculateStatsParams) SetContext(ctx context.Context) {
	o.Context = ctx
}

// WithHTTPClient adds the HTTPClient to the get deployed block by name calculate stats params
func (o *GetDeployedBlockByNameCalculateStatsParams) WithHTTPClient(client *http.Client) *GetDeployedBlockByNameCalculateStatsParams {
	o.SetHTTPClient(client)
	return o
}

// SetHTTPClient adds the HTTPClient to the get deployed block by name calculate stats params
func (o *GetDeployedBlockByNameCalculateStatsParams) SetHTTPClient(client *http.Client) {
	o.HTTPClient = client
}

// WithContainer adds the container to the get deployed block by name calculate stats params
func (o *GetDeployedBlockByNameCalculateStatsParams) WithContainer(container *string) *GetDeployedBlockByNameCalculateStatsParams {
	o.SetContainer(container)
	return o
}

// SetContainer adds the container to the get deployed block by name calculate stats params
func (o *GetDeployedBlockByNameCalculateStatsParams) SetContainer(container *string) {
	o.Container = container
}

// WithName adds the name to the get deployed block by name calculate stats params
func (o *GetDeployedBlockByNameCalculateStatsParams) WithName(name string) *GetDeployedBlockByNameCalculateStatsParams {
	o.SetName(name)
	return o
}

// SetName adds the name to the get deployed block by name calculate stats params
func (o *GetDeployedBlockByNameCalculateStatsParams) SetName(name string) {
	o.Name = name
}

// WriteToRequest writes these params to a swagger request
func (o *GetDeployedBlockByNameCalculateStatsParams) WriteToRequest(r runtime.ClientRequest, reg strfmt.Registry) error {

	if err := r.SetTimeout(o.timeout); err != nil {
		return err
	}
	var res []error

	if o.Container != nil {

		// query param container
		var qrContainer string
		if o.Container != nil {
			qrContainer = *o.Container
		}
		qContainer := qrContainer
		if qContainer != "" {
			if err := r.SetQueryParam("container", qContainer); err != nil {
				return err
			}
		}

	}

	// query param name
	qrName := o.Name
	qName := qrName
	if qName != "" {
		if err := r.SetQueryParam("name", qName); err != nil {
			return err
		}
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}
