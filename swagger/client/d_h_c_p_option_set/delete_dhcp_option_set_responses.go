// Code generated by go-swagger; DO NOT EDIT.

package d_h_c_p_option_set

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

// DeleteDhcpOptionSetReader is a Reader for the DeleteDhcpOptionSet structure.
type DeleteDhcpOptionSetReader struct {
	formats strfmt.Registry
}

// ReadResponse reads a server response into the received o.
func (o *DeleteDhcpOptionSetReader) ReadResponse(response runtime.ClientResponse, consumer runtime.Consumer) (interface{}, error) {
	switch response.Code() {

	case 200:
		result := NewDeleteDhcpOptionSetOK()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return result, nil

	default:
		result := NewDeleteDhcpOptionSetDefault(response.Code())
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		if response.Code()/100 == 2 {
			return result, nil
		}
		return nil, result
	}
}

// NewDeleteDhcpOptionSetOK creates a DeleteDhcpOptionSetOK with default headers values
func NewDeleteDhcpOptionSetOK() *DeleteDhcpOptionSetOK {
	return &DeleteDhcpOptionSetOK{}
}

/*DeleteDhcpOptionSetOK handles this case with default header values.

Deleted a DhcpOptionSet
*/
type DeleteDhcpOptionSetOK struct {
}

func (o *DeleteDhcpOptionSetOK) Error() string {
	return fmt.Sprintf("[DELETE /Deletes/deleteDhcpOptionSet][%d] deleteDhcpOptionSetOK ", 200)
}

func (o *DeleteDhcpOptionSetOK) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	return nil
}

// NewDeleteDhcpOptionSetDefault creates a DeleteDhcpOptionSetDefault with default headers values
func NewDeleteDhcpOptionSetDefault(code int) *DeleteDhcpOptionSetDefault {
	return &DeleteDhcpOptionSetDefault{
		_statusCode: code,
	}
}

/*DeleteDhcpOptionSetDefault handles this case with default header values.

Error response
*/
type DeleteDhcpOptionSetDefault struct {
	_statusCode int

	Payload *models.Fault
}

// Code gets the status code for the delete dhcp option set default response
func (o *DeleteDhcpOptionSetDefault) Code() int {
	return o._statusCode
}

func (o *DeleteDhcpOptionSetDefault) Error() string {
	return fmt.Sprintf("[DELETE /Deletes/deleteDhcpOptionSet][%d] deleteDhcpOptionSet default  %+v", o._statusCode, o.Payload)
}

func (o *DeleteDhcpOptionSetDefault) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	o.Payload = new(models.Fault)

	// response payload
	if err := consumer.Consume(response.Body(), o.Payload); err != nil && err != io.EOF {
		return err
	}

	return nil
}

/*DeleteDhcpOptionSetBody delete dhcp option set body
swagger:model DeleteDhcpOptionSetBody
*/
type DeleteDhcpOptionSetBody struct {

	// inp option set
	InpOptionSet *models.WSDhcpOptionSet `json:"inpOptionSet,omitempty"`
}

// Validate validates this delete dhcp option set body
func (o *DeleteDhcpOptionSetBody) Validate(formats strfmt.Registry) error {
	var res []error

	if err := o.validateInpOptionSet(formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (o *DeleteDhcpOptionSetBody) validateInpOptionSet(formats strfmt.Registry) error {

	if swag.IsZero(o.InpOptionSet) { // not required
		return nil
	}

	if o.InpOptionSet != nil {
		if err := o.InpOptionSet.Validate(formats); err != nil {
			if ve, ok := err.(*errors.Validation); ok {
				return ve.ValidateName("Delete Parameters" + "." + "inpOptionSet")
			}
			return err
		}
	}

	return nil
}

// MarshalBinary interface implementation
func (o *DeleteDhcpOptionSetBody) MarshalBinary() ([]byte, error) {
	if o == nil {
		return nil, nil
	}
	return swag.WriteJSON(o)
}

// UnmarshalBinary interface implementation
func (o *DeleteDhcpOptionSetBody) UnmarshalBinary(b []byte) error {
	var res DeleteDhcpOptionSetBody
	if err := swag.ReadJSON(b, &res); err != nil {
		return err
	}
	*o = res
	return nil
}
