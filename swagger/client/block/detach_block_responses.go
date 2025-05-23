// Code generated by go-swagger; DO NOT EDIT.

package block

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

// DetachBlockReader is a Reader for the DetachBlock structure.
type DetachBlockReader struct {
	formats strfmt.Registry
}

// ReadResponse reads a server response into the received o.
func (o *DetachBlockReader) ReadResponse(response runtime.ClientResponse, consumer runtime.Consumer) (interface{}, error) {
	switch response.Code() {

	case 200:
		result := NewDetachBlockOK()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return result, nil

	default:
		result := NewDetachBlockDefault(response.Code())
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		if response.Code()/100 == 2 {
			return result, nil
		}
		return nil, result
	}
}

// NewDetachBlockOK creates a DetachBlockOK with default headers values
func NewDetachBlockOK() *DetachBlockOK {
	return &DetachBlockOK{}
}

/*DetachBlockOK handles this case with default header values.

Block returned
*/
type DetachBlockOK struct {
	Payload *DetachBlockOKBody
}

func (o *DetachBlockOK) Error() string {
	return fmt.Sprintf("[POST /Imports/detachBlock][%d] detachBlockOK  %+v", 200, o.Payload)
}

func (o *DetachBlockOK) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	o.Payload = new(DetachBlockOKBody)

	// response payload
	if err := consumer.Consume(response.Body(), o.Payload); err != nil && err != io.EOF {
		return err
	}

	return nil
}

// NewDetachBlockDefault creates a DetachBlockDefault with default headers values
func NewDetachBlockDefault(code int) *DetachBlockDefault {
	return &DetachBlockDefault{
		_statusCode: code,
	}
}

/*DetachBlockDefault handles this case with default header values.

Error response
*/
type DetachBlockDefault struct {
	_statusCode int

	Payload *models.Fault
}

// Code gets the status code for the detach block default response
func (o *DetachBlockDefault) Code() int {
	return o._statusCode
}

func (o *DetachBlockDefault) Error() string {
	return fmt.Sprintf("[POST /Imports/detachBlock][%d] detachBlock default  %+v", o._statusCode, o.Payload)
}

func (o *DetachBlockDefault) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	o.Payload = new(models.Fault)

	// response payload
	if err := consumer.Consume(response.Body(), o.Payload); err != nil && err != io.EOF {
		return err
	}

	return nil
}

/*DetachBlockBody detach block body
swagger:model DetachBlockBody
*/
type DetachBlockBody struct {

	// child block
	ChildBlock *models.WSChildBlock `json:"childBlock,omitempty"`
}

// Validate validates this detach block body
func (o *DetachBlockBody) Validate(formats strfmt.Registry) error {
	var res []error

	if err := o.validateChildBlock(formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (o *DetachBlockBody) validateChildBlock(formats strfmt.Registry) error {

	if swag.IsZero(o.ChildBlock) { // not required
		return nil
	}

	if o.ChildBlock != nil {
		if err := o.ChildBlock.Validate(formats); err != nil {
			if ve, ok := err.(*errors.Validation); ok {
				return ve.ValidateName("Modify Parameters" + "." + "childBlock")
			}
			return err
		}
	}

	return nil
}

// MarshalBinary interface implementation
func (o *DetachBlockBody) MarshalBinary() ([]byte, error) {
	if o == nil {
		return nil, nil
	}
	return swag.WriteJSON(o)
}

// UnmarshalBinary interface implementation
func (o *DetachBlockBody) UnmarshalBinary(b []byte) error {
	var res DetachBlockBody
	if err := swag.ReadJSON(b, &res); err != nil {
		return err
	}
	*o = res
	return nil
}

/*DetachBlockOKBody detach block o k body
swagger:model DetachBlockOKBody
*/
type DetachBlockOKBody struct {

	// result
	Result string `json:"result,omitempty"`
}

// Validate validates this detach block o k body
func (o *DetachBlockOKBody) Validate(formats strfmt.Registry) error {
	return nil
}

// MarshalBinary interface implementation
func (o *DetachBlockOKBody) MarshalBinary() ([]byte, error) {
	if o == nil {
		return nil, nil
	}
	return swag.WriteJSON(o)
}

// UnmarshalBinary interface implementation
func (o *DetachBlockOKBody) UnmarshalBinary(b []byte) error {
	var res DetachBlockOKBody
	if err := swag.ReadJSON(b, &res); err != nil {
		return err
	}
	*o = res
	return nil
}
