// Code generated by go-swagger; DO NOT EDIT.

package block

// This file was generated by the swagger tool.
// Editing this file might prove futile when you re-run the swagger generate command

import (
	"fmt"
	"io"

	"github.com/go-openapi/runtime"

	strfmt "github.com/go-openapi/strfmt"

	models "github.com/CMSgov/CMS-AWS-West-Network-Architecture/vpc-automation/swagger/models"
)

// GetDeployedBlockByIPAddressCalculateStatsReader is a Reader for the GetDeployedBlockByIPAddressCalculateStats structure.
type GetDeployedBlockByIPAddressCalculateStatsReader struct {
	formats strfmt.Registry
}

// ReadResponse reads a server response into the received o.
func (o *GetDeployedBlockByIPAddressCalculateStatsReader) ReadResponse(response runtime.ClientResponse, consumer runtime.Consumer) (interface{}, error) {
	switch response.Code() {

	case 200:
		result := NewGetDeployedBlockByIPAddressCalculateStatsOK()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return result, nil

	default:
		result := NewGetDeployedBlockByIPAddressCalculateStatsDefault(response.Code())
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		if response.Code()/100 == 2 {
			return result, nil
		}
		return nil, result
	}
}

// NewGetDeployedBlockByIPAddressCalculateStatsOK creates a GetDeployedBlockByIPAddressCalculateStatsOK with default headers values
func NewGetDeployedBlockByIPAddressCalculateStatsOK() *GetDeployedBlockByIPAddressCalculateStatsOK {
	return &GetDeployedBlockByIPAddressCalculateStatsOK{}
}

/*GetDeployedBlockByIPAddressCalculateStatsOK handles this case with default header values.

DeployedBlock
*/
type GetDeployedBlockByIPAddressCalculateStatsOK struct {
	Payload *models.WSGenericBlock
}

func (o *GetDeployedBlockByIPAddressCalculateStatsOK) Error() string {
	return fmt.Sprintf("[GET /Gets/getDeployedBlockByIpAddressCalculateStats][%d] getDeployedBlockByIpAddressCalculateStatsOK  %+v", 200, o.Payload)
}

func (o *GetDeployedBlockByIPAddressCalculateStatsOK) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	o.Payload = new(models.WSGenericBlock)

	// response payload
	if err := consumer.Consume(response.Body(), o.Payload); err != nil && err != io.EOF {
		return err
	}

	return nil
}

// NewGetDeployedBlockByIPAddressCalculateStatsDefault creates a GetDeployedBlockByIPAddressCalculateStatsDefault with default headers values
func NewGetDeployedBlockByIPAddressCalculateStatsDefault(code int) *GetDeployedBlockByIPAddressCalculateStatsDefault {
	return &GetDeployedBlockByIPAddressCalculateStatsDefault{
		_statusCode: code,
	}
}

/*GetDeployedBlockByIPAddressCalculateStatsDefault handles this case with default header values.

Error response
*/
type GetDeployedBlockByIPAddressCalculateStatsDefault struct {
	_statusCode int

	Payload *models.Fault
}

// Code gets the status code for the get deployed block by Ip address calculate stats default response
func (o *GetDeployedBlockByIPAddressCalculateStatsDefault) Code() int {
	return o._statusCode
}

func (o *GetDeployedBlockByIPAddressCalculateStatsDefault) Error() string {
	return fmt.Sprintf("[GET /Gets/getDeployedBlockByIpAddressCalculateStats][%d] getDeployedBlockByIpAddressCalculateStats default  %+v", o._statusCode, o.Payload)
}

func (o *GetDeployedBlockByIPAddressCalculateStatsDefault) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	o.Payload = new(models.Fault)

	// response payload
	if err := consumer.Consume(response.Body(), o.Payload); err != nil && err != io.EOF {
		return err
	}

	return nil
}
