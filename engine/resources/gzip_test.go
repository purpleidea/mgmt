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

package resources

import (
	"compress/gzip"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/purpleidea/mgmt/engine"
)

type gzipDelayedCancelContext struct {
	context.Context
	calls int
	done  chan struct{}
}

func (obj *gzipDelayedCancelContext) Done() <-chan struct{} {
	return obj.done
}

func (obj *gzipDelayedCancelContext) Err() error {
	if obj.calls > 0 {
		obj.calls--
		return nil
	}
	select {
	case <-obj.done:
	default:
		close(obj.done)
	}
	return context.Canceled
}

func TestGzipCheckApplyCancellationRemovesOutput(t *testing.T) {
	dir := t.TempDir()
	output := filepath.Join(dir, "output.gz")
	obj := &GzipRes{
		Path:    output,
		Content: "hello",
		Level:   gzip.DefaultCompression,
	}
	obj.SetName("test")

	init := &engine.Init{
		VarDir: func(string) (string, error) {
			return dir, nil
		},
		Logf: t.Logf,
	}
	if err := obj.Init(init); err != nil {
		t.Fatalf("init failed: %+v", err)
	}

	ctx := &gzipDelayedCancelContext{
		Context: context.Background(),
		calls:   2,
		done:    make(chan struct{}),
	}

	checkOK, err := obj.CheckApply(ctx, true)
	if checkOK {
		t.Errorf("func CheckApply returned checkOK after cancellation")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("func CheckApply error is not context cancellation: %+v", err)
	}
	if _, err := os.Stat(output); !os.IsNotExist(err) {
		t.Errorf("output exists after cancellation: %+v", err)
	}
}
