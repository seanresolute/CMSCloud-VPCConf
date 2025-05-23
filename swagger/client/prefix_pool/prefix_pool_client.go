// Code generated by go-swagger; DO NOT EDIT.

package prefix_pool

// This file was generated by the swagger tool.
// Editing this file might prove futile when you re-run the swagger generate command

import (
	"github.com/go-openapi/runtime"

	strfmt "github.com/go-openapi/strfmt"
)

// New creates a new prefix pool API client.
func New(transport runtime.ClientTransport, formats strfmt.Registry) *Client {
	return &Client{transport: transport, formats: formats}
}

/*
Client for prefix pool API
*/
type Client struct {
	transport runtime.ClientTransport
	formats   strfmt.Registry
}

/*
DeletePrefixPool deletes a prefix pool

The deletePrefixPool operation enables you to delete a prefix pool from IPControl. This will also delete the delegated prefixes inside the prefix pool.
*/
func (a *Client) DeletePrefixPool(params *DeletePrefixPoolParams, authInfo runtime.ClientAuthInfoWriter) (*DeletePrefixPoolOK, error) {
	// TODO: Validate the params before sending
	if params == nil {
		params = NewDeletePrefixPoolParams()
	}

	result, err := a.transport.Submit(&runtime.ClientOperation{
		ID:                 "deletePrefixPool",
		Method:             "DELETE",
		PathPattern:        "/Deletes/deletePrefixPool",
		ProducesMediaTypes: []string{"application/json"},
		ConsumesMediaTypes: []string{"application/json"},
		Schemes:            []string{"http", "https"},
		Params:             params,
		Reader:             &DeletePrefixPoolReader{formats: a.formats},
		AuthInfo:           authInfo,
		Context:            params.Context,
		Client:             params.HTTPClient,
	})
	if err != nil {
		return nil, err
	}
	return result.(*DeletePrefixPoolOK), nil

}

/*
EndExportPrefixPool exports prefix pools

The endExportPrefixPool operation is an optional final step in the series of calls to export prefix pools from IPControl. Invoking this operation enables IPControl to clean up session information.
*/
func (a *Client) EndExportPrefixPool(params *EndExportPrefixPoolParams, authInfo runtime.ClientAuthInfoWriter) (*EndExportPrefixPoolOK, error) {
	// TODO: Validate the params before sending
	if params == nil {
		params = NewEndExportPrefixPoolParams()
	}

	result, err := a.transport.Submit(&runtime.ClientOperation{
		ID:                 "endExportPrefixPool",
		Method:             "POST",
		PathPattern:        "/Exports/endExportPrefixPool",
		ProducesMediaTypes: []string{"application/json"},
		ConsumesMediaTypes: []string{"application/json"},
		Schemes:            []string{"http", "https"},
		Params:             params,
		Reader:             &EndExportPrefixPoolReader{formats: a.formats},
		AuthInfo:           authInfo,
		Context:            params.Context,
		Client:             params.HTTPClient,
	})
	if err != nil {
		return nil, err
	}
	return result.(*EndExportPrefixPoolOK), nil

}

/*
ExportDevice exports prefix pools

The exportPrefixPool operation enables you to export prefix pools from IPControl. Before invoking the exportPrefixPool operation, you must use initExportPrefixPool to initialize the API. The response returned from the init operation becomes the input to this operation.
*/
func (a *Client) ExportDevice(params *ExportDeviceParams, authInfo runtime.ClientAuthInfoWriter) (*ExportDeviceOK, error) {
	// TODO: Validate the params before sending
	if params == nil {
		params = NewExportDeviceParams()
	}

	result, err := a.transport.Submit(&runtime.ClientOperation{
		ID:                 "exportDevice",
		Method:             "POST",
		PathPattern:        "/Exports/exportPrefixPool",
		ProducesMediaTypes: []string{"application/json"},
		ConsumesMediaTypes: []string{"application/json"},
		Schemes:            []string{"http", "https"},
		Params:             params,
		Reader:             &ExportDeviceReader{formats: a.formats},
		AuthInfo:           authInfo,
		Context:            params.Context,
		Client:             params.HTTPClient,
	})
	if err != nil {
		return nil, err
	}
	return result.(*ExportDeviceOK), nil

}

/*
GetPrefixPoolByName gets a prefix pool by identifying its name

Retrieve information about a prefix pool by specifying its name.
*/
func (a *Client) GetPrefixPoolByName(params *GetPrefixPoolByNameParams, authInfo runtime.ClientAuthInfoWriter) (*GetPrefixPoolByNameOK, error) {
	// TODO: Validate the params before sending
	if params == nil {
		params = NewGetPrefixPoolByNameParams()
	}

	result, err := a.transport.Submit(&runtime.ClientOperation{
		ID:                 "getPrefixPoolByName",
		Method:             "GET",
		PathPattern:        "/Gets/getPrefixPoolByName",
		ProducesMediaTypes: []string{"application/json"},
		ConsumesMediaTypes: []string{"application/json"},
		Schemes:            []string{"http", "https"},
		Params:             params,
		Reader:             &GetPrefixPoolByNameReader{formats: a.formats},
		AuthInfo:           authInfo,
		Context:            params.Context,
		Client:             params.HTTPClient,
	})
	if err != nil {
		return nil, err
	}
	return result.(*GetPrefixPoolByNameOK), nil

}

/*
GetPrefixPoolByStartAddr gets a prefix pool by identifying its IP address

Retrieve information about a prefix pool by specifying the prefix pool starting IP Address.
*/
func (a *Client) GetPrefixPoolByStartAddr(params *GetPrefixPoolByStartAddrParams, authInfo runtime.ClientAuthInfoWriter) (*GetPrefixPoolByStartAddrOK, error) {
	// TODO: Validate the params before sending
	if params == nil {
		params = NewGetPrefixPoolByStartAddrParams()
	}

	result, err := a.transport.Submit(&runtime.ClientOperation{
		ID:                 "getPrefixPoolByStartAddr",
		Method:             "GET",
		PathPattern:        "/Gets/getPrefixPoolByStartAddr",
		ProducesMediaTypes: []string{"application/json"},
		ConsumesMediaTypes: []string{"application/json"},
		Schemes:            []string{"http", "https"},
		Params:             params,
		Reader:             &GetPrefixPoolByStartAddrReader{formats: a.formats},
		AuthInfo:           authInfo,
		Context:            params.Context,
		Client:             params.HTTPClient,
	})
	if err != nil {
		return nil, err
	}
	return result.(*GetPrefixPoolByStartAddrOK), nil

}

/*
ImportPrefixPool imports a prefix pool

The importPrefixPool operation enables you to import or modify prefix pools. It can also be used to modify an existing prefix pool by specifying its id.
*/
func (a *Client) ImportPrefixPool(params *ImportPrefixPoolParams, authInfo runtime.ClientAuthInfoWriter) (*ImportPrefixPoolOK, error) {
	// TODO: Validate the params before sending
	if params == nil {
		params = NewImportPrefixPoolParams()
	}

	result, err := a.transport.Submit(&runtime.ClientOperation{
		ID:                 "importPrefixPool",
		Method:             "POST",
		PathPattern:        "/Imports/importPrefixPool",
		ProducesMediaTypes: []string{"application/json"},
		ConsumesMediaTypes: []string{"application/json"},
		Schemes:            []string{"http", "https"},
		Params:             params,
		Reader:             &ImportPrefixPoolReader{formats: a.formats},
		AuthInfo:           authInfo,
		Context:            params.Context,
		Client:             params.HTTPClient,
	})
	if err != nil {
		return nil, err
	}
	return result.(*ImportPrefixPoolOK), nil

}

/*
InitExportPrefixPool exports prefix pools

The initExportPrefixPool operation enables you to issue a request to retrieve a list of prefix pools. There are no options defined for this operation. When pageSize and firstResultPos are specified, is a list of structures is returned as described for the exportPrefixPool operation. Otherwise, the returned structure can be passed on a subsequent exportPrefixPool operation to retrieve results.
*/
func (a *Client) InitExportPrefixPool(params *InitExportPrefixPoolParams, authInfo runtime.ClientAuthInfoWriter) (*InitExportPrefixPoolOK, error) {
	// TODO: Validate the params before sending
	if params == nil {
		params = NewInitExportPrefixPoolParams()
	}

	result, err := a.transport.Submit(&runtime.ClientOperation{
		ID:                 "initExportPrefixPool",
		Method:             "POST",
		PathPattern:        "/Exports/initExportPrefixPool",
		ProducesMediaTypes: []string{"application/json"},
		ConsumesMediaTypes: []string{"application/json"},
		Schemes:            []string{"http", "https"},
		Params:             params,
		Reader:             &InitExportPrefixPoolReader{formats: a.formats},
		AuthInfo:           authInfo,
		Context:            params.Context,
		Client:             params.HTTPClient,
	})
	if err != nil {
		return nil, err
	}
	return result.(*InitExportPrefixPoolOK), nil

}

// SetTransport changes the transport on the client
func (a *Client) SetTransport(transport runtime.ClientTransport) {
	a.transport = transport
}
