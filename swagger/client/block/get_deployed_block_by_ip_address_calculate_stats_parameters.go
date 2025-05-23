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
	"github.com/go-openapi/swag"

	strfmt "github.com/go-openapi/strfmt"
)

// NewGetDeployedBlockByIPAddressCalculateStatsParams creates a new GetDeployedBlockByIPAddressCalculateStatsParams object
// with the default values initialized.
func NewGetDeployedBlockByIPAddressCalculateStatsParams() *GetDeployedBlockByIPAddressCalculateStatsParams {
	var ()
	return &GetDeployedBlockByIPAddressCalculateStatsParams{

		timeout: cr.DefaultTimeout,
	}
}

// NewGetDeployedBlockByIPAddressCalculateStatsParamsWithTimeout creates a new GetDeployedBlockByIPAddressCalculateStatsParams object
// with the default values initialized, and the ability to set a timeout on a request
func NewGetDeployedBlockByIPAddressCalculateStatsParamsWithTimeout(timeout time.Duration) *GetDeployedBlockByIPAddressCalculateStatsParams {
	var ()
	return &GetDeployedBlockByIPAddressCalculateStatsParams{

		timeout: timeout,
	}
}

// NewGetDeployedBlockByIPAddressCalculateStatsParamsWithContext creates a new GetDeployedBlockByIPAddressCalculateStatsParams object
// with the default values initialized, and the ability to set a context for a request
func NewGetDeployedBlockByIPAddressCalculateStatsParamsWithContext(ctx context.Context) *GetDeployedBlockByIPAddressCalculateStatsParams {
	var ()
	return &GetDeployedBlockByIPAddressCalculateStatsParams{

		Context: ctx,
	}
}

// NewGetDeployedBlockByIPAddressCalculateStatsParamsWithHTTPClient creates a new GetDeployedBlockByIPAddressCalculateStatsParams object
// with the default values initialized, and the ability to set a custom HTTPClient for a request
func NewGetDeployedBlockByIPAddressCalculateStatsParamsWithHTTPClient(client *http.Client) *GetDeployedBlockByIPAddressCalculateStatsParams {
	var ()
	return &GetDeployedBlockByIPAddressCalculateStatsParams{
		HTTPClient: client,
	}
}

/*GetDeployedBlockByIPAddressCalculateStatsParams contains all the parameters to send to the API endpoint
for the get deployed block by Ip address calculate stats operation typically these are written to a http.Request
*/
type GetDeployedBlockByIPAddressCalculateStatsParams struct {

	/*Bsize
	  The block's CIDR size. Required only if there are multiple blocks with the same starting address, e.g., 10.0.0.0/8 (aggregate) and 10.0.0.0/24 (child block).

	*/
	Bsize *int64
	/*Container
	  The container holding the block. Required only if IP address is not unique.

	*/
	Container *string
	/*IPAddress
	  The starting IP address of the block, e.g. 10.0.0.0

	*/
	IPAddress string

	timeout    time.Duration
	Context    context.Context
	HTTPClient *http.Client
}

// WithTimeout adds the timeout to the get deployed block by Ip address calculate stats params
func (o *GetDeployedBlockByIPAddressCalculateStatsParams) WithTimeout(timeout time.Duration) *GetDeployedBlockByIPAddressCalculateStatsParams {
	o.SetTimeout(timeout)
	return o
}

// SetTimeout adds the timeout to the get deployed block by Ip address calculate stats params
func (o *GetDeployedBlockByIPAddressCalculateStatsParams) SetTimeout(timeout time.Duration) {
	o.timeout = timeout
}

// WithContext adds the context to the get deployed block by Ip address calculate stats params
func (o *GetDeployedBlockByIPAddressCalculateStatsParams) WithContext(ctx context.Context) *GetDeployedBlockByIPAddressCalculateStatsParams {
	o.SetContext(ctx)
	return o
}

// SetContext adds the context to the get deployed block by Ip address calculate stats params
func (o *GetDeployedBlockByIPAddressCalculateStatsParams) SetContext(ctx context.Context) {
	o.Context = ctx
}

// WithHTTPClient adds the HTTPClient to the get deployed block by Ip address calculate stats params
func (o *GetDeployedBlockByIPAddressCalculateStatsParams) WithHTTPClient(client *http.Client) *GetDeployedBlockByIPAddressCalculateStatsParams {
	o.SetHTTPClient(client)
	return o
}

// SetHTTPClient adds the HTTPClient to the get deployed block by Ip address calculate stats params
func (o *GetDeployedBlockByIPAddressCalculateStatsParams) SetHTTPClient(client *http.Client) {
	o.HTTPClient = client
}

// WithBsize adds the bsize to the get deployed block by Ip address calculate stats params
func (o *GetDeployedBlockByIPAddressCalculateStatsParams) WithBsize(bsize *int64) *GetDeployedBlockByIPAddressCalculateStatsParams {
	o.SetBsize(bsize)
	return o
}

// SetBsize adds the bsize to the get deployed block by Ip address calculate stats params
func (o *GetDeployedBlockByIPAddressCalculateStatsParams) SetBsize(bsize *int64) {
	o.Bsize = bsize
}

// WithContainer adds the container to the get deployed block by Ip address calculate stats params
func (o *GetDeployedBlockByIPAddressCalculateStatsParams) WithContainer(container *string) *GetDeployedBlockByIPAddressCalculateStatsParams {
	o.SetContainer(container)
	return o
}

// SetContainer adds the container to the get deployed block by Ip address calculate stats params
func (o *GetDeployedBlockByIPAddressCalculateStatsParams) SetContainer(container *string) {
	o.Container = container
}

// WithIPAddress adds the iPAddress to the get deployed block by Ip address calculate stats params
func (o *GetDeployedBlockByIPAddressCalculateStatsParams) WithIPAddress(iPAddress string) *GetDeployedBlockByIPAddressCalculateStatsParams {
	o.SetIPAddress(iPAddress)
	return o
}

// SetIPAddress adds the ipAddress to the get deployed block by Ip address calculate stats params
func (o *GetDeployedBlockByIPAddressCalculateStatsParams) SetIPAddress(iPAddress string) {
	o.IPAddress = iPAddress
}

// WriteToRequest writes these params to a swagger request
func (o *GetDeployedBlockByIPAddressCalculateStatsParams) WriteToRequest(r runtime.ClientRequest, reg strfmt.Registry) error {

	if err := r.SetTimeout(o.timeout); err != nil {
		return err
	}
	var res []error

	if o.Bsize != nil {

		// query param bsize
		var qrBsize int64
		if o.Bsize != nil {
			qrBsize = *o.Bsize
		}
		qBsize := swag.FormatInt64(qrBsize)
		if qBsize != "" {
			if err := r.SetQueryParam("bsize", qBsize); err != nil {
				return err
			}
		}

	}

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

	// query param ipAddress
	qrIPAddress := o.IPAddress
	qIPAddress := qrIPAddress
	if qIPAddress != "" {
		if err := r.SetQueryParam("ipAddress", qIPAddress); err != nil {
			return err
		}
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}
