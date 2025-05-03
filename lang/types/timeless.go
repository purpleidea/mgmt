// Mgmt
// Copyright (C) 2013-2023+ James Shubin and the project contributors
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
// along with this program.  If not, see <http://www.gnu.org/licenses/>.

package types

import "fmt"

// This next struct is used to determine whether an expression which has a
// function type can be given to a resource, like this:
//
//	test "example1" {
//		func1 => func($x) { $x + 1 }
//	}
//
// The golang implementation of that resource is going to receive a golang
// function, not a FuncValue. A golang function receives a single value and
// outputs a single value, while a FuncValue receives a stream of values over
// time and emits a stream of values over time, not necessarily one output in
// response to each input. Let's call a function which, when given a single
// input, returns a single output, a "timeless" function. All golang functions
// are timeless, but only some FuncValues are timeless. Other FuncValues are
// "timeful". We want to statically guarantee that only timeless FuncValue are
// passed to resources.
//
// One thing which is making that difficult is that we want to allow the
// expression to change over time which timeless function it is passing to the
// resource. For example, we want to allow this:
//
//	test "example2" {
//		func1 => if os.system("sleep 1; echo 1; sleep 1; echo 2") == "1" {
//			func($x) { $x + 1 }
//		} else {
//			func($x) { $x * 2 }
//		}
//	}
//
// But not this:
//
//	test "example3" {
//		func1 => func($x) {
//			if os.system("sleep 1; echo 1; sleep 1; echo 2") == "1" {
//				$x + 1
//	 		} else {
//				$x * 2
//			}
//		}
//	}
//
// The difference is that in the first case, the resource does not receive any
// function until one second has passed, at which point it receive one timeless
// function, and then one second later it receives a different timeless
// function. Whereas in the second case, the resource receives a function right
// away, but it is not possible to receive an output from it until one second
// has passed. Thus, the second function is definitely a timeful function which
// plays some shenanigans with time.
//
// In order to distinguish between these two cases, it is not enough to know
// that os.system is timeful and that math operations are timeless, we also need
// to know whether the timeful functions are being used to pick between timeless
// functions or to calculate the output of a timeless function. This is made
// more difficult in the presence of higher-order functions:
//
//	$apply = func($f, $x) { $f($x) }
//	$makeMakeFunc = func($step) {
//		func($one) {
//			if $step == $one {
//				func($x) { $x + strconv.atoi($one) }
//			} else {
//				func($x) { $x * 2 }
//			}
//		}
//	}
//	$makeFunc = $apply(
//		$makeMakeFunc,
//		os.system("sleep 1; echo 1; sleep 1; echo 2")
//	)
//	$func = $apply($makeFunc, "1")
//	test "example4" {
//		func1 => $func
//	}
//
// If we inline the function calls, we can see that the above is equivalent to
// example2, and thus should be accepted.
//
// Note that in order to figure out that the above should be accepted, it is not
// sufficient to keep track of whether $apply, $makeMakeFunc, and $makeFunc are
// timeless or timeful. Instead, we need to keep track of the fact that:
//
//  1. $makeMakeFunc produces a function ($makeFunc) which produces a function
//     ($func) which is timeless.
//  2. When $apply is applied to $makeMakeFunc and os.system, it produces a
//     function ($makeFunc) which produces a function ($func) which is timeless.
//  3. When $apply is applied to $makeFunc and "1", it produces a function
//     ($func) which is timeless.
//
// This means that our analysis needs to produce a value of a different golang
// type depending on its MCL type:
//
// 1. For expressions of type string like os.system(...) and "1", we need to
// produce a boolean indicating whether the string changes over time or not.
// Same for int, bool, etc.
// 2. For expressions of type func(something) something like $makeMakeFunc, we
// need to produce a golang function which takes a list of inputs and produces
// an output. The type of the inputs and outputs depends on the type of the
// inputs and outputs of the MCL function. For an input of type int, the golang
// function would receive a boolean, for an input of type func(int) int, the
// golang function would receive a golang function of type func(bool) bool, etc.
// 3. For expressions of type []something, we need to produce a golang value
// whose type depends on the MCL type of the elements of the list. For an
// element of type int, the golang value would be a boolean, for an element of
// type func(int) int, the golang value would be a golang function of type
// func(bool) bool, etc.
//
//	// int
//	IsTimeless bool
//
//	// func() int
//	IsTimeless func() bool
//
//	// func(int, int) int
//	IsTimeless func(bool, bool) bool
//
//	// func(func(int) int, int) int
//	IsTimeless func(func(bool) bool, bool) bool
//
//	// func([]int, int) []int
//	IsTimeless func(bool, bool) bool
//
//	// func(int, int) func(int) int
//	IsTimeless func(bool, bool) func(bool) bool
//
// The following struct can represent all of the types above.
type Timeless struct {
	IsTimeless         *bool
	PropagatesTimeless *func([]*Timeless) (*Timeless, error)
}

var (
	alwaysTrue     = true
	alwaysFalse    = false
	AlwaysTimeless = &Timeless{
		IsTimeless:         &alwaysTrue,
		PropagatesTimeless: nil,
	}
	AlwaysTimeful = &Timeless{
		IsTimeless:         &alwaysFalse,
		PropagatesTimeless: nil,
	}
)

func IsCompletelyTimeless(timeless *Timeless, typ *Type) (bool, error) {
	if timeless.IsTimeless != nil {
		return *timeless.IsTimeless, nil
	}

	if timeless.PropagatesTimeless != nil {
		switch typ.Kind {
		case KindBool:
			return false, fmt.Errorf("IsCompletelyTimeless: timeless is invalid for the given type")
		case KindStr:
			return false, fmt.Errorf("IsCompletelyTimeless: timeless is invalid for the given type")
		case KindInt:
			return false, fmt.Errorf("IsCompletelyTimeless: timeless is invalid for the given type")
		case KindFloat:
			return false, fmt.Errorf("IsCompletelyTimeless: timeless is invalid for the given type")

		case KindList:
			return IsCompletelyTimeless(timeless, typ.Val)

		case KindMap:
			// We are assuming that maps cannot contain functions as keys.
			if typ.Key == nil || typ.Val == nil {
				panic("malformed map type")
			}
			return IsCompletelyTimeless(timeless, typ.Val)

		case KindStruct: // {a bool; b int}
			if typ.Map == nil {
				panic("malformed struct type")
			}
			if len(typ.Map) != len(typ.Ord) {
				panic("malformed struct length")
			}
			for _, k := range typ.Ord {
				t, ok := typ.Map[k]
				if !ok {
					panic("malformed struct order")
				}
				if t == nil {
					panic("malformed struct field")
				}
				r, err := IsCompletelyTimeless(timeless, t)
				if err != nil {
					return false, err
				}
				if !r {
					return false, nil
				}
			}
			return true, nil

		case KindFunc:
			if typ.Map == nil {
				panic("malformed func type")
			}
			if len(typ.Map) != len(typ.Ord) {
				panic("malformed func length")
			}

			timelessArgs := []*Timeless{}
			for _ = range typ.Ord {
				timelessArgs = append(timelessArgs, AlwaysTimeless)
			}

			timelessOutput, err := ApplyTimeless(timeless, timelessArgs)
			if err != nil {
				return false, err
			}

			return IsCompletelyTimeless(timelessOutput, typ.Out)

		case KindVariant:
			// Pessimistically assume that variants are timeful
			return false, nil

		default:
			panic("unknown type kind")
		}
	}

	return false, fmt.Errorf("IsCompletelyTimeless: timeless is invalid")
}

func ApplyTimeless(timelessFn *Timeless, inputs []*Timeless) (*Timeless, error) {
	if timelessFn.IsTimeless != nil {
		return timelessFn, nil
	} else if timelessFn.PropagatesTimeless != nil {
		return (*timelessFn.PropagatesTimeless)(inputs)
	} else {
		return nil, fmt.Errorf("ApplyTimeless: timelessFn is invalid")
	}
}

// The timelessness analysis must be pessimistic: when combining a timeless
// expression with a timeful expression, the overall result is timeful. For
// example, the list ["1", os.system(...)] is timeful because one of its
// elements is timeful, and the function func($x) { if ... { "1" } else {
// os.system(...) } } is timeful because it sometimes returns a timeful value.
func CombineTimeless(timeless1, timeless2 *Timeless) (*Timeless, error) {
	if timeless1.IsTimeless != nil && timeless2.IsTimeless != nil {
		if *timeless1.IsTimeless && *timeless2.IsTimeless {
			return AlwaysTimeless, nil
		} else {
			return AlwaysTimeful, nil
		}
	} else {
		// combineTimeless(func($x) { $x }, func($x) { $x }) => func($x) { $x }
		// combineTimeless(func($x) { $x }, func($x) { timeless }) => func($x) { $x }
		// combineTimeless(func($x) { $x }, func($x) { timeful }) => func($x) { timeful }

		propagatesTimeless := func(inputs []*Timeless) (*Timeless, error) {
			output1, err1 := ApplyTimeless(timeless1, inputs)
			if err1 != nil {
				return nil, err1
			}
			output2, err2 := ApplyTimeless(timeless2, inputs)
			if err2 != nil {
				return nil, err2
			}

			return CombineTimeless(output1, output2)
		}
		return &Timeless{
			IsTimeless:         nil,
			PropagatesTimeless: &propagatesTimeless,
		}, nil
	}
}
