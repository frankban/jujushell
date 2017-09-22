// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiparams

// Request holds a request to the server.
type Request struct {
	// Operation holds the requested operation.
	Operation string `json:"operation"`
	// Params holds any required parameters for the operation.
	Params map[string]interface{} `json:"params"`
}

// Response holds a server response.
type Response struct {
	// Code is the response code.
	Code ResponseCode `json:"code"`
	// Message holds an optional response message.
	Message string `json:"message"`
}

// ResponseCode is a server response code.
type ResponseCode string

const (
	OK    ResponseCode = "OK"
	Error ResponseCode = "Error"
)
