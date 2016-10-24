// libmgmt example
package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/purpleidea/mgmt/gconfig"
	mgmt "github.com/purpleidea/mgmt/mgmtmain"
	"github.com/purpleidea/mgmt/resources"
)

func generateGraphConfig() *gconfig.GraphConfig {

	n1, err := resources.NewNoopRes("noop1")
	if err != nil {
		return nil // error
	}

	gc := &gconfig.GraphConfig{
		Graph: "libmgmt",
		Resources: gconfig.Resources{ // must redefine anonymous struct :(
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
		//Hostname: "???",
		//Remote: "???",
	}
	return gc
}

// Run runs an embedded mgmt server.
func Run() error {

	obj := &mgmt.Main{}
	obj.Program = "mgmtlib" // TODO: set on compilation
	obj.Version = "0.0.1"   // TODO: set on compilation
	obj.TmpPrefix = true
	obj.IdealClusterSize = -1
	obj.ConvergedTimeout = -1
	obj.Noop = true

	obj.GAPI = generateGraphConfig // graph API function

	if err := obj.Init(); err != nil {
		return err
	}

	go func() {
		for {
			log.Printf("Generating new graph...")
			obj.Switch(generateGraphConfig) // pass in function to run...

			time.Sleep(15 * time.Second) // XXX: arbitrarily change graph every 30 seconds
		}
	}()

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
