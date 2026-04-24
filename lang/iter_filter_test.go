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

package lang

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/purpleidea/mgmt/lang/inputs"
	"github.com/purpleidea/mgmt/lang/interfaces"
	"github.com/purpleidea/mgmt/util"
	"github.com/purpleidea/mgmt/util/errwrap"

	"github.com/spf13/afero"
)

func runLangEvents(t *testing.T, code string, events int, timeout time.Duration) error {
	t.Helper()

	logf := func(format string, v ...interface{}) {
		t.Logf("test: lang: "+format, v...)
	}
	mmFs := afero.NewMemMapFs()
	afs := &afero.Afero{Fs: mmFs}
	fs := &util.AferoFs{Afero: afs}

	output, err := inputs.ParseInput(code, fs)
	if err != nil {
		return errwrap.Wrapf(err, "ParseInput failed")
	}
	for _, fn := range output.Workers {
		if err := fn(fs); err != nil {
			return err
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	lang := &Lang{
		Fs:    fs,
		Input: "/" + interfaces.MetadataFilename,
		Data: &Data{
			UnificationStrategy: make(map[string]string),
		},
		Debug: false,
		Logf:  logf,
	}
	if err := lang.Init(ctx); err != nil {
		return errwrap.Wrapf(err, "init failed")
	}
	defer lang.Cleanup()

	errChan := make(chan error, 1)
	go func() {
		err := lang.Run(ctx)
		if err == context.Canceled {
			err = nil
		}
		errChan <- err
	}()

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	stream := lang.Stream(ctx)
	for i := 0; i < events; i++ {
		select {
		case _, ok := <-stream:
			if !ok {
				select {
				case err := <-errChan:
					if err != nil {
						return err
					}
				case <-time.After(timeout):
					return fmt.Errorf("timed out waiting for lang shutdown")
				}
				return fmt.Errorf("stream closed after %d events", i)
			}

		case err := <-errChan:
			if err != nil {
				return err
			}
			return fmt.Errorf("lang stopped after %d events", i)

		case <-timer.C:
			return fmt.Errorf("timed out after %d events", i)
		}
	}

	cancel()
	select {
	case err := <-errChan:
		return err

	case <-time.After(timeout):
		return fmt.Errorf("timed out waiting for lang shutdown")
	}
}

func TestIterFilterInputShrink(t *testing.T) {
	code := `
import "fmt"
import "iter"
import "math"
import "test"

$count = test.fastcount()
$long = [
	0, 1, 2, 3, 4, 5, 6, 7, 8, 9,
	10, 11, 12, 13, 14, 15, 16, 17, 18, 19,
	20, 21, 22, 23, 24, 25, 26, 27, 28, 29,
	30, 31, 32, 33, 34, 35, 36, 37, 38, 39,
	40, 41, 42, 43, 44, 45, 46, 47, 48, 49,
	50, 51, 52, 53, 54, 55, 56, 57, 58, 59,
	60, 61, 62, 63, 64, 65, 66,
]
$short = [
	0, 1, 2, 3, 4, 5, 6, 7, 8, 9,
	10, 11, 12, 13, 14, 15, 16, 17, 18, 19,
	20, 21, 22, 23, 24, 25, 26, 27, 28, 29,
	30, 31, 32, 33, 34, 35, 36, 37, 38, 39,
	40, 41, 42, 43, 44, 45, 46, 47, 48, 49,
	50, 51, 52, 53, 54, 55, 56, 57, 58, 59,
	60, 61, 62, 63, 64, 65,
]
$input = if math.mod($count, 2) == 0 {
	$long
} else {
	$short
}
$output = iter.filter($input, func($x) {
	true
})

test "filter-shrink" {
	stringptr => fmt.printf("%d", len($output)),
}
`

	if err := runLangEvents(t, code, 20, 10*time.Second); err != nil {
		t.Fatalf("lang errored: %+v", err)
	}
}
