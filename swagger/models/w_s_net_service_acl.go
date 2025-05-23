// Code generated by go-swagger; DO NOT EDIT.

package models

// This file was generated by the swagger tool.
// Editing this file might prove futile when you re-run the swagger generate command

import (
	strfmt "github.com/go-openapi/strfmt"

	"github.com/go-openapi/swag"
)

// WSNetServiceACL w s net service ACL
// swagger:model WSNetServiceACL
type WSNetServiceACL struct {

	// allow deploy
	AllowDeploy bool `json:"allowDeploy,omitempty"`

	// allow read
	AllowRead bool `json:"allowRead,omitempty"`

	// allow write
	AllowWrite bool `json:"allowWrite,omitempty"`

	// server name
	ServerName string `json:"serverName,omitempty"`

	// server type
	ServerType string `json:"serverType,omitempty"`
}

// Validate validates this w s net service ACL
func (m *WSNetServiceACL) Validate(formats strfmt.Registry) error {
	return nil
}

// MarshalBinary interface implementation
func (m *WSNetServiceACL) MarshalBinary() ([]byte, error) {
	if m == nil {
		return nil, nil
	}
	return swag.WriteJSON(m)
}

// UnmarshalBinary interface implementation
func (m *WSNetServiceACL) UnmarshalBinary(b []byte) error {
	var res WSNetServiceACL
	if err := swag.ReadJSON(b, &res); err != nil {
		return err
	}
	*m = res
	return nil
}
