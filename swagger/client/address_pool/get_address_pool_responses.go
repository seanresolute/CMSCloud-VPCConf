// Code generated by go-swagger; DO NOT EDIT.

package address_pool

// This file was generated by the swagger tool.
// Editing this file might prove futile when you re-run the swagger generate command

import (
	"fmt"
	"io"

	"github.com/go-openapi/runtime"

	strfmt "github.com/go-openapi/strfmt"

	models "github.com/CMSgov/CMS-AWS-West-Network-Architecture/vpc-automation/swagger/models"
)

// GetAddressPoolReader is a Reader for the GetAddressPool structure.
type GetAddressPoolReader struct {
	formats strfmt.Registry
}

// ReadResponse reads a server response into the received o.
func (o *GetAddressPoolReader) ReadResponse(response runtime.ClientResponse, consumer runtime.Consumer) (interface{}, error) {
	switch response.Code() {

	case 200:
		result := NewGetAddressPoolOK()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return result, nil

	default:
		result := NewGetAddressPoolDefault(response.Code())
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		if response.Code()/100 == 2 {
			return result, nil
		}
		return nil, result
	}
}

// NewGetAddressPoolOK creates a GetAddressPoolOK with default headers values
func NewGetAddressPoolOK() *GetAddressPoolOK {
	return &GetAddressPoolOK{}
}

/*GetAddressPoolOK handles this case with default header values.

AddressPool
*/
type GetAddressPoolOK struct {
	Payload *models.WSAddrpool
}

func (o *GetAddressPoolOK) Error() string {
	return fmt.Sprintf("[GET /Gets/getAddressPool][%d] getAddressPoolOK  %+v", 200, o.Payload)
}

func (o *GetAddressPoolOK) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	o.Payload = new(models.WSAddrpool)

	// response payload
	if err := consumer.Consume(response.Body(), o.Payload); err != nil && err != io.EOF {
		return err
	}

	return nil
}

// NewGetAddressPoolDefault creates a GetAddressPoolDefault with default headers values
func NewGetAddressPoolDefault(code int) *GetAddressPoolDefault {
	return &GetAddressPoolDefault{
		_statusCode: code,
	}
}

/*GetAddressPoolDefault handles this case with default header values.

Error response
*/
type GetAddressPoolDefault struct {
	_statusCode int

	Payload *models.Fault
}

// Code gets the status code for the get address pool default response
func (o *GetAddressPoolDefault) Code() int {
	return o._statusCode
}

func (o *GetAddressPoolDefault) Error() string {
	return fmt.Sprintf("[GET /Gets/getAddressPool][%d] getAddressPool default  %+v", o._statusCode, o.Payload)
}

func (o *GetAddressPoolDefault) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	o.Payload = new(models.Fault)

	// response payload
	if err := consumer.Consume(response.Body(), o.Payload); err != nil && err != io.EOF {
		return err
	}

	return nil
}
