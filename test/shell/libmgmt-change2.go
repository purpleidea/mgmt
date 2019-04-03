package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync"
	"time"

	"github.com/purpleidea/mgmt/engine"
	"github.com/purpleidea/mgmt/gapi"
	mgmt "github.com/purpleidea/mgmt/lib"
	"github.com/purpleidea/mgmt/pgraph"

	"golang.org/x/sys/unix"
)

// MyGAPI implements the main GAPI interface.
type MyGAPI struct {
	Name     string // graph name
	Interval uint   // refresh interval, 0 to never refresh

	flipflop  bool // flip flop
	autoGroup bool

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

	//obj.autoGroup = false // XXX: no panic
	obj.autoGroup = true // XXX: causes panic!

	obj.data = data // store for later
	obj.closeChan = make(chan struct{})
	obj.initialized = true
	return nil
}

// Graph returns a current Graph.
func (obj *MyGAPI) Graph() (*pgraph.Graph, error) {
	if !obj.initialized {
		return nil, fmt.Errorf("%s: MyGAPI is not initialized", obj.Name)
	}

	var err error
	g, err := pgraph.NewGraph(obj.Name)
	if err != nil {
		return nil, err
	}

	if !obj.flipflop {
		n0, err := engine.NewNamedResource("noop", "noop0")
		if err != nil {
			return nil, err
		}
		g.AddVertex(n0)

	} else {
		// NOTE: these will get autogrouped
		n1, err := engine.NewNamedResource("noop", "noop1")
		if err != nil {
			return nil, err
		}
		n1.Meta().AutoGroup = obj.autoGroup // enable or disable it
		g.AddVertex(n1)

		n2, err := engine.NewNamedResource("noop", "noop2")
		if err != nil {
			return nil, err
		}
		n2.Meta().AutoGroup = obj.autoGroup // enable or disable it
		g.AddVertex(n2)
	}
	obj.flipflop = !obj.flipflop // flip the bool

	//g, err := config.NewGraphFromConfig(obj.data.Hostname, obj.data.World, obj.data.Noop)
	return g, nil
}

// Next returns nil errors every time there could be a new graph.
func (obj *MyGAPI) Next() chan gapi.Next {
	ch := make(chan gapi.Next)
	obj.wg.Add(1)
	go func() {
		defer obj.wg.Done()
		defer close(ch) // this will run before the obj.wg.Done()
		if !obj.initialized {
			next := gapi.Next{
				Err:  fmt.Errorf("%s: MyGAPI is not initialized", obj.Name),
				Exit: true, // exit, b/c programming error?
			}
			ch <- next
			return
		}

		log.Printf("%s: Generating a bunch of new graphs...", obj.Name)
		ch <- gapi.Next{}
		log.Printf("%s: Second generation...", obj.Name)
		ch <- gapi.Next{}
		log.Printf("%s: Third generation...", obj.Name)
		ch <- gapi.Next{}
		time.Sleep(1 * time.Second)
		log.Printf("%s: Done generating graphs!", obj.Name)
	}()
	return ch
}

// Close shuts down the MyGAPI.
func (obj *MyGAPI) Close() error {
	if !obj.initialized {
		return fmt.Errorf("%s: MyGAPI is not initialized", obj.Name)
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
	obj.ConvergedTimeout = 5
	obj.Noop = false // does stuff!

	obj.GAPI = &MyGAPI{ // graph API
		Name:     obj.Program, // graph name
		Interval: 15,          // arbitrarily change graph every 15 seconds
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
		signal.Notify(signals, unix.SIGTERM)

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

	return obj.Run()
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
