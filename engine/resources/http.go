// Mgmt
// Copyright (C) James Shubin and the project contributors
// Written by James Shubin <james@shubin.ca> and the project contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.
//
// Additional permission under GNU GPL version 3 section 7
//
// If you modify this program, or any covered work, by linking or combining it
// with embedded mcl code and modules (and that the embedded mcl code and
// modules which link with this program, contain a copy of their source code in
// the authoritative form) containing parts covered by the terms of any other
// license, the licensors of this program grant you additional permission to
// convey the resulting work. Furthermore, the licensors of this program grant
// the original author, James Shubin, additional permission to update this
// additional permission if he deems it necessary to achieve the goals of this
// additional permission.

package resources

import (
	"net/http"
	"os"
	"strconv"
)

const (
	httpKind = "http"
)

// httpError represents a specific HTTP error to send, but can be stored as an
// internal golang `error` type.
type httpError struct {
	msg  string
	code int
}

// Error is required to implement the `error` type interface.
func (obj *httpError) Error() string {
	return strconv.Itoa(obj.code) + " " + obj.msg
}

// newHTTPError generates a new httpError based on a single status code. It gets
// the msg text from the http.StatusText method.
func newHTTPError(code int) error {
	return &httpError{
		msg:  http.StatusText(code),
		code: code,
	}
}

// toHTTPError returns a non-specific HTTP error message and status code for a
// given non-nil error value. It's important that toHTTPError does not actually
// return err.Error(), since msg and httpStatus are returned to users, and
// historically Go's ServeContent always returned just "404 Not Found" for all
// errors. We don't want to start leaking information in error messages.
// NOTE: This was copied and modified slightly from the golang net/http package.
// See: https://github.com/golang/go/issues/38375
func toHTTPError(err error) (msg string, httpStatus int) {
	if e, ok := err.(*httpError); ok {
		return e.msg, e.code
	}
	if os.IsNotExist(err) {
		//return "404 page not found", http.StatusNotFound
		return http.StatusText(http.StatusNotFound), http.StatusNotFound
	}
	if os.IsPermission(err) {
		//return "403 Forbidden", http.StatusForbidden
		return http.StatusText(http.StatusForbidden), http.StatusForbidden
	}
	// Default:
	//return "500 Internal Server Error", http.StatusInternalServerError
	return http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError
}

// sendHTTPError is a helper function for sending an http error response.
func sendHTTPError(w http.ResponseWriter, err error) {
	msg, httpStatus := toHTTPError(err)
	http.Error(w, msg, httpStatus)
}
