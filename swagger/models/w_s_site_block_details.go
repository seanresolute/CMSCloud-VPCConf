// Code generated by go-swagger; DO NOT EDIT.

package models

// This file was generated by the swagger tool.
// Editing this file might prove futile when you re-run the swagger generate command

import (
	"strconv"

	strfmt "github.com/go-openapi/strfmt"

	"github.com/go-openapi/errors"
	"github.com/go-openapi/swag"
)

// WSSiteBlockDetails w s site block details
// swagger:model WSSiteBlockDetails
type WSSiteBlockDetails struct {

	// addr details
	AddrDetails []*WSAllocationTemplateDetails `json:"addrDetails"`

	// allocation reason
	AllocationReason string `json:"allocationReason,omitempty"`

	// allocation reason description
	AllocationReasonDescription string `json:"allocationReasonDescription,omitempty"`

	// interface name
	InterfaceName string `json:"interfaceName,omitempty"`

	// swip name
	SwipName string `json:"swipName,omitempty"`

	// user defined fields
	UserDefinedFields []string `json:"userDefinedFields"`
}

// Validate validates this w s site block details
func (m *WSSiteBlockDetails) Validate(formats strfmt.Registry) error {
	var res []error

	if err := m.validateAddrDetails(formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (m *WSSiteBlockDetails) validateAddrDetails(formats strfmt.Registry) error {

	if swag.IsZero(m.AddrDetails) { // not required
		return nil
	}

	for i := 0; i < len(m.AddrDetails); i++ {
		if swag.IsZero(m.AddrDetails[i]) { // not required
			continue
		}

		if m.AddrDetails[i] != nil {
			if err := m.AddrDetails[i].Validate(formats); err != nil {
				if ve, ok := err.(*errors.Validation); ok {
					return ve.ValidateName("addrDetails" + "." + strconv.Itoa(i))
				}
				return err
			}
		}

	}

	return nil
}

// MarshalBinary interface implementation
func (m *WSSiteBlockDetails) MarshalBinary() ([]byte, error) {
	if m == nil {
		return nil, nil
	}
	return swag.WriteJSON(m)
}

// UnmarshalBinary interface implementation
func (m *WSSiteBlockDetails) UnmarshalBinary(b []byte) error {
	var res WSSiteBlockDetails
	if err := swag.ReadJSON(b, &res); err != nil {
		return err
	}
	*m = res
	return nil
}
