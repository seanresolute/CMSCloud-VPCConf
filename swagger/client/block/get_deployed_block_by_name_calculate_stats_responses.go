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

// GetDeployedBlockByNameCalculateStatsReader is a Reader for the GetDeployedBlockByNameCalculateStats structure.
type GetDeployedBlockByNameCalculateStatsReader struct {
	formats strfmt.Registry
}

// ReadResponse reads a server response into the received o.
func (o *GetDeployedBlockByNameCalculateStatsReader) ReadResponse(response runtime.ClientResponse, consumer runtime.Consumer) (interface{}, error) {
	switch response.Code() {

	case 200:
		result := NewGetDeployedBlockByNameCalculateStatsOK()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return result, nil

	default:
		result := NewGetDeployedBlockByNameCalculateStatsDefault(response.Code())
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		if response.Code()/100 == 2 {
			return result, nil
		}
		return nil, result
	}
}

// NewGetDeployedBlockByNameCalculateStatsOK creates a GetDeployedBlockByNameCalculateStatsOK with default headers values
func NewGetDeployedBlockByNameCalculateStatsOK() *GetDeployedBlockByNameCalculateStatsOK {
	return &GetDeployedBlockByNameCalculateStatsOK{}
}

/*GetDeployedBlockByNameCalculateStatsOK handles this case with default header values.

DeployedBlock
*/
type GetDeployedBlockByNameCalculateStatsOK struct {
	Payload *models.WSGenericBlock
}

func (o *GetDeployedBlockByNameCalculateStatsOK) Error() string {
	return fmt.Sprintf("[GET /Gets/getDeployedBlockByNameCalculateStats][%d] getDeployedBlockByNameCalculateStatsOK  %+v", 200, o.Payload)
}

func (o *GetDeployedBlockByNameCalculateStatsOK) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	o.Payload = new(models.WSGenericBlock)

	// response payload
	if err := consumer.Consume(response.Body(), o.Payload); err != nil && err != io.EOF {
		return err
	}

	return nil
}

// NewGetDeployedBlockByNameCalculateStatsDefault creates a GetDeployedBlockByNameCalculateStatsDefault with default headers values
func NewGetDeployedBlockByNameCalculateStatsDefault(code int) *GetDeployedBlockByNameCalculateStatsDefault {
	return &GetDeployedBlockByNameCalculateStatsDefault{
		_statusCode: code,
	}
}

/*GetDeployedBlockByNameCalculateStatsDefault handles this case with default header values.

Error response
*/
type GetDeployedBlockByNameCalculateStatsDefault struct {
	_statusCode int

	Payload *models.Fault
}

// Code gets the status code for the get deployed block by name calculate stats default response
func (o *GetDeployedBlockByNameCalculateStatsDefault) Code() int {
	return o._statusCode
}

func (o *GetDeployedBlockByNameCalculateStatsDefault) Error() string {
	return fmt.Sprintf("[GET /Gets/getDeployedBlockByNameCalculateStats][%d] getDeployedBlockByNameCalculateStats default  %+v", o._statusCode, o.Payload)
}

func (o *GetDeployedBlockByNameCalculateStatsDefault) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	o.Payload = new(models.Fault)

	// response payload
	if err := consumer.Consume(response.Body(), o.Payload); err != nil && err != io.EOF {
		return err
	}

	return nil
}
