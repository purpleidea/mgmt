// Mgmt
// Copyright (C) 2013-2017+ James Shubin and the project contributors
// Written by James Shubin <james@shubin.ca> and the project contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <http://www.gnu.org/licenses/>.

package resources

import (
	"testing"

	"github.com/purpleidea/mgmt/pgraph"
)

func TestSemaSize(t *testing.T) {
	pairs := map[string]int{
		"id:42":   42,
		":13":     13,
		"some_id": 1,
	}
	for id, size := range pairs {
		if i := SemaSize(id); i != size {
			t.Errorf("sema id `%s`, expected: `%d`, got: `%d`", id, size, i)
		}
	}
}

func NewNoopResTestSema(name string, semas []string) *NoopResTest {
	obj := &NoopResTest{
		NoopRes: NoopRes{
			BaseRes: BaseRes{
				Name: name,
				MetaParams: MetaParams{
					AutoGroup: true, // always autogroup
					Sema:      semas,
				},
			},
		},
	}
	return obj
}

func TestPgraphSemaphoreGrouping1(t *testing.T) {
	g1, _ := pgraph.NewGraph("g1") // original graph
	{
		a1 := pgraph.NewVertex(NewNoopResTestSema("a1", []string{"s:1"}))
		a2 := pgraph.NewVertex(NewNoopResTestSema("a2", []string{"s:2"}))
		a3 := pgraph.NewVertex(NewNoopResTestSema("a3", []string{"s:3"}))
		g1.AddVertex(a1)
		g1.AddVertex(a2)
		g1.AddVertex(a3)
	}
	g2, _ := pgraph.NewGraph("g2") // expected result
	{
		a123 := pgraph.NewVertex(NewNoopResTestSema("a1,a2,a3", []string{"s:1", "s:2", "s:3"}))
		g2.AddVertex(a123)
	}
	runGraphCmp(t, g1, g2)
}

func TestPgraphSemaphoreGrouping2(t *testing.T) {
	g1, _ := pgraph.NewGraph("g1") // original graph
	{
		a1 := pgraph.NewVertex(NewNoopResTestSema("a1", []string{"s:10", "s:11"}))
		a2 := pgraph.NewVertex(NewNoopResTestSema("a2", []string{"s:2"}))
		a3 := pgraph.NewVertex(NewNoopResTestSema("a3", []string{"s:3"}))
		g1.AddVertex(a1)
		g1.AddVertex(a2)
		g1.AddVertex(a3)
	}
	g2, _ := pgraph.NewGraph("g2") // expected result
	{
		a123 := pgraph.NewVertex(NewNoopResTestSema("a1,a2,a3", []string{"s:10", "s:11", "s:2", "s:3"}))
		g2.AddVertex(a123)
	}
	runGraphCmp(t, g1, g2)
}

func TestPgraphSemaphoreGrouping3(t *testing.T) {
	g1, _ := pgraph.NewGraph("g1") // original graph
	{
		a1 := pgraph.NewVertex(NewNoopResTestSema("a1", []string{"s:1", "s:2"}))
		a2 := pgraph.NewVertex(NewNoopResTestSema("a2", []string{"s:2"}))
		a3 := pgraph.NewVertex(NewNoopResTestSema("a3", []string{"s:3"}))
		g1.AddVertex(a1)
		g1.AddVertex(a2)
		g1.AddVertex(a3)
	}
	g2, _ := pgraph.NewGraph("g2") // expected result
	{
		a123 := pgraph.NewVertex(NewNoopResTestSema("a1,a2,a3", []string{"s:1", "s:2", "s:3"}))
		g2.AddVertex(a123)
	}
	runGraphCmp(t, g1, g2)
}
