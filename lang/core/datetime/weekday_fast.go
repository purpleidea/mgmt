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

package coredatetime

import (
	"context"
	"strings"
	"time"

	"github.com/purpleidea/mgmt/lang/funcs"
	"github.com/purpleidea/mgmt/lang/interfaces"
	"github.com/purpleidea/mgmt/lang/types"
)

const (
	// WeekdayFastFuncName is the name this func is registered as.
	WeekdayFastFuncName = "weekday_fast"
)

func init() {
	funcs.ModuleRegister(ModuleName, WeekdayFastFuncName, func() interfaces.Func { return &WeekdayFast{} }) // must register the fact and name
}

// WeekdayFast is a fact which returns the current weekday. It does so more
// efficiently than calling datetime.weekday(datetime.now()) because we haven't
// yet merged any clever function engine optimizations. Once we do, this
// function can be deprecated.
type WeekdayFast struct {
	init *interfaces.Init
}

// String returns a simple name for this fact. This is needed so this struct can
// satisfy the pgraph.Vertex interface.
func (obj *WeekdayFast) String() string {
	return WeekdayFastFuncName
}

// Validate makes sure we've built our struct properly.
func (obj *WeekdayFast) Validate() error {
	return nil
}

// Info returns some static info about itself.
func (obj *WeekdayFast) Info() *interfaces.Info {
	return &interfaces.Info{
		Pure: false, // non-constant facts can't be pure!
		Memo: false,
		Fast: false,
		Spec: false,
		Sig:  types.NewType("func() str"),
	}
}

// Init runs some startup code for this fact.
func (obj *WeekdayFast) Init(init *interfaces.Init) error {
	obj.init = init
	return nil
}

// Stream starts a mainloop and runs Event when it's time to Call() again.
func (obj *WeekdayFast) Stream(ctx context.Context) error {
	// streams must generate an initial event on startup
	startChan := make(chan struct{}) // start signal
	close(startChan)                 // kick it off!

	for {
		select {
		case <-startChan:
			startChan = nil // disable

		case <-time.After(nextDay()):
			// pass

		case <-ctx.Done():
			return nil
		}

		if err := obj.init.Event(ctx); err != nil {
			return err
		}
	}
}

// Call this fact and return the value if it is possible to do so at this time.
func (obj *WeekdayFast) Call(ctx context.Context, args []types.Value) (types.Value, error) {
	return &types.StrValue{
		V: strings.ToLower(time.Now().Weekday().String()),
	}, nil
}

func nextDay() time.Duration {
	now := time.Now()
	next := time.Date(
		now.Year(),
		now.Month(),
		now.Day()+1, // next day
		0, 0, 0, 0,
		now.Location(),
	)

	return time.Until(next)
}
