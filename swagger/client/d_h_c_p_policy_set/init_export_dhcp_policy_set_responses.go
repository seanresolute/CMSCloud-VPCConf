// Code generated by go-swagger; DO NOT EDIT.

package d_h_c_p_policy_set

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

// InitExportDhcpPolicySetReader is a Reader for the InitExportDhcpPolicySet structure.
type InitExportDhcpPolicySetReader struct {
	formats strfmt.Registry
}

// ReadResponse reads a server response into the received o.
func (o *InitExportDhcpPolicySetReader) ReadResponse(response runtime.ClientResponse, consumer runtime.Consumer) (interface{}, error) {
	switch response.Code() {

	case 200:
		result := NewInitExportDhcpPolicySetOK()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return result, nil

	default:
		result := NewInitExportDhcpPolicySetDefault(response.Code())
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		if response.Code()/100 == 2 {
			return result, nil
		}
		return nil, result
	}
}

// NewInitExportDhcpPolicySetOK creates a InitExportDhcpPolicySetOK with default headers values
func NewInitExportDhcpPolicySetOK() *InitExportDhcpPolicySetOK {
	return &InitExportDhcpPolicySetOK{}
}

/*InitExportDhcpPolicySetOK handles this case with default header values.

Exports of DhcpPolicySet initialized
*/
type InitExportDhcpPolicySetOK struct {
	Payload *models.WSContext
}

func (o *InitExportDhcpPolicySetOK) Error() string {
	return fmt.Sprintf("[POST /Exports/initExportDhcpPolicySet][%d] initExportDhcpPolicySetOK  %+v", 200, o.Payload)
}

func (o *InitExportDhcpPolicySetOK) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	o.Payload = new(models.WSContext)

	// response payload
	if err := consumer.Consume(response.Body(), o.Payload); err != nil && err != io.EOF {
		return err
	}

	return nil
}

// NewInitExportDhcpPolicySetDefault creates a InitExportDhcpPolicySetDefault with default headers values
func NewInitExportDhcpPolicySetDefault(code int) *InitExportDhcpPolicySetDefault {
	return &InitExportDhcpPolicySetDefault{
		_statusCode: code,
	}
}

/*InitExportDhcpPolicySetDefault handles this case with default header values.

Error response
*/
type InitExportDhcpPolicySetDefault struct {
	_statusCode int

	Payload *models.Fault
}

// Code gets the status code for the init export dhcp policy set default response
func (o *InitExportDhcpPolicySetDefault) Code() int {
	return o._statusCode
}

func (o *InitExportDhcpPolicySetDefault) Error() string {
	return fmt.Sprintf("[POST /Exports/initExportDhcpPolicySet][%d] initExportDhcpPolicySet default  %+v", o._statusCode, o.Payload)
}

func (o *InitExportDhcpPolicySetDefault) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	o.Payload = new(models.Fault)

	// response payload
	if err := consumer.Consume(response.Body(), o.Payload); err != nil && err != io.EOF {
		return err
	}

	return nil
}

/*InitExportDhcpPolicySetBody init export dhcp policy set body
swagger:model InitExportDhcpPolicySetBody
*/
type InitExportDhcpPolicySetBody struct {

	// filter
	Filter string `json:"filter,omitempty"`

	// first result pos
	FirstResultPos int64 `json:"firstResultPos,omitempty"`

	// options
	Options []string `json:"options"`

	// page size
	PageSize int64 `json:"pageSize,omitempty"`
}

// Validate validates this init export dhcp policy set body
func (o *InitExportDhcpPolicySetBody) Validate(formats strfmt.Registry) error {
	return nil
}

// MarshalBinary interface implementation
func (o *InitExportDhcpPolicySetBody) MarshalBinary() ([]byte, error) {
	if o == nil {
		return nil, nil
	}
	return swag.WriteJSON(o)
}

// UnmarshalBinary interface implementation
func (o *InitExportDhcpPolicySetBody) UnmarshalBinary(b []byte) error {
	var res InitExportDhcpPolicySetBody
	if err := swag.ReadJSON(b, &res); err != nil {
		return err
	}
	*o = res
	return nil
}
