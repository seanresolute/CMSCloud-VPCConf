// Code generated by go-swagger; DO NOT EDIT.

package resource_record

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

// ExportZoneResourceRecReader is a Reader for the ExportZoneResourceRec structure.
type ExportZoneResourceRecReader struct {
	formats strfmt.Registry
}

// ReadResponse reads a server response into the received o.
func (o *ExportZoneResourceRecReader) ReadResponse(response runtime.ClientResponse, consumer runtime.Consumer) (interface{}, error) {
	switch response.Code() {

	case 200:
		result := NewExportZoneResourceRecOK()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return result, nil

	default:
		result := NewExportZoneResourceRecDefault(response.Code())
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		if response.Code()/100 == 2 {
			return result, nil
		}
		return nil, result
	}
}

// NewExportZoneResourceRecOK creates a ExportZoneResourceRecOK with default headers values
func NewExportZoneResourceRecOK() *ExportZoneResourceRecOK {
	return &ExportZoneResourceRecOK{}
}

/*ExportZoneResourceRecOK handles this case with default header values.

ZoneResourceRecord returned
*/
type ExportZoneResourceRecOK struct {
	Payload []*models.WSZoneResourceRec
}

func (o *ExportZoneResourceRecOK) Error() string {
	return fmt.Sprintf("[POST /Exports/exportZoneResourceRecord][%d] exportZoneResourceRecOK  %+v", 200, o.Payload)
}

func (o *ExportZoneResourceRecOK) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	// response payload
	if err := consumer.Consume(response.Body(), &o.Payload); err != nil && err != io.EOF {
		return err
	}

	return nil
}

// NewExportZoneResourceRecDefault creates a ExportZoneResourceRecDefault with default headers values
func NewExportZoneResourceRecDefault(code int) *ExportZoneResourceRecDefault {
	return &ExportZoneResourceRecDefault{
		_statusCode: code,
	}
}

/*ExportZoneResourceRecDefault handles this case with default header values.

Error response
*/
type ExportZoneResourceRecDefault struct {
	_statusCode int

	Payload *models.Fault
}

// Code gets the status code for the export zone resource rec default response
func (o *ExportZoneResourceRecDefault) Code() int {
	return o._statusCode
}

func (o *ExportZoneResourceRecDefault) Error() string {
	return fmt.Sprintf("[POST /Exports/exportZoneResourceRecord][%d] exportZoneResourceRec default  %+v", o._statusCode, o.Payload)
}

func (o *ExportZoneResourceRecDefault) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	o.Payload = new(models.Fault)

	// response payload
	if err := consumer.Consume(response.Body(), o.Payload); err != nil && err != io.EOF {
		return err
	}

	return nil
}

/*ExportZoneResourceRecBody export zone resource rec body
swagger:model ExportZoneResourceRecBody
*/
type ExportZoneResourceRecBody struct {

	// context
	Context *models.WSContext `json:"context,omitempty"`
}

// Validate validates this export zone resource rec body
func (o *ExportZoneResourceRecBody) Validate(formats strfmt.Registry) error {
	var res []error

	if err := o.validateContext(formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (o *ExportZoneResourceRecBody) validateContext(formats strfmt.Registry) error {

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
func (o *ExportZoneResourceRecBody) MarshalBinary() ([]byte, error) {
	if o == nil {
		return nil, nil
	}
	return swag.WriteJSON(o)
}

// UnmarshalBinary interface implementation
func (o *ExportZoneResourceRecBody) UnmarshalBinary(b []byte) error {
	var res ExportZoneResourceRecBody
	if err := swag.ReadJSON(b, &res); err != nil {
		return err
	}
	*o = res
	return nil
}
