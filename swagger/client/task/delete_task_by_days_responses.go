// Code generated by go-swagger; DO NOT EDIT.

package task

// This file was generated by the swagger tool.
// Editing this file might prove futile when you re-run the swagger generate command

import (
	"fmt"
	"io"

	"github.com/go-openapi/runtime"
	"github.com/go-openapi/swag"

	strfmt "github.com/go-openapi/strfmt"

	models "github.com/CMSgov/CMS-AWS-West-Network-Architecture/vpc-automation/swagger/models"
)

// DeleteTaskByDaysReader is a Reader for the DeleteTaskByDays structure.
type DeleteTaskByDaysReader struct {
	formats strfmt.Registry
}

// ReadResponse reads a server response into the received o.
func (o *DeleteTaskByDaysReader) ReadResponse(response runtime.ClientResponse, consumer runtime.Consumer) (interface{}, error) {
	switch response.Code() {

	case 200:
		result := NewDeleteTaskByDaysOK()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return result, nil

	default:
		result := NewDeleteTaskByDaysDefault(response.Code())
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		if response.Code()/100 == 2 {
			return result, nil
		}
		return nil, result
	}
}

// NewDeleteTaskByDaysOK creates a DeleteTaskByDaysOK with default headers values
func NewDeleteTaskByDaysOK() *DeleteTaskByDaysOK {
	return &DeleteTaskByDaysOK{}
}

/*DeleteTaskByDaysOK handles this case with default header values.

Deleted a Task
*/
type DeleteTaskByDaysOK struct {
	Payload *DeleteTaskByDaysOKBody
}

func (o *DeleteTaskByDaysOK) Error() string {
	return fmt.Sprintf("[DELETE /Deletes/deleteTaskByDays][%d] deleteTaskByDaysOK  %+v", 200, o.Payload)
}

func (o *DeleteTaskByDaysOK) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	o.Payload = new(DeleteTaskByDaysOKBody)

	// response payload
	if err := consumer.Consume(response.Body(), o.Payload); err != nil && err != io.EOF {
		return err
	}

	return nil
}

// NewDeleteTaskByDaysDefault creates a DeleteTaskByDaysDefault with default headers values
func NewDeleteTaskByDaysDefault(code int) *DeleteTaskByDaysDefault {
	return &DeleteTaskByDaysDefault{
		_statusCode: code,
	}
}

/*DeleteTaskByDaysDefault handles this case with default header values.

Error response
*/
type DeleteTaskByDaysDefault struct {
	_statusCode int

	Payload *models.Fault
}

// Code gets the status code for the delete task by days default response
func (o *DeleteTaskByDaysDefault) Code() int {
	return o._statusCode
}

func (o *DeleteTaskByDaysDefault) Error() string {
	return fmt.Sprintf("[DELETE /Deletes/deleteTaskByDays][%d] deleteTaskByDays default  %+v", o._statusCode, o.Payload)
}

func (o *DeleteTaskByDaysDefault) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	o.Payload = new(models.Fault)

	// response payload
	if err := consumer.Consume(response.Body(), o.Payload); err != nil && err != io.EOF {
		return err
	}

	return nil
}

/*DeleteTaskByDaysBody delete task by days body
swagger:model DeleteTaskByDaysBody
*/
type DeleteTaskByDaysBody struct {

	// days
	Days int64 `json:"days,omitempty"`
}

// Validate validates this delete task by days body
func (o *DeleteTaskByDaysBody) Validate(formats strfmt.Registry) error {
	return nil
}

// MarshalBinary interface implementation
func (o *DeleteTaskByDaysBody) MarshalBinary() ([]byte, error) {
	if o == nil {
		return nil, nil
	}
	return swag.WriteJSON(o)
}

// UnmarshalBinary interface implementation
func (o *DeleteTaskByDaysBody) UnmarshalBinary(b []byte) error {
	var res DeleteTaskByDaysBody
	if err := swag.ReadJSON(b, &res); err != nil {
		return err
	}
	*o = res
	return nil
}

/*DeleteTaskByDaysOKBody delete task by days o k body
swagger:model DeleteTaskByDaysOKBody
*/
type DeleteTaskByDaysOKBody struct {

	// result
	Result int64 `json:"result,omitempty"`
}

// Validate validates this delete task by days o k body
func (o *DeleteTaskByDaysOKBody) Validate(formats strfmt.Registry) error {
	return nil
}

// MarshalBinary interface implementation
func (o *DeleteTaskByDaysOKBody) MarshalBinary() ([]byte, error) {
	if o == nil {
		return nil, nil
	}
	return swag.WriteJSON(o)
}

// UnmarshalBinary interface implementation
func (o *DeleteTaskByDaysOKBody) UnmarshalBinary(b []byte) error {
	var res DeleteTaskByDaysOKBody
	if err := swag.ReadJSON(b, &res); err != nil {
		return err
	}
	*o = res
	return nil
}
