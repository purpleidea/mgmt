// Mgmt
// Copyright (C) 2013-2021+ James Shubin and the project contributors
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
	"net/url"
	"sync"

	"github.com/purpleidea/mgmt/engine"
	"github.com/purpleidea/mgmt/engine/traits"
	"github.com/purpleidea/mgmt/util"
	"github.com/purpleidea/mgmt/util/errwrap"

	"github.com/hashicorp/consul/api"
)

func init() {
	engine.RegisterResource("consul:kv", func() engine.Res { return &ConsulKVRes{} })
}

// ConsulKVRes is a resource that writes a value into a Consul datastore. The
// name of the resource can either be the key name, or the concatenation of the
// server address and the key name: http://127.0.0.1:8500/my-key. If the param
// keys are specified, then those are used. If the Name cannot be properly
// parsed by url.Parse, then it will be considered as the Key's value. If the
// Key is specified explicitly, then we won't use anything from the Name.
type ConsulKVRes struct {
	traits.Base
	init *engine.Init

	// Key is the name of the key. Defaults to the name of the resource.
	Key string `lang:"key" yaml:"key"`

	// Value is the value for the key.
	Value string `lang:"value" yaml:"value"`

	// Scheme is the URI scheme for the Consul server. Default: http.
	Scheme string `lang:"scheme" yaml:"scheme"`

	// Address is the address of the Consul server. Default: 127.0.0.1:8500.
	Address string `lang:"address" yaml:"address"`

	// Token is used to provide an ACL token to use for this resource.
	Token string `lang:"token" yaml:"token"`

	client *api.Client
	config *api.Config // needed to close the idle connections
	once   bool        // safety token
	key    string      // cache the key name to avoid re-running the parser
}

// Default returns some sensible defaults for this resource.
func (obj *ConsulKVRes) Default() engine.Res {
	return &ConsulKVRes{}
}

// Validate if the params passed in are valid data.
func (obj *ConsulKVRes) Validate() error {
	s, _, k := obj.inputParser()
	if k == "" {
		return fmt.Errorf("the Key is empty")
	}
	if s != "" && s != "http" && s != "https" {
		return fmt.Errorf("unknown Scheme")
	}
	return nil
}

// Init runs some startup code for this resource.
func (obj *ConsulKVRes) Init(init *engine.Init) error {
	obj.init = init // save for later

	s, a, k := obj.inputParser()

	obj.config = api.DefaultConfig()
	if s != "" {
		obj.config.Scheme = s
	}
	if a != "" {
		obj.config.Address = obj.Address
	}
	obj.key = k // store the key
	obj.init.Logf("using consul key: %s", obj.key)

	if obj.Token != "" {
		obj.config.Token = obj.Token
	}

	var err error
	obj.client, err = api.NewClient(obj.config)
	return errwrap.Wrapf(err, "could not create Consul client")
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
	exit := make(chan struct{})

	kv := obj.client.KV()

	wg.Add(1)
	go func() {
		defer close(ch)
		defer wg.Done()

		opts := &api.QueryOptions{RequireConsistent: true}
		ctx, cancel := util.ContextWithCloser(context.Background(), exit)
		defer cancel()
		opts = opts.WithContext(ctx)

		for {
			_, meta, err := kv.Get(obj.key, opts)
			select {
			case ch <- err: // send
				if err != nil {
					return
				}

				// WaitIndex = 0, which means that it is the
				// first time we run the query, as we are about
				// to change the WaitIndex to make a blocking
				// query, we can consider the watch started.
				opts.WaitIndex = meta.LastIndex
				if opts.WaitIndex != 0 {
					continue
				}

				if !obj.once {
					obj.init.Running()
					obj.once = true
					continue
				}

				// Unexpected situation, bug in consul API...
				select {
				case ch <- fmt.Errorf("unexpected behaviour in Consul API"):
				case <-obj.init.Done: // signal for shutdown request
				}

			case <-obj.init.Done: // signal for shutdown request
			}
			return
		}
	}()

	defer close(exit)
	for {
		select {
		case err, ok := <-ch:
			if !ok { // channel shutdown
				return nil
			}
			if err != nil {
				return errwrap.Wrapf(err, "unknown %s watcher error", obj)
			}
			if obj.init.Debug {
				obj.init.Logf("event!")
			}
			obj.init.Event()

		case <-obj.init.Done: // signal for shutdown request
			return nil
		}
	}
}

// CheckApply is run to check the state and, if apply is true, to apply the
// necessary changes to reach the desired state. This is run before Watch and
// again if Watch finds a change occurring to the state.
func (obj *ConsulKVRes) CheckApply(apply bool) (bool, error) {
	if obj.init.Debug {
		obj.init.Logf("consul key: %s", obj.key)
	}
	kv := obj.client.KV()
	pair, _, err := kv.Get(obj.key, nil)
	if err != nil {
		return false, err
	}

	if pair != nil && string(pair.Value) == obj.Value {
		return true, nil
	}

	if !apply {
		return false, nil
	}

	p := &api.KVPair{Key: obj.key, Value: []byte(obj.Value)}
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
	if obj.Scheme != res.Scheme {
		return fmt.Errorf("the Scheme param differs")
	}
	if obj.Address != res.Address {
		return fmt.Errorf("the Address param differs")
	}
	if obj.Token != res.Token {
		return fmt.Errorf("the Token param differs")
	}

	return nil
}

// inputParser parses the Name() of a resource and extracts the scheme, address,
// and key name of a consul key. We don't have an error, because if we have one,
// then it means the input must be a raw key. Output of this function is scheme,
// address (includes hostname and port), and key. This also takes our parameters
// in to account, and applies the correct overrides if they are specified there.
func (obj *ConsulKVRes) inputParser() (string, string, string) {
	// If the key is specified explicitly, then we're not going to parse the
	// resource name for a pattern, and we use our given params as they are.
	if obj.Key != "" {
		return obj.Scheme, obj.Address, obj.Key
	}

	// Now we parse...
	u, err := url.Parse(obj.Name())
	if err != nil {
		// If this didn't work, then we know it's explicitly a raw key.
		return obj.Scheme, obj.Address, obj.Name()
	}

	// Otherwise, we use the parse result, and we overwrite any of the
	// fields if we have an explicit param that was specified.
	k := u.Path
	s := u.Scheme
	a := u.Host

	//if obj.Key != "" { // this is now guaranteed to never happen
	//	k = obj.Key
	//}
	if obj.Scheme != "" {
		s = obj.Scheme
	}
	if obj.Address != "" {
		a = obj.Address
	}

	return s, a, k
}
