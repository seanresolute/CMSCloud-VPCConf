// Code generated by go-swagger; DO NOT EDIT.

package address_pool

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

// DeleteAddrPoolReader is a Reader for the DeleteAddrPool structure.
type DeleteAddrPoolReader struct {
	formats strfmt.Registry
}

// ReadResponse reads a server response into the received o.
func (o *DeleteAddrPoolReader) ReadResponse(response runtime.ClientResponse, consumer runtime.Consumer) (interface{}, error) {
	switch response.Code() {

	case 200:
		result := NewDeleteAddrPoolOK()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return result, nil

	default:
		result := NewDeleteAddrPoolDefault(response.Code())
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		if response.Code()/100 == 2 {
			return result, nil
		}
		return nil, result
	}
}

// NewDeleteAddrPoolOK creates a DeleteAddrPoolOK with default headers values
func NewDeleteAddrPoolOK() *DeleteAddrPoolOK {
	return &DeleteAddrPoolOK{}
}

/*DeleteAddrPoolOK handles this case with default header values.

Deleted a AddrPool
*/
type DeleteAddrPoolOK struct {
}

func (o *DeleteAddrPoolOK) Error() string {
	return fmt.Sprintf("[DELETE /Deletes/deleteAddrPool][%d] deleteAddrPoolOK ", 200)
}

func (o *DeleteAddrPoolOK) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	return nil
}

// NewDeleteAddrPoolDefault creates a DeleteAddrPoolDefault with default headers values
func NewDeleteAddrPoolDefault(code int) *DeleteAddrPoolDefault {
	return &DeleteAddrPoolDefault{
		_statusCode: code,
	}
}

/*DeleteAddrPoolDefault handles this case with default header values.

Error response
*/
type DeleteAddrPoolDefault struct {
	_statusCode int

	Payload *models.Fault
}

// Code gets the status code for the delete addr pool default response
func (o *DeleteAddrPoolDefault) Code() int {
	return o._statusCode
}

func (o *DeleteAddrPoolDefault) Error() string {
	return fmt.Sprintf("[DELETE /Deletes/deleteAddrPool][%d] deleteAddrPool default  %+v", o._statusCode, o.Payload)
}

func (o *DeleteAddrPoolDefault) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	o.Payload = new(models.Fault)

	// response payload
	if err := consumer.Consume(response.Body(), o.Payload); err != nil && err != io.EOF {
		return err
	}

	return nil
}

/*DeleteAddrPoolBody delete addr pool body
swagger:model DeleteAddrPoolBody
*/
type DeleteAddrPoolBody struct {

	// delete devices in addrpool
	DeleteDevicesInAddrpool bool `json:"deleteDevicesInAddrpool,omitempty"`

	// inp addr pool
	InpAddrPool *models.WSAddrpool `json:"inpAddrPool,omitempty"`
}

// Validate validates this delete addr pool body
func (o *DeleteAddrPoolBody) Validate(formats strfmt.Registry) error {
	var res []error

	if err := o.validateInpAddrPool(formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (o *DeleteAddrPoolBody) validateInpAddrPool(formats strfmt.Registry) error {

	if swag.IsZero(o.InpAddrPool) { // not required
		return nil
	}

	if o.InpAddrPool != nil {
		if err := o.InpAddrPool.Validate(formats); err != nil {
			if ve, ok := err.(*errors.Validation); ok {
				return ve.ValidateName("Delete Parameters" + "." + "inpAddrPool")
			}
			return err
		}
	}

	return nil
}

// MarshalBinary interface implementation
func (o *DeleteAddrPoolBody) MarshalBinary() ([]byte, error) {
	if o == nil {
		return nil, nil
	}
	return swag.WriteJSON(o)
}

// UnmarshalBinary interface implementation
func (o *DeleteAddrPoolBody) UnmarshalBinary(b []byte) error {
	var res DeleteAddrPoolBody
	if err := swag.ReadJSON(b, &res); err != nil {
		return err
	}
	*o = res
	return nil
}
