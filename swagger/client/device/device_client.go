// Code generated by go-swagger; DO NOT EDIT.

package device

// This file was generated by the swagger tool.
// Editing this file might prove futile when you re-run the swagger generate command

import (
	"github.com/go-openapi/runtime"

	strfmt "github.com/go-openapi/strfmt"
)

// New creates a new device API client.
func New(transport runtime.ClientTransport, formats strfmt.Registry) *Client {
	return &Client{transport: transport, formats: formats}
}

/*
Client for device API
*/
type Client struct {
	transport runtime.ClientTransport
	formats   strfmt.Registry
}

/*
DeleteDevice deletes a device

The deleteDevice operation enables you to delete a device from IPControl. Note that this is not used to delete devices of type 'Interface'. Use the ModifyBlock API to delete Interface-type devices.
*/
func (a *Client) DeleteDevice(params *DeleteDeviceParams, authInfo runtime.ClientAuthInfoWriter) (*DeleteDeviceOK, error) {
	// TODO: Validate the params before sending
	if params == nil {
		params = NewDeleteDeviceParams()
	}

	result, err := a.transport.Submit(&runtime.ClientOperation{
		ID:                 "deleteDevice",
		Method:             "DELETE",
		PathPattern:        "/Deletes/deleteDevice",
		ProducesMediaTypes: []string{"application/json"},
		ConsumesMediaTypes: []string{"application/json"},
		Schemes:            []string{"http", "https"},
		Params:             params,
		Reader:             &DeleteDeviceReader{formats: a.formats},
		AuthInfo:           authInfo,
		Context:            params.Context,
		Client:             params.HTTPClient,
	})
	if err != nil {
		return nil, err
	}
	return result.(*DeleteDeviceOK), nil

}

/*
DeleteDeviceInterface deletes a device interface

The deleteDeviceInterface operation enables you to delete a device interfaces from IPControl. Note that this is not used to delete devices of type 'Interface'. Use the ModifyBlock API to delete Interface-type devices.
*/
func (a *Client) DeleteDeviceInterface(params *DeleteDeviceInterfaceParams, authInfo runtime.ClientAuthInfoWriter) (*DeleteDeviceInterfaceOK, error) {
	// TODO: Validate the params before sending
	if params == nil {
		params = NewDeleteDeviceInterfaceParams()
	}

	result, err := a.transport.Submit(&runtime.ClientOperation{
		ID:                 "deleteDeviceInterface",
		Method:             "DELETE",
		PathPattern:        "/Deletes/deleteDeviceInterface",
		ProducesMediaTypes: []string{"application/json"},
		ConsumesMediaTypes: []string{"application/json"},
		Schemes:            []string{"http", "https"},
		Params:             params,
		Reader:             &DeleteDeviceInterfaceReader{formats: a.formats},
		AuthInfo:           authInfo,
		Context:            params.Context,
		Client:             params.HTTPClient,
	})
	if err != nil {
		return nil, err
	}
	return result.(*DeleteDeviceInterfaceOK), nil

}

/*
EndExportDevice exports devices

The endExportDevice operation is an optional final step in the series of calls to export devices from IPControl. Invoking this operation enables IPControl to clean up session information.
*/
func (a *Client) EndExportDevice(params *EndExportDeviceParams, authInfo runtime.ClientAuthInfoWriter) (*EndExportDeviceOK, error) {
	// TODO: Validate the params before sending
	if params == nil {
		params = NewEndExportDeviceParams()
	}

	result, err := a.transport.Submit(&runtime.ClientOperation{
		ID:                 "endExportDevice",
		Method:             "POST",
		PathPattern:        "/Exports/endExportDevice",
		ProducesMediaTypes: []string{"application/json"},
		ConsumesMediaTypes: []string{"application/json"},
		Schemes:            []string{"http", "https"},
		Params:             params,
		Reader:             &EndExportDeviceReader{formats: a.formats},
		AuthInfo:           authInfo,
		Context:            params.Context,
		Client:             params.HTTPClient,
	})
	if err != nil {
		return nil, err
	}
	return result.(*EndExportDeviceOK), nil

}

/*
EndExportDeviceRestoreList exports device restore list

The endExportDeviceRestoreList operation is an optional final step in the series of calls to export a list of devices that have been deleted and may be eligible for restoring. Invoking this operation enables IPControl to clean up session information.
*/
func (a *Client) EndExportDeviceRestoreList(params *EndExportDeviceRestoreListParams, authInfo runtime.ClientAuthInfoWriter) (*EndExportDeviceRestoreListOK, error) {
	// TODO: Validate the params before sending
	if params == nil {
		params = NewEndExportDeviceRestoreListParams()
	}

	result, err := a.transport.Submit(&runtime.ClientOperation{
		ID:                 "endExportDeviceRestoreList",
		Method:             "POST",
		PathPattern:        "/Exports/endExportDeviceRestoreList",
		ProducesMediaTypes: []string{"application/json"},
		ConsumesMediaTypes: []string{"application/json"},
		Schemes:            []string{"http", "https"},
		Params:             params,
		Reader:             &EndExportDeviceRestoreListReader{formats: a.formats},
		AuthInfo:           authInfo,
		Context:            params.Context,
		Client:             params.HTTPClient,
	})
	if err != nil {
		return nil, err
	}
	return result.(*EndExportDeviceRestoreListOK), nil

}

/*
ExportDeviceRestoreList exports device restore list

The exportDeviceRestoreList API enables you to issue a request to retrieve a list of devices that have been deleted and may be eligible for restoring. Before invoking the exportDeviceRestoreList operation, you must use initExportDeviceRestoreList to initialize the API. The response returned from the init operation becomes the input to this operation.
*/
func (a *Client) ExportDeviceRestoreList(params *ExportDeviceRestoreListParams, authInfo runtime.ClientAuthInfoWriter) (*ExportDeviceRestoreListOK, error) {
	// TODO: Validate the params before sending
	if params == nil {
		params = NewExportDeviceRestoreListParams()
	}

	result, err := a.transport.Submit(&runtime.ClientOperation{
		ID:                 "exportDeviceRestoreList",
		Method:             "POST",
		PathPattern:        "/Exports/exportDeviceRestoreList",
		ProducesMediaTypes: []string{"application/json"},
		ConsumesMediaTypes: []string{"application/json"},
		Schemes:            []string{"http", "https"},
		Params:             params,
		Reader:             &ExportDeviceRestoreListReader{formats: a.formats},
		AuthInfo:           authInfo,
		Context:            params.Context,
		Client:             params.HTTPClient,
	})
	if err != nil {
		return nil, err
	}
	return result.(*ExportDeviceRestoreListOK), nil

}

/*
GetDeviceByHostname gets a device by identifying its host name

Retrieve information about a device by specifying the host name.
*/
func (a *Client) GetDeviceByHostname(params *GetDeviceByHostnameParams, authInfo runtime.ClientAuthInfoWriter) (*GetDeviceByHostnameOK, error) {
	// TODO: Validate the params before sending
	if params == nil {
		params = NewGetDeviceByHostnameParams()
	}

	result, err := a.transport.Submit(&runtime.ClientOperation{
		ID:                 "getDeviceByHostname",
		Method:             "GET",
		PathPattern:        "/Gets/getDeviceByHostname",
		ProducesMediaTypes: []string{"application/json"},
		ConsumesMediaTypes: []string{"application/json"},
		Schemes:            []string{"http", "https"},
		Params:             params,
		Reader:             &GetDeviceByHostnameReader{formats: a.formats},
		AuthInfo:           authInfo,
		Context:            params.Context,
		Client:             params.HTTPClient,
	})
	if err != nil {
		return nil, err
	}
	return result.(*GetDeviceByHostnameOK), nil

}

/*
GetDeviceByIPAddr gets a device by identifying its IP address

Retrieve information about a device, by specifying the IP address and, optionally, the container name (if required for overlapping space).
*/
func (a *Client) GetDeviceByIPAddr(params *GetDeviceByIPAddrParams, authInfo runtime.ClientAuthInfoWriter) (*GetDeviceByIPAddrOK, error) {
	// TODO: Validate the params before sending
	if params == nil {
		params = NewGetDeviceByIPAddrParams()
	}

	result, err := a.transport.Submit(&runtime.ClientOperation{
		ID:                 "getDeviceByIPAddr",
		Method:             "GET",
		PathPattern:        "/Gets/getDeviceByIPAddr",
		ProducesMediaTypes: []string{"application/json"},
		ConsumesMediaTypes: []string{"application/json"},
		Schemes:            []string{"http", "https"},
		Params:             params,
		Reader:             &GetDeviceByIPAddrReader{formats: a.formats},
		AuthInfo:           authInfo,
		Context:            params.Context,
		Client:             params.HTTPClient,
	})
	if err != nil {
		return nil, err
	}
	return result.(*GetDeviceByIPAddrOK), nil

}

/*
GetDeviceByMACAddress gets a device by identifying its hardware address

Retrieve information about a device by specifying the MAC address.
*/
func (a *Client) GetDeviceByMACAddress(params *GetDeviceByMACAddressParams, authInfo runtime.ClientAuthInfoWriter) (*GetDeviceByMACAddressOK, error) {
	// TODO: Validate the params before sending
	if params == nil {
		params = NewGetDeviceByMACAddressParams()
	}

	result, err := a.transport.Submit(&runtime.ClientOperation{
		ID:                 "getDeviceByMACAddress",
		Method:             "GET",
		PathPattern:        "/Gets/getDeviceByMACAddress",
		ProducesMediaTypes: []string{"application/json"},
		ConsumesMediaTypes: []string{"application/json"},
		Schemes:            []string{"http", "https"},
		Params:             params,
		Reader:             &GetDeviceByMACAddressReader{formats: a.formats},
		AuthInfo:           authInfo,
		Context:            params.Context,
		Client:             params.HTTPClient,
	})
	if err != nil {
		return nil, err
	}
	return result.(*GetDeviceByMACAddressOK), nil

}

/*
ImportDevice imports a device

The importDevice operation enables you to import a device to IPControl. It can also be used to modify an existing device by specifying its id.
*/
func (a *Client) ImportDevice(params *ImportDeviceParams, authInfo runtime.ClientAuthInfoWriter) (*ImportDeviceOK, error) {
	// TODO: Validate the params before sending
	if params == nil {
		params = NewImportDeviceParams()
	}

	result, err := a.transport.Submit(&runtime.ClientOperation{
		ID:                 "importDevice",
		Method:             "POST",
		PathPattern:        "/Imports/importDevice",
		ProducesMediaTypes: []string{"application/json"},
		ConsumesMediaTypes: []string{"application/json"},
		Schemes:            []string{"http", "https"},
		Params:             params,
		Reader:             &ImportDeviceReader{formats: a.formats},
		AuthInfo:           authInfo,
		Context:            params.Context,
		Client:             params.HTTPClient,
	})
	if err != nil {
		return nil, err
	}
	return result.(*ImportDeviceOK), nil

}

/*
ImportIPAddressRange imports an IP address range

The ImportIpAddressRange operation enables you to import a range of IP addresses to IPControl. Specify domainName if resource record creation is requested and no forward domain is defined in the block policies. Specify primaryDHCPServer when adding DHCP addresses and no primary DHCP server is defined in the block policies.
*/
func (a *Client) ImportIPAddressRange(params *ImportIPAddressRangeParams, authInfo runtime.ClientAuthInfoWriter) (*ImportIPAddressRangeOK, error) {
	// TODO: Validate the params before sending
	if params == nil {
		params = NewImportIPAddressRangeParams()
	}

	result, err := a.transport.Submit(&runtime.ClientOperation{
		ID:                 "importIpAddressRange",
		Method:             "POST",
		PathPattern:        "/Imports/importIpAddressRange",
		ProducesMediaTypes: []string{"application/json"},
		ConsumesMediaTypes: []string{"application/json"},
		Schemes:            []string{"http", "https"},
		Params:             params,
		Reader:             &ImportIPAddressRangeReader{formats: a.formats},
		AuthInfo:           authInfo,
		Context:            params.Context,
		Client:             params.HTTPClient,
	})
	if err != nil {
		return nil, err
	}
	return result.(*ImportIPAddressRangeOK), nil

}

/*
InitExportDevice exports devices

The initExportDevice operation enables you to export devices from IPControl. You can filter the list of devices retrieved. When recurseContainerHierarchy is specified in the options array, the service recursively exports all devices within all child containers specified within the Container Selector filter. This flag is ignored if a Container Selector is not included. When pageSize and firstResultPos are specified, a list of structures is returned as described for the exportDevice operation. Otherwise, the returned structure can be passed on a subsequent exportDevice operation to retrieve results.
*/
func (a *Client) InitExportDevice(params *InitExportDeviceParams, authInfo runtime.ClientAuthInfoWriter) (*InitExportDeviceOK, error) {
	// TODO: Validate the params before sending
	if params == nil {
		params = NewInitExportDeviceParams()
	}

	result, err := a.transport.Submit(&runtime.ClientOperation{
		ID:                 "initExportDevice",
		Method:             "POST",
		PathPattern:        "/Exports/initExportDevice",
		ProducesMediaTypes: []string{"application/json"},
		ConsumesMediaTypes: []string{"application/json"},
		Schemes:            []string{"http", "https"},
		Params:             params,
		Reader:             &InitExportDeviceReader{formats: a.formats},
		AuthInfo:           authInfo,
		Context:            params.Context,
		Client:             params.HTTPClient,
	})
	if err != nil {
		return nil, err
	}
	return result.(*InitExportDeviceOK), nil

}

/*
InitExportDeviceRestoreList exports device restore list

The initExportDeviceRestoreList API enables the web service client to issue a request to retrieve a list of devices that have been deleted and may be eligible for restoring. This service enables the client to filter the list of devices(deleted) exported. A superuser may filter the results based on the requesting administrator(not available for non-superusers). All users may filter the list based on IP Address, hostname, block name, container, address type, device type, or IP address range. There are no options defined for this operation. When pageSize and firstResultPos are specified, a list of structures is returned as described for the exportDeviceRestoreList operation. Otherwise, the returned structure can be passed on a subsequent exportDeviceRestoreList operation to retrieve results.
*/
func (a *Client) InitExportDeviceRestoreList(params *InitExportDeviceRestoreListParams, authInfo runtime.ClientAuthInfoWriter) (*InitExportDeviceRestoreListOK, error) {
	// TODO: Validate the params before sending
	if params == nil {
		params = NewInitExportDeviceRestoreListParams()
	}

	result, err := a.transport.Submit(&runtime.ClientOperation{
		ID:                 "initExportDeviceRestoreList",
		Method:             "POST",
		PathPattern:        "/Exports/initExportDeviceRestoreList",
		ProducesMediaTypes: []string{"application/json"},
		ConsumesMediaTypes: []string{"application/json"},
		Schemes:            []string{"http", "https"},
		Params:             params,
		Reader:             &InitExportDeviceRestoreListReader{formats: a.formats},
		AuthInfo:           authInfo,
		Context:            params.Context,
		Client:             params.HTTPClient,
	})
	if err != nil {
		return nil, err
	}
	return result.(*InitExportDeviceRestoreListOK), nil

}

/*
InitExportDeviceUDFTags exports device u d f tags

After the initExportDevice API is called to initialize the API, the web service client can, optionally, call the initExportDeviceUDFTags API. This service is used by the ExportDevice CLI to create the header line used when exporting with the expanded format option. The result returned from the initExportDeviceUDFTags service is an array of strings. These are the field names/tags of the user defined fields defined for the devices that will be returned on subsequent calls to the exportDevice service.
*/
func (a *Client) InitExportDeviceUDFTags(params *InitExportDeviceUDFTagsParams, authInfo runtime.ClientAuthInfoWriter) (*InitExportDeviceUDFTagsOK, error) {
	// TODO: Validate the params before sending
	if params == nil {
		params = NewInitExportDeviceUDFTagsParams()
	}

	result, err := a.transport.Submit(&runtime.ClientOperation{
		ID:                 "initExportDeviceUDFTags",
		Method:             "POST",
		PathPattern:        "/Exports/initExportDeviceUDFTags",
		ProducesMediaTypes: []string{"application/json"},
		ConsumesMediaTypes: []string{"application/json"},
		Schemes:            []string{"http", "https"},
		Params:             params,
		Reader:             &InitExportDeviceUDFTagsReader{formats: a.formats},
		AuthInfo:           authInfo,
		Context:            params.Context,
		Client:             params.HTTPClient,
	})
	if err != nil {
		return nil, err
	}
	return result.(*InitExportDeviceUDFTagsOK), nil

}

/*
MergeDevice merges device

The MergeDevice operation enables you to merge the IP addresses and interfaces of an existing (source) device to a target device. For example, there are 2 IP addresses in the system that were either discovered separately, or entered separately, but the 2 addresses are actually the same device. For example, a laptop can have a v4 and a v6 address that were discovered separately. This API enables you to combine these interfaces into a single device.
*/
func (a *Client) MergeDevice(params *MergeDeviceParams, authInfo runtime.ClientAuthInfoWriter) (*MergeDeviceOK, error) {
	// TODO: Validate the params before sending
	if params == nil {
		params = NewMergeDeviceParams()
	}

	result, err := a.transport.Submit(&runtime.ClientOperation{
		ID:                 "mergeDevice",
		Method:             "POST",
		PathPattern:        "/Imports/mergeDevice",
		ProducesMediaTypes: []string{"application/json"},
		ConsumesMediaTypes: []string{"application/json"},
		Schemes:            []string{"http", "https"},
		Params:             params,
		Reader:             &MergeDeviceReader{formats: a.formats},
		AuthInfo:           authInfo,
		Context:            params.Context,
		Client:             params.HTTPClient,
	})
	if err != nil {
		return nil, err
	}
	return result.(*MergeDeviceOK), nil

}

/*
RestoreDeletedDevice restores a deleted device

The restoreDeletedDevice operation enables you to restore a deleted device. To obtain the required restoreId, use the ExportDeviceRestoreList operation to export a list of the devices that were deleted and are available to be restored.
*/
func (a *Client) RestoreDeletedDevice(params *RestoreDeletedDeviceParams, authInfo runtime.ClientAuthInfoWriter) (*RestoreDeletedDeviceOK, error) {
	// TODO: Validate the params before sending
	if params == nil {
		params = NewRestoreDeletedDeviceParams()
	}

	result, err := a.transport.Submit(&runtime.ClientOperation{
		ID:                 "restoreDeletedDevice",
		Method:             "POST",
		PathPattern:        "/Imports/restoreDeletedDevice",
		ProducesMediaTypes: []string{"application/json"},
		ConsumesMediaTypes: []string{"application/json"},
		Schemes:            []string{"http", "https"},
		Params:             params,
		Reader:             &RestoreDeletedDeviceReader{formats: a.formats},
		AuthInfo:           authInfo,
		Context:            params.Context,
		Client:             params.HTTPClient,
	})
	if err != nil {
		return nil, err
	}
	return result.(*RestoreDeletedDeviceOK), nil

}

/*
UseNextReservedIPAddress updates next reserved IP address to static

The UseNextReservedIPAddress API enables the web service client to update the next reserved IP Address in the specified block, for the specified device type, to static. The block must have a status of 'In Use/Deployed'. Within this block, there should be a range of addresses with a type of 'Reserved' for the given device type. The next lowest IP address within the range will be assigned a type of 'Static'. If a hostname is specified, it will be applied to the device associated with the IP Address. In addition, there is an option to add resource records for the device.
*/
func (a *Client) UseNextReservedIPAddress(params *UseNextReservedIPAddressParams, authInfo runtime.ClientAuthInfoWriter) (*UseNextReservedIPAddressOK, error) {
	// TODO: Validate the params before sending
	if params == nil {
		params = NewUseNextReservedIPAddressParams()
	}

	result, err := a.transport.Submit(&runtime.ClientOperation{
		ID:                 "useNextReservedIpAddress",
		Method:             "POST",
		PathPattern:        "/IncUseNextReservedIPAddress/useNextReservedIPAddress",
		ProducesMediaTypes: []string{"application/json"},
		ConsumesMediaTypes: []string{"application/json"},
		Schemes:            []string{"http", "https"},
		Params:             params,
		Reader:             &UseNextReservedIPAddressReader{formats: a.formats},
		AuthInfo:           authInfo,
		Context:            params.Context,
		Client:             params.HTTPClient,
	})
	if err != nil {
		return nil, err
	}
	return result.(*UseNextReservedIPAddressOK), nil

}

// SetTransport changes the transport on the client
func (a *Client) SetTransport(transport runtime.ClientTransport) {
	a.transport = transport
}
