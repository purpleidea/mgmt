// Mgmt
// Copyright (C) James Shubin and the project contributors
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
// along with this program.  If not, see <https://www.gnu.org/licenses/>.
//
// Additional permission under GNU GPL version 3 section 7
//
// If you modify this program, or any covered work, by linking or combining it
// with embedded mcl code and modules (and that the embedded mcl code and
// modules which link with this program, contain a copy of their source code in
// the authoritative form) containing parts covered by the terms of any other
// license, the licensors of this program grant you additional permission to
// convey the resulting work. Furthermore, the licensors of this program grant
// the original author, James Shubin, additional permission to update this
// additional permission if he deems it necessary to achieve the goals of this
// additional permission.

// Package pprof is a simple wrapper around the pprof utility code which we use.
package pprof

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/pprof"
	"sync"
	"time"
)

// DefaultListen is the default listen address for the pprof HTTP server.
const DefaultListen = "127.0.0.1:6060"

// Server is the HTTP pprof server.
type Server struct {
	// Listen is the listen specification for the net/http server. This gets
	// rewritten cleanly after Init.
	Listen string

	Debug bool
	Logf  func(format string, v ...interface{})

	server *http.Server
}

// Init initializes the pprof server defaults.
func (obj *Server) Init() error {
	if obj.Listen == "" {
		obj.Listen = DefaultListen
	}

	return nil
}

// Run starts the pprof HTTP server and runs until the ctx is cancelled.
func (obj *Server) Run(ctx context.Context) (reterr error) {
	if obj.server != nil {
		return fmt.Errorf("pprof server is already started")
	}
	// TODO: is this the correct way to do all of this manually?
	mux := http.NewServeMux()
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
	mux.Handle("/debug/pprof/allocs", pprof.Handler("allocs"))
	mux.Handle("/debug/pprof/block", pprof.Handler("block"))
	mux.Handle("/debug/pprof/goroutine", pprof.Handler("goroutine"))
	mux.Handle("/debug/pprof/heap", pprof.Handler("heap"))
	mux.Handle("/debug/pprof/mutex", pprof.Handler("mutex"))
	mux.Handle("/debug/pprof/threadcreate", pprof.Handler("threadcreate"))

	listener, err := net.Listen("tcp", obj.Listen)
	if err != nil {
		return err
	}
	obj.Listen = listener.Addr().String() // rewrite cleanly
	obj.server = &http.Server{
		Addr:              obj.Listen,
		Handler:           mux,
		ReadHeaderTimeout: 60 * time.Second, // safety against slowloris
	}

	wg := &sync.WaitGroup{}
	defer wg.Wait()

	wg.Add(1)
	go func() {
		defer wg.Done()
		err := obj.server.Serve(listener)
		if err != http.ErrServerClosed {
			reterr = err
		}
	}()
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = obj.server.Shutdown(shutdownCtx) // error comes in goroutine
		obj.server = nil
	}()

	select {
	case <-ctx.Done():
	}
	return nil
}
