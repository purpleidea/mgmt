// Mgmt
// Copyright (C) 2013-2019+ James Shubin and the project contributors
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
// along with this program.  If not, see <http://www.gnu.org/licenses/>.

package gapi

import (
	"bytes"
	"encoding/base64"
	"encoding/gob"

	"github.com/purpleidea/mgmt/util/errwrap"
)

func init() {
	gob.Register(&Deploy{})
}

// Deploy represents a deploy action, include the type of GAPI to deploy, the
// payload of that GAPI, and any deploy specific parameters that were chosen.
// TODO: add staged rollout functionality to this struct
// TODO: add proper authentication with gpg key signing
type Deploy struct {
	ID   uint64
	Name string // lang, puppet, yaml, etc...
	//Sync bool // wait for everyone to close previous GAPI before switching
	Noop bool
	Sema int // sema override
	GAPI GAPI
}

// ToB64 encodes a deploy struct as a base64 encoded string.
func (obj *Deploy) ToB64() (string, error) {
	b := bytes.Buffer{}
	e := gob.NewEncoder(&b)
	err := e.Encode(&obj) // pass with &
	if err != nil {
		return "", errwrap.Wrapf(err, "gob failed to encode")
	}
	return base64.StdEncoding.EncodeToString(b.Bytes()), nil
}

// NewDeployFromB64 decodes a deploy struct from a base64 encoded string.
func NewDeployFromB64(str string) (*Deploy, error) {
	var deploy *Deploy
	bb, err := base64.StdEncoding.DecodeString(str)
	if err != nil {
		return nil, errwrap.Wrapf(err, "base64 failed to decode")
	}
	b := bytes.NewBuffer(bb)
	d := gob.NewDecoder(b)
	if err := d.Decode(&deploy); err != nil { // pass with &
		return nil, errwrap.Wrapf(err, "gob failed to decode")
	}
	return deploy, nil
}
