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

//go:build !root

package dage

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/purpleidea/mgmt/lang/interfaces"
	"github.com/purpleidea/mgmt/lang/types"
)

// benchSourceFunc is a streamable int source. Each receive on the trigger
// channel increments the value and sends one event into the engine, so a
// benchmark can drive traversals in lockstep.
type benchSourceFunc struct {
	interfaces.Textarea

	trigger chan struct{}

	init  *interfaces.Init
	value int64
}

func (obj *benchSourceFunc) String() string { return "benchSource" }

func (obj *benchSourceFunc) Validate() error { return nil }

func (obj *benchSourceFunc) Info() *interfaces.Info {
	return &interfaces.Info{
		Pure: false,
		Memo: false,
		Fast: true,
		Spec: false,
		Sig:  types.NewType("func() int"),
	}
}

func (obj *benchSourceFunc) Init(init *interfaces.Init) error {
	obj.init = init
	return nil
}

func (obj *benchSourceFunc) Stream(ctx context.Context) error {
	for {
		select {
		case _, ok := <-obj.trigger:
			if !ok {
				return nil
			}
			atomic.AddInt64(&obj.value, 1)
			if err := obj.init.Event(ctx); err != nil {
				return err
			}

		case <-ctx.Done():
			return nil
		}
	}
}

func (obj *benchSourceFunc) Call(ctx context.Context, args []types.Value) (types.Value, error) {
	return &types.IntValue{V: atomic.LoadInt64(&obj.value)}, nil
}

// benchAddFunc is a pure two-arg adder so that traversals must build the args
// list for every vertex on every event, which is the hot path being measured.
type benchAddFunc struct {
	interfaces.Textarea

	name string
}

func (obj *benchAddFunc) String() string { return obj.name }

func (obj *benchAddFunc) Validate() error { return nil }

func (obj *benchAddFunc) Info() *interfaces.Info {
	return &interfaces.Info{
		Pure: true,
		Memo: false,
		Fast: true,
		Spec: false,
		Sig:  types.NewType("func(a int, b int) int"),
	}
}

func (obj *benchAddFunc) Init(init *interfaces.Init) error { return nil }

func (obj *benchAddFunc) Call(ctx context.Context, args []types.Value) (types.Value, error) {
	return &types.IntValue{V: args[0].Int() + args[1].Int()}, nil
}

// BenchmarkEngineEvents measures the per-event traversal cost of the function
// engine. The graph is one streamable source feeding a layered DAG of two-arg
// adders: the first layer reads both args from the source over one shared edge,
// and each later vertex reads from two parents in the previous layer. Every
// iteration triggers one source event and waits for the resulting table, so b.N
// counts complete traversals of the whole graph.
func BenchmarkEngineEvents(b *testing.B) {
	benchCases := []struct {
		name   string
		width  int
		layers int
	}{
		{name: "layered/5x5", width: 5, layers: 5},
		{name: "layered/10x10", width: 10, layers: 10},
		{name: "layered/20x25", width: 20, layers: 25},
	}
	for _, tc := range benchCases {
		b.Run(tc.name, func(b *testing.B) {
			source := &benchSourceFunc{
				trigger: make(chan struct{}),
			}

			engine := &Engine{
				Name: "bench",
				Logf: func(format string, v ...interface{}) {},
			}
			if err := engine.Setup(); err != nil {
				b.Fatalf("setup failed: %+v", err)
			}

			txn := engine.Txn()
			defer txn.Free()
			prev := make([]interfaces.Func, tc.width)
			for j := 0; j < tc.width; j++ { // first layer from source
				f := &benchAddFunc{name: fmt.Sprintf("add_0_%d", j)}
				txn.AddEdge(source, f, &interfaces.FuncEdge{
					Args: []string{"a", "b"}, // one shared edge
				})
				prev[j] = f
			}
			for l := 1; l < tc.layers; l++ {
				curr := make([]interfaces.Func, tc.width)
				for j := 0; j < tc.width; j++ {
					f := &benchAddFunc{name: fmt.Sprintf("add_%d_%d", l, j)}
					txn.AddEdge(prev[j], f, &interfaces.FuncEdge{
						Args: []string{"a"},
					})
					txn.AddEdge(prev[(j+1)%tc.width], f, &interfaces.FuncEdge{
						Args: []string{"b"},
					})
					curr[j] = f
				}
				prev = curr
			}
			if err := txn.Commit(); err != nil {
				b.Fatalf("commit failed: %+v", err)
			}

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			runErr := make(chan error, 1)
			go func() {
				runErr <- engine.Run(ctx)
			}()

			stream := engine.Stream()
			if _, ok := <-stream; !ok { // wait for the initial table
				b.Fatalf("unexpected stream closure")
			}

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				source.trigger <- struct{}{} // cause one event
				if _, ok := <-stream; !ok {  // wait for the traversal
					b.Fatalf("unexpected stream closure")
				}
			}
			b.StopTimer()

			cancel()
			for range stream { // drain until closed
			}
			if err := <-runErr; err != nil && err != context.Canceled {
				b.Fatalf("unexpected run error: %+v", err)
			}
		})
	}
}
