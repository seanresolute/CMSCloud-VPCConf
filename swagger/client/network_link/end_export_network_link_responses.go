// Code generated by go-swagger; DO NOT EDIT.

package network_link

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

// EndExportNetworkLinkReader is a Reader for the EndExportNetworkLink structure.
type EndExportNetworkLinkReader struct {
	formats strfmt.Registry
}

// ReadResponse reads a server response into the received o.
func (o *EndExportNetworkLinkReader) ReadResponse(response runtime.ClientResponse, consumer runtime.Consumer) (interface{}, error) {
	switch response.Code() {

	case 200:
		result := NewEndExportNetworkLinkOK()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return result, nil

	default:
		result := NewEndExportNetworkLinkDefault(response.Code())
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		if response.Code()/100 == 2 {
			return result, nil
		}
		return nil, result
	}
}

// NewEndExportNetworkLinkOK creates a EndExportNetworkLinkOK with default headers values
func NewEndExportNetworkLinkOK() *EndExportNetworkLinkOK {
	return &EndExportNetworkLinkOK{}
}

/*EndExportNetworkLinkOK handles this case with default header values.

Exports of NetworkLink completed
*/
type EndExportNetworkLinkOK struct {
}

func (o *EndExportNetworkLinkOK) Error() string {
	return fmt.Sprintf("[POST /Exports/endExportNetworkLink][%d] endExportNetworkLinkOK ", 200)
}

func (o *EndExportNetworkLinkOK) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	return nil
}

// NewEndExportNetworkLinkDefault creates a EndExportNetworkLinkDefault with default headers values
func NewEndExportNetworkLinkDefault(code int) *EndExportNetworkLinkDefault {
	return &EndExportNetworkLinkDefault{
		_statusCode: code,
	}
}

/*EndExportNetworkLinkDefault handles this case with default header values.

Error response
*/
type EndExportNetworkLinkDefault struct {
	_statusCode int

	Payload *models.Fault
}

// Code gets the status code for the end export network link default response
func (o *EndExportNetworkLinkDefault) Code() int {
	return o._statusCode
}

func (o *EndExportNetworkLinkDefault) Error() string {
	return fmt.Sprintf("[POST /Exports/endExportNetworkLink][%d] endExportNetworkLink default  %+v", o._statusCode, o.Payload)
}

func (o *EndExportNetworkLinkDefault) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	o.Payload = new(models.Fault)

	// response payload
	if err := consumer.Consume(response.Body(), o.Payload); err != nil && err != io.EOF {
		return err
	}

	return nil
}

/*EndExportNetworkLinkBody end export network link body
swagger:model EndExportNetworkLinkBody
*/
type EndExportNetworkLinkBody struct {

	// context
	Context *models.WSContext `json:"context,omitempty"`
}

// Validate validates this end export network link body
func (o *EndExportNetworkLinkBody) Validate(formats strfmt.Registry) error {
	var res []error

	if err := o.validateContext(formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (o *EndExportNetworkLinkBody) validateContext(formats strfmt.Registry) error {

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
func (o *EndExportNetworkLinkBody) MarshalBinary() ([]byte, error) {
	if o == nil {
		return nil, nil
	}
	return swag.WriteJSON(o)
}

// UnmarshalBinary interface implementation
func (o *EndExportNetworkLinkBody) UnmarshalBinary(b []byte) error {
	var res EndExportNetworkLinkBody
	if err := swag.ReadJSON(b, &res); err != nil {
		return err
	}
	*o = res
	return nil
}
