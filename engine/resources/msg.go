// Mgmt
// Copyright (C) 2013-2020+ James Shubin and the project contributors
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

package resources

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/purpleidea/mgmt/engine"
	"github.com/purpleidea/mgmt/engine/traits"

	"github.com/coreos/go-systemd/journal"
)

func init() {
	engine.RegisterResource("msg", func() engine.Res { return &MsgRes{} })
}

// MsgRes is a resource that writes messages to logs.
type MsgRes struct {
	traits.Base // add the base methods without re-implementation
	traits.Refreshable

	init *engine.Init

	Body           string            `yaml:"body"`
	Priority       string            `yaml:"priority"`
	Fields         map[string]string `yaml:"fields"`
	Journal        bool              `yaml:"journal"` // enable systemd journal output
	Syslog         bool              `yaml:"syslog"`  // enable syslog output
	logStateOK     bool
	journalStateOK bool
	syslogStateOK  bool
}

// Default returns some sensible defaults for this resource.
func (obj *MsgRes) Default() engine.Res {
	return &MsgRes{}
}

// Validate the params that are passed to MsgRes.
func (obj *MsgRes) Validate() error {
	invalidCharacters := regexp.MustCompile("[^a-zA-Z0-9_]")
	for field := range obj.Fields {
		if invalidCharacters.FindString(field) != "" {
			return fmt.Errorf("invalid character in field %s", field)
		}
		if strings.HasPrefix(field, "_") {
			return fmt.Errorf("fields cannot begin with _")
		}
	}
	switch obj.Priority {
	case "Emerg":
	case "Alert":
	case "Crit":
	case "Err":
	case "Warning":
	case "Notice":
	case "Info":
	case "Debug":
	default:
		return fmt.Errorf("invalid Priority '%s'", obj.Priority)
	}
	return nil
}

// Init runs some startup code for this resource.
func (obj *MsgRes) Init(init *engine.Init) error {
	obj.init = init // save for later

	return nil
}

// Close is run by the engine to clean up after the resource is done.
func (obj *MsgRes) Close() error {
	return nil
}

// Watch is the primary listener for this resource and it outputs events.
func (obj *MsgRes) Watch() error {
	obj.init.Running() // when started, notify engine that we're running

	//var send = false // send event?
	for {
		select {
		case <-obj.init.Done: // closed by the engine to signal shutdown
			return nil
		}

		// do all our event sending all together to avoid duplicate msgs
		//if send {
		//	send = false
		//	obj.init.Event() // notify engine of an event (this can block)
		//}
	}
}

// isAllStateOK derives a compound state from all internal cache flags that
// apply to this resource.
func (obj *MsgRes) isAllStateOK() bool {
	if obj.Journal && !obj.journalStateOK {
		return false
	}
	if obj.Syslog && !obj.syslogStateOK {
		return false
	}
	return obj.logStateOK
}

// updateStateOK sets the global state so it can be read by the engine.
func (obj *MsgRes) updateStateOK() {
	// XXX: this resource doesn't entirely make sense to me at the moment.
	if !obj.isAllStateOK() {
		//obj.init.Dirty() // XXX: removed with API cleanup
	}
}

// JournalPriority converts a string description to a numeric priority.
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

// CheckApply method for Msg resource. Every check leads to an apply, meaning
// that the message is flushed to the journal.
func (obj *MsgRes) CheckApply(apply bool) (bool, error) {
	// isStateOK() done by engine, so we updateStateOK() to pass in value
	//if obj.isAllStateOK() {
	//	return true, nil
	//}

	if obj.init.Refresh() { // if we were notified...
		// invalidate cached state...
		obj.logStateOK = false
		if obj.Journal {
			obj.journalStateOK = false
		}
		if obj.Syslog {
			obj.syslogStateOK = false
		}
		obj.updateStateOK()
	}

	if !obj.logStateOK {
		obj.init.Logf("Body: %s", obj.Body)
		obj.logStateOK = true
		obj.updateStateOK()
	}

	if !apply {
		return false, nil
	}
	if obj.Journal && !obj.journalStateOK {
		if err := journal.Send(obj.Body, obj.journalPriority(), obj.Fields); err != nil {
			return false, err
		}
		obj.journalStateOK = true
		obj.updateStateOK()
	}
	if obj.Syslog && !obj.syslogStateOK {
		// TODO: implement syslog client
		obj.syslogStateOK = true
		obj.updateStateOK()
	}
	return false, nil
}

// Cmp compares two resources and returns an error if they are not equivalent.
func (obj *MsgRes) Cmp(r engine.Res) error {
	// we can only compare MsgRes to others of the same resource kind
	res, ok := r.(*MsgRes)
	if !ok {
		return fmt.Errorf("not a %s", obj.Kind())
	}

	if obj.Body != res.Body {
		return fmt.Errorf("the Body differs")
	}
	if obj.Priority != res.Priority {
		return fmt.Errorf("the Priority differs")
	}
	if len(obj.Fields) != len(res.Fields) {
		return fmt.Errorf("the length of Fields differs")
	}
	for field, value := range obj.Fields {
		if res.Fields[field] != value {
			return fmt.Errorf("the Fields differ")
		}
	}

	return nil
}

// MsgUID is a unique representation for a MsgRes object.
type MsgUID struct {
	engine.BaseUID

	body string
}

// UIDs includes all params to make a unique identification of this object.
// Most resources only return one, although some resources can return multiple.
func (obj *MsgRes) UIDs() []engine.ResUID {
	x := &MsgUID{
		BaseUID: engine.BaseUID{Name: obj.Name(), Kind: obj.Kind()},
		body:    obj.Body,
	}
	return []engine.ResUID{x}
}

// UnmarshalYAML is the custom unmarshal handler for this struct.
// It is primarily useful for setting the defaults.
func (obj *MsgRes) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type rawRes MsgRes // indirection to avoid infinite recursion

	def := obj.Default()     // get the default
	res, ok := def.(*MsgRes) // put in the right format
	if !ok {
		return fmt.Errorf("could not convert to MsgRes")
	}
	raw := rawRes(*res) // convert; the defaults go here

	if err := unmarshal(&raw); err != nil {
		return err
	}

	*obj = MsgRes(raw) // restore from indirection with type conversion!
	return nil
}
