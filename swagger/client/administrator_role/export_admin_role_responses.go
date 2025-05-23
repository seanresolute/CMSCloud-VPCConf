// Code generated by go-swagger; DO NOT EDIT.

package administrator_role

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

// ExportAdminRoleReader is a Reader for the ExportAdminRole structure.
type ExportAdminRoleReader struct {
	formats strfmt.Registry
}

// ReadResponse reads a server response into the received o.
func (o *ExportAdminRoleReader) ReadResponse(response runtime.ClientResponse, consumer runtime.Consumer) (interface{}, error) {
	switch response.Code() {

	case 200:
		result := NewExportAdminRoleOK()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return result, nil

	default:
		result := NewExportAdminRoleDefault(response.Code())
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		if response.Code()/100 == 2 {
			return result, nil
		}
		return nil, result
	}
}

// NewExportAdminRoleOK creates a ExportAdminRoleOK with default headers values
func NewExportAdminRoleOK() *ExportAdminRoleOK {
	return &ExportAdminRoleOK{}
}

/*ExportAdminRoleOK handles this case with default header values.

AdminRole returned
*/
type ExportAdminRoleOK struct {
	Payload []*models.WSAdminRole
}

func (o *ExportAdminRoleOK) Error() string {
	return fmt.Sprintf("[POST /Exports/exportAdminRole][%d] exportAdminRoleOK  %+v", 200, o.Payload)
}

func (o *ExportAdminRoleOK) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	// response payload
	if err := consumer.Consume(response.Body(), &o.Payload); err != nil && err != io.EOF {
		return err
	}

	return nil
}

// NewExportAdminRoleDefault creates a ExportAdminRoleDefault with default headers values
func NewExportAdminRoleDefault(code int) *ExportAdminRoleDefault {
	return &ExportAdminRoleDefault{
		_statusCode: code,
	}
}

/*ExportAdminRoleDefault handles this case with default header values.

Error response
*/
type ExportAdminRoleDefault struct {
	_statusCode int

	Payload *models.Fault
}

// Code gets the status code for the export admin role default response
func (o *ExportAdminRoleDefault) Code() int {
	return o._statusCode
}

func (o *ExportAdminRoleDefault) Error() string {
	return fmt.Sprintf("[POST /Exports/exportAdminRole][%d] exportAdminRole default  %+v", o._statusCode, o.Payload)
}

func (o *ExportAdminRoleDefault) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	o.Payload = new(models.Fault)

	// response payload
	if err := consumer.Consume(response.Body(), o.Payload); err != nil && err != io.EOF {
		return err
	}

	return nil
}

/*ExportAdminRoleBody export admin role body
swagger:model ExportAdminRoleBody
*/
type ExportAdminRoleBody struct {

	// context
	Context *models.WSContext `json:"context,omitempty"`
}

// Validate validates this export admin role body
func (o *ExportAdminRoleBody) Validate(formats strfmt.Registry) error {
	var res []error

	if err := o.validateContext(formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (o *ExportAdminRoleBody) validateContext(formats strfmt.Registry) error {

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
func (o *ExportAdminRoleBody) MarshalBinary() ([]byte, error) {
	if o == nil {
		return nil, nil
	}
	return swag.WriteJSON(o)
}

// UnmarshalBinary interface implementation
func (o *ExportAdminRoleBody) UnmarshalBinary(b []byte) error {
	var res ExportAdminRoleBody
	if err := swag.ReadJSON(b, &res); err != nil {
		return err
	}
	*o = res
	return nil
}
