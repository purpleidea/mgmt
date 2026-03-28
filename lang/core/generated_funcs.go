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

package core

import (
	"context"
	"html"
	"math"
	rand "math/rand"
	"os"
	exec "os/exec"
	"path"
	filepath "path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/purpleidea/mgmt/lang/funcs/funcgen/util"
	"github.com/purpleidea/mgmt/lang/funcs/simple"
	"github.com/purpleidea/mgmt/lang/types"
)

func init() {
	simple.ModuleRegister("golang/html", "escape_string", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func(s str) str"),
		F: HTMLEscapeString,
	})
	simple.ModuleRegister("golang/html", "unescape_string", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func(s str) str"),
		F: HTMLUnescapeString,
	})
	simple.ModuleRegister("golang/math", "abs", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func(x float) float"),
		F: MathAbs,
	})
	simple.ModuleRegister("golang/math", "acos", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func(x float) float"),
		F: MathAcos,
	})
	simple.ModuleRegister("golang/math", "acosh", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func(x float) float"),
		F: MathAcosh,
	})
	simple.ModuleRegister("golang/math", "asin", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func(x float) float"),
		F: MathAsin,
	})
	simple.ModuleRegister("golang/math", "asinh", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func(x float) float"),
		F: MathAsinh,
	})
	simple.ModuleRegister("golang/math", "atan", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func(x float) float"),
		F: MathAtan,
	})
	simple.ModuleRegister("golang/math", "atan_2", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func(y float, x float) float"),
		F: MathAtan2,
	})
	simple.ModuleRegister("golang/math", "atanh", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func(x float) float"),
		F: MathAtanh,
	})
	simple.ModuleRegister("golang/math", "cbrt", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func(x float) float"),
		F: MathCbrt,
	})
	simple.ModuleRegister("golang/math", "ceil", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func(x float) float"),
		F: MathCeil,
	})
	simple.ModuleRegister("golang/math", "copysign", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func(f float, sign float) float"),
		F: MathCopysign,
	})
	simple.ModuleRegister("golang/math", "cos", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func(x float) float"),
		F: MathCos,
	})
	simple.ModuleRegister("golang/math", "cosh", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func(x float) float"),
		F: MathCosh,
	})
	simple.ModuleRegister("golang/math", "dim", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func(x float, y float) float"),
		F: MathDim,
	})
	simple.ModuleRegister("golang/math", "erf", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func(x float) float"),
		F: MathErf,
	})
	simple.ModuleRegister("golang/math", "erfc", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func(x float) float"),
		F: MathErfc,
	})
	simple.ModuleRegister("golang/math", "erfcinv", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func(x float) float"),
		F: MathErfcinv,
	})
	simple.ModuleRegister("golang/math", "erfinv", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func(x float) float"),
		F: MathErfinv,
	})
	simple.ModuleRegister("golang/math", "exp", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func(x float) float"),
		F: MathExp,
	})
	simple.ModuleRegister("golang/math", "exp_2", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func(x float) float"),
		F: MathExp2,
	})
	simple.ModuleRegister("golang/math", "expm_1", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func(x float) float"),
		F: MathExpm1,
	})
	simple.ModuleRegister("golang/math", "fma", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func(x float, y float, z float) float"),
		F: MathFMA,
	})
	simple.ModuleRegister("golang/math", "floor", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func(x float) float"),
		F: MathFloor,
	})
	simple.ModuleRegister("golang/math", "gamma", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func(x float) float"),
		F: MathGamma,
	})
	simple.ModuleRegister("golang/math", "hypot", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func(p float, q float) float"),
		F: MathHypot,
	})
	simple.ModuleRegister("golang/math", "ilogb", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func(x float) int"),
		F: MathIlogb,
	})
	simple.ModuleRegister("golang/math", "inf", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func(sign int) float"),
		F: MathInf,
	})
	simple.ModuleRegister("golang/math", "is_inf", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func(f float, sign int) bool"),
		F: MathIsInf,
	})
	simple.ModuleRegister("golang/math", "is_na_n", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func(f float) bool"),
		F: MathIsNaN,
	})
	simple.ModuleRegister("golang/math", "j_0", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func(x float) float"),
		F: MathJ0,
	})
	simple.ModuleRegister("golang/math", "j_1", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func(x float) float"),
		F: MathJ1,
	})
	simple.ModuleRegister("golang/math", "jn", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func(n int, x float) float"),
		F: MathJn,
	})
	simple.ModuleRegister("golang/math", "ldexp", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func(frac float, exp int) float"),
		F: MathLdexp,
	})
	simple.ModuleRegister("golang/math", "log", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func(x float) float"),
		F: MathLog,
	})
	simple.ModuleRegister("golang/math", "log_10", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func(x float) float"),
		F: MathLog10,
	})
	simple.ModuleRegister("golang/math", "log_1_p", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func(x float) float"),
		F: MathLog1p,
	})
	simple.ModuleRegister("golang/math", "log_2", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func(x float) float"),
		F: MathLog2,
	})
	simple.ModuleRegister("golang/math", "logb", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func(x float) float"),
		F: MathLogb,
	})
	simple.ModuleRegister("golang/math", "max", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func(x float, y float) float"),
		F: MathMax,
	})
	simple.ModuleRegister("golang/math", "min", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func(x float, y float) float"),
		F: MathMin,
	})
	simple.ModuleRegister("golang/math", "mod", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func(x float, y float) float"),
		F: MathMod,
	})
	simple.ModuleRegister("golang/math", "na_n", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func() float"),
		F: MathNaN,
	})
	simple.ModuleRegister("golang/math", "nextafter", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func(x float, y float) float"),
		F: MathNextafter,
	})
	simple.ModuleRegister("golang/math", "pow", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func(x float, y float) float"),
		F: MathPow,
	})
	simple.ModuleRegister("golang/math", "pow_10", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func(n int) float"),
		F: MathPow10,
	})
	simple.ModuleRegister("golang/math", "remainder", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func(x float, y float) float"),
		F: MathRemainder,
	})
	simple.ModuleRegister("golang/math", "round", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func(x float) float"),
		F: MathRound,
	})
	simple.ModuleRegister("golang/math", "round_to_even", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func(x float) float"),
		F: MathRoundToEven,
	})
	simple.ModuleRegister("golang/math", "signbit", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func(x float) bool"),
		F: MathSignbit,
	})
	simple.ModuleRegister("golang/math", "sin", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func(x float) float"),
		F: MathSin,
	})
	simple.ModuleRegister("golang/math", "sinh", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func(x float) float"),
		F: MathSinh,
	})
	simple.ModuleRegister("golang/math", "sqrt", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func(x float) float"),
		F: MathSqrt,
	})
	simple.ModuleRegister("golang/math", "tan", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func(x float) float"),
		F: MathTan,
	})
	simple.ModuleRegister("golang/math", "tanh", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func(x float) float"),
		F: MathTanh,
	})
	simple.ModuleRegister("golang/math", "trunc", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func(x float) float"),
		F: MathTrunc,
	})
	simple.ModuleRegister("golang/math", "y_0", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func(x float) float"),
		F: MathY0,
	})
	simple.ModuleRegister("golang/math", "y_1", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func(x float) float"),
		F: MathY1,
	})
	simple.ModuleRegister("golang/math", "yn", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func(n int, x float) float"),
		F: MathYn,
	})
	simple.ModuleRegister("golang/math/rand", "exp_float_64", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func() float"),
		F: MathrandExpFloat64,
	})
	simple.ModuleRegister("golang/math/rand", "float_64", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func() float"),
		F: MathrandFloat64,
	})
	simple.ModuleRegister("golang/math/rand", "int", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func() int"),
		F: MathrandInt,
	})
	simple.ModuleRegister("golang/math/rand", "int_63", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func() int"),
		F: MathrandInt63,
	})
	simple.ModuleRegister("golang/math/rand", "int_63_n", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func(n int) int"),
		F: MathrandInt63n,
	})
	simple.ModuleRegister("golang/math/rand", "intn", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func(n int) int"),
		F: MathrandIntn,
	})
	simple.ModuleRegister("golang/math/rand", "norm_float_64", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func() float"),
		F: MathrandNormFloat64,
	})
	simple.ModuleRegister("golang/math/rand", "read", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func(p str) int"),
		F: MathrandRead,
	})
	simple.ModuleRegister("golang/os", "executable", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func() str"),
		F: OsExecutable,
	})
	simple.ModuleRegister("golang/os", "expand_env", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func(s str) str"),
		F: OsExpandEnv,
	})
	simple.ModuleRegister("golang/os", "getegid", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func() int"),
		F: OsGetegid,
	})
	simple.ModuleRegister("golang/os", "getenv", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func(key str) str"),
		F: OsGetenv,
	})
	simple.ModuleRegister("golang/os", "geteuid", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func() int"),
		F: OsGeteuid,
	})
	simple.ModuleRegister("golang/os", "getgid", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func() int"),
		F: OsGetgid,
	})
	simple.ModuleRegister("golang/os", "getpagesize", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func() int"),
		F: OsGetpagesize,
	})
	simple.ModuleRegister("golang/os", "getpid", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func() int"),
		F: OsGetpid,
	})
	simple.ModuleRegister("golang/os", "getppid", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func() int"),
		F: OsGetppid,
	})
	simple.ModuleRegister("golang/os", "getuid", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func() int"),
		F: OsGetuid,
	})
	simple.ModuleRegister("golang/os", "getwd", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func() str"),
		F: OsGetwd,
	})
	simple.ModuleRegister("golang/os", "hostname", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func() str"),
		F: OsHostname,
	})
	simple.ModuleRegister("golang/os", "mkdir_temp", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func(dir str, pattern str) str"),
		F: OsMkdirTemp,
	})
	simple.ModuleRegister("golang/os", "read_file", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func(name str) str"),
		F: OsReadFile,
	})
	simple.ModuleRegister("golang/os", "readlink", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func(name str) str"),
		F: OsReadlink,
	})
	simple.ModuleRegister("golang/os", "temp_dir", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func() str"),
		F: OsTempDir,
	})
	simple.ModuleRegister("golang/os", "user_cache_dir", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func() str"),
		F: OsUserCacheDir,
	})
	simple.ModuleRegister("golang/os", "user_config_dir", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func() str"),
		F: OsUserConfigDir,
	})
	simple.ModuleRegister("golang/os", "user_home_dir", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func() str"),
		F: OsUserHomeDir,
	})
	simple.ModuleRegister("golang/os/exec", "look_path", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func(file str) str"),
		F: OsexecLookPath,
	})
	simple.ModuleRegister("golang/path", "base", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func(path str) str"),
		F: PathBase,
	})
	simple.ModuleRegister("golang/path", "clean", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func(path str) str"),
		F: PathClean,
	})
	simple.ModuleRegister("golang/path", "dir", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func(path str) str"),
		F: PathDir,
	})
	simple.ModuleRegister("golang/path", "ext", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func(path str) str"),
		F: PathExt,
	})
	simple.ModuleRegister("golang/path", "is_abs", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func(path str) bool"),
		F: PathIsAbs,
	})
	simple.ModuleRegister("golang/path", "join", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func(elem []str) str"),
		F: PathJoin,
	})
	simple.ModuleRegister("golang/path", "match", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func(pattern str, name str) bool"),
		F: PathMatch,
	})
	simple.ModuleRegister("golang/path/filepath", "abs", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func(path str) str"),
		F: PathfilepathAbs,
	})
	simple.ModuleRegister("golang/path/filepath", "base", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func(path str) str"),
		F: PathfilepathBase,
	})
	simple.ModuleRegister("golang/path/filepath", "clean", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func(path str) str"),
		F: PathfilepathClean,
	})
	simple.ModuleRegister("golang/path/filepath", "dir", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func(path str) str"),
		F: PathfilepathDir,
	})
	simple.ModuleRegister("golang/path/filepath", "eval_symlinks", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func(path str) str"),
		F: PathfilepathEvalSymlinks,
	})
	simple.ModuleRegister("golang/path/filepath", "ext", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func(path str) str"),
		F: PathfilepathExt,
	})
	simple.ModuleRegister("golang/path/filepath", "from_slash", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func(path str) str"),
		F: PathfilepathFromSlash,
	})
	simple.ModuleRegister("golang/path/filepath", "has_prefix", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func(p str, prefix str) bool"),
		F: PathfilepathHasPrefix,
	})
	simple.ModuleRegister("golang/path/filepath", "is_abs", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func(path str) bool"),
		F: PathfilepathIsAbs,
	})
	simple.ModuleRegister("golang/path/filepath", "is_local", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func(path str) bool"),
		F: PathfilepathIsLocal,
	})
	simple.ModuleRegister("golang/path/filepath", "join", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func(elem []str) str"),
		F: PathfilepathJoin,
	})
	simple.ModuleRegister("golang/path/filepath", "localize", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func(path str) str"),
		F: PathfilepathLocalize,
	})
	simple.ModuleRegister("golang/path/filepath", "match", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func(pattern str, name str) bool"),
		F: PathfilepathMatch,
	})
	simple.ModuleRegister("golang/path/filepath", "rel", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func(basepath str, targpath str) str"),
		F: PathfilepathRel,
	})
	simple.ModuleRegister("golang/path/filepath", "to_slash", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func(path str) str"),
		F: PathfilepathToSlash,
	})
	simple.ModuleRegister("golang/path/filepath", "volume_name", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func(path str) str"),
		F: PathfilepathVolumeName,
	})
	simple.ModuleRegister("golang/runtime", "cpu_profile", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func() str"),
		F: RuntimeCPUProfile,
	})
	simple.ModuleRegister("golang/runtime", "gomaxprocs", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func(n int) int"),
		F: RuntimeGOMAXPROCS,
	})
	simple.ModuleRegister("golang/runtime", "goroot", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func() str"),
		F: RuntimeGOROOT,
	})
	simple.ModuleRegister("golang/runtime", "num_cpu", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func() int"),
		F: RuntimeNumCPU,
	})
	simple.ModuleRegister("golang/runtime", "num_cgo_call", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func() int"),
		F: RuntimeNumCgoCall,
	})
	simple.ModuleRegister("golang/runtime", "num_goroutine", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func() int"),
		F: RuntimeNumGoroutine,
	})
	simple.ModuleRegister("golang/runtime", "read_trace", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func() str"),
		F: RuntimeReadTrace,
	})
	simple.ModuleRegister("golang/runtime", "set_mutex_profile_fraction", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func(rate int) int"),
		F: RuntimeSetMutexProfileFraction,
	})
	simple.ModuleRegister("golang/runtime", "stack", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func(buf str, all bool) int"),
		F: RuntimeStack,
	})
	simple.ModuleRegister("golang/runtime", "version", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func() str"),
		F: RuntimeVersion,
	})
	simple.ModuleRegister("golang/strconv", "append_bool", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func(dst str, b bool) str"),
		F: StrconvAppendBool,
	})
	simple.ModuleRegister("golang/strconv", "append_int", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func(dst str, i int, base int) str"),
		F: StrconvAppendInt,
	})
	simple.ModuleRegister("golang/strconv", "append_quote", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func(dst str, s str) str"),
		F: StrconvAppendQuote,
	})
	simple.ModuleRegister("golang/strconv", "append_quote_to_ascii", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func(dst str, s str) str"),
		F: StrconvAppendQuoteToASCII,
	})
	simple.ModuleRegister("golang/strconv", "append_quote_to_graphic", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func(dst str, s str) str"),
		F: StrconvAppendQuoteToGraphic,
	})
	simple.ModuleRegister("golang/strconv", "atoi", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func(s str) int"),
		F: StrconvAtoi,
	})
	simple.ModuleRegister("golang/strconv", "can_backquote", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func(s str) bool"),
		F: StrconvCanBackquote,
	})
	simple.ModuleRegister("golang/strconv", "format_bool", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func(b bool) str"),
		F: StrconvFormatBool,
	})
	simple.ModuleRegister("golang/strconv", "format_int", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func(i int, base int) str"),
		F: StrconvFormatInt,
	})
	simple.ModuleRegister("golang/strconv", "itoa", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func(i int) str"),
		F: StrconvItoa,
	})
	simple.ModuleRegister("golang/strconv", "parse_bool", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func(str str) bool"),
		F: StrconvParseBool,
	})
	simple.ModuleRegister("golang/strconv", "parse_float", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func(s str, bitSize int) float"),
		F: StrconvParseFloat,
	})
	simple.ModuleRegister("golang/strconv", "parse_int", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func(s str, base int, bitSize int) int"),
		F: StrconvParseInt,
	})
	simple.ModuleRegister("golang/strconv", "quote", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func(s str) str"),
		F: StrconvQuote,
	})
	simple.ModuleRegister("golang/strconv", "quote_to_ascii", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func(s str) str"),
		F: StrconvQuoteToASCII,
	})
	simple.ModuleRegister("golang/strconv", "quote_to_graphic", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func(s str) str"),
		F: StrconvQuoteToGraphic,
	})
	simple.ModuleRegister("golang/strconv", "quoted_prefix", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func(s str) str"),
		F: StrconvQuotedPrefix,
	})
	simple.ModuleRegister("golang/strconv", "unquote", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func(s str) str"),
		F: StrconvUnquote,
	})
	simple.ModuleRegister("golang/strings", "clone", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func(s str) str"),
		F: StringsClone,
	})
	simple.ModuleRegister("golang/strings", "compare", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func(a str, b str) int"),
		F: StringsCompare,
	})
	simple.ModuleRegister("golang/strings", "contains", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func(s str, substr str) bool"),
		F: StringsContains,
	})
	simple.ModuleRegister("golang/strings", "contains_any", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func(s str, chars str) bool"),
		F: StringsContainsAny,
	})
	simple.ModuleRegister("golang/strings", "count", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func(s str, substr str) int"),
		F: StringsCount,
	})
	simple.ModuleRegister("golang/strings", "equal_fold", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func(s str, t str) bool"),
		F: StringsEqualFold,
	})
	simple.ModuleRegister("golang/strings", "has_prefix", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func(s str, prefix str) bool"),
		F: StringsHasPrefix,
	})
	simple.ModuleRegister("golang/strings", "has_suffix", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func(s str, suffix str) bool"),
		F: StringsHasSuffix,
	})
	simple.ModuleRegister("golang/strings", "index", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func(s str, substr str) int"),
		F: StringsIndex,
	})
	simple.ModuleRegister("golang/strings", "index_any", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func(s str, chars str) int"),
		F: StringsIndexAny,
	})
	simple.ModuleRegister("golang/strings", "join", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func(elems []str, sep str) str"),
		F: StringsJoin,
	})
	simple.ModuleRegister("golang/strings", "last_index", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func(s str, substr str) int"),
		F: StringsLastIndex,
	})
	simple.ModuleRegister("golang/strings", "last_index_any", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func(s str, chars str) int"),
		F: StringsLastIndexAny,
	})
	simple.ModuleRegister("golang/strings", "repeat", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func(s str, count int) str"),
		F: StringsRepeat,
	})
	simple.ModuleRegister("golang/strings", "replace", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func(s str, old str, new str, n int) str"),
		F: StringsReplace,
	})
	simple.ModuleRegister("golang/strings", "replace_all", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func(s str, old str, new str) str"),
		F: StringsReplaceAll,
	})
	simple.ModuleRegister("golang/strings", "title", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func(s str) str"),
		F: StringsTitle,
	})
	simple.ModuleRegister("golang/strings", "to_lower", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func(s str) str"),
		F: StringsToLower,
	})
	simple.ModuleRegister("golang/strings", "to_title", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func(s str) str"),
		F: StringsToTitle,
	})
	simple.ModuleRegister("golang/strings", "to_upper", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func(s str) str"),
		F: StringsToUpper,
	})
	simple.ModuleRegister("golang/strings", "to_valid_utf_8", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func(s str, replacement str) str"),
		F: StringsToValidUTF8,
	})
	simple.ModuleRegister("golang/strings", "trim", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func(s str, cutset str) str"),
		F: StringsTrim,
	})
	simple.ModuleRegister("golang/strings", "trim_left", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func(s str, cutset str) str"),
		F: StringsTrimLeft,
	})
	simple.ModuleRegister("golang/strings", "trim_prefix", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func(s str, prefix str) str"),
		F: StringsTrimPrefix,
	})
	simple.ModuleRegister("golang/strings", "trim_right", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func(s str, cutset str) str"),
		F: StringsTrimRight,
	})
	simple.ModuleRegister("golang/strings", "trim_space", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func(s str) str"),
		F: StringsTrimSpace,
	})
	simple.ModuleRegister("golang/strings", "trim_suffix", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func(s str, suffix str) str"),
		F: StringsTrimSuffix,
	})

}

// HTMLEscapeString is an autogenerated function.
// func EscapeString(s string) string
// EscapeString escapes special characters like "<" to become "&lt;". It
// escapes only five such characters: <, >, &, ' and ".
// UnescapeString(EscapeString(s)) == s always holds, but the converse isn't
// always true.
func HTMLEscapeString(ctx context.Context, args []types.Value) (types.Value, error) {
	return &types.StrValue{
		V: html.EscapeString(args[0].Str()),
	}, nil
}

// HTMLUnescapeString is an autogenerated function.
// func UnescapeString(s string) string
// UnescapeString unescapes entities like "&lt;" to become "<". It unescapes a
// larger range of entities than EscapeString escapes. For example, "&aacute;"
// unescapes to "á", as does "&#225;" and "&#xE1;".
// UnescapeString(EscapeString(s)) == s always holds, but the converse isn't
// always true.
func HTMLUnescapeString(ctx context.Context, args []types.Value) (types.Value, error) {
	return &types.StrValue{
		V: html.UnescapeString(args[0].Str()),
	}, nil
}

// MathAbs is an autogenerated function.
// func Abs(x float64) float64
// Abs returns the absolute value of x.
//
// Special cases are:
//
// Abs(±Inf) = +Inf
// Abs(NaN) = NaN
func MathAbs(ctx context.Context, args []types.Value) (types.Value, error) {
	return &types.FloatValue{
		V: math.Abs(args[0].Float()),
	}, nil
}

// MathAcos is an autogenerated function.
// func Acos(x float64) float64
// Acos returns the arccosine, in radians, of x.
//
// Special case is:
//
// Acos(x) = NaN if x < -1 or x > 1
func MathAcos(ctx context.Context, args []types.Value) (types.Value, error) {
	return &types.FloatValue{
		V: math.Acos(args[0].Float()),
	}, nil
}

// MathAcosh is an autogenerated function.
// func Acosh(x float64) float64
// Acosh returns the inverse hyperbolic cosine of x.
//
// Special cases are:
//
// Acosh(+Inf) = +Inf
// Acosh(x) = NaN if x < 1
// Acosh(NaN) = NaN
func MathAcosh(ctx context.Context, args []types.Value) (types.Value, error) {
	return &types.FloatValue{
		V: math.Acosh(args[0].Float()),
	}, nil
}

// MathAsin is an autogenerated function.
// func Asin(x float64) float64
// Asin returns the arcsine, in radians, of x.
//
// Special cases are:
//
// Asin(±0) = ±0
// Asin(x) = NaN if x < -1 or x > 1
func MathAsin(ctx context.Context, args []types.Value) (types.Value, error) {
	return &types.FloatValue{
		V: math.Asin(args[0].Float()),
	}, nil
}

// MathAsinh is an autogenerated function.
// func Asinh(x float64) float64
// Asinh returns the inverse hyperbolic sine of x.
//
// Special cases are:
//
// Asinh(±0) = ±0
// Asinh(±Inf) = ±Inf
// Asinh(NaN) = NaN
func MathAsinh(ctx context.Context, args []types.Value) (types.Value, error) {
	return &types.FloatValue{
		V: math.Asinh(args[0].Float()),
	}, nil
}

// MathAtan is an autogenerated function.
// func Atan(x float64) float64
// Atan returns the arctangent, in radians, of x.
//
// Special cases are:
//
// Atan(±0) = ±0
// Atan(±Inf) = ±Pi/2
func MathAtan(ctx context.Context, args []types.Value) (types.Value, error) {
	return &types.FloatValue{
		V: math.Atan(args[0].Float()),
	}, nil
}

// MathAtan2 is an autogenerated function.
// func Atan2(y float64, x float64) float64
// Atan2 returns the arc tangent of y/x, using
// the signs of the two to determine the quadrant
// of the return value.
//
// Special cases are (in order):
//
// Atan2(y, NaN) = NaN
// Atan2(NaN, x) = NaN
// Atan2(+0, x>=0) = +0
// Atan2(-0, x>=0) = -0
// Atan2(+0, x<=-0) = +Pi
// Atan2(-0, x<=-0) = -Pi
// Atan2(y>0, 0) = +Pi/2
// Atan2(y<0, 0) = -Pi/2
// Atan2(+Inf, +Inf) = +Pi/4
// Atan2(-Inf, +Inf) = -Pi/4
// Atan2(+Inf, -Inf) = 3Pi/4
// Atan2(-Inf, -Inf) = -3Pi/4
// Atan2(y, +Inf) = 0
// Atan2(y>0, -Inf) = +Pi
// Atan2(y<0, -Inf) = -Pi
// Atan2(+Inf, x) = +Pi/2
// Atan2(-Inf, x) = -Pi/2
func MathAtan2(ctx context.Context, args []types.Value) (types.Value, error) {
	return &types.FloatValue{
		V: math.Atan2(args[0].Float(), args[1].Float()),
	}, nil
}

// MathAtanh is an autogenerated function.
// func Atanh(x float64) float64
// Atanh returns the inverse hyperbolic tangent of x.
//
// Special cases are:
//
// Atanh(1) = +Inf
// Atanh(±0) = ±0
// Atanh(-1) = -Inf
// Atanh(x) = NaN if x < -1 or x > 1
// Atanh(NaN) = NaN
func MathAtanh(ctx context.Context, args []types.Value) (types.Value, error) {
	return &types.FloatValue{
		V: math.Atanh(args[0].Float()),
	}, nil
}

// MathCbrt is an autogenerated function.
// func Cbrt(x float64) float64
// Cbrt returns the cube root of x.
//
// Special cases are:
//
// Cbrt(±0) = ±0
// Cbrt(±Inf) = ±Inf
// Cbrt(NaN) = NaN
func MathCbrt(ctx context.Context, args []types.Value) (types.Value, error) {
	return &types.FloatValue{
		V: math.Cbrt(args[0].Float()),
	}, nil
}

// MathCeil is an autogenerated function.
// func Ceil(x float64) float64
// Ceil returns the least integer value greater than or equal to x.
//
// Special cases are:
//
// Ceil(±0) = ±0
// Ceil(±Inf) = ±Inf
// Ceil(NaN) = NaN
func MathCeil(ctx context.Context, args []types.Value) (types.Value, error) {
	return &types.FloatValue{
		V: math.Ceil(args[0].Float()),
	}, nil
}

// MathCopysign is an autogenerated function.
// func Copysign(f float64, sign float64) float64
// Copysign returns a value with the magnitude of f
// and the sign of sign.
func MathCopysign(ctx context.Context, args []types.Value) (types.Value, error) {
	return &types.FloatValue{
		V: math.Copysign(args[0].Float(), args[1].Float()),
	}, nil
}

// MathCos is an autogenerated function.
// func Cos(x float64) float64
// Cos returns the cosine of the radian argument x.
//
// Special cases are:
//
// Cos(±Inf) = NaN
// Cos(NaN) = NaN
func MathCos(ctx context.Context, args []types.Value) (types.Value, error) {
	return &types.FloatValue{
		V: math.Cos(args[0].Float()),
	}, nil
}

// MathCosh is an autogenerated function.
// func Cosh(x float64) float64
// Cosh returns the hyperbolic cosine of x.
//
// Special cases are:
//
// Cosh(±0) = 1
// Cosh(±Inf) = +Inf
// Cosh(NaN) = NaN
func MathCosh(ctx context.Context, args []types.Value) (types.Value, error) {
	return &types.FloatValue{
		V: math.Cosh(args[0].Float()),
	}, nil
}

// MathDim is an autogenerated function.
// func Dim(x float64, y float64) float64
// Dim returns the maximum of x-y or 0.
//
// Special cases are:
//
// Dim(+Inf, +Inf) = NaN
// Dim(-Inf, -Inf) = NaN
// Dim(x, NaN) = Dim(NaN, x) = NaN
func MathDim(ctx context.Context, args []types.Value) (types.Value, error) {
	return &types.FloatValue{
		V: math.Dim(args[0].Float(), args[1].Float()),
	}, nil
}

// MathErf is an autogenerated function.
// func Erf(x float64) float64
// Erf returns the error function of x.
//
// Special cases are:
//
// Erf(+Inf) = 1
// Erf(-Inf) = -1
// Erf(NaN) = NaN
func MathErf(ctx context.Context, args []types.Value) (types.Value, error) {
	return &types.FloatValue{
		V: math.Erf(args[0].Float()),
	}, nil
}

// MathErfc is an autogenerated function.
// func Erfc(x float64) float64
// Erfc returns the complementary error function of x.
//
// Special cases are:
//
// Erfc(+Inf) = 0
// Erfc(-Inf) = 2
// Erfc(NaN) = NaN
func MathErfc(ctx context.Context, args []types.Value) (types.Value, error) {
	return &types.FloatValue{
		V: math.Erfc(args[0].Float()),
	}, nil
}

// MathErfcinv is an autogenerated function.
// func Erfcinv(x float64) float64
// Erfcinv returns the inverse of [Erfc](x).
//
// Special cases are:
//
// Erfcinv(0) = +Inf
// Erfcinv(2) = -Inf
// Erfcinv(x) = NaN if x < 0 or x > 2
// Erfcinv(NaN) = NaN
func MathErfcinv(ctx context.Context, args []types.Value) (types.Value, error) {
	return &types.FloatValue{
		V: math.Erfcinv(args[0].Float()),
	}, nil
}

// MathErfinv is an autogenerated function.
// func Erfinv(x float64) float64
// Erfinv returns the inverse error function of x.
//
// Special cases are:
//
// Erfinv(1) = +Inf
// Erfinv(-1) = -Inf
// Erfinv(x) = NaN if x < -1 or x > 1
// Erfinv(NaN) = NaN
func MathErfinv(ctx context.Context, args []types.Value) (types.Value, error) {
	return &types.FloatValue{
		V: math.Erfinv(args[0].Float()),
	}, nil
}

// MathExp is an autogenerated function.
// func Exp(x float64) float64
// Exp returns e**x, the base-e exponential of x.
//
// Special cases are:
//
// Exp(+Inf) = +Inf
// Exp(NaN) = NaN
//
// Very large values overflow to 0 or +Inf.
// Very small values underflow to 1.
func MathExp(ctx context.Context, args []types.Value) (types.Value, error) {
	return &types.FloatValue{
		V: math.Exp(args[0].Float()),
	}, nil
}

// MathExp2 is an autogenerated function.
// func Exp2(x float64) float64
// Exp2 returns 2**x, the base-2 exponential of x.
//
// Special cases are the same as [Exp].
func MathExp2(ctx context.Context, args []types.Value) (types.Value, error) {
	return &types.FloatValue{
		V: math.Exp2(args[0].Float()),
	}, nil
}

// MathExpm1 is an autogenerated function.
// func Expm1(x float64) float64
// Expm1 returns e**x - 1, the base-e exponential of x minus 1.
// It is more accurate than [Exp](x) - 1 when x is near zero.
//
// Special cases are:
//
// Expm1(+Inf) = +Inf
// Expm1(-Inf) = -1
// Expm1(NaN) = NaN
//
// Very large values overflow to -1 or +Inf.
func MathExpm1(ctx context.Context, args []types.Value) (types.Value, error) {
	return &types.FloatValue{
		V: math.Expm1(args[0].Float()),
	}, nil
}

// MathFMA is an autogenerated function.
// func FMA(x float64, y float64, z float64) float64
// FMA returns x * y + z, computed with only one rounding.
// (That is, FMA returns the fused multiply-add of x, y, and z.)
func MathFMA(ctx context.Context, args []types.Value) (types.Value, error) {
	return &types.FloatValue{
		V: math.FMA(args[0].Float(), args[1].Float(), args[2].Float()),
	}, nil
}

// MathFloor is an autogenerated function.
// func Floor(x float64) float64
// Floor returns the greatest integer value less than or equal to x.
//
// Special cases are:
//
// Floor(±0) = ±0
// Floor(±Inf) = ±Inf
// Floor(NaN) = NaN
func MathFloor(ctx context.Context, args []types.Value) (types.Value, error) {
	return &types.FloatValue{
		V: math.Floor(args[0].Float()),
	}, nil
}

// MathGamma is an autogenerated function.
// func Gamma(x float64) float64
// Gamma returns the Gamma function of x.
//
// Special cases are:
//
// Gamma(+Inf) = +Inf
// Gamma(+0) = +Inf
// Gamma(-0) = -Inf
// Gamma(x) = NaN for integer x < 0
// Gamma(-Inf) = NaN
// Gamma(NaN) = NaN
func MathGamma(ctx context.Context, args []types.Value) (types.Value, error) {
	return &types.FloatValue{
		V: math.Gamma(args[0].Float()),
	}, nil
}

// MathHypot is an autogenerated function.
// func Hypot(p float64, q float64) float64
// Hypot returns [Sqrt](p*p + q*q), taking care to avoid
// unnecessary overflow and underflow.
//
// Special cases are:
//
// Hypot(±Inf, q) = +Inf
// Hypot(p, ±Inf) = +Inf
// Hypot(NaN, q) = NaN
// Hypot(p, NaN) = NaN
func MathHypot(ctx context.Context, args []types.Value) (types.Value, error) {
	return &types.FloatValue{
		V: math.Hypot(args[0].Float(), args[1].Float()),
	}, nil
}

// MathIlogb is an autogenerated function.
// func Ilogb(x float64) int
// Ilogb returns the binary exponent of x as an integer.
//
// Special cases are:
//
// Ilogb(±Inf) = MaxInt32
// Ilogb(0) = MinInt32
// Ilogb(NaN) = MaxInt32
func MathIlogb(ctx context.Context, args []types.Value) (types.Value, error) {
	return &types.IntValue{
		V: int64(math.Ilogb(args[0].Float())),
	}, nil
}

// MathInf is an autogenerated function.
// func Inf(sign int) float64
// Inf returns positive infinity if sign >= 0, negative infinity if sign < 0.
func MathInf(ctx context.Context, args []types.Value) (types.Value, error) {
	return &types.FloatValue{
		V: math.Inf(int(args[0].Int())),
	}, nil
}

// MathIsInf is an autogenerated function.
// func IsInf(f float64, sign int) bool
// IsInf reports whether f is an infinity, according to sign.
// If sign > 0, IsInf reports whether f is positive infinity.
// If sign < 0, IsInf reports whether f is negative infinity.
// If sign == 0, IsInf reports whether f is either infinity.
func MathIsInf(ctx context.Context, args []types.Value) (types.Value, error) {
	return &types.BoolValue{
		V: math.IsInf(args[0].Float(), int(args[1].Int())),
	}, nil
}

// MathIsNaN is an autogenerated function.
// func IsNaN(f float64) (is bool)
// IsNaN reports whether f is an IEEE 754 “not-a-number” value.
func MathIsNaN(ctx context.Context, args []types.Value) (types.Value, error) {
	return &types.BoolValue{
		V: math.IsNaN(args[0].Float()),
	}, nil
}

// MathJ0 is an autogenerated function.
// func J0(x float64) float64
// J0 returns the order-zero Bessel function of the first kind.
//
// Special cases are:
//
// J0(±Inf) = 0
// J0(0) = 1
// J0(NaN) = NaN
func MathJ0(ctx context.Context, args []types.Value) (types.Value, error) {
	return &types.FloatValue{
		V: math.J0(args[0].Float()),
	}, nil
}

// MathJ1 is an autogenerated function.
// func J1(x float64) float64
// J1 returns the order-one Bessel function of the first kind.
//
// Special cases are:
//
// J1(±Inf) = 0
// J1(NaN) = NaN
func MathJ1(ctx context.Context, args []types.Value) (types.Value, error) {
	return &types.FloatValue{
		V: math.J1(args[0].Float()),
	}, nil
}

// MathJn is an autogenerated function.
// func Jn(n int, x float64) float64
// Jn returns the order-n Bessel function of the first kind.
//
// Special cases are:
//
// Jn(n, ±Inf) = 0
// Jn(n, NaN) = NaN
func MathJn(ctx context.Context, args []types.Value) (types.Value, error) {
	return &types.FloatValue{
		V: math.Jn(int(args[0].Int()), args[1].Float()),
	}, nil
}

// MathLdexp is an autogenerated function.
// func Ldexp(frac float64, exp int) float64
// Ldexp is the inverse of [Frexp].
// It returns frac × 2**exp.
//
// Special cases are:
//
// Ldexp(±0, exp) = ±0
// Ldexp(±Inf, exp) = ±Inf
// Ldexp(NaN, exp) = NaN
func MathLdexp(ctx context.Context, args []types.Value) (types.Value, error) {
	return &types.FloatValue{
		V: math.Ldexp(args[0].Float(), int(args[1].Int())),
	}, nil
}

// MathLog is an autogenerated function.
// func Log(x float64) float64
// Log returns the natural logarithm of x.
//
// Special cases are:
//
// Log(+Inf) = +Inf
// Log(0) = -Inf
// Log(x < 0) = NaN
// Log(NaN) = NaN
func MathLog(ctx context.Context, args []types.Value) (types.Value, error) {
	return &types.FloatValue{
		V: math.Log(args[0].Float()),
	}, nil
}

// MathLog10 is an autogenerated function.
// func Log10(x float64) float64
// Log10 returns the decimal logarithm of x.
// The special cases are the same as for [Log].
func MathLog10(ctx context.Context, args []types.Value) (types.Value, error) {
	return &types.FloatValue{
		V: math.Log10(args[0].Float()),
	}, nil
}

// MathLog1p is an autogenerated function.
// func Log1p(x float64) float64
// Log1p returns the natural logarithm of 1 plus its argument x.
// It is more accurate than [Log](1 + x) when x is near zero.
//
// Special cases are:
//
// Log1p(+Inf) = +Inf
// Log1p(±0) = ±0
// Log1p(-1) = -Inf
// Log1p(x < -1) = NaN
// Log1p(NaN) = NaN
func MathLog1p(ctx context.Context, args []types.Value) (types.Value, error) {
	return &types.FloatValue{
		V: math.Log1p(args[0].Float()),
	}, nil
}

// MathLog2 is an autogenerated function.
// func Log2(x float64) float64
// Log2 returns the binary logarithm of x.
// The special cases are the same as for [Log].
func MathLog2(ctx context.Context, args []types.Value) (types.Value, error) {
	return &types.FloatValue{
		V: math.Log2(args[0].Float()),
	}, nil
}

// MathLogb is an autogenerated function.
// func Logb(x float64) float64
// Logb returns the binary exponent of x.
//
// Special cases are:
//
// Logb(±Inf) = +Inf
// Logb(0) = -Inf
// Logb(NaN) = NaN
func MathLogb(ctx context.Context, args []types.Value) (types.Value, error) {
	return &types.FloatValue{
		V: math.Logb(args[0].Float()),
	}, nil
}

// MathMax is an autogenerated function.
// func Max(x float64, y float64) float64
// Max returns the larger of x or y.
//
// Special cases are:
//
// Max(x, +Inf) = Max(+Inf, x) = +Inf
// Max(x, NaN) = Max(NaN, x) = NaN
// Max(+0, ±0) = Max(±0, +0) = +0
// Max(-0, -0) = -0
//
// Note that this differs from the built-in function max when called
// with NaN and +Inf.
func MathMax(ctx context.Context, args []types.Value) (types.Value, error) {
	return &types.FloatValue{
		V: math.Max(args[0].Float(), args[1].Float()),
	}, nil
}

// MathMin is an autogenerated function.
// func Min(x float64, y float64) float64
// Min returns the smaller of x or y.
//
// Special cases are:
//
// Min(x, -Inf) = Min(-Inf, x) = -Inf
// Min(x, NaN) = Min(NaN, x) = NaN
// Min(-0, ±0) = Min(±0, -0) = -0
//
// Note that this differs from the built-in function min when called
// with NaN and -Inf.
func MathMin(ctx context.Context, args []types.Value) (types.Value, error) {
	return &types.FloatValue{
		V: math.Min(args[0].Float(), args[1].Float()),
	}, nil
}

// MathMod is an autogenerated function.
// func Mod(x float64, y float64) float64
// Mod returns the floating-point remainder of x/y.
// The magnitude of the result is less than y and its
// sign agrees with that of x.
//
// Special cases are:
//
// Mod(±Inf, y) = NaN
// Mod(NaN, y) = NaN
// Mod(x, 0) = NaN
// Mod(x, ±Inf) = x
// Mod(x, NaN) = NaN
func MathMod(ctx context.Context, args []types.Value) (types.Value, error) {
	return &types.FloatValue{
		V: math.Mod(args[0].Float(), args[1].Float()),
	}, nil
}

// MathNaN is an autogenerated function.
// func NaN() float64
// NaN returns an IEEE 754 “not-a-number” value.
func MathNaN(ctx context.Context, args []types.Value) (types.Value, error) {
	return &types.FloatValue{
		V: math.NaN(),
	}, nil
}

// MathNextafter is an autogenerated function.
// func Nextafter(x float64, y float64) (r float64)
// Nextafter returns the next representable float64 value after x towards y.
//
// Special cases are:
//
// Nextafter(x, x)   = x
// Nextafter(NaN, y) = NaN
// Nextafter(x, NaN) = NaN
func MathNextafter(ctx context.Context, args []types.Value) (types.Value, error) {
	return &types.FloatValue{
		V: math.Nextafter(args[0].Float(), args[1].Float()),
	}, nil
}

// MathPow is an autogenerated function.
// func Pow(x float64, y float64) float64
// Pow returns x**y, the base-x exponential of y.
//
// Special cases are (in order):
//
// Pow(x, ±0) = 1 for any x
// Pow(1, y) = 1 for any y
// Pow(x, 1) = x for any x
// Pow(NaN, y) = NaN
// Pow(x, NaN) = NaN
// Pow(±0, y) = ±Inf for y an odd integer < 0
// Pow(±0, -Inf) = +Inf
// Pow(±0, +Inf) = +0
// Pow(±0, y) = +Inf for finite y < 0 and not an odd integer
// Pow(±0, y) = ±0 for y an odd integer > 0
// Pow(±0, y) = +0 for finite y > 0 and not an odd integer
// Pow(-1, ±Inf) = 1
// Pow(x, +Inf) = +Inf for |x| > 1
// Pow(x, -Inf) = +0 for |x| > 1
// Pow(x, +Inf) = +0 for |x| < 1
// Pow(x, -Inf) = +Inf for |x| < 1
// Pow(+Inf, y) = +Inf for y > 0
// Pow(+Inf, y) = +0 for y < 0
// Pow(-Inf, y) = Pow(-0, -y)
// Pow(x, y) = NaN for finite x < 0 and finite non-integer y
func MathPow(ctx context.Context, args []types.Value) (types.Value, error) {
	return &types.FloatValue{
		V: math.Pow(args[0].Float(), args[1].Float()),
	}, nil
}

// MathPow10 is an autogenerated function.
// func Pow10(n int) float64
// Pow10 returns 10**n, the base-10 exponential of n.
//
// Special cases are:
//
// Pow10(n) =    0 for n < -323
// Pow10(n) = +Inf for n > 308
func MathPow10(ctx context.Context, args []types.Value) (types.Value, error) {
	return &types.FloatValue{
		V: math.Pow10(int(args[0].Int())),
	}, nil
}

// MathRemainder is an autogenerated function.
// func Remainder(x float64, y float64) float64
// Remainder returns the IEEE 754 floating-point remainder of x/y.
//
// Special cases are:
//
// Remainder(±Inf, y) = NaN
// Remainder(NaN, y) = NaN
// Remainder(x, 0) = NaN
// Remainder(x, ±Inf) = x
// Remainder(x, NaN) = NaN
func MathRemainder(ctx context.Context, args []types.Value) (types.Value, error) {
	return &types.FloatValue{
		V: math.Remainder(args[0].Float(), args[1].Float()),
	}, nil
}

// MathRound is an autogenerated function.
// func Round(x float64) float64
// Round returns the nearest integer, rounding half away from zero.
//
// Special cases are:
//
// Round(±0) = ±0
// Round(±Inf) = ±Inf
// Round(NaN) = NaN
func MathRound(ctx context.Context, args []types.Value) (types.Value, error) {
	return &types.FloatValue{
		V: math.Round(args[0].Float()),
	}, nil
}

// MathRoundToEven is an autogenerated function.
// func RoundToEven(x float64) float64
// RoundToEven returns the nearest integer, rounding ties to even.
//
// Special cases are:
//
// RoundToEven(±0) = ±0
// RoundToEven(±Inf) = ±Inf
// RoundToEven(NaN) = NaN
func MathRoundToEven(ctx context.Context, args []types.Value) (types.Value, error) {
	return &types.FloatValue{
		V: math.RoundToEven(args[0].Float()),
	}, nil
}

// MathSignbit is an autogenerated function.
// func Signbit(x float64) bool
// Signbit reports whether x is negative or negative zero.
func MathSignbit(ctx context.Context, args []types.Value) (types.Value, error) {
	return &types.BoolValue{
		V: math.Signbit(args[0].Float()),
	}, nil
}

// MathSin is an autogenerated function.
// func Sin(x float64) float64
// Sin returns the sine of the radian argument x.
//
// Special cases are:
//
// Sin(±0) = ±0
// Sin(±Inf) = NaN
// Sin(NaN) = NaN
func MathSin(ctx context.Context, args []types.Value) (types.Value, error) {
	return &types.FloatValue{
		V: math.Sin(args[0].Float()),
	}, nil
}

// MathSinh is an autogenerated function.
// func Sinh(x float64) float64
// Sinh returns the hyperbolic sine of x.
//
// Special cases are:
//
// Sinh(±0) = ±0
// Sinh(±Inf) = ±Inf
// Sinh(NaN) = NaN
func MathSinh(ctx context.Context, args []types.Value) (types.Value, error) {
	return &types.FloatValue{
		V: math.Sinh(args[0].Float()),
	}, nil
}

// MathSqrt is an autogenerated function.
// func Sqrt(x float64) float64
// Sqrt returns the square root of x.
//
// Special cases are:
//
// Sqrt(+Inf) = +Inf
// Sqrt(±0) = ±0
// Sqrt(x < 0) = NaN
// Sqrt(NaN) = NaN
func MathSqrt(ctx context.Context, args []types.Value) (types.Value, error) {
	return &types.FloatValue{
		V: math.Sqrt(args[0].Float()),
	}, nil
}

// MathTan is an autogenerated function.
// func Tan(x float64) float64
// Tan returns the tangent of the radian argument x.
//
// Special cases are:
//
// Tan(±0) = ±0
// Tan(±Inf) = NaN
// Tan(NaN) = NaN
func MathTan(ctx context.Context, args []types.Value) (types.Value, error) {
	return &types.FloatValue{
		V: math.Tan(args[0].Float()),
	}, nil
}

// MathTanh is an autogenerated function.
// func Tanh(x float64) float64
// Tanh returns the hyperbolic tangent of x.
//
// Special cases are:
//
// Tanh(±0) = ±0
// Tanh(±Inf) = ±1
// Tanh(NaN) = NaN
func MathTanh(ctx context.Context, args []types.Value) (types.Value, error) {
	return &types.FloatValue{
		V: math.Tanh(args[0].Float()),
	}, nil
}

// MathTrunc is an autogenerated function.
// func Trunc(x float64) float64
// Trunc returns the integer value of x.
//
// Special cases are:
//
// Trunc(±0) = ±0
// Trunc(±Inf) = ±Inf
// Trunc(NaN) = NaN
func MathTrunc(ctx context.Context, args []types.Value) (types.Value, error) {
	return &types.FloatValue{
		V: math.Trunc(args[0].Float()),
	}, nil
}

// MathY0 is an autogenerated function.
// func Y0(x float64) float64
// Y0 returns the order-zero Bessel function of the second kind.
//
// Special cases are:
//
// Y0(+Inf) = 0
// Y0(0) = -Inf
// Y0(x < 0) = NaN
// Y0(NaN) = NaN
func MathY0(ctx context.Context, args []types.Value) (types.Value, error) {
	return &types.FloatValue{
		V: math.Y0(args[0].Float()),
	}, nil
}

// MathY1 is an autogenerated function.
// func Y1(x float64) float64
// Y1 returns the order-one Bessel function of the second kind.
//
// Special cases are:
//
// Y1(+Inf) = 0
// Y1(0) = -Inf
// Y1(x < 0) = NaN
// Y1(NaN) = NaN
func MathY1(ctx context.Context, args []types.Value) (types.Value, error) {
	return &types.FloatValue{
		V: math.Y1(args[0].Float()),
	}, nil
}

// MathYn is an autogenerated function.
// func Yn(n int, x float64) float64
// Yn returns the order-n Bessel function of the second kind.
//
// Special cases are:
//
// Yn(n, +Inf) = 0
// Yn(n ≥ 0, 0) = -Inf
// Yn(n < 0, 0) = +Inf if n is odd, -Inf if n is even
// Yn(n, x < 0) = NaN
// Yn(n, NaN) = NaN
func MathYn(ctx context.Context, args []types.Value) (types.Value, error) {
	return &types.FloatValue{
		V: math.Yn(int(args[0].Int()), args[1].Float()),
	}, nil
}

// MathrandExpFloat64 is an autogenerated function.
// func ExpFloat64() float64
// ExpFloat64 returns an exponentially distributed float64 in the range
// (0, +[math.MaxFloat64]] with an exponential distribution whose rate parameter
// (lambda) is 1 and whose mean is 1/lambda (1) from the default [Source].
// To produce a distribution with a different rate parameter,
// callers can adjust the output using:
//
// sample = ExpFloat64() / desiredRateParameter
func MathrandExpFloat64(ctx context.Context, args []types.Value) (types.Value, error) {
	return &types.FloatValue{
		V: rand.ExpFloat64(),
	}, nil
}

// MathrandFloat64 is an autogenerated function.
// func Float64() float64
// Float64 returns, as a float64, a pseudo-random number in the half-open interval [0.0,1.0)
// from the default [Source].
func MathrandFloat64(ctx context.Context, args []types.Value) (types.Value, error) {
	return &types.FloatValue{
		V: rand.Float64(),
	}, nil
}

// MathrandInt is an autogenerated function.
// func Int() int
// Int returns a non-negative pseudo-random int from the default [Source].
func MathrandInt(ctx context.Context, args []types.Value) (types.Value, error) {
	return &types.IntValue{
		V: int64(rand.Int()),
	}, nil
}

// MathrandInt63 is an autogenerated function.
// func Int63() int64
// Int63 returns a non-negative pseudo-random 63-bit integer as an int64
// from the default [Source].
func MathrandInt63(ctx context.Context, args []types.Value) (types.Value, error) {
	return &types.IntValue{
		V: rand.Int63(),
	}, nil
}

// MathrandInt63n is an autogenerated function.
// func Int63n(n int64) int64
// Int63n returns, as an int64, a non-negative pseudo-random number in the half-open interval [0,n)
// from the default [Source].
// It panics if n <= 0.
func MathrandInt63n(ctx context.Context, args []types.Value) (types.Value, error) {
	return &types.IntValue{
		V: rand.Int63n(args[0].Int()),
	}, nil
}

// MathrandIntn is an autogenerated function.
// func Intn(n int) int
// Intn returns, as an int, a non-negative pseudo-random number in the half-open interval [0,n)
// from the default [Source].
// It panics if n <= 0.
func MathrandIntn(ctx context.Context, args []types.Value) (types.Value, error) {
	return &types.IntValue{
		V: int64(rand.Intn(int(args[0].Int()))),
	}, nil
}

// MathrandNormFloat64 is an autogenerated function.
// func NormFloat64() float64
// NormFloat64 returns a normally distributed float64 in the range
// [-[math.MaxFloat64], +[math.MaxFloat64]] with
// standard normal distribution (mean = 0, stddev = 1)
// from the default [Source].
// To produce a different normal distribution, callers can
// adjust the output using:
//
// sample = NormFloat64() * desiredStdDev + desiredMean
func MathrandNormFloat64(ctx context.Context, args []types.Value) (types.Value, error) {
	return &types.FloatValue{
		V: rand.NormFloat64(),
	}, nil
}

// MathrandRead is an autogenerated function.
// func Read(p []byte) (n int, err error)
// Read generates len(p) random bytes from the default [Source] and
// writes them into p. It always returns len(p) and a nil error.
// Read, unlike the [Rand.Read] method, is safe for concurrent use.
//
// Deprecated: For almost all use cases, [crypto/rand.Read] is more appropriate.
// If a deterministic source is required, use [math/rand/v2.ChaCha8.Read].
func MathrandRead(ctx context.Context, args []types.Value) (types.Value, error) {
	v, err := rand.Read([]byte(args[0].Str()))
	if err != nil {
		return nil, err
	}
	return &types.IntValue{
		V: int64(v),
	}, nil
}

// OsExecutable is an autogenerated function.
// func Executable() (string, error)
// Executable returns the path name for the executable that started
// the current process. There is no guarantee that the path is still
// pointing to the correct executable. If a symlink was used to start
// the process, depending on the operating system, the result might
// be the symlink or the path it pointed to. If a stable result is
// needed, [path/filepath.EvalSymlinks] might help.
//
// Executable returns an absolute path unless an error occurred.
//
// The main use case is finding resources located relative to an
// executable.
func OsExecutable(ctx context.Context, args []types.Value) (types.Value, error) {
	v, err := os.Executable()
	if err != nil {
		return nil, err
	}
	return &types.StrValue{
		V: v,
	}, nil
}

// OsExpandEnv is an autogenerated function.
// func ExpandEnv(s string) string
// ExpandEnv replaces ${var} or $var in the string according to the values
// of the current environment variables. References to undefined
// variables are replaced by the empty string.
func OsExpandEnv(ctx context.Context, args []types.Value) (types.Value, error) {
	return &types.StrValue{
		V: os.ExpandEnv(args[0].Str()),
	}, nil
}

// OsGetegid is an autogenerated function.
// func Getegid() int
// Getegid returns the numeric effective group id of the caller.
//
// On Windows, it returns -1.
func OsGetegid(ctx context.Context, args []types.Value) (types.Value, error) {
	return &types.IntValue{
		V: int64(os.Getegid()),
	}, nil
}

// OsGetenv is an autogenerated function.
// func Getenv(key string) string
// Getenv retrieves the value of the environment variable named by the key.
// It returns the value, which will be empty if the variable is not present.
// To distinguish between an empty value and an unset value, use [LookupEnv].
func OsGetenv(ctx context.Context, args []types.Value) (types.Value, error) {
	return &types.StrValue{
		V: os.Getenv(args[0].Str()),
	}, nil
}

// OsGeteuid is an autogenerated function.
// func Geteuid() int
// Geteuid returns the numeric effective user id of the caller.
//
// On Windows, it returns -1.
func OsGeteuid(ctx context.Context, args []types.Value) (types.Value, error) {
	return &types.IntValue{
		V: int64(os.Geteuid()),
	}, nil
}

// OsGetgid is an autogenerated function.
// func Getgid() int
// Getgid returns the numeric group id of the caller.
//
// On Windows, it returns -1.
func OsGetgid(ctx context.Context, args []types.Value) (types.Value, error) {
	return &types.IntValue{
		V: int64(os.Getgid()),
	}, nil
}

// OsGetpagesize is an autogenerated function.
// func Getpagesize() int
// Getpagesize returns the underlying system's memory page size.
func OsGetpagesize(ctx context.Context, args []types.Value) (types.Value, error) {
	return &types.IntValue{
		V: int64(os.Getpagesize()),
	}, nil
}

// OsGetpid is an autogenerated function.
// func Getpid() int
// Getpid returns the process id of the caller.
func OsGetpid(ctx context.Context, args []types.Value) (types.Value, error) {
	return &types.IntValue{
		V: int64(os.Getpid()),
	}, nil
}

// OsGetppid is an autogenerated function.
// func Getppid() int
// Getppid returns the process id of the caller's parent.
func OsGetppid(ctx context.Context, args []types.Value) (types.Value, error) {
	return &types.IntValue{
		V: int64(os.Getppid()),
	}, nil
}

// OsGetuid is an autogenerated function.
// func Getuid() int
// Getuid returns the numeric user id of the caller.
//
// On Windows, it returns -1.
func OsGetuid(ctx context.Context, args []types.Value) (types.Value, error) {
	return &types.IntValue{
		V: int64(os.Getuid()),
	}, nil
}

// OsGetwd is an autogenerated function.
// func Getwd() (dir string, err error)
// Getwd returns an absolute path name corresponding to the
// current directory. If the current directory can be
// reached via multiple paths (due to symbolic links),
// Getwd may return any one of them.
//
// On Unix platforms, if the environment variable PWD
// provides an absolute name, and it is a name of the
// current directory, it is returned.
func OsGetwd(ctx context.Context, args []types.Value) (types.Value, error) {
	v, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	return &types.StrValue{
		V: v,
	}, nil
}

// OsHostname is an autogenerated function.
// func Hostname() (name string, err error)
// Hostname returns the host name reported by the kernel.
func OsHostname(ctx context.Context, args []types.Value) (types.Value, error) {
	v, err := os.Hostname()
	if err != nil {
		return nil, err
	}
	return &types.StrValue{
		V: v,
	}, nil
}

// OsMkdirTemp is an autogenerated function.
// func MkdirTemp(dir string, pattern string) (string, error)
// MkdirTemp creates a new temporary directory in the directory dir
// and returns the pathname of the new directory.
// The new directory's name is generated by adding a random string to the end of pattern.
// If pattern includes a "*", the random string replaces the last "*" instead.
// The directory is created with mode 0o700 (before umask).
// If dir is the empty string, MkdirTemp uses the default directory for temporary files, as returned by TempDir.
// Multiple programs or goroutines calling MkdirTemp simultaneously will not choose the same directory.
// It is the caller's responsibility to remove the directory when it is no longer needed.
func OsMkdirTemp(ctx context.Context, args []types.Value) (types.Value, error) {
	v, err := os.MkdirTemp(args[0].Str(), args[1].Str())
	if err != nil {
		return nil, err
	}
	return &types.StrValue{
		V: v,
	}, nil
}

// OsReadFile is an autogenerated function.
// func ReadFile(name string) ([]byte, error)
// ReadFile reads the named file and returns the contents.
// A successful call returns err == nil, not err == EOF.
// Because ReadFile reads the whole file, it does not treat an EOF from Read
// as an error to be reported.
func OsReadFile(ctx context.Context, args []types.Value) (types.Value, error) {
	v, err := os.ReadFile(args[0].Str())
	if err != nil {
		return nil, err
	}
	return &types.StrValue{
		V: string(v),
	}, nil
}

// OsReadlink is an autogenerated function.
// func Readlink(name string) (string, error)
// Readlink returns the destination of the named symbolic link.
// If there is an error, it will be of type *PathError.
//
// If the link destination is relative, Readlink returns the relative path
// without resolving it to an absolute one.
func OsReadlink(ctx context.Context, args []types.Value) (types.Value, error) {
	v, err := os.Readlink(args[0].Str())
	if err != nil {
		return nil, err
	}
	return &types.StrValue{
		V: v,
	}, nil
}

// OsTempDir is an autogenerated function.
// func TempDir() string
// TempDir returns the default directory to use for temporary files.
//
// On Unix systems, it returns $TMPDIR if non-empty, else /tmp.
// On Windows, it uses GetTempPath, returning the first non-empty
// value from %TMP%, %TEMP%, %USERPROFILE%, or the Windows directory.
// On Plan 9, it returns /tmp.
//
// The directory is neither guaranteed to exist nor have accessible
// permissions.
func OsTempDir(ctx context.Context, args []types.Value) (types.Value, error) {
	return &types.StrValue{
		V: os.TempDir(),
	}, nil
}

// OsUserCacheDir is an autogenerated function.
// func UserCacheDir() (string, error)
// UserCacheDir returns the default root directory to use for user-specific
// cached data. Users should create their own application-specific subdirectory
// within this one and use that.
//
// On Unix systems, it returns $XDG_CACHE_HOME as specified by
// https://specifications.freedesktop.org/basedir-spec/basedir-spec-latest.html if
// non-empty, else $HOME/.cache.
// On Darwin, it returns $HOME/Library/Caches.
// On Windows, it returns %LocalAppData%.
// On Plan 9, it returns $home/lib/cache.
//
// If the location cannot be determined (for example, $HOME is not defined) or
// the path in $XDG_CACHE_HOME is relative, then it will return an error.
func OsUserCacheDir(ctx context.Context, args []types.Value) (types.Value, error) {
	v, err := os.UserCacheDir()
	if err != nil {
		return nil, err
	}
	return &types.StrValue{
		V: v,
	}, nil
}

// OsUserConfigDir is an autogenerated function.
// func UserConfigDir() (string, error)
// UserConfigDir returns the default root directory to use for user-specific
// configuration data. Users should create their own application-specific
// subdirectory within this one and use that.
//
// On Unix systems, it returns $XDG_CONFIG_HOME as specified by
// https://specifications.freedesktop.org/basedir-spec/basedir-spec-latest.html if
// non-empty, else $HOME/.config.
// On Darwin, it returns $HOME/Library/Application Support.
// On Windows, it returns %AppData%.
// On Plan 9, it returns $home/lib.
//
// If the location cannot be determined (for example, $HOME is not defined) or
// the path in $XDG_CONFIG_HOME is relative, then it will return an error.
func OsUserConfigDir(ctx context.Context, args []types.Value) (types.Value, error) {
	v, err := os.UserConfigDir()
	if err != nil {
		return nil, err
	}
	return &types.StrValue{
		V: v,
	}, nil
}

// OsUserHomeDir is an autogenerated function.
// func UserHomeDir() (string, error)
// UserHomeDir returns the current user's home directory.
//
// On Unix, including macOS, it returns the $HOME environment variable.
// On Windows, it returns %USERPROFILE%.
// On Plan 9, it returns the $home environment variable.
//
// If the expected variable is not set in the environment, UserHomeDir
// returns either a platform-specific default value or a non-nil error.
func OsUserHomeDir(ctx context.Context, args []types.Value) (types.Value, error) {
	v, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	return &types.StrValue{
		V: v,
	}, nil
}

// OsexecLookPath is an autogenerated function.
// func LookPath(file string) (string, error)
// LookPath searches for an executable named file in the
// directories named by the PATH environment variable.
// If file contains a slash, it is tried directly and the PATH is not consulted.
// Otherwise, on success, the result is an absolute path.
//
// In older versions of Go, LookPath could return a path relative to the current directory.
// As of Go 1.19, LookPath will instead return that path along with an error satisfying
// [errors.Is](err, [ErrDot]). See the package documentation for more details.
func OsexecLookPath(ctx context.Context, args []types.Value) (types.Value, error) {
	v, err := exec.LookPath(args[0].Str())
	if err != nil {
		return nil, err
	}
	return &types.StrValue{
		V: v,
	}, nil
}

// PathBase is an autogenerated function.
// func Base(path string) string
// Base returns the last element of path.
// Trailing slashes are removed before extracting the last element.
// If the path is empty, Base returns ".".
// If the path consists entirely of slashes, Base returns "/".
func PathBase(ctx context.Context, args []types.Value) (types.Value, error) {
	return &types.StrValue{
		V: path.Base(args[0].Str()),
	}, nil
}

// PathClean is an autogenerated function.
// func Clean(path string) string
// Clean returns the shortest path name equivalent to path
// by purely lexical processing. It applies the following rules
// iteratively until no further processing can be done:
//
// 1. Replace multiple slashes with a single slash.
// 2. Eliminate each . path name element (the current directory).
// 3. Eliminate each inner .. path name element (the parent directory)
// along with the non-.. element that precedes it.
// 4. Eliminate .. elements that begin a rooted path:
// that is, replace "/.." by "/" at the beginning of a path.
//
// The returned path ends in a slash only if it is the root "/".
//
// If the result of this process is an empty string, Clean
// returns the string ".".
//
// See also Rob Pike, “Lexical File Names in Plan 9 or
// Getting Dot-Dot Right,”
// https://9p.io/sys/doc/lexnames.html
func PathClean(ctx context.Context, args []types.Value) (types.Value, error) {
	return &types.StrValue{
		V: path.Clean(args[0].Str()),
	}, nil
}

// PathDir is an autogenerated function.
// func Dir(path string) string
// Dir returns all but the last element of path, typically the path's directory.
// After dropping the final element using [Split], the path is Cleaned and trailing
// slashes are removed.
// If the path is empty, Dir returns ".".
// If the path consists entirely of slashes followed by non-slash bytes, Dir
// returns a single slash. In any other case, the returned path does not end in a
// slash.
func PathDir(ctx context.Context, args []types.Value) (types.Value, error) {
	return &types.StrValue{
		V: path.Dir(args[0].Str()),
	}, nil
}

// PathExt is an autogenerated function.
// func Ext(path string) string
// Ext returns the file name extension used by path.
// The extension is the suffix beginning at the final dot
// in the final slash-separated element of path;
// it is empty if there is no dot.
func PathExt(ctx context.Context, args []types.Value) (types.Value, error) {
	return &types.StrValue{
		V: path.Ext(args[0].Str()),
	}, nil
}

// PathIsAbs is an autogenerated function.
// func IsAbs(path string) bool
// IsAbs reports whether the path is absolute.
func PathIsAbs(ctx context.Context, args []types.Value) (types.Value, error) {
	return &types.BoolValue{
		V: path.IsAbs(args[0].Str()),
	}, nil
}

// PathJoin is an autogenerated function.
// func Join(elem ...string) string
// Join joins any number of path elements into a single path,
// separating them with slashes. Empty elements are ignored.
// The result is Cleaned. However, if the argument list is
// empty or all its elements are empty, Join returns
// an empty string.
func PathJoin(ctx context.Context, args []types.Value) (types.Value, error) {
	return &types.StrValue{
		V: path.Join(util.MclListToGolang(args[0])...),
	}, nil
}

// PathMatch is an autogenerated function.
// func Match(pattern string, name string) (matched bool, err error)
// Match reports whether name matches the shell pattern.
// The pattern syntax is:
//
// pattern:
// { term }
// term:
// '*'         matches any sequence of non-/ characters
// '?'         matches any single non-/ character
// '[' [ '^' ] { character-range } ']'
// character class (must be non-empty)
// c           matches character c (c != '*', '?', '\\', '[')
// '\\' c      matches character c
//
// character-range:
// c           matches character c (c != '\\', '-', ']')
// '\\' c      matches character c
// lo '-' hi   matches character c for lo <= c <= hi
//
// Match requires pattern to match all of name, not just a substring.
// The only possible returned error is [ErrBadPattern], when pattern
// is malformed.
func PathMatch(ctx context.Context, args []types.Value) (types.Value, error) {
	v, err := path.Match(args[0].Str(), args[1].Str())
	if err != nil {
		return nil, err
	}
	return &types.BoolValue{
		V: v,
	}, nil
}

// PathfilepathAbs is an autogenerated function.
// func Abs(path string) (string, error)
// Abs returns an absolute representation of path.
// If the path is not absolute it will be joined with the current
// working directory to turn it into an absolute path. The absolute
// path name for a given file is not guaranteed to be unique.
// Abs calls [Clean] on the result.
func PathfilepathAbs(ctx context.Context, args []types.Value) (types.Value, error) {
	v, err := filepath.Abs(args[0].Str())
	if err != nil {
		return nil, err
	}
	return &types.StrValue{
		V: v,
	}, nil
}

// PathfilepathBase is an autogenerated function.
// func Base(path string) string
// Base returns the last element of path.
// Trailing path separators are removed before extracting the last element.
// If the path is empty, Base returns ".".
// If the path consists entirely of separators, Base returns a single separator.
func PathfilepathBase(ctx context.Context, args []types.Value) (types.Value, error) {
	return &types.StrValue{
		V: filepath.Base(args[0].Str()),
	}, nil
}

// PathfilepathClean is an autogenerated function.
// func Clean(path string) string
// Clean returns the shortest path name equivalent to path
// by purely lexical processing. It applies the following rules
// iteratively until no further processing can be done:
//
// 1. Replace multiple [Separator] elements with a single one.
// 2. Eliminate each . path name element (the current directory).
// 3. Eliminate each inner .. path name element (the parent directory)
// along with the non-.. element that precedes it.
// 4. Eliminate .. elements that begin a rooted path:
// that is, replace "/.." by "/" at the beginning of a path,
// assuming Separator is '/'.
//
// The returned path ends in a slash only if it represents a root directory,
// such as "/" on Unix or `C:\` on Windows.
//
// Finally, any occurrences of slash are replaced by Separator.
//
// If the result of this process is an empty string, Clean
// returns the string ".".
//
// On Windows, Clean does not modify the volume name other than to replace
// occurrences of "/" with `\`.
// For example, Clean("//host/share/../x") returns `\\host\share\x`.
//
// See also Rob Pike, “Lexical File Names in Plan 9 or
// Getting Dot-Dot Right,”
// https://9p.io/sys/doc/lexnames.html
func PathfilepathClean(ctx context.Context, args []types.Value) (types.Value, error) {
	return &types.StrValue{
		V: filepath.Clean(args[0].Str()),
	}, nil
}

// PathfilepathDir is an autogenerated function.
// func Dir(path string) string
// Dir returns all but the last element of path, typically the path's directory.
// After dropping the final element, Dir calls [Clean] on the path and trailing
// slashes are removed.
// If the path is empty, Dir returns ".".
// If the path consists entirely of separators, Dir returns a single separator.
// The returned path does not end in a separator unless it is the root directory.
func PathfilepathDir(ctx context.Context, args []types.Value) (types.Value, error) {
	return &types.StrValue{
		V: filepath.Dir(args[0].Str()),
	}, nil
}

// PathfilepathEvalSymlinks is an autogenerated function.
// func EvalSymlinks(path string) (string, error)
// EvalSymlinks returns the path name after the evaluation of any symbolic
// links.
// If path is relative the result will be relative to the current directory,
// unless one of the components is an absolute symbolic link.
// EvalSymlinks calls [Clean] on the result.
func PathfilepathEvalSymlinks(ctx context.Context, args []types.Value) (types.Value, error) {
	v, err := filepath.EvalSymlinks(args[0].Str())
	if err != nil {
		return nil, err
	}
	return &types.StrValue{
		V: v,
	}, nil
}

// PathfilepathExt is an autogenerated function.
// func Ext(path string) string
// Ext returns the file name extension used by path.
// The extension is the suffix beginning at the final dot
// in the final element of path; it is empty if there is
// no dot.
func PathfilepathExt(ctx context.Context, args []types.Value) (types.Value, error) {
	return &types.StrValue{
		V: filepath.Ext(args[0].Str()),
	}, nil
}

// PathfilepathFromSlash is an autogenerated function.
// func FromSlash(path string) string
// FromSlash returns the result of replacing each slash ('/') character
// in path with a separator character. Multiple slashes are replaced
// by multiple separators.
//
// See also the Localize function, which converts a slash-separated path
// as used by the io/fs package to an operating system path.
func PathfilepathFromSlash(ctx context.Context, args []types.Value) (types.Value, error) {
	return &types.StrValue{
		V: filepath.FromSlash(args[0].Str()),
	}, nil
}

// PathfilepathHasPrefix is an autogenerated function.
// func HasPrefix(p string, prefix string) bool
// HasPrefix exists for historical compatibility and should not be used.
//
// Deprecated: HasPrefix does not respect path boundaries and
// does not ignore case when required.
func PathfilepathHasPrefix(ctx context.Context, args []types.Value) (types.Value, error) {
	return &types.BoolValue{
		V: filepath.HasPrefix(args[0].Str(), args[1].Str()),
	}, nil
}

// PathfilepathIsAbs is an autogenerated function.
// func IsAbs(path string) bool
// IsAbs reports whether the path is absolute.
func PathfilepathIsAbs(ctx context.Context, args []types.Value) (types.Value, error) {
	return &types.BoolValue{
		V: filepath.IsAbs(args[0].Str()),
	}, nil
}

// PathfilepathIsLocal is an autogenerated function.
// func IsLocal(path string) bool
// IsLocal reports whether path, using lexical analysis only, has all of these properties:
//
// - is within the subtree rooted at the directory in which path is evaluated
// - is not an absolute path
// - is not empty
// - on Windows, is not a reserved name such as "NUL"
//
// If IsLocal(path) returns true, then
// Join(base, path) will always produce a path contained within base and
// Clean(path) will always produce an unrooted path with no ".." path elements.
//
// IsLocal is a purely lexical operation.
// In particular, it does not account for the effect of any symbolic links
// that may exist in the filesystem.
func PathfilepathIsLocal(ctx context.Context, args []types.Value) (types.Value, error) {
	return &types.BoolValue{
		V: filepath.IsLocal(args[0].Str()),
	}, nil
}

// PathfilepathJoin is an autogenerated function.
// func Join(elem ...string) string
// Join joins any number of path elements into a single path,
// separating them with an OS specific [Separator]. Empty elements
// are ignored. The result is Cleaned. However, if the argument
// list is empty or all its elements are empty, Join returns
// an empty string.
// On Windows, the result will only be a UNC path if the first
// non-empty element is a UNC path.
func PathfilepathJoin(ctx context.Context, args []types.Value) (types.Value, error) {
	return &types.StrValue{
		V: filepath.Join(util.MclListToGolang(args[0])...),
	}, nil
}

// PathfilepathLocalize is an autogenerated function.
// func Localize(path string) (string, error)
// Localize converts a slash-separated path into an operating system path.
// The input path must be a valid path as reported by [io/fs.ValidPath].
//
// Localize returns an error if the path cannot be represented by the operating system.
// For example, the path a\b is rejected on Windows, on which \ is a separator
// character and cannot be part of a filename.
//
// The path returned by Localize will always be local, as reported by IsLocal.
func PathfilepathLocalize(ctx context.Context, args []types.Value) (types.Value, error) {
	v, err := filepath.Localize(args[0].Str())
	if err != nil {
		return nil, err
	}
	return &types.StrValue{
		V: v,
	}, nil
}

// PathfilepathMatch is an autogenerated function.
// func Match(pattern string, name string) (matched bool, err error)
// Match reports whether name matches the shell file name pattern.
// The pattern syntax is:
//
// pattern:
// { term }
// term:
// '*'         matches any sequence of non-Separator characters
// '?'         matches any single non-Separator character
// '[' [ '^' ] { character-range } ']'
// character class (must be non-empty)
// c           matches character c (c != '*', '?', '\\', '[')
// '\\' c      matches character c
//
// character-range:
// c           matches character c (c != '\\', '-', ']')
// '\\' c      matches character c
// lo '-' hi   matches character c for lo <= c <= hi
//
// Match requires pattern to match all of name, not just a substring.
// The only possible returned error is [ErrBadPattern], when pattern
// is malformed.
//
// On Windows, escaping is disabled. Instead, '\\' is treated as
// path separator.
func PathfilepathMatch(ctx context.Context, args []types.Value) (types.Value, error) {
	v, err := filepath.Match(args[0].Str(), args[1].Str())
	if err != nil {
		return nil, err
	}
	return &types.BoolValue{
		V: v,
	}, nil
}

// PathfilepathRel is an autogenerated function.
// func Rel(basepath string, targpath string) (string, error)
// Rel returns a relative path that is lexically equivalent to targpath when
// joined to basepath with an intervening separator. That is,
// [Join](basepath, Rel(basepath, targpath)) is equivalent to targpath itself.
// On success, the returned path will always be relative to basepath,
// even if basepath and targpath share no elements.
// An error is returned if targpath can't be made relative to basepath or if
// knowing the current working directory would be necessary to compute it.
// Rel calls [Clean] on the result.
func PathfilepathRel(ctx context.Context, args []types.Value) (types.Value, error) {
	v, err := filepath.Rel(args[0].Str(), args[1].Str())
	if err != nil {
		return nil, err
	}
	return &types.StrValue{
		V: v,
	}, nil
}

// PathfilepathToSlash is an autogenerated function.
// func ToSlash(path string) string
// ToSlash returns the result of replacing each separator character
// in path with a slash ('/') character. Multiple separators are
// replaced by multiple slashes.
func PathfilepathToSlash(ctx context.Context, args []types.Value) (types.Value, error) {
	return &types.StrValue{
		V: filepath.ToSlash(args[0].Str()),
	}, nil
}

// PathfilepathVolumeName is an autogenerated function.
// func VolumeName(path string) string
// VolumeName returns leading volume name.
// Given "C:\foo\bar" it returns "C:" on Windows.
// Given "\\host\share\foo" it returns "\\host\share".
// On other platforms it returns "".
func PathfilepathVolumeName(ctx context.Context, args []types.Value) (types.Value, error) {
	return &types.StrValue{
		V: filepath.VolumeName(args[0].Str()),
	}, nil
}

// RuntimeCPUProfile is an autogenerated function.
// func CPUProfile() []byte
// CPUProfile panics.
// It formerly provided raw access to chunks of
// a pprof-format profile generated by the runtime.
// The details of generating that format have changed,
// so this functionality has been removed.
//
// Deprecated: Use the [runtime/pprof] package,
// or the handlers in the [net/http/pprof] package,
// or the [testing] package's -test.cpuprofile flag instead.
func RuntimeCPUProfile(ctx context.Context, args []types.Value) (types.Value, error) {
	return &types.StrValue{
		V: string(runtime.CPUProfile()),
	}, nil
}

// RuntimeGOMAXPROCS is an autogenerated function.
// func GOMAXPROCS(n int) int
// GOMAXPROCS sets the maximum number of CPUs that can be executing
// simultaneously and returns the previous setting. It defaults to
// the value of [runtime.NumCPU]. If n < 1, it does not change the current setting.
// This call will go away when the scheduler improves.
func RuntimeGOMAXPROCS(ctx context.Context, args []types.Value) (types.Value, error) {
	return &types.IntValue{
		V: int64(runtime.GOMAXPROCS(int(args[0].Int()))),
	}, nil
}

// RuntimeGOROOT is an autogenerated function.
// func GOROOT() string
// GOROOT returns the root of the Go tree. It uses the
// GOROOT environment variable, if set at process start,
// or else the root used during the Go build.
//
// Deprecated: The root used during the Go build will not be
// meaningful if the binary is copied to another machine.
// Use the system path to locate the “go” binary, and use
// “go env GOROOT” to find its GOROOT.
func RuntimeGOROOT(ctx context.Context, args []types.Value) (types.Value, error) {
	return &types.StrValue{
		V: runtime.GOROOT(),
	}, nil
}

// RuntimeNumCPU is an autogenerated function.
// func NumCPU() int
// NumCPU returns the number of logical CPUs usable by the current process.
//
// The set of available CPUs is checked by querying the operating system
// at process startup. Changes to operating system CPU allocation after
// process startup are not reflected.
func RuntimeNumCPU(ctx context.Context, args []types.Value) (types.Value, error) {
	return &types.IntValue{
		V: int64(runtime.NumCPU()),
	}, nil
}

// RuntimeNumCgoCall is an autogenerated function.
// func NumCgoCall() int64
// NumCgoCall returns the number of cgo calls made by the current process.
func RuntimeNumCgoCall(ctx context.Context, args []types.Value) (types.Value, error) {
	return &types.IntValue{
		V: runtime.NumCgoCall(),
	}, nil
}

// RuntimeNumGoroutine is an autogenerated function.
// func NumGoroutine() int
// NumGoroutine returns the number of goroutines that currently exist.
func RuntimeNumGoroutine(ctx context.Context, args []types.Value) (types.Value, error) {
	return &types.IntValue{
		V: int64(runtime.NumGoroutine()),
	}, nil
}

// RuntimeReadTrace is an autogenerated function.
// func ReadTrace() []byte
// ReadTrace returns the next chunk of binary tracing data, blocking until data
// is available. If tracing is turned off and all the data accumulated while it
// was on has been returned, ReadTrace returns nil. The caller must copy the
// returned data before calling ReadTrace again.
// ReadTrace must be called from one goroutine at a time.
func RuntimeReadTrace(ctx context.Context, args []types.Value) (types.Value, error) {
	return &types.StrValue{
		V: string(runtime.ReadTrace()),
	}, nil
}

// RuntimeSetMutexProfileFraction is an autogenerated function.
// func SetMutexProfileFraction(rate int) int
// SetMutexProfileFraction controls the fraction of mutex contention events
// that are reported in the mutex profile. On average 1/rate events are
// reported. The previous rate is returned.
//
// To turn off profiling entirely, pass rate 0.
// To just read the current rate, pass rate < 0.
// (For n>1 the details of sampling may change.)
func RuntimeSetMutexProfileFraction(ctx context.Context, args []types.Value) (types.Value, error) {
	return &types.IntValue{
		V: int64(runtime.SetMutexProfileFraction(int(args[0].Int()))),
	}, nil
}

// RuntimeStack is an autogenerated function.
// func Stack(buf []byte, all bool) int
// Stack formats a stack trace of the calling goroutine into buf
// and returns the number of bytes written to buf.
// If all is true, Stack formats stack traces of all other goroutines
// into buf after the trace for the current goroutine.
func RuntimeStack(ctx context.Context, args []types.Value) (types.Value, error) {
	return &types.IntValue{
		V: int64(runtime.Stack([]byte(args[0].Str()), args[1].Bool())),
	}, nil
}

// RuntimeVersion is an autogenerated function.
// func Version() string
// Version returns the Go tree's version string.
// It is either the commit hash and date at the time of the build or,
// when possible, a release tag like "go1.3".
func RuntimeVersion(ctx context.Context, args []types.Value) (types.Value, error) {
	return &types.StrValue{
		V: runtime.Version(),
	}, nil
}

// StrconvAppendBool is an autogenerated function.
// func AppendBool(dst []byte, b bool) []byte
// AppendBool appends "true" or "false", according to the value of b,
// to dst and returns the extended buffer.
func StrconvAppendBool(ctx context.Context, args []types.Value) (types.Value, error) {
	return &types.StrValue{
		V: string(strconv.AppendBool([]byte(args[0].Str()), args[1].Bool())),
	}, nil
}

// StrconvAppendInt is an autogenerated function.
// func AppendInt(dst []byte, i int64, base int) []byte
// AppendInt appends the string form of the integer i,
// as generated by [FormatInt], to dst and returns the extended buffer.
func StrconvAppendInt(ctx context.Context, args []types.Value) (types.Value, error) {
	return &types.StrValue{
		V: string(strconv.AppendInt([]byte(args[0].Str()), args[1].Int(), int(args[2].Int()))),
	}, nil
}

// StrconvAppendQuote is an autogenerated function.
// func AppendQuote(dst []byte, s string) []byte
// AppendQuote appends a double-quoted Go string literal representing s,
// as generated by [Quote], to dst and returns the extended buffer.
func StrconvAppendQuote(ctx context.Context, args []types.Value) (types.Value, error) {
	return &types.StrValue{
		V: string(strconv.AppendQuote([]byte(args[0].Str()), args[1].Str())),
	}, nil
}

// StrconvAppendQuoteToASCII is an autogenerated function.
// func AppendQuoteToASCII(dst []byte, s string) []byte
// AppendQuoteToASCII appends a double-quoted Go string literal representing s,
// as generated by [QuoteToASCII], to dst and returns the extended buffer.
func StrconvAppendQuoteToASCII(ctx context.Context, args []types.Value) (types.Value, error) {
	return &types.StrValue{
		V: string(strconv.AppendQuoteToASCII([]byte(args[0].Str()), args[1].Str())),
	}, nil
}

// StrconvAppendQuoteToGraphic is an autogenerated function.
// func AppendQuoteToGraphic(dst []byte, s string) []byte
// AppendQuoteToGraphic appends a double-quoted Go string literal representing s,
// as generated by [QuoteToGraphic], to dst and returns the extended buffer.
func StrconvAppendQuoteToGraphic(ctx context.Context, args []types.Value) (types.Value, error) {
	return &types.StrValue{
		V: string(strconv.AppendQuoteToGraphic([]byte(args[0].Str()), args[1].Str())),
	}, nil
}

// StrconvAtoi is an autogenerated function.
// func Atoi(s string) (int, error)
// Atoi is equivalent to ParseInt(s, 10, 0), converted to type int.
func StrconvAtoi(ctx context.Context, args []types.Value) (types.Value, error) {
	v, err := strconv.Atoi(args[0].Str())
	if err != nil {
		return nil, err
	}
	return &types.IntValue{
		V: int64(v),
	}, nil
}

// StrconvCanBackquote is an autogenerated function.
// func CanBackquote(s string) bool
// CanBackquote reports whether the string s can be represented
// unchanged as a single-line backquoted string without control
// characters other than tab.
func StrconvCanBackquote(ctx context.Context, args []types.Value) (types.Value, error) {
	return &types.BoolValue{
		V: strconv.CanBackquote(args[0].Str()),
	}, nil
}

// StrconvFormatBool is an autogenerated function.
// func FormatBool(b bool) string
// FormatBool returns "true" or "false" according to the value of b.
func StrconvFormatBool(ctx context.Context, args []types.Value) (types.Value, error) {
	return &types.StrValue{
		V: strconv.FormatBool(args[0].Bool()),
	}, nil
}

// StrconvFormatInt is an autogenerated function.
// func FormatInt(i int64, base int) string
// FormatInt returns the string representation of i in the given base,
// for 2 <= base <= 36. The result uses the lower-case letters 'a' to 'z'
// for digit values >= 10.
func StrconvFormatInt(ctx context.Context, args []types.Value) (types.Value, error) {
	return &types.StrValue{
		V: strconv.FormatInt(args[0].Int(), int(args[1].Int())),
	}, nil
}

// StrconvItoa is an autogenerated function.
// func Itoa(i int) string
// Itoa is equivalent to [FormatInt](int64(i), 10).
func StrconvItoa(ctx context.Context, args []types.Value) (types.Value, error) {
	return &types.StrValue{
		V: strconv.Itoa(int(args[0].Int())),
	}, nil
}

// StrconvParseBool is an autogenerated function.
// func ParseBool(str string) (bool, error)
// ParseBool returns the boolean value represented by the string.
// It accepts 1, t, T, TRUE, true, True, 0, f, F, FALSE, false, False.
// Any other value returns an error.
func StrconvParseBool(ctx context.Context, args []types.Value) (types.Value, error) {
	v, err := strconv.ParseBool(args[0].Str())
	if err != nil {
		return nil, err
	}
	return &types.BoolValue{
		V: v,
	}, nil
}

// StrconvParseFloat is an autogenerated function.
// func ParseFloat(s string, bitSize int) (float64, error)
// ParseFloat converts the string s to a floating-point number
// with the precision specified by bitSize: 32 for float32, or 64 for float64.
// When bitSize=32, the result still has type float64, but it will be
// convertible to float32 without changing its value.
//
// ParseFloat accepts decimal and hexadecimal floating-point numbers
// as defined by the Go syntax for [floating-point literals].
// If s is well-formed and near a valid floating-point number,
// ParseFloat returns the nearest floating-point number rounded
// using IEEE754 unbiased rounding.
// (Parsing a hexadecimal floating-point value only rounds when
// there are more bits in the hexadecimal representation than
// will fit in the mantissa.)
//
// The errors that ParseFloat returns have concrete type *NumError
// and include err.Num = s.
//
// If s is not syntactically well-formed, ParseFloat returns err.Err = ErrSyntax.
//
// If s is syntactically well-formed but is more than 1/2 ULP
// away from the largest floating point number of the given size,
// ParseFloat returns f = ±Inf, err.Err = ErrRange.
//
// ParseFloat recognizes the string "NaN", and the (possibly signed) strings "Inf" and "Infinity"
// as their respective special floating point values. It ignores case when matching.
//
// [floating-point literals]: https://go.dev/ref/spec#Floating-point_literals
func StrconvParseFloat(ctx context.Context, args []types.Value) (types.Value, error) {
	v, err := strconv.ParseFloat(args[0].Str(), int(args[1].Int()))
	if err != nil {
		return nil, err
	}
	return &types.FloatValue{
		V: v,
	}, nil
}

// StrconvParseInt is an autogenerated function.
// func ParseInt(s string, base int, bitSize int) (i int64, err error)
// ParseInt interprets a string s in the given base (0, 2 to 36) and
// bit size (0 to 64) and returns the corresponding value i.
//
// The string may begin with a leading sign: "+" or "-".
//
// If the base argument is 0, the true base is implied by the string's
// prefix following the sign (if present): 2 for "0b", 8 for "0" or "0o",
// 16 for "0x", and 10 otherwise. Also, for argument base 0 only,
// underscore characters are permitted as defined by the Go syntax for
// [integer literals].
//
// The bitSize argument specifies the integer type
// that the result must fit into. Bit sizes 0, 8, 16, 32, and 64
// correspond to int, int8, int16, int32, and int64.
// If bitSize is below 0 or above 64, an error is returned.
//
// The errors that ParseInt returns have concrete type [*NumError]
// and include err.Num = s. If s is empty or contains invalid
// digits, err.Err = [ErrSyntax] and the returned value is 0;
// if the value corresponding to s cannot be represented by a
// signed integer of the given size, err.Err = [ErrRange] and the
// returned value is the maximum magnitude integer of the
// appropriate bitSize and sign.
//
// [integer literals]: https://go.dev/ref/spec#Integer_literals
func StrconvParseInt(ctx context.Context, args []types.Value) (types.Value, error) {
	v, err := strconv.ParseInt(args[0].Str(), int(args[1].Int()), int(args[2].Int()))
	if err != nil {
		return nil, err
	}
	return &types.IntValue{
		V: v,
	}, nil
}

// StrconvQuote is an autogenerated function.
// func Quote(s string) string
// Quote returns a double-quoted Go string literal representing s. The
// returned string uses Go escape sequences (\t, \n, \xFF, \u0100) for
// control characters and non-printable characters as defined by
// [IsPrint].
func StrconvQuote(ctx context.Context, args []types.Value) (types.Value, error) {
	return &types.StrValue{
		V: strconv.Quote(args[0].Str()),
	}, nil
}

// StrconvQuoteToASCII is an autogenerated function.
// func QuoteToASCII(s string) string
// QuoteToASCII returns a double-quoted Go string literal representing s.
// The returned string uses Go escape sequences (\t, \n, \xFF, \u0100) for
// non-ASCII characters and non-printable characters as defined by [IsPrint].
func StrconvQuoteToASCII(ctx context.Context, args []types.Value) (types.Value, error) {
	return &types.StrValue{
		V: strconv.QuoteToASCII(args[0].Str()),
	}, nil
}

// StrconvQuoteToGraphic is an autogenerated function.
// func QuoteToGraphic(s string) string
// QuoteToGraphic returns a double-quoted Go string literal representing s.
// The returned string leaves Unicode graphic characters, as defined by
// [IsGraphic], unchanged and uses Go escape sequences (\t, \n, \xFF, \u0100)
// for non-graphic characters.
func StrconvQuoteToGraphic(ctx context.Context, args []types.Value) (types.Value, error) {
	return &types.StrValue{
		V: strconv.QuoteToGraphic(args[0].Str()),
	}, nil
}

// StrconvQuotedPrefix is an autogenerated function.
// func QuotedPrefix(s string) (string, error)
// QuotedPrefix returns the quoted string (as understood by [Unquote]) at the prefix of s.
// If s does not start with a valid quoted string, QuotedPrefix returns an error.
func StrconvQuotedPrefix(ctx context.Context, args []types.Value) (types.Value, error) {
	v, err := strconv.QuotedPrefix(args[0].Str())
	if err != nil {
		return nil, err
	}
	return &types.StrValue{
		V: v,
	}, nil
}

// StrconvUnquote is an autogenerated function.
// func Unquote(s string) (string, error)
// Unquote interprets s as a single-quoted, double-quoted,
// or backquoted Go string literal, returning the string value
// that s quotes.  (If s is single-quoted, it would be a Go
// character literal; Unquote returns the corresponding
// one-character string. For an empty character literal
// Unquote returns the empty string.)
func StrconvUnquote(ctx context.Context, args []types.Value) (types.Value, error) {
	v, err := strconv.Unquote(args[0].Str())
	if err != nil {
		return nil, err
	}
	return &types.StrValue{
		V: v,
	}, nil
}

// StringsClone is an autogenerated function.
// func Clone(s string) string
// Clone returns a fresh copy of s.
// It guarantees to make a copy of s into a new allocation,
// which can be important when retaining only a small substring
// of a much larger string. Using Clone can help such programs
// use less memory. Of course, since using Clone makes a copy,
// overuse of Clone can make programs use more memory.
// Clone should typically be used only rarely, and only when
// profiling indicates that it is needed.
// For strings of length zero the string "" will be returned
// and no allocation is made.
func StringsClone(ctx context.Context, args []types.Value) (types.Value, error) {
	return &types.StrValue{
		V: strings.Clone(args[0].Str()),
	}, nil
}

// StringsCompare is an autogenerated function.
// func Compare(a string, b string) int
// Compare returns an integer comparing two strings lexicographically.
// The result will be 0 if a == b, -1 if a < b, and +1 if a > b.
//
// Use Compare when you need to perform a three-way comparison (with
// [slices.SortFunc], for example). It is usually clearer and always faster
// to use the built-in string comparison operators ==, <, >, and so on.
func StringsCompare(ctx context.Context, args []types.Value) (types.Value, error) {
	return &types.IntValue{
		V: int64(strings.Compare(args[0].Str(), args[1].Str())),
	}, nil
}

// StringsContains is an autogenerated function.
// func Contains(s string, substr string) bool
// Contains reports whether substr is within s.
func StringsContains(ctx context.Context, args []types.Value) (types.Value, error) {
	return &types.BoolValue{
		V: strings.Contains(args[0].Str(), args[1].Str()),
	}, nil
}

// StringsContainsAny is an autogenerated function.
// func ContainsAny(s string, chars string) bool
// ContainsAny reports whether any Unicode code points in chars are within s.
func StringsContainsAny(ctx context.Context, args []types.Value) (types.Value, error) {
	return &types.BoolValue{
		V: strings.ContainsAny(args[0].Str(), args[1].Str()),
	}, nil
}

// StringsCount is an autogenerated function.
// func Count(s string, substr string) int
// Count counts the number of non-overlapping instances of substr in s.
// If substr is an empty string, Count returns 1 + the number of Unicode code points in s.
func StringsCount(ctx context.Context, args []types.Value) (types.Value, error) {
	return &types.IntValue{
		V: int64(strings.Count(args[0].Str(), args[1].Str())),
	}, nil
}

// StringsEqualFold is an autogenerated function.
// func EqualFold(s string, t string) bool
// EqualFold reports whether s and t, interpreted as UTF-8 strings,
// are equal under simple Unicode case-folding, which is a more general
// form of case-insensitivity.
func StringsEqualFold(ctx context.Context, args []types.Value) (types.Value, error) {
	return &types.BoolValue{
		V: strings.EqualFold(args[0].Str(), args[1].Str()),
	}, nil
}

// StringsHasPrefix is an autogenerated function.
// func HasPrefix(s string, prefix string) bool
// HasPrefix reports whether the string s begins with prefix.
func StringsHasPrefix(ctx context.Context, args []types.Value) (types.Value, error) {
	return &types.BoolValue{
		V: strings.HasPrefix(args[0].Str(), args[1].Str()),
	}, nil
}

// StringsHasSuffix is an autogenerated function.
// func HasSuffix(s string, suffix string) bool
// HasSuffix reports whether the string s ends with suffix.
func StringsHasSuffix(ctx context.Context, args []types.Value) (types.Value, error) {
	return &types.BoolValue{
		V: strings.HasSuffix(args[0].Str(), args[1].Str()),
	}, nil
}

// StringsIndex is an autogenerated function.
// func Index(s string, substr string) int
// Index returns the index of the first instance of substr in s, or -1 if substr is not present in s.
func StringsIndex(ctx context.Context, args []types.Value) (types.Value, error) {
	return &types.IntValue{
		V: int64(strings.Index(args[0].Str(), args[1].Str())),
	}, nil
}

// StringsIndexAny is an autogenerated function.
// func IndexAny(s string, chars string) int
// IndexAny returns the index of the first instance of any Unicode code point
// from chars in s, or -1 if no Unicode code point from chars is present in s.
func StringsIndexAny(ctx context.Context, args []types.Value) (types.Value, error) {
	return &types.IntValue{
		V: int64(strings.IndexAny(args[0].Str(), args[1].Str())),
	}, nil
}

// StringsJoin is an autogenerated function.
// func Join(elems []string, sep string) string
// Join concatenates the elements of its first argument to create a single string. The separator
// string sep is placed between elements in the resulting string.
func StringsJoin(ctx context.Context, args []types.Value) (types.Value, error) {
	return &types.StrValue{
		V: strings.Join(util.MclListToGolang(args[0]), args[1].Str()),
	}, nil
}

// StringsLastIndex is an autogenerated function.
// func LastIndex(s string, substr string) int
// LastIndex returns the index of the last instance of substr in s, or -1 if substr is not present in s.
func StringsLastIndex(ctx context.Context, args []types.Value) (types.Value, error) {
	return &types.IntValue{
		V: int64(strings.LastIndex(args[0].Str(), args[1].Str())),
	}, nil
}

// StringsLastIndexAny is an autogenerated function.
// func LastIndexAny(s string, chars string) int
// LastIndexAny returns the index of the last instance of any Unicode code
// point from chars in s, or -1 if no Unicode code point from chars is
// present in s.
func StringsLastIndexAny(ctx context.Context, args []types.Value) (types.Value, error) {
	return &types.IntValue{
		V: int64(strings.LastIndexAny(args[0].Str(), args[1].Str())),
	}, nil
}

// StringsRepeat is an autogenerated function.
// func Repeat(s string, count int) string
// Repeat returns a new string consisting of count copies of the string s.
//
// It panics if count is negative or if the result of (len(s) * count)
// overflows.
func StringsRepeat(ctx context.Context, args []types.Value) (types.Value, error) {
	return &types.StrValue{
		V: strings.Repeat(args[0].Str(), int(args[1].Int())),
	}, nil
}

// StringsReplace is an autogenerated function.
// func Replace(s string, old string, new string, n int) string
// Replace returns a copy of the string s with the first n
// non-overlapping instances of old replaced by new.
// If old is empty, it matches at the beginning of the string
// and after each UTF-8 sequence, yielding up to k+1 replacements
// for a k-rune string.
// If n < 0, there is no limit on the number of replacements.
func StringsReplace(ctx context.Context, args []types.Value) (types.Value, error) {
	return &types.StrValue{
		V: strings.Replace(args[0].Str(), args[1].Str(), args[2].Str(), int(args[3].Int())),
	}, nil
}

// StringsReplaceAll is an autogenerated function.
// func ReplaceAll(s string, old string, new string) string
// ReplaceAll returns a copy of the string s with all
// non-overlapping instances of old replaced by new.
// If old is empty, it matches at the beginning of the string
// and after each UTF-8 sequence, yielding up to k+1 replacements
// for a k-rune string.
func StringsReplaceAll(ctx context.Context, args []types.Value) (types.Value, error) {
	return &types.StrValue{
		V: strings.ReplaceAll(args[0].Str(), args[1].Str(), args[2].Str()),
	}, nil
}

// StringsTitle is an autogenerated function.
// func Title(s string) string
// Title returns a copy of the string s with all Unicode letters that begin words
// mapped to their Unicode title case.
//
// Deprecated: The rule Title uses for word boundaries does not handle Unicode
// punctuation properly. Use golang.org/x/text/cases instead.
func StringsTitle(ctx context.Context, args []types.Value) (types.Value, error) {
	return &types.StrValue{
		V: strings.Title(args[0].Str()),
	}, nil
}

// StringsToLower is an autogenerated function.
// func ToLower(s string) string
// ToLower returns s with all Unicode letters mapped to their lower case.
func StringsToLower(ctx context.Context, args []types.Value) (types.Value, error) {
	return &types.StrValue{
		V: strings.ToLower(args[0].Str()),
	}, nil
}

// StringsToTitle is an autogenerated function.
// func ToTitle(s string) string
// ToTitle returns a copy of the string s with all Unicode letters mapped to
// their Unicode title case.
func StringsToTitle(ctx context.Context, args []types.Value) (types.Value, error) {
	return &types.StrValue{
		V: strings.ToTitle(args[0].Str()),
	}, nil
}

// StringsToUpper is an autogenerated function.
// func ToUpper(s string) string
// ToUpper returns s with all Unicode letters mapped to their upper case.
func StringsToUpper(ctx context.Context, args []types.Value) (types.Value, error) {
	return &types.StrValue{
		V: strings.ToUpper(args[0].Str()),
	}, nil
}

// StringsToValidUTF8 is an autogenerated function.
// func ToValidUTF8(s string, replacement string) string
// ToValidUTF8 returns a copy of the string s with each run of invalid UTF-8 byte sequences
// replaced by the replacement string, which may be empty.
func StringsToValidUTF8(ctx context.Context, args []types.Value) (types.Value, error) {
	return &types.StrValue{
		V: strings.ToValidUTF8(args[0].Str(), args[1].Str()),
	}, nil
}

// StringsTrim is an autogenerated function.
// func Trim(s string, cutset string) string
// Trim returns a slice of the string s with all leading and
// trailing Unicode code points contained in cutset removed.
func StringsTrim(ctx context.Context, args []types.Value) (types.Value, error) {
	return &types.StrValue{
		V: strings.Trim(args[0].Str(), args[1].Str()),
	}, nil
}

// StringsTrimLeft is an autogenerated function.
// func TrimLeft(s string, cutset string) string
// TrimLeft returns a slice of the string s with all leading
// Unicode code points contained in cutset removed.
//
// To remove a prefix, use [TrimPrefix] instead.
func StringsTrimLeft(ctx context.Context, args []types.Value) (types.Value, error) {
	return &types.StrValue{
		V: strings.TrimLeft(args[0].Str(), args[1].Str()),
	}, nil
}

// StringsTrimPrefix is an autogenerated function.
// func TrimPrefix(s string, prefix string) string
// TrimPrefix returns s without the provided leading prefix string.
// If s doesn't start with prefix, s is returned unchanged.
func StringsTrimPrefix(ctx context.Context, args []types.Value) (types.Value, error) {
	return &types.StrValue{
		V: strings.TrimPrefix(args[0].Str(), args[1].Str()),
	}, nil
}

// StringsTrimRight is an autogenerated function.
// func TrimRight(s string, cutset string) string
// TrimRight returns a slice of the string s, with all trailing
// Unicode code points contained in cutset removed.
//
// To remove a suffix, use [TrimSuffix] instead.
func StringsTrimRight(ctx context.Context, args []types.Value) (types.Value, error) {
	return &types.StrValue{
		V: strings.TrimRight(args[0].Str(), args[1].Str()),
	}, nil
}

// StringsTrimSpace is an autogenerated function.
// func TrimSpace(s string) string
// TrimSpace returns a slice of the string s, with all leading
// and trailing white space removed, as defined by Unicode.
func StringsTrimSpace(ctx context.Context, args []types.Value) (types.Value, error) {
	return &types.StrValue{
		V: strings.TrimSpace(args[0].Str()),
	}, nil
}

// StringsTrimSuffix is an autogenerated function.
// func TrimSuffix(s string, suffix string) string
// TrimSuffix returns s without the provided trailing suffix string.
// If s doesn't end with suffix, s is returned unchanged.
func StringsTrimSuffix(ctx context.Context, args []types.Value) (types.Value, error) {
	return &types.StrValue{
		V: strings.TrimSuffix(args[0].Str(), args[1].Str()),
	}, nil
}
