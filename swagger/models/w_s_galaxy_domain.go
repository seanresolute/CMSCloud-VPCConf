// Code generated by go-swagger; DO NOT EDIT.

package models

// This file was generated by the swagger tool.
// Editing this file might prove futile when you re-run the swagger generate command

import (
	strfmt "github.com/go-openapi/strfmt"

	"github.com/go-openapi/swag"
)

// WSGalaxyDomain w s galaxy domain
// swagger:model WSGalaxyDomain
type WSGalaxyDomain struct {

	// domain name
	DomainName string `json:"domainName,omitempty"`

	// domain type
	DomainType string `json:"domainType,omitempty"`

	// galaxy name
	GalaxyName string `json:"galaxyName,omitempty"`

	// view
	View string `json:"view,omitempty"`
}

// Validate validates this w s galaxy domain
func (m *WSGalaxyDomain) Validate(formats strfmt.Registry) error {
	return nil
}

// MarshalBinary interface implementation
func (m *WSGalaxyDomain) MarshalBinary() ([]byte, error) {
	if m == nil {
		return nil, nil
	}
	return swag.WriteJSON(m)
}

// UnmarshalBinary interface implementation
func (m *WSGalaxyDomain) UnmarshalBinary(b []byte) error {
	var res WSGalaxyDomain
	if err := swag.ReadJSON(b, &res); err != nil {
		return err
	}
	*m = res
	return nil
}
