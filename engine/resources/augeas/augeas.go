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

//go:build !noaugeas

// Package augeas is a small binding to the parts of the augeas library that we
// use. By default the library is linked, but when built with the augeas_dlopen
// tag, it is loaded at runtime instead, the first time that it is needed. This
// lets the binary run on machines that don't have augeas, as long as nothing
// tries to use it, and lets it build without the augeas development headers.
package augeas

// #include <stdlib.h>
//
// // These are implemented in link.go or in dlopen.go, depending on the build.
// typedef struct augeas augeas;
// const char *mgmt_augeas_library(void);
// augeas *mgmt_aug_init(const char *root, const char *loadpath, unsigned int flags);
// int mgmt_aug_get(const augeas *aug, const char *path, const char **value);
// int mgmt_aug_set(augeas *aug, const char *path, const char *value);
// int mgmt_aug_load(augeas *aug);
// int mgmt_aug_save(augeas *aug);
// void mgmt_aug_close(augeas *aug);
// int mgmt_aug_error(augeas *aug);
// const char *mgmt_aug_error_message(augeas *aug);
// const char *mgmt_aug_error_minor_message(augeas *aug);
// const char *mgmt_aug_error_details(augeas *aug);
import "C"

import (
	"fmt"
	"strings"
	"sync"
	"unsafe"
)

// Flag is a set of flags that change how augeas behaves. They are or'ed
// together.
type Flag uint

const (
	// None is the default, which means no flags.
	None Flag = 0

	// NoModlAutoload means don't load the modules automatically, which is
	// what you want when you specify the lens to use.
	NoModlAutoload Flag = 1 << 6 // AUG_NO_MODL_AUTOLOAD

	// noErrClose means don't close on errors during initialization, so that
	// we can read the error. We always set this.
	noErrClose Flag = 1 << 8 // AUG_NO_ERR_CLOSE
)

var (
	libraryOnce  = &sync.Once{}
	libraryError error
)

// Available returns an error if the augeas library can't be used. When it is
// loaded at runtime, the first call loads it, and this can fail if it is not
// installed. When it is linked, this never fails.
func Available() error {
	libraryOnce.Do(func() {
		if msg := C.mgmt_augeas_library(); msg != nil {
			libraryError = fmt.Errorf("can't load the augeas library: %s", C.GoString(msg))
		}
	})
	return libraryError
}

// Augeas is a handle to an augeas tree. It must not be used concurrently.
type Augeas struct {
	handle *C.augeas
}

// New returns a new augeas handle, with the file system root, a list of module
// directories which can be empty, and some flags. You must Close it when done.
func New(root, loadPath string, flags Flag) (*Augeas, error) {
	if err := Available(); err != nil {
		return nil, err
	}

	cRoot := C.CString(root)
	defer C.free(unsafe.Pointer(cRoot))
	cLoadPath := C.CString(loadPath)
	defer C.free(unsafe.Pointer(cLoadPath))

	handle := C.mgmt_aug_init(cRoot, cLoadPath, C.uint(flags|noErrClose))
	if handle == nil {
		return nil, fmt.Errorf("could not initialize augeas")
	}
	obj := &Augeas{
		handle: handle,
	}
	if err := obj.error("init"); err != nil {
		obj.Close()
		return nil, err
	}

	return obj, nil
}

// Get returns the value at the path. It errors if no node, or more than one,
// matches the path, or if the path is not valid.
func (obj *Augeas) Get(path string) (string, error) {
	cPath := C.CString(path)
	defer C.free(unsafe.Pointer(cPath))

	var cValue *C.char
	ret := C.mgmt_aug_get(obj.handle, cPath, &cValue)
	if ret < 0 {
		return "", obj.error("get")
	}
	if ret == 0 {
		return "", fmt.Errorf("no matching node for: %s", path)
	}
	return C.GoString(cValue), nil // cValue is owned by augeas
}

// Set sets the value at the path, and creates any missing nodes on the way.
func (obj *Augeas) Set(path, value string) error {
	cPath := C.CString(path)
	defer C.free(unsafe.Pointer(cPath))
	cValue := C.CString(value)
	defer C.free(unsafe.Pointer(cValue))

	if C.mgmt_aug_set(obj.handle, cPath, cValue) < 0 {
		return obj.error("set")
	}
	return nil
}

// Load loads the files into the tree, which are specified under /augeas/load.
// Files which couldn't be loaded don't cause an error, but are shown under the
// /augeas//error path.
func (obj *Augeas) Load() error {
	if C.mgmt_aug_load(obj.handle) < 0 {
		return obj.error("load")
	}
	return nil
}

// Save writes all the pending changes to disk.
func (obj *Augeas) Save() error {
	if C.mgmt_aug_save(obj.handle) < 0 {
		return obj.error("save")
	}
	return nil
}

// Close frees the handle. It must not be used after this.
func (obj *Augeas) Close() {
	C.mgmt_aug_close(obj.handle)
	obj.handle = nil
}

// error returns the current augeas error for the named operation, if any.
func (obj *Augeas) error(op string) error {
	if C.mgmt_aug_error(obj.handle) == 0 { // AUG_NOERROR
		if op == "init" {
			return nil
		}
		return fmt.Errorf("augeas %s failed", op)
	}

	msg := []string{C.GoString(C.mgmt_aug_error_message(obj.handle))}
	if s := C.GoString(C.mgmt_aug_error_minor_message(obj.handle)); s != "" {
		msg = append(msg, s)
	}
	if s := C.GoString(C.mgmt_aug_error_details(obj.handle)); s != "" {
		msg = append(msg, s)
	}
	return fmt.Errorf("augeas %s failed: %s", op, strings.Join(msg, ": "))
}
