// libmgmt example
package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/purpleidea/mgmt/gapi"
	mgmt "github.com/purpleidea/mgmt/lib"
	"github.com/purpleidea/mgmt/pgraph"
	"github.com/purpleidea/mgmt/resources"
	"github.com/purpleidea/mgmt/yamlgraph"
)

// MyGAPI implements the main GAPI interface.
type MyGAPI struct {
	Name     string // graph name
	Interval uint   // refresh interval, 0 to never refresh

	data        gapi.Data
	initialized bool
	closeChan   chan struct{}
	wg          sync.WaitGroup // sync group for tunnel go routines
}

// NewMyGAPI creates a new MyGAPI struct and calls Init().
func NewMyGAPI(data gapi.Data, name string, interval uint) (*MyGAPI, error) {
	obj := &MyGAPI{
		Name:     name,
		Interval: interval,
	}
	return obj, obj.Init(data)
}

// Init initializes the MyGAPI struct.
func (obj *MyGAPI) Init(data gapi.Data) error {
	if obj.initialized {
		return fmt.Errorf("already initialized")
	}
	if obj.Name == "" {
		return fmt.Errorf("the graph name must be specified")
	}
	obj.data = data // store for later
	obj.closeChan = make(chan struct{})
	obj.initialized = true
	return nil
}

// Graph returns a current Graph.
func (obj *MyGAPI) Graph() (*pgraph.Graph, error) {
	if !obj.initialized {
		return nil, fmt.Errorf("libmgmt: MyGAPI is not initialized")
	}

	n1, err := resources.NewNoopRes("noop1")
	if err != nil {
		return nil, fmt.Errorf("can't create resource: %v", err)
	}

	// we can still build a graph via the yaml method
	gc := &yamlgraph.GraphConfig{
		Graph: obj.Name,
		Resources: yamlgraph.Resources{ // must redefine anonymous struct :(
			// in alphabetical order
			Exec:  []*resources.ExecRes{},
			File:  []*resources.FileRes{},
			Msg:   []*resources.MsgRes{},
			Noop:  []*resources.NoopRes{n1},
			Pkg:   []*resources.PkgRes{},
			Svc:   []*resources.SvcRes{},
			Timer: []*resources.TimerRes{},
			Virt:  []*resources.VirtRes{},
		},
		//Collector: []collectorResConfig{},
		//Edges:     []Edge{},
		Comment: "comment!",
	}

	g, err := gc.NewGraphFromConfig(obj.data.Hostname, obj.data.World, obj.data.Noop)
	return g, err
}

// Next returns nil errors every time there could be a new graph.
func (obj *MyGAPI) Next() chan error {
	if obj.data.NoWatch || obj.Interval <= 0 {
		return nil
	}
	ch := make(chan error)
	obj.wg.Add(1)
	go func() {
		defer obj.wg.Done()
		defer close(ch) // this will run before the obj.wg.Done()
		if !obj.initialized {
			ch <- fmt.Errorf("libmgmt: MyGAPI is not initialized")
			return
		}

		// arbitrarily change graph every interval seconds
		ticker := time.NewTicker(time.Duration(obj.Interval) * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				log.Printf("libmgmt: Generating new graph...")
				select {
				case ch <- nil: // trigger a run
				case <-obj.closeChan:
					return
				}
			case <-obj.closeChan:
				return
			}
		}
	}()
	return ch
}

// Close shuts down the MyGAPI.
func (obj *MyGAPI) Close() error {
	if !obj.initialized {
		return fmt.Errorf("libmgmt: MyGAPI is not initialized")
	}
	close(obj.closeChan)
	obj.wg.Wait()
	obj.initialized = false // closed = true
	return nil
}

// Run runs an embedded mgmt server.
func Run() error {

	obj := &mgmt.Main{}
	obj.Program = "libmgmt" // TODO: set on compilation
	obj.Version = "0.0.1"   // TODO: set on compilation
	obj.TmpPrefix = true
	obj.IdealClusterSize = -1
	obj.ConvergedTimeout = -1
	obj.Noop = true

	obj.GAPI = &MyGAPI{ // graph API
		Name:     "libmgmt", // TODO: set on compilation
		Interval: 15,        // arbitrarily change graph every 15 seconds
	}

	if err := obj.Init(); err != nil {
		return err
	}

	// install the exit signal handler
	exit := make(chan struct{})
	defer close(exit)
	go func() {
		signals := make(chan os.Signal, 1)
		signal.Notify(signals, os.Interrupt) // catch ^C
		//signal.Notify(signals, os.Kill) // catch signals
		signal.Notify(signals, syscall.SIGTERM)

		select {
		case sig := <-signals: // any signal will do
			if sig == os.Interrupt {
				log.Println("Interrupted by ^C")
				obj.Exit(nil)
				return
			}
			log.Println("Interrupted by signal")
			obj.Exit(fmt.Errorf("killed by %v", sig))
			return
		case <-exit:
			return
		}
	}()

	if err := obj.Run(); err != nil {
		return err
	}
	return nil
}

func main() {
	log.Printf("Hello!")
	if err := Run(); err != nil {
		fmt.Println(err)
		os.Exit(1)
		return
	}
	log.Printf("Goodbye!")
}
