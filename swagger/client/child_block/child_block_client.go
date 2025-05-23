// Code generated by go-swagger; DO NOT EDIT.

package child_block

// This file was generated by the swagger tool.
// Editing this file might prove futile when you re-run the swagger generate command

import (
	"github.com/go-openapi/runtime"

	strfmt "github.com/go-openapi/strfmt"
)

// New creates a new child block API client.
func New(transport runtime.ClientTransport, formats strfmt.Registry) *Client {
	return &Client{transport: transport, formats: formats}
}

/*
Client for child block API
*/
type Client struct {
	transport runtime.ClientTransport
	formats   strfmt.Registry
}

/*
EndExportChildBlock exports child blocks

The endExportChildBlock operation is an optional final step in the series of calls to export child blocks from IPControl. Invoking this operation enables IPControl to clean up session information.
*/
func (a *Client) EndExportChildBlock(params *EndExportChildBlockParams, authInfo runtime.ClientAuthInfoWriter) (*EndExportChildBlockOK, error) {
	// TODO: Validate the params before sending
	if params == nil {
		params = NewEndExportChildBlockParams()
	}

	result, err := a.transport.Submit(&runtime.ClientOperation{
		ID:                 "endExportChildBlock",
		Method:             "POST",
		PathPattern:        "/Exports/endExportChildBlock",
		ProducesMediaTypes: []string{"application/json"},
		ConsumesMediaTypes: []string{"application/json"},
		Schemes:            []string{"http", "https"},
		Params:             params,
		Reader:             &EndExportChildBlockReader{formats: a.formats},
		AuthInfo:           authInfo,
		Context:            params.Context,
		Client:             params.HTTPClient,
	})
	if err != nil {
		return nil, err
	}
	return result.(*EndExportChildBlockOK), nil

}

/*
ExportChildBlock exports child blocks

The exportChildBlock operation enables you to export child blocks from IPControl. Before invoking the exportChildBlock operation, you must use initExportChildBlock to initialize the API. The response returned from the init operation becomes the input to this operation.
*/
func (a *Client) ExportChildBlock(params *ExportChildBlockParams, authInfo runtime.ClientAuthInfoWriter) (*ExportChildBlockOK, error) {
	// TODO: Validate the params before sending
	if params == nil {
		params = NewExportChildBlockParams()
	}

	result, err := a.transport.Submit(&runtime.ClientOperation{
		ID:                 "exportChildBlock",
		Method:             "POST",
		PathPattern:        "/Exports/exportChildBlock",
		ProducesMediaTypes: []string{"application/json"},
		ConsumesMediaTypes: []string{"application/json"},
		Schemes:            []string{"http", "https"},
		Params:             params,
		Reader:             &ExportChildBlockReader{formats: a.formats},
		AuthInfo:           authInfo,
		Context:            params.Context,
		Client:             params.HTTPClient,
	})
	if err != nil {
		return nil, err
	}
	return result.(*ExportChildBlockOK), nil

}

/*
ImportChildBlock imports a child block

The ImportChildBlock API enables you to import child blocks into IPControl. This operation is used to define sub-allocations of address space, taken from parent address space. This space is allocated from the parent, and then marked with the status that is specified in the request. The name of the block allocated is returned to the client application in the response. This API can also be used to attach existing blocks to another container by specifying an existing blockAddr. To modify a child block, use the modifyBlock operation.
*/
func (a *Client) ImportChildBlock(params *ImportChildBlockParams, authInfo runtime.ClientAuthInfoWriter) (*ImportChildBlockOK, error) {
	// TODO: Validate the params before sending
	if params == nil {
		params = NewImportChildBlockParams()
	}

	result, err := a.transport.Submit(&runtime.ClientOperation{
		ID:                 "importChildBlock",
		Method:             "POST",
		PathPattern:        "/Imports/importChildBlock",
		ProducesMediaTypes: []string{"application/json"},
		ConsumesMediaTypes: []string{"application/json"},
		Schemes:            []string{"http", "https"},
		Params:             params,
		Reader:             &ImportChildBlockReader{formats: a.formats},
		AuthInfo:           authInfo,
		Context:            params.Context,
		Client:             params.HTTPClient,
	})
	if err != nil {
		return nil, err
	}
	return result.(*ImportChildBlockOK), nil

}

/*
InitExportChildBlock exports child blocks

The initExportChildBlock operation enables you to export child blocks from IPControl. You can filter the list of blocks retrieved. In addition, the initExportChildBlock operation requires a boolean flag, includeFreeBlocks, that specifies if the free blocks maintained by IPControl should be included in the export. When pageSize and firstResultPos are specified, a list of structures is returned as described for the exportChildBlock operation. Otherwise, the returned structure can be passed on a subsequent exportChildBlock operation to retrieve results.
*/
func (a *Client) InitExportChildBlock(params *InitExportChildBlockParams, authInfo runtime.ClientAuthInfoWriter) (*InitExportChildBlockOK, error) {
	// TODO: Validate the params before sending
	if params == nil {
		params = NewInitExportChildBlockParams()
	}

	result, err := a.transport.Submit(&runtime.ClientOperation{
		ID:                 "initExportChildBlock",
		Method:             "POST",
		PathPattern:        "/Exports/initExportChildBlock",
		ProducesMediaTypes: []string{"application/json"},
		ConsumesMediaTypes: []string{"application/json"},
		Schemes:            []string{"http", "https"},
		Params:             params,
		Reader:             &InitExportChildBlockReader{formats: a.formats},
		AuthInfo:           authInfo,
		Context:            params.Context,
		Client:             params.HTTPClient,
	})
	if err != nil {
		return nil, err
	}
	return result.(*InitExportChildBlockOK), nil

}

/*
InitExportChildBlockUDFTags exports child block u d f tags

After the initExportChildBlock API is called to initialize the API, the web service client can, optionally, call the initExportChildBlockUDFTags API. This service is used by the ExportChildBlock CLI to create the header line used when exporting with the expanded format option. The result returned from the initExportChildBlockUDFTags service is an array of strings. These are the field names/tags of the user defined fields defined for the blocks that will be returned on subsequent calls to the exportChildBlock service.
*/
func (a *Client) InitExportChildBlockUDFTags(params *InitExportChildBlockUDFTagsParams, authInfo runtime.ClientAuthInfoWriter) (*InitExportChildBlockUDFTagsOK, error) {
	// TODO: Validate the params before sending
	if params == nil {
		params = NewInitExportChildBlockUDFTagsParams()
	}

	result, err := a.transport.Submit(&runtime.ClientOperation{
		ID:                 "initExportChildBlockUDFTags",
		Method:             "POST",
		PathPattern:        "/Exports/initExportChildBlockUDFTags",
		ProducesMediaTypes: []string{"application/json"},
		ConsumesMediaTypes: []string{"application/json"},
		Schemes:            []string{"http", "https"},
		Params:             params,
		Reader:             &InitExportChildBlockUDFTagsReader{formats: a.formats},
		AuthInfo:           authInfo,
		Context:            params.Context,
		Client:             params.HTTPClient,
	})
	if err != nil {
		return nil, err
	}
	return result.(*InitExportChildBlockUDFTagsOK), nil

}

// SetTransport changes the transport on the client
func (a *Client) SetTransport(transport runtime.ClientTransport) {
	a.transport = transport
}
