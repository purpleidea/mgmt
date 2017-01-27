// libmgmt example of send->recv
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

	"golang.org/x/time/rate"
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
		return fmt.Errorf("Already initialized!")
	}
	if obj.Name == "" {
		return fmt.Errorf("The graph name must be specified!")
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

	g := pgraph.NewGraph(obj.Name)

	// FIXME: these are being specified temporarily until it's the default!
	metaparams := resources.MetaParams{
		Limit: rate.Inf,
		Burst: 0,
	}

	content := "Delete me to trigger a notification!\n"
	f0 := &resources.FileRes{
		BaseRes: resources.BaseRes{
			Name:       "README",
			MetaParams: metaparams,
		},
		Path:    "/tmp/mgmt/README",
		Content: &content,
		State:   "present",
	}

	v0 := pgraph.NewVertex(f0)
	g.AddVertex(v0)

	p1 := &resources.PasswordRes{
		BaseRes: resources.BaseRes{
			Name:       "password1",
			MetaParams: metaparams,
		},
		Length: 8,    // generated string will have this many characters
		Saved:  true, // this causes passwords to be stored in plain text!
	}
	v1 := pgraph.NewVertex(p1)
	g.AddVertex(v1)

	f1 := &resources.FileRes{
		BaseRes: resources.BaseRes{
			Name:       "file1",
			MetaParams: metaparams,
			// send->recv!
			Recv: map[string]*resources.Send{
				"Content": {Res: p1, Key: "Password"},
			},
		},
		Path: "/tmp/mgmt/secret",
		//Content:  p1.Password, // won't work
		State: "present",
	}

	v2 := pgraph.NewVertex(f1)
	g.AddVertex(v2)

	n1 := &resources.NoopRes{
		BaseRes: resources.BaseRes{
			Name:       "noop1",
			MetaParams: metaparams,
		},
	}

	v3 := pgraph.NewVertex(n1)
	g.AddVertex(v3)

	e0 := pgraph.NewEdge("e0")
	e0.Notify = true // send a notification from v0 to v1
	g.AddEdge(v0, v1, e0)

	g.AddEdge(v1, v2, pgraph.NewEdge("e1"))

	e2 := pgraph.NewEdge("e2")
	e2.Notify = true // send a notification from v2 to v3
	g.AddEdge(v2, v3, e2)

	//g, err := config.NewGraphFromConfig(obj.data.Hostname, obj.data.World, obj.data.Noop)
	return g, nil
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
				ch <- nil // trigger a run
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
	obj.TmpPrefix = true    // disable for easy debugging
	//prefix := "/tmp/testprefix/"
	//obj.Prefix = &p // enable for easy debugging
	obj.IdealClusterSize = -1
	obj.ConvergedTimeout = -1
	obj.Noop = false // FIXME: careful!

	obj.GAPI = &MyGAPI{ // graph API
		Name:     "libmgmt", // TODO: set on compilation
		Interval: 60 * 10,   // arbitrarily change graph every 15 seconds
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
			obj.Exit(fmt.Errorf("Killed by %v", sig))
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
