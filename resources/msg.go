// Mgmt
// Copyright (C) 2013-2016+ James Shubin and the project contributors
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

package resources

import (
	"encoding/gob"
	"fmt"
	"log"
	"regexp"
	"strings"
	"time"

	"github.com/purpleidea/mgmt/event"

	"github.com/coreos/go-systemd/journal"
)

func init() {
	gob.Register(&MsgRes{})
}

// MsgRes is a resource that writes messages to logs.
type MsgRes struct {
	BaseRes        `yaml:",inline"`
	Body           string            `yaml:"body"`
	Priority       string            `yaml:"priority"`
	Fields         map[string]string `yaml:"fields"`
	Journal        bool              `yaml:"journal"` // enable systemd journal output
	Syslog         bool              `yaml:"syslog"`  // enable syslog output
	logStateOK     bool
	journalStateOK bool
	syslogStateOK  bool
}

// MsgUUID is a unique representation for a MsgRes object.
type MsgUUID struct {
	BaseUUID
	body string
}

// NewMsgRes is a constructor for this resource.
func NewMsgRes(name, body, priority string, journal, syslog bool, fields map[string]string) *MsgRes {
	message := name
	if body != "" {
		message = body
	}

	obj := &MsgRes{
		BaseRes: BaseRes{
			Name: name,
		},
		Body:     message,
		Priority: priority,
		Fields:   fields,
		Journal:  journal,
		Syslog:   syslog,
	}

	obj.Init()
	return obj
}

// Init runs some startup code for this resource.
func (obj *MsgRes) Init() error {
	obj.BaseRes.kind = "Msg"
	return obj.BaseRes.Init() // call base init, b/c we're overrriding
}

// Validate the params that are passed to MsgRes
func (obj *MsgRes) Validate() error {
	invalidCharacters := regexp.MustCompile("[^a-zA-Z0-9_]")
	for field := range obj.Fields {
		if invalidCharacters.FindString(field) != "" {
			return fmt.Errorf("Invalid character in field %s.", field)
		}
		if strings.HasPrefix(field, "_") {
			return fmt.Errorf("Fields cannot begin with _.")
		}
	}
	return nil
}

// Watch is the primary listener for this resource and it outputs events.
func (obj *MsgRes) Watch(processChan chan event.Event) error {
	if obj.IsWatching() {
		return nil
	}
	obj.SetWatching(true)
	defer obj.SetWatching(false)
	cuuid := obj.converger.Register()
	defer cuuid.Unregister()

	var startup bool
	Startup := func(block bool) <-chan time.Time {
		if block {
			return nil // blocks forever
			//return make(chan time.Time) // blocks forever
		}
		return time.After(time.Duration(500) * time.Millisecond) // 1/2 the resolution of converged timeout
	}

	var send = false // send event?
	var exit = false
	for {
		obj.SetState(ResStateWatching) // reset
		select {
		case event := <-obj.events:
			cuuid.SetConverged(false)
			// we avoid sending events on unpause
			if exit, send = obj.ReadEvent(&event); exit {
				return nil // exit
			}

			/*
				// TODO: invalidate cached state on poke events
				obj.logStateOK = false
				if obj.Journal {
					obj.journalStateOK = false
				}
				if obj.Syslog {
					obj.syslogStateOK = false
				}
			*/
			send = true

		case <-cuuid.ConvergedTimer():
			cuuid.SetConverged(true) // converged!
			continue

		case <-Startup(startup):
			cuuid.SetConverged(false)
			send = true
		}

		// do all our event sending all together to avoid duplicate msgs
		if send {
			startup = true // startup finished
			send = false
			// only do this on certain types of events
			//obj.isStateOK = false // something made state dirty
			if exit, err := obj.DoSend(processChan, ""); exit || err != nil {
				return err // we exit or bubble up a NACK...
			}
		}
	}
}

// GetUUIDs includes all params to make a unique identification of this object.
// Most resources only return one, although some resources can return multiple.
func (obj *MsgRes) GetUUIDs() []ResUUID {
	x := &MsgUUID{
		BaseUUID: BaseUUID{
			name: obj.GetName(),
			kind: obj.Kind(),
		},
		body: obj.Body,
	}
	return []ResUUID{x}
}

// AutoEdges returns the AutoEdges. In this case none are used.
func (obj *MsgRes) AutoEdges() AutoEdge {
	return nil
}

// Compare two resources and return if they are equivalent.
func (obj *MsgRes) Compare(res Res) bool {
	switch res.(type) {
	case *MsgRes:
		res := res.(*MsgRes)
		if !obj.BaseRes.Compare(res) {
			return false
		}
		if obj.Body != res.Body {
			return false
		}
		if obj.Priority != res.Priority {
			return false
		}
		if len(obj.Fields) != len(res.Fields) {
			return false
		}
		for field, value := range obj.Fields {
			if res.Fields[field] != value {
				return false
			}
		}
	default:
		return false
	}
	return true
}

// IsAllStateOK derives a compound state from all internal cache flags that apply to this resource.
func (obj *MsgRes) isAllStateOK() bool {
	if obj.Journal && !obj.journalStateOK {
		return false
	}
	if obj.Syslog && !obj.syslogStateOK {
		return false
	}
	return obj.logStateOK
}

// JournalPriority converts a string description to a numeric priority.
// XXX Have Validate() make sure it actually is one of these.
func (obj *MsgRes) journalPriority() journal.Priority {
	switch obj.Priority {
	case "Emerg":
		return journal.PriEmerg
	case "Alert":
		return journal.PriAlert
	case "Crit":
		return journal.PriCrit
	case "Err":
		return journal.PriErr
	case "Warning":
		return journal.PriWarning
	case "Notice":
		return journal.PriNotice
	case "Info":
		return journal.PriInfo
	case "Debug":
		return journal.PriDebug
	}
	return journal.PriNotice
}

// CheckApply method for Msg resource.
// Every check leads to an apply, meaning that the message is flushed to the journal.
func (obj *MsgRes) CheckApply(apply bool) (bool, error) {
	log.Printf("%s[%s]: CheckApply(%t)", obj.Kind(), obj.GetName(), apply)

	if obj.isAllStateOK() {
		return true, nil
	}

	if !obj.logStateOK {
		log.Printf("%s[%s]: Body: %s", obj.Kind(), obj.GetName(), obj.Body)
		obj.logStateOK = true
	}

	if !apply {
		return false, nil
	}
	if obj.Journal && !obj.journalStateOK {
		if err := journal.Send(obj.Body, obj.journalPriority(), obj.Fields); err != nil {
			return false, err
		}
		obj.journalStateOK = true
	}
	if obj.Syslog && !obj.syslogStateOK {
		// TODO: implement syslog client
		obj.syslogStateOK = true
	}
	return false, nil
}
