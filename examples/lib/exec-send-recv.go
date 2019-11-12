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

	"github.com/purpleidea/mgmt/engine"
	"github.com/purpleidea/mgmt/engine/resources"
	"github.com/purpleidea/mgmt/gapi"
	mgmt "github.com/purpleidea/mgmt/lib"
	"github.com/purpleidea/mgmt/pgraph"

	"github.com/urfave/cli"
)

// XXX: this has not been updated to latest GAPI/Deploy API. Patches welcome!

const (
	// Name is the name of this frontend.
	Name = "libmgmt"
)

// MyGAPI implements the main GAPI interface.
type MyGAPI struct {
	Name     string // graph name
	Interval uint   // refresh interval, 0 to never refresh

	data        *gapi.Data
	initialized bool
	closeChan   chan struct{}
	wg          sync.WaitGroup // sync group for tunnel go routines
}

// NewMyGAPI creates a new MyGAPI struct and calls Init().
func NewMyGAPI(data *gapi.Data, name string, interval uint) (*MyGAPI, error) {
	obj := &MyGAPI{
		Name:     name,
		Interval: interval,
	}
	return obj, obj.Init(data)
}

// CliFlags returns a list of flags used by the passed in subcommand.
func (obj *MyGAPI) CliFlags(string) []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{
			Name:  obj.Name,
			Value: "",
			Usage: "run",
		},
	}
}

// Cli takes a cli.Context and some other info, and returns our GAPI. If there
// are any validation problems, you should return an error.
func (obj *MyGAPI) Cli(cliInfo *gapi.CliInfo) (*gapi.Deploy, error) {
	c := cliInfo.CliContext
	//fs := cliInfo.Fs // copy files from local filesystem *into* this fs...
	//debug := cliInfo.Debug
	//logf := func(format string, v ...interface{}) {
	//	cliInfo.Logf(Name+": "+format, v...)
	//}

	return &gapi.Deploy{
		Name: obj.Name,
		Noop: c.Bool("noop"),
		Sema: c.Int("sema"),
		GAPI: &MyGAPI{},
	}, nil
}

// Init initializes the MyGAPI struct.
func (obj *MyGAPI) Init(data *gapi.Data) error {
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
		return nil, fmt.Errorf("%s: MyGAPI is not initialized", Name)
	}

	g, err := pgraph.NewGraph(obj.Name)
	if err != nil {
		return nil, err
	}

	exec1 := &resources.ExecRes{
		Cmd:   "echo hello world && echo goodbye world 1>&2", // to stdout && stderr
		Shell: "/bin/bash",
	}
	g.AddVertex(exec1)

	output := &resources.FileRes{
		Path:  "/tmp/mgmt/output",
		State: "present",
	}
	// XXX: add send->recv!
	//Recv: map[string]*engine.Send{
	//	"Content": {Res: exec1, Key: "Output"},
	//},

	g.AddVertex(output)
	g.AddEdge(exec1, output, &engine.Edge{Name: "e0"})

	stdout := &resources.FileRes{
		Path:  "/tmp/mgmt/stdout",
		State: "present",
	}
	// XXX: add send->recv!
	//Recv: map[string]*engine.Send{
	//	"Content": {Res: exec1, Key: "Stdout"},
	//},
	g.AddVertex(stdout)
	g.AddEdge(exec1, stdout, &engine.Edge{Name: "e1"})

	stderr := &resources.FileRes{
		Path:  "/tmp/mgmt/stderr",
		State: "present",
	}
	// XXX: add send->recv!
	//Recv: map[string]*engine.Send{
	//	"Content": {Res: exec1, Key: "Stderr"},
	//},

	g.AddVertex(stderr)
	g.AddEdge(exec1, stderr, &engine.Edge{Name: "e2"})

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
				Err:  fmt.Errorf("%s: MyGAPI is not initialized", Name),
				Exit: true, // exit, b/c programming error?
			}
			ch <- next
			return
		}
		startChan := make(chan struct{}) // start signal
		close(startChan)                 // kick it off!

		ticker := make(<-chan time.Time)
		if obj.data.NoStreamWatch || obj.Interval <= 0 {
			ticker = nil
		} else {
			// arbitrarily change graph every interval seconds
			t := time.NewTicker(time.Duration(obj.Interval) * time.Second)
			defer t.Stop()
			ticker = t.C
		}
		for {
			select {
			case <-startChan: // kick the loop once at start
				startChan = nil // disable
				// pass
			case <-ticker:
				// pass
			case <-obj.closeChan:
				return
			}

			log.Printf("%s: Generating new graph...", Name)
			select {
			case ch <- gapi.Next{}: // trigger a run
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
		return fmt.Errorf("%s: MyGAPI is not initialized", Name)
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

	//obj.GAPI = &MyGAPI{ // graph API
	//	Name:     "libmgmt", // TODO: set on compilation
	//	Interval: 60 * 10,   // arbitrarily change graph every 15 seconds
	//}

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
