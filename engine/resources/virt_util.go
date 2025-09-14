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

//go:build !novirt

package resources

import (
	"fmt"
	"math/rand"
	"sync"
	"time"

	"github.com/purpleidea/mgmt/util/errwrap"

	libvirt "libvirt.org/go/libvirt" // gitlab.com/libvirt/libvirt-go-module
)

var (
	// shared by all virt resources
	libvirtInitialized = false
	libvirtMutex       *sync.Mutex
)

func init() {
	libvirtMutex = &sync.Mutex{}
}

type virtURISchemeType int

const (
	defaultURI virtURISchemeType = iota
	lxcURI
)

// libvirtInit is called in the Init method of any virt resource. It must be run
// before any connection to the hypervisor is made!
func libvirtInit() error {
	libvirtMutex.Lock()
	defer libvirtMutex.Unlock()

	if libvirtInitialized {
		return nil // done early
	}

	if err := libvirt.EventRegisterDefaultImpl(); err != nil {
		return errwrap.Wrapf(err, "method EventRegisterDefaultImpl failed")
	}
	libvirtInitialized = true

	return nil
}

// randMAC returns a random mac address in the libvirt range.
func randMAC() string {
	rand.Seed(time.Now().UnixNano())
	return "52:54:00" +
		fmt.Sprintf(":%x", rand.Intn(255)) +
		fmt.Sprintf(":%x", rand.Intn(255)) +
		fmt.Sprintf(":%x", rand.Intn(255))
}

// isNotFound tells us if this is a domain or network not found error.
// TODO: expand this with other ERR_NO_? values eventually.
func isNotFound(err error) bool {
	if err == nil {
		return false
	}

	virErr, ok := err.(libvirt.Error)
	if !ok {
		return false
	}

	if virErr.Code == libvirt.ERR_NO_DOMAIN {
		// domain not found
		return true
	}
	if virErr.Code == libvirt.ERR_NO_NETWORK {
		// network not found
		return true
	}

	return false // some other error
}

// VirtAuth is used to pass credentials to libvirt.
type VirtAuth struct {
	Username string `lang:"username" yaml:"username"`
	Password string `lang:"password" yaml:"password"`
}

// Cmp compares two VirtAuth structs. It errors if they are not identical.
func (obj *VirtAuth) Cmp(auth *VirtAuth) error {
	if (obj == nil) != (auth == nil) { // xor
		return fmt.Errorf("the VirtAuth differs")
	}
	if obj == nil && auth == nil {
		return nil
	}

	if obj.Username != auth.Username {
		return fmt.Errorf("the Username differs")
	}
	if obj.Password != auth.Password {
		return fmt.Errorf("the Password differs")
	}
	return nil
}

// Connect is the connect helper for the libvirt connection. It can handle auth.
func (obj *VirtAuth) Connect(uri string) (conn *libvirt.Connect, version uint32, err error) {
	if obj != nil {
		callback := func(creds []*libvirt.ConnectCredential) {
			// Populate credential structs with the
			// prepared username/password values
			for _, cred := range creds {
				if cred.Type == libvirt.CRED_AUTHNAME {
					cred.Result = obj.Username
					cred.ResultLen = len(cred.Result)
				} else if cred.Type == libvirt.CRED_PASSPHRASE {
					cred.Result = obj.Password
					cred.ResultLen = len(cred.Result)
				}
			}
		}
		auth := &libvirt.ConnectAuth{
			CredType: []libvirt.ConnectCredentialType{
				libvirt.CRED_AUTHNAME, libvirt.CRED_PASSPHRASE,
			},
			Callback: callback,
		}
		conn, err = libvirt.NewConnectWithAuth(uri, auth, 0)
		if err == nil {
			if v, err := conn.GetLibVersion(); err == nil {
				version = v
			}
		}
	}
	if obj == nil || err != nil {
		conn, err = libvirt.NewConnect(uri)
		if err == nil {
			if v, err := conn.GetLibVersion(); err == nil {
				version = v
			}
		}
	}
	return
}
