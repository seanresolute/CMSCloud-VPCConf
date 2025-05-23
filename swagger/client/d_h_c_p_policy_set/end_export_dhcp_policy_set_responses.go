// Code generated by go-swagger; DO NOT EDIT.

package d_h_c_p_policy_set

// This file was generated by the swagger tool.
// Editing this file might prove futile when you re-run the swagger generate command

import (
	"fmt"
	"io"

	"github.com/go-openapi/errors"
	"github.com/go-openapi/runtime"
	"github.com/go-openapi/swag"

	strfmt "github.com/go-openapi/strfmt"

	models "github.com/CMSgov/CMS-AWS-West-Network-Architecture/vpc-automation/swagger/models"
)

// EndExportDhcpPolicySetReader is a Reader for the EndExportDhcpPolicySet structure.
type EndExportDhcpPolicySetReader struct {
	formats strfmt.Registry
}

// ReadResponse reads a server response into the received o.
func (o *EndExportDhcpPolicySetReader) ReadResponse(response runtime.ClientResponse, consumer runtime.Consumer) (interface{}, error) {
	switch response.Code() {

	case 200:
		result := NewEndExportDhcpPolicySetOK()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return result, nil

	default:
		result := NewEndExportDhcpPolicySetDefault(response.Code())
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		if response.Code()/100 == 2 {
			return result, nil
		}
		return nil, result
	}
}

// NewEndExportDhcpPolicySetOK creates a EndExportDhcpPolicySetOK with default headers values
func NewEndExportDhcpPolicySetOK() *EndExportDhcpPolicySetOK {
	return &EndExportDhcpPolicySetOK{}
}

/*EndExportDhcpPolicySetOK handles this case with default header values.

Exports of DhcpPolicySet completed
*/
type EndExportDhcpPolicySetOK struct {
}

func (o *EndExportDhcpPolicySetOK) Error() string {
	return fmt.Sprintf("[POST /Exports/endExportDhcpPolicySet][%d] endExportDhcpPolicySetOK ", 200)
}

func (o *EndExportDhcpPolicySetOK) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	return nil
}

// NewEndExportDhcpPolicySetDefault creates a EndExportDhcpPolicySetDefault with default headers values
func NewEndExportDhcpPolicySetDefault(code int) *EndExportDhcpPolicySetDefault {
	return &EndExportDhcpPolicySetDefault{
		_statusCode: code,
	}
}

/*EndExportDhcpPolicySetDefault handles this case with default header values.

Error response
*/
type EndExportDhcpPolicySetDefault struct {
	_statusCode int

	Payload *models.Fault
}

// Code gets the status code for the end export dhcp policy set default response
func (o *EndExportDhcpPolicySetDefault) Code() int {
	return o._statusCode
}

func (o *EndExportDhcpPolicySetDefault) Error() string {
	return fmt.Sprintf("[POST /Exports/endExportDhcpPolicySet][%d] endExportDhcpPolicySet default  %+v", o._statusCode, o.Payload)
}

func (o *EndExportDhcpPolicySetDefault) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	o.Payload = new(models.Fault)

	// response payload
	if err := consumer.Consume(response.Body(), o.Payload); err != nil && err != io.EOF {
		return err
	}

	return nil
}

/*EndExportDhcpPolicySetBody end export dhcp policy set body
swagger:model EndExportDhcpPolicySetBody
*/
type EndExportDhcpPolicySetBody struct {

	// context
	Context *models.WSContext `json:"context,omitempty"`
}

// Validate validates this end export dhcp policy set body
func (o *EndExportDhcpPolicySetBody) Validate(formats strfmt.Registry) error {
	var res []error

	if err := o.validateContext(formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (o *EndExportDhcpPolicySetBody) validateContext(formats strfmt.Registry) error {

	if swag.IsZero(o.Context) { // not required
		return nil
	}

	if o.Context != nil {
		if err := o.Context.Validate(formats); err != nil {
			if ve, ok := err.(*errors.Validation); ok {
				return ve.ValidateName("wscontext" + "." + "context")
			}
			return err
		}
	}

	return nil
}

// MarshalBinary interface implementation
func (o *EndExportDhcpPolicySetBody) MarshalBinary() ([]byte, error) {
	if o == nil {
		return nil, nil
	}
	return swag.WriteJSON(o)
}

// UnmarshalBinary interface implementation
func (o *EndExportDhcpPolicySetBody) UnmarshalBinary(b []byte) error {
	var res EndExportDhcpPolicySetBody
	if err := swag.ReadJSON(b, &res); err != nil {
		return err
	}
	*o = res
	return nil
}
