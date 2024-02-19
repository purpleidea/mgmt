// Mgmt
// Copyright (C) 2013-2019+ James Shubin and the project contributors
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

package coreprometheus

import (
	"context"
	"fmt"
	"time"

	"github.com/purpleidea/mgmt/lang/funcs/simple"
	"github.com/purpleidea/mgmt/lang/types"
	"github.com/purpleidea/mgmt/util/errwrap"

	"github.com/prometheus/client_golang/api"
	"github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
)

func init() {
	simple.ModuleRegister(ModuleName, "instant_query_value", &types.FuncValue{
		T: types.NewType("func(query str, ts int, config struct{address str}) str"),
		V: InstantQueryValue,
	})
}

// InstantQuery runs a Prometheus query at a specific time and returns the
// output as a list of structs
func InstantQueryValue(input []types.Value) (types.Value, error) {
	query := input[0].Str()
	ts := input[1].Int()
	config := input[2].Struct()

	client, err := api.NewClient(api.Config{
		Address: config["address"].Str(),
	})
	if err != nil {
		return nil, errwrap.Wrapf(err, "error creating prometheus client")
	}

	api := v1.NewAPI(client)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	result, _, err := api.Query(ctx, query, time.Unix(ts, 0))
	if err != nil {
		return nil, errwrap.Wrapf(err, "error querying prometheus")
	}
	v, ok := result.(model.Vector)
	if !ok {
		return nil, errwrap.Wrapf(err, "error casting prometheus query result")
	}
	if len(v) != 1 {
		return nil, fmt.Errorf("bad number of results: expected: 1, got: %d", len(v))
	}
	return &types.StrValue{
		V: v[0].Value.String(),
	}, nil
}
