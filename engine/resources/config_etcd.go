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
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/purpleidea/mgmt/engine"
	"github.com/purpleidea/mgmt/engine/traits"
	"github.com/purpleidea/mgmt/util"
	"github.com/purpleidea/mgmt/util/errwrap"
)

func init() {
	engine.RegisterResource("config:etcd", func() engine.Res { return &ConfigEtcdRes{} })
}

const (
	sizeCheckApplyTimeout = 5 * time.Second
)

// ConfigEtcdRes is a resource that sets mgmt's etcd configuration.
type ConfigEtcdRes struct {
	traits.Base // add the base methods without re-implementation

	init *engine.Init

	// IdealClusterSize is the requested minimum size of the cluster. If you
	// set this to zero, it will cause a cluster wide shutdown if
	// AllowSizeShutdown is true. If it's not true, then it will cause a
	// validation error.
	IdealClusterSize uint16 `lang:"idealclustersize"`
	// AllowSizeShutdown is a required safety flag that you must set to true
	// if you want to allow causing a cluster shutdown by setting
	// IdealClusterSize to zero.
	AllowSizeShutdown bool `lang:"allow_size_shutdown"`

	// sizeFlag determines whether sizeCheckApply already ran or not.
	sizeFlag bool

	interruptChan chan struct{}
	wg            *sync.WaitGroup
}

// Default returns some sensible defaults for this resource.
func (obj *ConfigEtcdRes) Default() engine.Res {
	return &ConfigEtcdRes{}
}

// Validate if the params passed in are valid data.
func (obj *ConfigEtcdRes) Validate() error {
	if obj.IdealClusterSize < 0 {
		return fmt.Errorf("the IdealClusterSize param must be positive")
	}

	if obj.IdealClusterSize == 0 && !obj.AllowSizeShutdown {
		return fmt.Errorf("the IdealClusterSize can't be zero if AllowSizeShutdown is false")
	}

	return nil
}

// Init runs some startup code for this resource.
func (obj *ConfigEtcdRes) Init(init *engine.Init) error {
	obj.init = init // save for later

	obj.interruptChan = make(chan struct{})
	obj.wg = &sync.WaitGroup{}

	return nil
}

// Close is run by the engine to clean up after the resource is done.
func (obj *ConfigEtcdRes) Close() error {
	obj.wg.Wait() // bonus
	return nil
}

// Watch is the primary listener for this resource and it outputs events.
func (obj *ConfigEtcdRes) Watch() error {
	obj.wg.Add(1)
	defer obj.wg.Done()
	// FIXME: add timeout to context
	// The obj.init.Done channel is closed by the engine to signal shutdown.
	ctx, cancel := util.ContextWithCloser(context.Background(), obj.init.Done)
	defer cancel()
	ch, err := obj.init.World.IdealClusterSizeWatch(util.CtxWithWg(ctx, obj.wg))
	if err != nil {
		return errwrap.Wrapf(err, "could not watch ideal cluster size")
	}

	obj.init.Running() // when started, notify engine that we're running

Loop:
	for {
		select {
		case event, ok := <-ch:
			if !ok {
				break Loop
			}
			if obj.init.Debug {
				obj.init.Logf("event: %+v", event)
			}
			// pass through and send an event

		case <-obj.init.Done: // closed by the engine to signal shutdown
		}

		obj.init.Event() // notify engine of an event (this can block)
	}

	return nil
}

// sizeCheckApply sets the IdealClusterSize parameter. If it sees a value change
// to zero, then it *won't* try and change it away from zero, because it assumes
// that someone has requested a shutdown. If the value is seen on first startup,
// then it will change it, because it might be a zero from the previous cluster.
func (obj *ConfigEtcdRes) sizeCheckApply(apply bool) (bool, error) {
	wg := &sync.WaitGroup{}
	defer wg.Wait() // this must be above the defer cancel() call
	ctx, cancel := context.WithTimeout(context.Background(), sizeCheckApplyTimeout)
	defer cancel()
	wg.Add(1)
	go func() {
		defer wg.Done()
		select {
		case <-obj.interruptChan:
			cancel()
		case <-ctx.Done():
			// let this exit
		}
	}()

	val, err := obj.init.World.IdealClusterSizeGet(ctx)
	if err != nil {
		return false, errwrap.Wrapf(err, "could not get ideal cluster size")
	}

	// if we got a value of zero, and we've already run before, then it's ok
	if obj.IdealClusterSize != 0 && val == 0 && obj.sizeFlag {
		obj.init.Logf("impending cluster shutdown, not setting ideal cluster size")
		return true, nil // impending shutdown, don't try and cancel it.
	}
	obj.sizeFlag = true

	// must be done after setting the above flag
	if obj.IdealClusterSize == val { // state is correct
		return true, nil
	}

	if !apply {
		return false, nil
	}

	// set!
	// This is run as a transaction so we detect if we needed to change it.
	changed, err := obj.init.World.IdealClusterSizeSet(ctx, obj.IdealClusterSize)
	if err != nil {
		return false, errwrap.Wrapf(err, "could not set ideal cluster size")
	}
	if !changed {
		return true, nil // we lost a race, which means no change needed
	}
	obj.init.Logf("set dynamic cluster size to: %d", obj.IdealClusterSize)

	return false, nil
}

// CheckApply method for Noop resource. Does nothing, returns happy!
func (obj *ConfigEtcdRes) CheckApply(apply bool) (bool, error) {
	checkOK := true

	if c, err := obj.sizeCheckApply(apply); err != nil {
		return false, err
	} else if !c {
		checkOK = false
	}

	// TODO: add more config settings management here...
	//if c, err := obj.TODOCheckApply(apply); err != nil {
	//	return false, err
	//} else if !c {
	//	checkOK = false
	//}

	return checkOK, nil // w00t
}

// Cmp compares two resources and returns an error if they are not equivalent.
func (obj *ConfigEtcdRes) Cmp(r engine.Res) error {
	// we can only compare ConfigEtcdRes to others of the same resource kind
	res, ok := r.(*ConfigEtcdRes)
	if !ok {
		return fmt.Errorf("not a %s", obj.Kind())
	}

	if obj.IdealClusterSize != res.IdealClusterSize {
		return fmt.Errorf("the IdealClusterSize param differs")
	}
	if obj.AllowSizeShutdown != res.AllowSizeShutdown {
		return fmt.Errorf("the AllowSizeShutdown param differs")
	}

	return nil
}

// Interrupt is called to ask the execution of this resource to end early.
func (obj *ConfigEtcdRes) Interrupt() error {
	close(obj.interruptChan)
	return nil
}

// UnmarshalYAML is the custom unmarshal handler for this struct.
// It is primarily useful for setting the defaults.
func (obj *ConfigEtcdRes) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type rawRes ConfigEtcdRes // indirection to avoid infinite recursion

	def := obj.Default()            // get the default
	res, ok := def.(*ConfigEtcdRes) // put in the right format
	if !ok {
		return fmt.Errorf("could not convert to ConfigEtcdRes")
	}
	raw := rawRes(*res) // convert; the defaults go here

	if err := unmarshal(&raw); err != nil {
		return err
	}

	*obj = ConfigEtcdRes(raw) // restore from indirection with type conversion!
	return nil
}
