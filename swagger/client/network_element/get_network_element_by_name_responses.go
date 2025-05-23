// Code generated by go-swagger; DO NOT EDIT.

package network_element

// This file was generated by the swagger tool.
// Editing this file might prove futile when you re-run the swagger generate command

import (
	"fmt"
	"io"

	"github.com/go-openapi/runtime"

	strfmt "github.com/go-openapi/strfmt"

	models "github.com/CMSgov/CMS-AWS-West-Network-Architecture/vpc-automation/swagger/models"
)

// GetNetworkElementByNameReader is a Reader for the GetNetworkElementByName structure.
type GetNetworkElementByNameReader struct {
	formats strfmt.Registry
}

// ReadResponse reads a server response into the received o.
func (o *GetNetworkElementByNameReader) ReadResponse(response runtime.ClientResponse, consumer runtime.Consumer) (interface{}, error) {
	switch response.Code() {

	case 200:
		result := NewGetNetworkElementByNameOK()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return result, nil

	default:
		result := NewGetNetworkElementByNameDefault(response.Code())
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		if response.Code()/100 == 2 {
			return result, nil
		}
		return nil, result
	}
}

// NewGetNetworkElementByNameOK creates a GetNetworkElementByNameOK with default headers values
func NewGetNetworkElementByNameOK() *GetNetworkElementByNameOK {
	return &GetNetworkElementByNameOK{}
}

/*GetNetworkElementByNameOK handles this case with default header values.

NetworkElement
*/
type GetNetworkElementByNameOK struct {
	Payload *models.WSNetworkElement
}

func (o *GetNetworkElementByNameOK) Error() string {
	return fmt.Sprintf("[GET /Gets/getNetworkElementByName][%d] getNetworkElementByNameOK  %+v", 200, o.Payload)
}

func (o *GetNetworkElementByNameOK) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	o.Payload = new(models.WSNetworkElement)

	// response payload
	if err := consumer.Consume(response.Body(), o.Payload); err != nil && err != io.EOF {
		return err
	}

	return nil
}

// NewGetNetworkElementByNameDefault creates a GetNetworkElementByNameDefault with default headers values
func NewGetNetworkElementByNameDefault(code int) *GetNetworkElementByNameDefault {
	return &GetNetworkElementByNameDefault{
		_statusCode: code,
	}
}

/*GetNetworkElementByNameDefault handles this case with default header values.

Error response
*/
type GetNetworkElementByNameDefault struct {
	_statusCode int

	Payload *models.Fault
}

// Code gets the status code for the get network element by name default response
func (o *GetNetworkElementByNameDefault) Code() int {
	return o._statusCode
}

func (o *GetNetworkElementByNameDefault) Error() string {
	return fmt.Sprintf("[GET /Gets/getNetworkElementByName][%d] getNetworkElementByName default  %+v", o._statusCode, o.Payload)
}

func (o *GetNetworkElementByNameDefault) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	o.Payload = new(models.Fault)

	// response payload
	if err := consumer.Consume(response.Body(), o.Payload); err != nil && err != io.EOF {
		return err
	}

	return nil
}
