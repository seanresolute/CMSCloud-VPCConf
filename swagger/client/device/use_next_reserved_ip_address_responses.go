// Code generated by go-swagger; DO NOT EDIT.

package device

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

// UseNextReservedIPAddressReader is a Reader for the UseNextReservedIPAddress structure.
type UseNextReservedIPAddressReader struct {
	formats strfmt.Registry
}

// ReadResponse reads a server response into the received o.
func (o *UseNextReservedIPAddressReader) ReadResponse(response runtime.ClientResponse, consumer runtime.Consumer) (interface{}, error) {
	switch response.Code() {

	case 200:
		result := NewUseNextReservedIPAddressOK()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return result, nil

	default:
		result := NewUseNextReservedIPAddressDefault(response.Code())
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		if response.Code()/100 == 2 {
			return result, nil
		}
		return nil, result
	}
}

// NewUseNextReservedIPAddressOK creates a UseNextReservedIPAddressOK with default headers values
func NewUseNextReservedIPAddressOK() *UseNextReservedIPAddressOK {
	return &UseNextReservedIPAddressOK{}
}

/*UseNextReservedIPAddressOK handles this case with default header values.

IPAddress returned
*/
type UseNextReservedIPAddressOK struct {
	Payload *UseNextReservedIPAddressOKBody
}

func (o *UseNextReservedIPAddressOK) Error() string {
	return fmt.Sprintf("[POST /IncUseNextReservedIPAddress/useNextReservedIPAddress][%d] useNextReservedIpAddressOK  %+v", 200, o.Payload)
}

func (o *UseNextReservedIPAddressOK) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	o.Payload = new(UseNextReservedIPAddressOKBody)

	// response payload
	if err := consumer.Consume(response.Body(), o.Payload); err != nil && err != io.EOF {
		return err
	}

	return nil
}

// NewUseNextReservedIPAddressDefault creates a UseNextReservedIPAddressDefault with default headers values
func NewUseNextReservedIPAddressDefault(code int) *UseNextReservedIPAddressDefault {
	return &UseNextReservedIPAddressDefault{
		_statusCode: code,
	}
}

/*UseNextReservedIPAddressDefault handles this case with default header values.

Error response
*/
type UseNextReservedIPAddressDefault struct {
	_statusCode int

	Payload *models.Fault
}

// Code gets the status code for the use next reserved Ip address default response
func (o *UseNextReservedIPAddressDefault) Code() int {
	return o._statusCode
}

func (o *UseNextReservedIPAddressDefault) Error() string {
	return fmt.Sprintf("[POST /IncUseNextReservedIPAddress/useNextReservedIPAddress][%d] useNextReservedIpAddress default  %+v", o._statusCode, o.Payload)
}

func (o *UseNextReservedIPAddressDefault) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	o.Payload = new(models.Fault)

	// response payload
	if err := consumer.Consume(response.Body(), o.Payload); err != nil && err != io.EOF {
		return err
	}

	return nil
}

/*UseNextReservedIPAddressBody use next reserved IP address body
swagger:model UseNextReservedIPAddressBody
*/
type UseNextReservedIPAddressBody struct {

	// inp device
	InpDevice *models.WSDevice `json:"inpDevice,omitempty"`
}

// Validate validates this use next reserved IP address body
func (o *UseNextReservedIPAddressBody) Validate(formats strfmt.Registry) error {
	var res []error

	if err := o.validateInpDevice(formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (o *UseNextReservedIPAddressBody) validateInpDevice(formats strfmt.Registry) error {

	if swag.IsZero(o.InpDevice) { // not required
		return nil
	}

	if o.InpDevice != nil {
		if err := o.InpDevice.Validate(formats); err != nil {
			if ve, ok := err.(*errors.Validation); ok {
				return ve.ValidateName("Modify Parameters" + "." + "inpDevice")
			}
			return err
		}
	}

	return nil
}

// MarshalBinary interface implementation
func (o *UseNextReservedIPAddressBody) MarshalBinary() ([]byte, error) {
	if o == nil {
		return nil, nil
	}
	return swag.WriteJSON(o)
}

// UnmarshalBinary interface implementation
func (o *UseNextReservedIPAddressBody) UnmarshalBinary(b []byte) error {
	var res UseNextReservedIPAddressBody
	if err := swag.ReadJSON(b, &res); err != nil {
		return err
	}
	*o = res
	return nil
}

/*UseNextReservedIPAddressOKBody use next reserved IP address o k body
swagger:model UseNextReservedIPAddressOKBody
*/
type UseNextReservedIPAddressOKBody struct {

	// result
	Result string `json:"result,omitempty"`
}

// Validate validates this use next reserved IP address o k body
func (o *UseNextReservedIPAddressOKBody) Validate(formats strfmt.Registry) error {
	return nil
}

// MarshalBinary interface implementation
func (o *UseNextReservedIPAddressOKBody) MarshalBinary() ([]byte, error) {
	if o == nil {
		return nil, nil
	}
	return swag.WriteJSON(o)
}

// UnmarshalBinary interface implementation
func (o *UseNextReservedIPAddressOKBody) UnmarshalBinary(b []byte) error {
	var res UseNextReservedIPAddressOKBody
	if err := swag.ReadJSON(b, &res); err != nil {
		return err
	}
	*o = res
	return nil
}
