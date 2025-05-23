// Code generated by go-swagger; DO NOT EDIT.

package device

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

// NewUseNextReservedIPAddressParams creates a new UseNextReservedIPAddressParams object
// with the default values initialized.
func NewUseNextReservedIPAddressParams() *UseNextReservedIPAddressParams {
	var ()
	return &UseNextReservedIPAddressParams{

		timeout: cr.DefaultTimeout,
	}
}

// NewUseNextReservedIPAddressParamsWithTimeout creates a new UseNextReservedIPAddressParams object
// with the default values initialized, and the ability to set a timeout on a request
func NewUseNextReservedIPAddressParamsWithTimeout(timeout time.Duration) *UseNextReservedIPAddressParams {
	var ()
	return &UseNextReservedIPAddressParams{

		timeout: timeout,
	}
}

// NewUseNextReservedIPAddressParamsWithContext creates a new UseNextReservedIPAddressParams object
// with the default values initialized, and the ability to set a context for a request
func NewUseNextReservedIPAddressParamsWithContext(ctx context.Context) *UseNextReservedIPAddressParams {
	var ()
	return &UseNextReservedIPAddressParams{

		Context: ctx,
	}
}

// NewUseNextReservedIPAddressParamsWithHTTPClient creates a new UseNextReservedIPAddressParams object
// with the default values initialized, and the ability to set a custom HTTPClient for a request
func NewUseNextReservedIPAddressParamsWithHTTPClient(client *http.Client) *UseNextReservedIPAddressParams {
	var ()
	return &UseNextReservedIPAddressParams{
		HTTPClient: client,
	}
}

/*UseNextReservedIPAddressParams contains all the parameters to send to the API endpoint
for the use next reserved Ip address operation typically these are written to a http.Request
*/
type UseNextReservedIPAddressParams struct {

	/*ModifyParameters
	  The input describing the IP Address. The following parameters are required: ipAddress, devicetype. Note the ipAddress must be specified in the main structure, not within the interfaces structure. You can also specify a hostname. Specify resourceRecordFlag if you want resource records to be created for the device. All other parameters are ignored.

	*/
	ModifyParameters UseNextReservedIPAddressBody

	timeout    time.Duration
	Context    context.Context
	HTTPClient *http.Client
}

// WithTimeout adds the timeout to the use next reserved Ip address params
func (o *UseNextReservedIPAddressParams) WithTimeout(timeout time.Duration) *UseNextReservedIPAddressParams {
	o.SetTimeout(timeout)
	return o
}

// SetTimeout adds the timeout to the use next reserved Ip address params
func (o *UseNextReservedIPAddressParams) SetTimeout(timeout time.Duration) {
	o.timeout = timeout
}

// WithContext adds the context to the use next reserved Ip address params
func (o *UseNextReservedIPAddressParams) WithContext(ctx context.Context) *UseNextReservedIPAddressParams {
	o.SetContext(ctx)
	return o
}

// SetContext adds the context to the use next reserved Ip address params
func (o *UseNextReservedIPAddressParams) SetContext(ctx context.Context) {
	o.Context = ctx
}

// WithHTTPClient adds the HTTPClient to the use next reserved Ip address params
func (o *UseNextReservedIPAddressParams) WithHTTPClient(client *http.Client) *UseNextReservedIPAddressParams {
	o.SetHTTPClient(client)
	return o
}

// SetHTTPClient adds the HTTPClient to the use next reserved Ip address params
func (o *UseNextReservedIPAddressParams) SetHTTPClient(client *http.Client) {
	o.HTTPClient = client
}

// WithModifyParameters adds the modifyParameters to the use next reserved Ip address params
func (o *UseNextReservedIPAddressParams) WithModifyParameters(modifyParameters UseNextReservedIPAddressBody) *UseNextReservedIPAddressParams {
	o.SetModifyParameters(modifyParameters)
	return o
}

// SetModifyParameters adds the modifyParameters to the use next reserved Ip address params
func (o *UseNextReservedIPAddressParams) SetModifyParameters(modifyParameters UseNextReservedIPAddressBody) {
	o.ModifyParameters = modifyParameters
}

// WriteToRequest writes these params to a swagger request
func (o *UseNextReservedIPAddressParams) WriteToRequest(r runtime.ClientRequest, reg strfmt.Registry) error {

	if err := r.SetTimeout(o.timeout); err != nil {
		return err
	}
	var res []error

	if err := r.SetBodyParam(o.ModifyParameters); err != nil {
		return err
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}
