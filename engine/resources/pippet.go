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

package resources

import (
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"sync"

	"github.com/purpleidea/mgmt/engine"
	"github.com/purpleidea/mgmt/engine/traits"
	"github.com/purpleidea/mgmt/util/errwrap"
)

var pippetReceiverInstance *pippetReceiver
var pippetReceiverOnce sync.Once

func init() {
	engine.RegisterResource("pippet", func() engine.Res { return &PippetRes{} })
}

// PippetRes is a wrapper resource for puppet. It implements the functional
// equivalent of an exec resource that calls "puppet resource <type> <title>
// <params>", but offers superior performance through a long-running Puppet
// process that receives resources through a pipe (hence the name).
type PippetRes struct {
	traits.Base // add the base methods without re-implementation
	traits.Refreshable

	init *engine.Init

	// Type is the exact name of the wrapped Puppet resource type, e.g.
	// "file", "mount". This needs not be a core type. It can be a type
	// from a module. The Puppet installation local to the mgmt agent
	// machine must be able recognize it. It has to be a native type though,
	// as opposed to defined types from your Puppet manifest code.
	Type string `yaml:"type" json:"type"`
	// Title is used by Puppet as the resource title. Puppet will often
	// assign special meaning to the title, e.g. use it as the path for a
	// file resource, or the name of a package.
	Title string `yaml:"title" json:"title"`
	// Params is expected to be a hash in YAML format, pairing resource
	// parameter names with their respective values, e.g. { ensure: present
	// }
	Params string `yaml:"params" json:"params"`

	runner *pippetReceiver
}

// Default returns an example Pippet resource.
func (obj *PippetRes) Default() engine.Res {
	return &PippetRes{
		Params: "{}", // use an empty params hash per default
	}
}

// Validate never errors out. We don't know the set of potential types that
// Puppet supports. Resource names are arbitrary. We cannot really validate the
// parameter YAML, because we cannot assume that it can be unmarshalled into a
// map[string]string; Puppet supports complex parameter values.
func (obj *PippetRes) Validate() error {
	return nil
}

// Init makes sure that the PippetReceiver object is initialized.
func (obj *PippetRes) Init(init *engine.Init) error {
	obj.init = init // save for later
	obj.runner = getPippetReceiverInstance()
	if err := obj.runner.Register(); err != nil {
		return err
	}
	return nil
}

// Close is run by the engine to clean up after the resource is done.
func (obj *PippetRes) Close() error {
	return obj.runner.Unregister()
}

// Watch is the primary listener for this resource and it outputs events.
func (obj *PippetRes) Watch() error {
	obj.init.Running() // when started, notify engine that we're running

	select {
	case <-obj.init.Done: // closed by the engine to signal shutdown
	}

	//obj.init.Event() // notify engine of an event (this can block)

	return nil
}

// CheckApply synchronizes the resource if required.
func (obj *PippetRes) CheckApply(apply bool) (bool, error) {
	changed, err := applyPippetRes(obj.runner, obj)
	if err != nil {
		return false, fmt.Errorf("pippet: %s[%s]: ERROR - %v", obj.Type, obj.Title, err)
	}
	return !changed, nil
}

// Cmp compares two resources and returns an error if they are not equivalent.
func (obj *PippetRes) Cmp(r engine.Res) error {
	res, ok := r.(*PippetRes)
	if !ok {
		return fmt.Errorf("not a %s", obj.Kind())
	}

	if obj.Type != res.Type {
		return fmt.Errorf("the Type param differs")
	}

	if obj.Title != res.Title {
		return fmt.Errorf("the Title param differs")
	}

	// FIXME: This is a lie. Parameter lists can be equivalent but not
	// lexically identical (e.g. whitespace differences, parameter order).
	// This is difficult to handle because we cannot casually unmarshall the
	// YAML content.
	if obj.Params != res.Params {
		return fmt.Errorf("the Param param differs")
	}

	return nil
}

// PippetUID is the UID struct for PippetRes.
type PippetUID struct {
	engine.BaseUID
	resourceType  string
	resourceTitle string
}

// UIDs includes all params to make a unique identification of this object. Most
// resources only return one, although some resources can return multiple.
func (obj *PippetRes) UIDs() []engine.ResUID {
	x := &PippetUID{
		BaseUID:       engine.BaseUID{Name: obj.Name(), Kind: obj.Kind()},
		resourceType:  obj.Type,
		resourceTitle: obj.Title,
	}
	return []engine.ResUID{x}
}

// UnmarshalYAML is the custom unmarshal handler for this struct. It is
// primarily useful for setting the defaults.
func (obj *PippetRes) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type rawRes PippetRes // indirection to avoid infinite recursion

	def := obj.Default()        // get the default
	res, ok := def.(*PippetRes) // put in the right format
	if !ok {
		return fmt.Errorf("could not convert to PippetRes")
	}
	raw := rawRes(*res) // convert; the defaults go here

	if err := unmarshal(&raw); err != nil {
		return err
	}

	*obj = PippetRes(raw) // restore from indirection with type conversion!
	return nil
}

// PippetRunner is the interface used to communicate with the PippetReceiver
// object. Its main purpose is dependency injection.
type PippetRunner interface {
	LockApply()
	UnlockApply()
	InputStream() io.WriteCloser
	OutputStream() io.ReadCloser
}

// PippetResult is the structured return value type for the PippetReceiver's
// Apply function.
type PippetResult struct {
	Error     bool
	Failed    bool
	Changed   bool
	Exception string
}

// GetPippetReceiverInstance returns a pointer to the PippetReceiver object. The
// PippetReceiver is supposed to be a singleton object. The pippet resource code
// should always use the PippetReceiverInstance function to gain access to the
// pippetReceiver object. Other objects of type pippetReceiver should not be
// created.
func getPippetReceiverInstance() *pippetReceiver {
	for pippetReceiverInstance == nil {
		pippetReceiverOnce.Do(func() { pippetReceiverInstance = &pippetReceiver{} })
	}
	return pippetReceiverInstance
}

type pippetReceiver struct {
	stdin         io.WriteCloser
	stdout        io.ReadCloser
	registerMutex sync.Mutex
	applyMutex    sync.Mutex
	registered    int
}

// Init runs the Puppet process that will perform the work of synchronizing
// resources that are sent to its stdin. The process will keep running until
// Close is called. Init should not be called directly. It is implicitly called
// by the Register function.
func (obj *pippetReceiver) Init() error {
	cmd := exec.Command("puppet", "yamlresource", "receive", "--color=no")
	var err error
	obj.stdin, err = cmd.StdinPipe()
	if err != nil {
		return err
	}
	obj.stdout, err = cmd.StdoutPipe()
	if err != nil {
		return errwrap.Append(err, obj.stdin.Close())
	}
	if err = cmd.Start(); err != nil {
		return errwrap.Append(err, obj.stdin.Close())
	}
	buf := make([]byte, 80)
	if _, err = obj.stdout.Read(buf); err != nil {
		return errwrap.Append(err, obj.stdin.Close())
	}
	return nil
}

// Register should be called by any user (i.e., any pippet resource) before
// using the PippetRunner functions on this receiver object. Register implicitly
// takes care of calling Init if required.
func (obj *pippetReceiver) Register() error {
	obj.registerMutex.Lock()
	defer obj.registerMutex.Unlock()
	obj.registered = obj.registered + 1
	if obj.registered > 1 {
		return nil
	}
	// count was increased from 0 to 1, we need to (re-)init
	var err error
	if err = obj.Init(); err != nil {
		obj.registered = 0
	}
	return err
}

// Unregister should be called by any object that registered itself using the
// Register function, and which no longer needs the receiver. This should
// typically happen at closing time of the pippet resource that registered
// itself. Unregister implicitly calls Close in case all registered resources
// have unregistered.
func (obj *pippetReceiver) Unregister() error {
	obj.registerMutex.Lock()
	defer obj.registerMutex.Unlock()
	obj.registered = obj.registered - 1
	if obj.registered == 0 {
		return obj.Close()
	}
	if obj.registered < 0 {
		return fmt.Errorf("pippet runner: ERROR: unregistered more resources than were registered")
	}
	return nil
}

// LockApply locks the pippetReceiver's mutex for an "Apply" transaction.
func (obj *pippetReceiver) LockApply() {
	obj.applyMutex.Lock()
}

// UnlockApply unlocks the pippetReceiver's mutex for an "Apply" transaction.
func (obj *pippetReceiver) UnlockApply() {
	obj.applyMutex.Unlock()
}

// InputStream returns the pippetReceiver's pipe writer.
func (obj *pippetReceiver) InputStream() io.WriteCloser {
	return obj.stdin
}

// OutputStream returns the pippetReceiver's pipe reader.
func (obj *pippetReceiver) OutputStream() io.ReadCloser {
	return obj.stdout
}

// Close stops the backend puppet process by closing its stdin handle. It should
// not be called directly. It is implicitly called by the Unregister function if
// appropriate.
func (obj *pippetReceiver) Close() error {
	return obj.stdin.Close()
}

// applyPippetRes does the actual work of making Puppet synchronize a resource.
func applyPippetRes(runner PippetRunner, resource *PippetRes) (bool, error) {
	runner.LockApply()
	defer runner.UnlockApply()
	if err := json.NewEncoder(runner.InputStream()).Encode(resource); err != nil {
		return false, errwrap.Wrapf(err, "failed to send resource to puppet")
	}

	result := PippetResult{
		Error:     true,
		Exception: "missing output fields",
	}
	if err := json.NewDecoder(runner.OutputStream()).Decode(&result); err != nil {
		return false, errwrap.Wrapf(err, "failed to read response from puppet")
	}

	if result.Error {
		return false, fmt.Errorf("puppet did not sync: %s", result.Exception)
	}
	if result.Failed {
		return false, fmt.Errorf("puppet failed to sync")
	}
	return result.Changed, nil
}
