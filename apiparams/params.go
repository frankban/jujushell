// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiparams

import macaroon "gopkg.in/macaroon.v1"

// Login holds parameters for making a login request.
type Login struct {
	// Operation holds the requested operation.
	Operation Operation `json:"operation"`
	// Username and Password hold traditional userpass information.
	Username string `json:"username"`
	Password string `json:"password"`
	// Macaroons holds the macaroons for logging in using an external identity.
	Macaroons []macaroon.Slice `json:"macaroons"`
}

// Start holds parameters for making a start request.
type Start struct {
	// Operation holds the requested operation.
	Operation Operation `json:"operation"`
}

// Response holds a server response.
type Response struct {
	// Code is the response code.
	Code ResponseCode `json:"code"`
	// Message holds an optional response message.
	Message string `json:"message"`
}

// Operation is a server operation.
type Operation string

const (
	OpLogin Operation = "login"
	OpStart Operation = "start"
)

// ResponseCode is a server response code.
type ResponseCode string

const (
	OK    ResponseCode = "ok"
	Error ResponseCode = "error"
)
