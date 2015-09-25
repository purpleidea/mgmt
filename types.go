// Mgmt
// Copyright (C) 2013-2015+ James Shubin and the project contributors
// Written by James Shubin <james@shubin.ca> and the project contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <http://www.gnu.org/licenses/>.

package main

import (
	"code.google.com/p/go-uuid/uuid"
	"fmt"
	"log"
)

type Type interface {
	//Name()  string
	Watch(*Vertex)
	StateOK() bool // TODO: can we rename this to something better?
	Apply() bool
	Exit() bool
}

type NoopType struct {
	uuid   string
	Type   string      // always "noop"
	Name   string      // name variable
	Events chan string // FIXME: eventually a struct for the event?
}

func NewNoopType(name string) *NoopType {
	return &NoopType{
		uuid:   uuid.New(),
		Type:   "noop",
		Name:   name,
		Events: make(chan string, 1), // XXX: chan size?
	}
}

func (obj NoopType) Watch(v *Vertex) {
	select {
	case exit := <-obj.Events:
		if exit == "exit" {
			return
		} else {
			log.Fatal("Unknown event: %v\n", exit)
		}
	}
}

func (obj NoopType) Exit() bool {
	obj.Events <- "exit"
	return true
}

func (obj NoopType) StateOK() bool {
	return true // never needs updating
}

func (obj NoopType) Apply() bool {
	fmt.Printf("Apply->%v[%v]\n", obj.Type, obj.Name)
	return true
}
