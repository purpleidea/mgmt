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

package main

import (
	"fmt"
	etcd_context "github.com/coreos/etcd/Godeps/_workspace/src/golang.org/x/net/context"
	etcd "github.com/coreos/etcd/client"
	"log"
	"math"
	"strings"
	"time"
)

//go:generate stringer -type=etcdMsg -output=etcdmsg_stringer.go
type etcdMsg int

const (
	etcdStart etcdMsg = iota
	etcdEvent
	etcdFoo
	etcdBar
)

//go:generate stringer -type=etcdState -output=etcdstate_stringer.go
type etcdState int

const (
	etcdNil etcdState = iota
	//etcdConverged
	etcdConvergedTimeout
)

type EtcdWObject struct { // etcd wrapper object
	seed      string
	ctimeout  int
	converged chan bool
	kapi      etcd.KeysAPI
	state     etcdState
}

func (obj *EtcdWObject) GetState() etcdState {
	return obj.state
}

func (obj *EtcdWObject) SetState(state etcdState) {
	obj.state = state
}

func (etcdO *EtcdWObject) GetKAPI() etcd.KeysAPI {
	if etcdO.kapi != nil { // memoize
		return etcdO.kapi
	}

	cfg := etcd.Config{
		Endpoints: []string{etcdO.seed},
		Transport: etcd.DefaultTransport,
		// set timeout per request to fail fast when the target endpoint is unavailable
		HeaderTimeoutPerRequest: time.Second,
	}

	var c etcd.Client
	var err error

	c, err = etcd.New(cfg)
	if err != nil {
		// XXX: not sure if this ever errors
		if cerr, ok := err.(*etcd.ClusterError); ok {
			// XXX: not sure if this part ever matches
			// not running or disconnected
			if cerr == etcd.ErrClusterUnavailable {
				log.Fatal("XXX: etcd: ErrClusterUnavailable")
			} else {
				log.Fatal("XXX: etcd: Unknown")
			}
		}
		log.Fatal(err) // some unhandled error
	}
	etcdO.kapi = etcd.NewKeysAPI(c)
	return etcdO.kapi
}

type EtcdChannelWatchResponse struct {
	resp *etcd.Response
	err  error
}

// wrap the etcd watcher.Next blocking function inside of a channel
func (etcdO *EtcdWObject) EtcdChannelWatch(watcher etcd.Watcher, context etcd_context.Context) chan *EtcdChannelWatchResponse {
	ch := make(chan *EtcdChannelWatchResponse)
	go func() {
		for {
			resp, err := watcher.Next(context) // blocks here
			ch <- &EtcdChannelWatchResponse{resp, err}
		}
	}()
	return ch
}

func (etcdO *EtcdWObject) EtcdWatch() chan etcdMsg {
	kapi := etcdO.GetKAPI()
	ctimeout := etcdO.ctimeout
	converged := etcdO.converged
	// XXX: i think we need this buffered so that when we're hanging on the
	// channel, which is inside the EtcdWatch main loop, we still want the
	// calls to Get/Set on etcd to succeed, so blocking them here would
	// kill the whole thing
	ch := make(chan etcdMsg, 1) // XXX: buffer of at least 1 is required
	go func(ch chan etcdMsg) {
		tmin := 500   // initial (min) delay in ms
		t := tmin     // current time
		tmult := 2    // multiplier for exponential delay
		tmax := 16000 // max delay
		watcher := kapi.Watcher("/exported/", &etcd.WatcherOptions{Recursive: true})
		etcdch := etcdO.EtcdChannelWatch(watcher, etcd_context.Background())
		for {
			log.Printf("Etcd: Watching...")
			var resp *etcd.Response = nil
			var err error = nil
			select {
			case out := <-etcdch:
				etcdO.SetState(etcdNil)
				resp, err = out.resp, out.err

			case _ = <-TimeAfterOrBlock(ctimeout):
				etcdO.SetState(etcdConvergedTimeout)
				converged <- true
				continue
			}

			if err != nil {
				if err == etcd_context.Canceled {
					// ctx is canceled by another routine
					log.Fatal("Canceled")
				} else if err == etcd_context.DeadlineExceeded {
					// ctx is attached with a deadline and it exceeded
					log.Fatal("Deadline")
				} else if cerr, ok := err.(*etcd.ClusterError); ok {
					// not running or disconnected
					// TODO: is there a better way to parse errors?
					for _, e := range cerr.Errors {
						if strings.HasSuffix(e.Error(), "getsockopt: connection refused") {
							t = int(math.Min(float64(t*tmult), float64(tmax)))
							log.Printf("Etcd: Waiting %d ms for connection...", t)
							time.Sleep(time.Duration(t) * time.Millisecond) // sleep for t ms

						} else if e.Error() == "unexpected EOF" {
							log.Printf("Etcd: Unexpected disconnect...")

						} else if e.Error() == "EOF" {
							log.Printf("Etcd: Disconnected...")

						} else if strings.HasPrefix(e.Error(), "unsupported protocol scheme") {
							// usually a bad peer endpoint value
							log.Fatal("Bad peer endpoint value?")

						} else {
							log.Fatal("Woops: ", e.Error())
						}
					}
				} else {
					// bad cluster endpoints, which are not etcd servers
					log.Fatal("Woops: ", err)
				}
			} else {
				//log.Print(resp)
				//log.Printf("Watcher().Node.Value(%v): %+v", key, resp.Node.Value)
				// FIXME: we should actually reset when the server comes back, not here on msg!
				//XXX: can we fix this with one of these patterns?: https://blog.golang.org/go-concurrency-patterns-timing-out-and
				t = tmin // reset timer

				// don't trigger event if nothing changed
				if n, p := resp.Node, resp.PrevNode; resp.Action == "set" && p != nil {
					if n.Key == p.Key && n.Value == p.Value {
						continue
					}
				}

				// FIXME: we get events on key/type/value changes for
				// each type directory... ignore the non final ones...
				// IOW, ignore everything except for the value or some
				// field which gets set last... this could be the max count field thing...

				log.Printf("Etcd: Value: %v", resp.Node.Value) // event
				ch <- etcdEvent                                // event
			}

		} // end for loop
		//close(ch)
	}(ch) // call go routine
	return ch
}

// helper function to store our data in etcd
func (etcdO *EtcdWObject) EtcdPut(hostname, key, typ string, obj interface{}) bool {
	kapi := etcdO.GetKAPI()
	output, ok := ObjToB64(obj)
	if !ok {
		log.Printf("Etcd: Could not encode %v key.", key)
		return false
	}

	path := fmt.Sprintf("/exported/%s/types/%s/type", hostname, key)
	_, err := kapi.Set(etcd_context.Background(), path, typ, nil)
	// XXX validate...

	path = fmt.Sprintf("/exported/%s/types/%s/value", hostname, key)
	resp, err := kapi.Set(etcd_context.Background(), path, output, nil)
	if err != nil {
		if cerr, ok := err.(*etcd.ClusterError); ok {
			// not running or disconnected
			for _, e := range cerr.Errors {
				if strings.HasSuffix(e.Error(), "getsockopt: connection refused") {
				}
				//if e == etcd.ErrClusterUnavailable
			}
		}
		log.Printf("Etcd: Could not store %v key.", key)
		return false
	}
	log.Print("Etcd: ", resp) // w00t... bonus
	return true
}

// lookup /exported/ node hierarchy
func (etcdO *EtcdWObject) EtcdGet() (etcd.Nodes, bool) {
	kapi := etcdO.GetKAPI()
	// key structure is /exported/<hostname>/types/...
	resp, err := kapi.Get(etcd_context.Background(), "/exported/", &etcd.GetOptions{Recursive: true})
	if err != nil {
		return nil, false // not found
	}
	return resp.Node.Nodes, true
}

func (etcdO *EtcdWObject) EtcdGetProcess(nodes etcd.Nodes, typ string) []string {
	//path := fmt.Sprintf("/exported/%s/types/", h)
	top := "/exported/"
	log.Printf("Etcd: Get: %+v", nodes) // Get().Nodes.Nodes
	output := make([]string, 0)

	for _, x := range nodes { // loop through hosts
		if !strings.HasPrefix(x.Key, top) {
			log.Fatal("Error!")
		}
		host := x.Key[len(top):]
		//log.Printf("Get().Nodes[%v]: %+v ==> %+v", -1, host, x.Nodes)
		//log.Printf("Get().Nodes[%v]: %+v ==> %+v", i, x.Key, x.Nodes)
		types, ok := EtcdGetChildNodeByKey(x, "types")
		if !ok {
			continue
		}
		for _, y := range types.Nodes { // loop through types
			//key := y.Key # UUID?
			//log.Printf("Get(%v): TYPE[%v]", host, y.Key)
			t, ok := EtcdGetChildNodeByKey(y, "type")
			if !ok {
				continue
			}
			if typ != "" && typ != t.Value {
				continue
			} // filter based on type

			v, ok := EtcdGetChildNodeByKey(y, "value") // B64ToObj this
			if !ok {
				continue
			}
			log.Printf("Etcd: Hostname: %v; Get: %v", host, t.Value)

			output = append(output, v.Value)
		}
	}
	return output
}

// TODO: wrap this somehow so it's a method of *etcd.Node
// helper function that returns the node for a particular key under a node
func EtcdGetChildNodeByKey(node *etcd.Node, key string) (*etcd.Node, bool) {
	for _, x := range node.Nodes {
		if x.Key == fmt.Sprintf("%s/%s", node.Key, key) {
			return x, true
		}
	}
	return nil, false // not found
}
