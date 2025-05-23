// Code generated by go-swagger; DO NOT EDIT.

package models

// This file was generated by the swagger tool.
// Editing this file might prove futile when you re-run the swagger generate command

import (
	strfmt "github.com/go-openapi/strfmt"

	"github.com/go-openapi/swag"
)

// WSPrefixPool w s prefix pool
// swagger:model WSPrefixPool
type WSPrefixPool struct {

	// allow client classes
	AllowClientClasses []string `json:"allowClientClasses"`

	// container
	Container string `json:"container,omitempty"`

	// delegated prefix length
	DelegatedPrefixLength int64 `json:"delegatedPrefixLength,omitempty"`

	// deny client classes
	DenyClientClasses []string `json:"denyClientClasses"`

	// dhcp option set
	DhcpOptionSet string `json:"dhcpOptionSet,omitempty"`

	// dhcp policy set
	DhcpPolicySet string `json:"dhcpPolicySet,omitempty"`

	// id
	ID int64 `json:"id,omitempty"`

	// length
	Length int64 `json:"length,omitempty"`

	// longest prefix length
	LongestPrefixLength int64 `json:"longestPrefixLength,omitempty"`

	// name
	Name string `json:"name,omitempty"`

	// overlap interface Ip
	OverlapInterfaceIP bool `json:"overlapInterfaceIp,omitempty"`

	// primary net service
	PrimaryNetService string `json:"primaryNetService,omitempty"`

	// shortest prefix length
	ShortestPrefixLength int64 `json:"shortestPrefixLength,omitempty"`

	// start addr
	StartAddr string `json:"startAddr,omitempty"`

	// type
	Type string `json:"type,omitempty"`
}

// Validate validates this w s prefix pool
func (m *WSPrefixPool) Validate(formats strfmt.Registry) error {
	return nil
}

// MarshalBinary interface implementation
func (m *WSPrefixPool) MarshalBinary() ([]byte, error) {
	if m == nil {
		return nil, nil
	}
	return swag.WriteJSON(m)
}

// UnmarshalBinary interface implementation
func (m *WSPrefixPool) UnmarshalBinary(b []byte) error {
	var res WSPrefixPool
	if err := swag.ReadJSON(b, &res); err != nil {
		return err
	}
	*m = res
	return nil
}
