// Code generated by go-swagger; DO NOT EDIT.

package task

// This file was generated by the swagger tool.
// Editing this file might prove futile when you re-run the swagger generate command

import (
	"context"
	"net/http"
	"time"

	"github.com/go-openapi/errors"
	"github.com/go-openapi/runtime"
	cr "github.com/go-openapi/runtime/client"
	"github.com/go-openapi/swag"

	strfmt "github.com/go-openapi/strfmt"
)

// NewGetTaskStatusParams creates a new GetTaskStatusParams object
// with the default values initialized.
func NewGetTaskStatusParams() *GetTaskStatusParams {
	var ()
	return &GetTaskStatusParams{

		timeout: cr.DefaultTimeout,
	}
}

// NewGetTaskStatusParamsWithTimeout creates a new GetTaskStatusParams object
// with the default values initialized, and the ability to set a timeout on a request
func NewGetTaskStatusParamsWithTimeout(timeout time.Duration) *GetTaskStatusParams {
	var ()
	return &GetTaskStatusParams{

		timeout: timeout,
	}
}

// NewGetTaskStatusParamsWithContext creates a new GetTaskStatusParams object
// with the default values initialized, and the ability to set a context for a request
func NewGetTaskStatusParamsWithContext(ctx context.Context) *GetTaskStatusParams {
	var ()
	return &GetTaskStatusParams{

		Context: ctx,
	}
}

// NewGetTaskStatusParamsWithHTTPClient creates a new GetTaskStatusParams object
// with the default values initialized, and the ability to set a custom HTTPClient for a request
func NewGetTaskStatusParamsWithHTTPClient(client *http.Client) *GetTaskStatusParams {
	var ()
	return &GetTaskStatusParams{
		HTTPClient: client,
	}
}

/*GetTaskStatusParams contains all the parameters to send to the API endpoint
for the get task status operation typically these are written to a http.Request
*/
type GetTaskStatusParams struct {

	/*TaskID
	  Use taskId to specify the task number to query.

	*/
	TaskID int64

	timeout    time.Duration
	Context    context.Context
	HTTPClient *http.Client
}

// WithTimeout adds the timeout to the get task status params
func (o *GetTaskStatusParams) WithTimeout(timeout time.Duration) *GetTaskStatusParams {
	o.SetTimeout(timeout)
	return o
}

// SetTimeout adds the timeout to the get task status params
func (o *GetTaskStatusParams) SetTimeout(timeout time.Duration) {
	o.timeout = timeout
}

// WithContext adds the context to the get task status params
func (o *GetTaskStatusParams) WithContext(ctx context.Context) *GetTaskStatusParams {
	o.SetContext(ctx)
	return o
}

// SetContext adds the context to the get task status params
func (o *GetTaskStatusParams) SetContext(ctx context.Context) {
	o.Context = ctx
}

// WithHTTPClient adds the HTTPClient to the get task status params
func (o *GetTaskStatusParams) WithHTTPClient(client *http.Client) *GetTaskStatusParams {
	o.SetHTTPClient(client)
	return o
}

// SetHTTPClient adds the HTTPClient to the get task status params
func (o *GetTaskStatusParams) SetHTTPClient(client *http.Client) {
	o.HTTPClient = client
}

// WithTaskID adds the taskID to the get task status params
func (o *GetTaskStatusParams) WithTaskID(taskID int64) *GetTaskStatusParams {
	o.SetTaskID(taskID)
	return o
}

// SetTaskID adds the taskId to the get task status params
func (o *GetTaskStatusParams) SetTaskID(taskID int64) {
	o.TaskID = taskID
}

// WriteToRequest writes these params to a swagger request
func (o *GetTaskStatusParams) WriteToRequest(r runtime.ClientRequest, reg strfmt.Registry) error {

	if err := r.SetTimeout(o.timeout); err != nil {
		return err
	}
	var res []error

	// query param taskId
	qrTaskID := o.TaskID
	qTaskID := swag.FormatInt64(qrTaskID)
	if qTaskID != "" {
		if err := r.SetQueryParam("taskId", qTaskID); err != nil {
			return err
		}
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}
