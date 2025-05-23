// Code generated by go-swagger; DO NOT EDIT.

package d_h_c_p_option_set

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

// NewGetDhcpOptionSetByNameParams creates a new GetDhcpOptionSetByNameParams object
// with the default values initialized.
func NewGetDhcpOptionSetByNameParams() *GetDhcpOptionSetByNameParams {
	var ()
	return &GetDhcpOptionSetByNameParams{

		timeout: cr.DefaultTimeout,
	}
}

// NewGetDhcpOptionSetByNameParamsWithTimeout creates a new GetDhcpOptionSetByNameParams object
// with the default values initialized, and the ability to set a timeout on a request
func NewGetDhcpOptionSetByNameParamsWithTimeout(timeout time.Duration) *GetDhcpOptionSetByNameParams {
	var ()
	return &GetDhcpOptionSetByNameParams{

		timeout: timeout,
	}
}

// NewGetDhcpOptionSetByNameParamsWithContext creates a new GetDhcpOptionSetByNameParams object
// with the default values initialized, and the ability to set a context for a request
func NewGetDhcpOptionSetByNameParamsWithContext(ctx context.Context) *GetDhcpOptionSetByNameParams {
	var ()
	return &GetDhcpOptionSetByNameParams{

		Context: ctx,
	}
}

// NewGetDhcpOptionSetByNameParamsWithHTTPClient creates a new GetDhcpOptionSetByNameParams object
// with the default values initialized, and the ability to set a custom HTTPClient for a request
func NewGetDhcpOptionSetByNameParamsWithHTTPClient(client *http.Client) *GetDhcpOptionSetByNameParams {
	var ()
	return &GetDhcpOptionSetByNameParams{
		HTTPClient: client,
	}
}

/*GetDhcpOptionSetByNameParams contains all the parameters to send to the API endpoint
for the get dhcp option set by name operation typically these are written to a http.Request
*/
type GetDhcpOptionSetByNameParams struct {

	/*IPV6
	  Set to true for IPv6

	*/
	IPV6 bool
	/*SetName
	  The name of the DHCP option set

	*/
	SetName string

	timeout    time.Duration
	Context    context.Context
	HTTPClient *http.Client
}

// WithTimeout adds the timeout to the get dhcp option set by name params
func (o *GetDhcpOptionSetByNameParams) WithTimeout(timeout time.Duration) *GetDhcpOptionSetByNameParams {
	o.SetTimeout(timeout)
	return o
}

// SetTimeout adds the timeout to the get dhcp option set by name params
func (o *GetDhcpOptionSetByNameParams) SetTimeout(timeout time.Duration) {
	o.timeout = timeout
}

// WithContext adds the context to the get dhcp option set by name params
func (o *GetDhcpOptionSetByNameParams) WithContext(ctx context.Context) *GetDhcpOptionSetByNameParams {
	o.SetContext(ctx)
	return o
}

// SetContext adds the context to the get dhcp option set by name params
func (o *GetDhcpOptionSetByNameParams) SetContext(ctx context.Context) {
	o.Context = ctx
}

// WithHTTPClient adds the HTTPClient to the get dhcp option set by name params
func (o *GetDhcpOptionSetByNameParams) WithHTTPClient(client *http.Client) *GetDhcpOptionSetByNameParams {
	o.SetHTTPClient(client)
	return o
}

// SetHTTPClient adds the HTTPClient to the get dhcp option set by name params
func (o *GetDhcpOptionSetByNameParams) SetHTTPClient(client *http.Client) {
	o.HTTPClient = client
}

// WithIPV6 adds the iPV6 to the get dhcp option set by name params
func (o *GetDhcpOptionSetByNameParams) WithIPV6(iPV6 bool) *GetDhcpOptionSetByNameParams {
	o.SetIPV6(iPV6)
	return o
}

// SetIPV6 adds the ipv6 to the get dhcp option set by name params
func (o *GetDhcpOptionSetByNameParams) SetIPV6(iPV6 bool) {
	o.IPV6 = iPV6
}

// WithSetName adds the setName to the get dhcp option set by name params
func (o *GetDhcpOptionSetByNameParams) WithSetName(setName string) *GetDhcpOptionSetByNameParams {
	o.SetSetName(setName)
	return o
}

// SetSetName adds the setName to the get dhcp option set by name params
func (o *GetDhcpOptionSetByNameParams) SetSetName(setName string) {
	o.SetName = setName
}

// WriteToRequest writes these params to a swagger request
func (o *GetDhcpOptionSetByNameParams) WriteToRequest(r runtime.ClientRequest, reg strfmt.Registry) error {

	if err := r.SetTimeout(o.timeout); err != nil {
		return err
	}
	var res []error

	// query param ipv6
	qrIPV6 := o.IPV6
	qIPV6 := swag.FormatBool(qrIPV6)
	if qIPV6 != "" {
		if err := r.SetQueryParam("ipv6", qIPV6); err != nil {
			return err
		}
	}

	// query param setName
	qrSetName := o.SetName
	qSetName := qrSetName
	if qSetName != "" {
		if err := r.SetQueryParam("setName", qSetName); err != nil {
			return err
		}
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}
