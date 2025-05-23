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

// NewGetTaskParams creates a new GetTaskParams object
// with the default values initialized.
func NewGetTaskParams() *GetTaskParams {
	var ()
	return &GetTaskParams{

		timeout: cr.DefaultTimeout,
	}
}

// NewGetTaskParamsWithTimeout creates a new GetTaskParams object
// with the default values initialized, and the ability to set a timeout on a request
func NewGetTaskParamsWithTimeout(timeout time.Duration) *GetTaskParams {
	var ()
	return &GetTaskParams{

		timeout: timeout,
	}
}

// NewGetTaskParamsWithContext creates a new GetTaskParams object
// with the default values initialized, and the ability to set a context for a request
func NewGetTaskParamsWithContext(ctx context.Context) *GetTaskParams {
	var ()
	return &GetTaskParams{

		Context: ctx,
	}
}

// NewGetTaskParamsWithHTTPClient creates a new GetTaskParams object
// with the default values initialized, and the ability to set a custom HTTPClient for a request
func NewGetTaskParamsWithHTTPClient(client *http.Client) *GetTaskParams {
	var ()
	return &GetTaskParams{
		HTTPClient: client,
	}
}

/*GetTaskParams contains all the parameters to send to the API endpoint
for the get task operation typically these are written to a http.Request
*/
type GetTaskParams struct {

	/*TaskID
	  Use taskId to specify the task number to query.

	*/
	TaskID int64

	timeout    time.Duration
	Context    context.Context
	HTTPClient *http.Client
}

// WithTimeout adds the timeout to the get task params
func (o *GetTaskParams) WithTimeout(timeout time.Duration) *GetTaskParams {
	o.SetTimeout(timeout)
	return o
}

// SetTimeout adds the timeout to the get task params
func (o *GetTaskParams) SetTimeout(timeout time.Duration) {
	o.timeout = timeout
}

// WithContext adds the context to the get task params
func (o *GetTaskParams) WithContext(ctx context.Context) *GetTaskParams {
	o.SetContext(ctx)
	return o
}

// SetContext adds the context to the get task params
func (o *GetTaskParams) SetContext(ctx context.Context) {
	o.Context = ctx
}

// WithHTTPClient adds the HTTPClient to the get task params
func (o *GetTaskParams) WithHTTPClient(client *http.Client) *GetTaskParams {
	o.SetHTTPClient(client)
	return o
}

// SetHTTPClient adds the HTTPClient to the get task params
func (o *GetTaskParams) SetHTTPClient(client *http.Client) {
	o.HTTPClient = client
}

// WithTaskID adds the taskID to the get task params
func (o *GetTaskParams) WithTaskID(taskID int64) *GetTaskParams {
	o.SetTaskID(taskID)
	return o
}

// SetTaskID adds the taskId to the get task params
func (o *GetTaskParams) SetTaskID(taskID int64) {
	o.TaskID = taskID
}

// WriteToRequest writes these params to a swagger request
func (o *GetTaskParams) WriteToRequest(r runtime.ClientRequest, reg strfmt.Registry) error {

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
