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

// GlobalNetElementSyncReader is a Reader for the GlobalNetElementSync structure.
type GlobalNetElementSyncReader struct {
	formats strfmt.Registry
}

// ReadResponse reads a server response into the received o.
func (o *GlobalNetElementSyncReader) ReadResponse(response runtime.ClientResponse, consumer runtime.Consumer) (interface{}, error) {
	switch response.Code() {

	case 200:
		result := NewGlobalNetElementSyncOK()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return result, nil

	default:
		result := NewGlobalNetElementSyncDefault(response.Code())
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		if response.Code()/100 == 2 {
			return result, nil
		}
		return nil, result
	}
}

// NewGlobalNetElementSyncOK creates a GlobalNetElementSyncOK with default headers values
func NewGlobalNetElementSyncOK() *GlobalNetElementSyncOK {
	return &GlobalNetElementSyncOK{}
}

/*GlobalNetElementSyncOK handles this case with default header values.

integer returned
*/
type GlobalNetElementSyncOK struct {
	Payload *GlobalNetElementSyncOKBody
}

func (o *GlobalNetElementSyncOK) Error() string {
	return fmt.Sprintf("[POST /TaskInvocation/globalNetElementSync][%d] globalNetElementSyncOK  %+v", 200, o.Payload)
}

func (o *GlobalNetElementSyncOK) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	o.Payload = new(GlobalNetElementSyncOKBody)

	// response payload
	if err := consumer.Consume(response.Body(), o.Payload); err != nil && err != io.EOF {
		return err
	}

	return nil
}

// NewGlobalNetElementSyncDefault creates a GlobalNetElementSyncDefault with default headers values
func NewGlobalNetElementSyncDefault(code int) *GlobalNetElementSyncDefault {
	return &GlobalNetElementSyncDefault{
		_statusCode: code,
	}
}

/*GlobalNetElementSyncDefault handles this case with default header values.

Error response
*/
type GlobalNetElementSyncDefault struct {
	_statusCode int

	Payload *models.Fault
}

// Code gets the status code for the global net element sync default response
func (o *GlobalNetElementSyncDefault) Code() int {
	return o._statusCode
}

func (o *GlobalNetElementSyncDefault) Error() string {
	return fmt.Sprintf("[POST /TaskInvocation/globalNetElementSync][%d] globalNetElementSync default  %+v", o._statusCode, o.Payload)
}

func (o *GlobalNetElementSyncDefault) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	o.Payload = new(models.Fault)

	// response payload
	if err := consumer.Consume(response.Body(), o.Payload); err != nil && err != io.EOF {
		return err
	}

	return nil
}

/*GlobalNetElementSyncBody global net element sync body
swagger:model GlobalNetElementSyncBody
*/
type GlobalNetElementSyncBody struct {

	// priority
	Priority bool `json:"priority,omitempty"`
}

// Validate validates this global net element sync body
func (o *GlobalNetElementSyncBody) Validate(formats strfmt.Registry) error {
	return nil
}

// MarshalBinary interface implementation
func (o *GlobalNetElementSyncBody) MarshalBinary() ([]byte, error) {
	if o == nil {
		return nil, nil
	}
	return swag.WriteJSON(o)
}

// UnmarshalBinary interface implementation
func (o *GlobalNetElementSyncBody) UnmarshalBinary(b []byte) error {
	var res GlobalNetElementSyncBody
	if err := swag.ReadJSON(b, &res); err != nil {
		return err
	}
	*o = res
	return nil
}

/*GlobalNetElementSyncOKBody global net element sync o k body
swagger:model GlobalNetElementSyncOKBody
*/
type GlobalNetElementSyncOKBody struct {

	// result
	Result int64 `json:"result,omitempty"`
}

// Validate validates this global net element sync o k body
func (o *GlobalNetElementSyncOKBody) Validate(formats strfmt.Registry) error {
	return nil
}

// MarshalBinary interface implementation
func (o *GlobalNetElementSyncOKBody) MarshalBinary() ([]byte, error) {
	if o == nil {
		return nil, nil
	}
	return swag.WriteJSON(o)
}

// UnmarshalBinary interface implementation
func (o *GlobalNetElementSyncOKBody) UnmarshalBinary(b []byte) error {
	var res GlobalNetElementSyncOKBody
	if err := swag.ReadJSON(b, &res); err != nil {
		return err
	}
	*o = res
	return nil
}
