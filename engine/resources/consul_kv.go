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

	"github.com/hashicorp/consul/api"
	"github.com/purpleidea/mgmt/engine"
	"github.com/purpleidea/mgmt/engine/traits"
	"github.com/purpleidea/mgmt/util"
	"github.com/purpleidea/mgmt/util/errwrap"
)

func init() {
	engine.RegisterResource("consul:kv", func() engine.Res { return &ConsulKVRes{} })
}

// ConsulKVRes is a resource that writes a value into a Consul datastore.
type ConsulKVRes struct {
	traits.Base
	init *engine.Init

	// Key is the name of the key. Defaults to the name of the resource.
	Key string `lang:"key" yaml:"key"`

	// Value is the value for the key.
	Value string `lang:"value" yaml:"value"`

	// cheme is the URI scheme for the Consul server. Default: http.
	Scheme string `lang:"scheme" yaml:"scheme"`

	// Address is the address of the Consul server. Default: 127.0.0.1:8500.
	Address string `lang:"address" yaml:"address"`

	// Token is used to provide an ACL token to use for this resource.
	Token string `lang:"token" yaml:"token"`

	client *api.Client
	config *api.Config // needed to close the idle connections
}

// Default returns some sensible defaults for this resource.
func (obj *ConsulKVRes) Default() engine.Res {
	return &ConsulKVRes{}
}

// Validate if the params passed in are valid data.
func (obj *ConsulKVRes) Validate() error {
	if obj.Scheme != "" && obj.Scheme != "http" && obj.Scheme != "https" {
		return fmt.Errorf("unknown Scheme")
	}
	return nil
}

// Init runs some startup code for this resource.
func (obj *ConsulKVRes) Init(init *engine.Init) error {
	obj.init = init // save for later

	// set the Key as the resource name if a specific key is not set
	if obj.Key == "" {
		obj.Key = obj.Name()
	}

	obj.config = api.DefaultConfig()
	if obj.Address != "" {
		obj.config.Address = obj.Address
	}
	if obj.Scheme != "" {
		obj.config.Scheme = obj.Scheme
	}
	if obj.Token != "" {
		obj.config.Token = obj.Token
	}

	var err error
	obj.client, err = api.NewClient(obj.config)
	return err
}

// Close is run by the engine to clean up after the resource is done.
func (obj *ConsulKVRes) Close() error {
	if obj.config != nil && obj.config.Transport != nil {
		obj.config.Transport.CloseIdleConnections()
	}
	return nil
}

// Watch is the listener and main loop for this resource and it outputs events.
func (obj *ConsulKVRes) Watch() error {
	wg := &sync.WaitGroup{}
	defer wg.Wait()

	ch := make(chan error)
	kv := obj.client.KV()

	wg.Add(1)
	go func() {
		defer wg.Done()

		opts := &api.QueryOptions{RequireConsistent: true}
		ctx, cancel := util.ContextWithCloser(context.Background(), obj.init.Done)
		defer cancel()
		opts = opts.WithContext(ctx)

		for {
			_, meta, err := kv.Get(obj.Key, opts)
			ch <- err
			if err != nil {
				return
			}
			if opts.WaitIndex == 0 {
				// WaitIndex = 0, which means that it is the first time we
				// run the query. as we are about to change the WaitIndex to
				// make a blocking query, we can consider the watch started.
				obj.init.Running()
			}
			opts.WaitIndex = meta.LastIndex
		}
	}()

	for {
		if err := <-ch; err != nil {
			return errwrap.Wrapf(err, "error while polling Consul")
		}
		obj.init.Event()
	}
}

// CheckApply is run to check the state and, if apply is true, to apply the
// necessary changes to reach the desired state. This is run before Watch and
// again if Watch finds a change occurring to the state.
func (obj *ConsulKVRes) CheckApply(apply bool) (bool, error) {
	kv := obj.client.KV()
	pair, _, err := kv.Get(obj.Key, nil)
	if err != nil {
		return false, err
	}

	if pair != nil && string(pair.Value) == obj.Value {
		return true, nil
	}

	if !apply {
		return false, nil
	}

	p := &api.KVPair{Key: obj.Key, Value: []byte(obj.Value)}
	_, err = kv.Put(p, nil)
	return false, err
}

// Cmp compares two resources and return if they are equivalent.
func (obj *ConsulKVRes) Cmp(r engine.Res) error {
	res, ok := r.(*ConsulKVRes)
	if !ok {
		return fmt.Errorf("not a %s", obj.Kind())
	}

	if obj.Key != res.Key {
		return fmt.Errorf("the Key param differs")
	}
	if obj.Value != res.Value {
		return fmt.Errorf("the Value param differs")
	}
	if obj.Token != res.Token {
		return fmt.Errorf("the Token param differs")
	}
	if obj.Address != res.Address {
		return fmt.Errorf("the Address param differs")
	}

	return nil
}
