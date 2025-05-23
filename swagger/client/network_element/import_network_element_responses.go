// Code generated by go-swagger; DO NOT EDIT.

package network_element

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

// ImportNetworkElementReader is a Reader for the ImportNetworkElement structure.
type ImportNetworkElementReader struct {
	formats strfmt.Registry
}

// ReadResponse reads a server response into the received o.
func (o *ImportNetworkElementReader) ReadResponse(response runtime.ClientResponse, consumer runtime.Consumer) (interface{}, error) {
	switch response.Code() {

	case 200:
		result := NewImportNetworkElementOK()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return result, nil

	default:
		result := NewImportNetworkElementDefault(response.Code())
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		if response.Code()/100 == 2 {
			return result, nil
		}
		return nil, result
	}
}

// NewImportNetworkElementOK creates a ImportNetworkElementOK with default headers values
func NewImportNetworkElementOK() *ImportNetworkElementOK {
	return &ImportNetworkElementOK{}
}

/*ImportNetworkElementOK handles this case with default header values.

Imports of NetworkElement completed
*/
type ImportNetworkElementOK struct {
}

func (o *ImportNetworkElementOK) Error() string {
	return fmt.Sprintf("[POST /Imports/importNetworkElement][%d] importNetworkElementOK ", 200)
}

func (o *ImportNetworkElementOK) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	return nil
}

// NewImportNetworkElementDefault creates a ImportNetworkElementDefault with default headers values
func NewImportNetworkElementDefault(code int) *ImportNetworkElementDefault {
	return &ImportNetworkElementDefault{
		_statusCode: code,
	}
}

/*ImportNetworkElementDefault handles this case with default header values.

Error response
*/
type ImportNetworkElementDefault struct {
	_statusCode int

	Payload *models.Fault
}

// Code gets the status code for the import network element default response
func (o *ImportNetworkElementDefault) Code() int {
	return o._statusCode
}

func (o *ImportNetworkElementDefault) Error() string {
	return fmt.Sprintf("[POST /Imports/importNetworkElement][%d] importNetworkElement default  %+v", o._statusCode, o.Payload)
}

func (o *ImportNetworkElementDefault) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	o.Payload = new(models.Fault)

	// response payload
	if err := consumer.Consume(response.Body(), o.Payload); err != nil && err != io.EOF {
		return err
	}

	return nil
}

/*ImportNetworkElementBody import network element body
swagger:model ImportNetworkElementBody
*/
type ImportNetworkElementBody struct {

	// inp network element
	InpNetworkElement *models.WSNetworkElement `json:"inpNetworkElement,omitempty"`
}

// Validate validates this import network element body
func (o *ImportNetworkElementBody) Validate(formats strfmt.Registry) error {
	var res []error

	if err := o.validateInpNetworkElement(formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (o *ImportNetworkElementBody) validateInpNetworkElement(formats strfmt.Registry) error {

	if swag.IsZero(o.InpNetworkElement) { // not required
		return nil
	}

	if o.InpNetworkElement != nil {
		if err := o.InpNetworkElement.Validate(formats); err != nil {
			if ve, ok := err.(*errors.Validation); ok {
				return ve.ValidateName("Import Parameters" + "." + "inpNetworkElement")
			}
			return err
		}
	}

	return nil
}

// MarshalBinary interface implementation
func (o *ImportNetworkElementBody) MarshalBinary() ([]byte, error) {
	if o == nil {
		return nil, nil
	}
	return swag.WriteJSON(o)
}

// UnmarshalBinary interface implementation
func (o *ImportNetworkElementBody) UnmarshalBinary(b []byte) error {
	var res ImportNetworkElementBody
	if err := swag.ReadJSON(b, &res); err != nil {
		return err
	}
	*o = res
	return nil
}
