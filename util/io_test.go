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

package util

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
)

type cancelReader struct {
	reader io.Reader
	cancel context.CancelFunc
}

func (obj *cancelReader) Read(p []byte) (int, error) {
	n, err := obj.reader.Read(p)
	if n > 0 && obj.cancel != nil {
		obj.cancel()
		obj.cancel = nil
	}
	return n, err
}

func TestCopyContextCanceled(t *testing.T) {
	const input = "hello"

	ctx, cancel := context.WithCancel(context.Background())
	reader := &cancelReader{
		reader: strings.NewReader(input),
		cancel: cancel,
	}

	var output strings.Builder
	count, err := CopyContext(ctx, &output, reader)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("copy error is not context cancellation: %+v", err)
	}
	if count != int64(len(input)) {
		t.Errorf("copy count differs: got %d, expected %d", count, len(input))
	}
	if output.String() != input {
		t.Errorf("copied output differs: got %q, expected %q", output.String(), input)
	}
}
