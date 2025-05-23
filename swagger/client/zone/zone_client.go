// Code generated by go-swagger; DO NOT EDIT.

package zone

// This file was generated by the swagger tool.
// Editing this file might prove futile when you re-run the swagger generate command

import (
	"github.com/go-openapi/runtime"

	strfmt "github.com/go-openapi/strfmt"
)

// New creates a new zone API client.
func New(transport runtime.ClientTransport, formats strfmt.Registry) *Client {
	return &Client{transport: transport, formats: formats}
}

/*
Client for zone API
*/
type Client struct {
	transport runtime.ClientTransport
	formats   strfmt.Registry
}

/*
DeleteZone deletes a zone

The deleteZone operation enables you to delete a zone from IPControl.
*/
func (a *Client) DeleteZone(params *DeleteZoneParams, authInfo runtime.ClientAuthInfoWriter) (*DeleteZoneOK, error) {
	// TODO: Validate the params before sending
	if params == nil {
		params = NewDeleteZoneParams()
	}

	result, err := a.transport.Submit(&runtime.ClientOperation{
		ID:                 "deleteZone",
		Method:             "DELETE",
		PathPattern:        "/Deletes/deleteZone",
		ProducesMediaTypes: []string{"application/json"},
		ConsumesMediaTypes: []string{"application/json"},
		Schemes:            []string{"http", "https"},
		Params:             params,
		Reader:             &DeleteZoneReader{formats: a.formats},
		AuthInfo:           authInfo,
		Context:            params.Context,
		Client:             params.HTTPClient,
	})
	if err != nil {
		return nil, err
	}
	return result.(*DeleteZoneOK), nil

}

/*
ImportDNSZone imports a DNS zone

The importDnsZone API enables you to import a DNS zone to IPControl. To modify an existing zone, set updateZone to true and specify all fields as you would for an import. Any field not specified on an update is cleared or set to its default value.
*/
func (a *Client) ImportDNSZone(params *ImportDNSZoneParams, authInfo runtime.ClientAuthInfoWriter) (*ImportDNSZoneOK, error) {
	// TODO: Validate the params before sending
	if params == nil {
		params = NewImportDNSZoneParams()
	}

	result, err := a.transport.Submit(&runtime.ClientOperation{
		ID:                 "importDnsZone",
		Method:             "POST",
		PathPattern:        "/Imports/importDnsZone",
		ProducesMediaTypes: []string{"application/json"},
		ConsumesMediaTypes: []string{"application/json"},
		Schemes:            []string{"http", "https"},
		Params:             params,
		Reader:             &ImportDNSZoneReader{formats: a.formats},
		AuthInfo:           authInfo,
		Context:            params.Context,
		Client:             params.HTTPClient,
	})
	if err != nil {
		return nil, err
	}
	return result.(*ImportDNSZoneOK), nil

}

// SetTransport changes the transport on the client
func (a *Client) SetTransport(transport runtime.ClientTransport) {
	a.transport = transport
}
