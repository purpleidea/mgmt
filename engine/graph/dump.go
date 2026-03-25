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

package graph

import (
	"fmt"
	"os"
	"reflect"
	"strings"

	"github.com/purpleidea/mgmt/engine"
	engineUtil "github.com/purpleidea/mgmt/engine/util"
	"github.com/purpleidea/mgmt/lang/types"
	"github.com/purpleidea/mgmt/pgraph"
	"github.com/purpleidea/mgmt/util/errwrap"

	"gopkg.in/yaml.v2"
)

// GraphDump is the structure used to serialize a graph to YAML.
type GraphDump struct {
	Name     string       `yaml:"name"`
	Vertices []VertexDump `yaml:"vertices"`
	Edges    []EdgeDump   `yaml:"edges"`
}

// VertexDump is the structure used to serialize a vertex (resource) to YAML.
type VertexDump struct {
	Kind      string                 `yaml:"kind"`
	Name      string                 `yaml:"name"`
	Meta      *engine.MetaParams     `yaml:"meta,omitempty"`
	AutoGroup *engine.AutoGroupMeta  `yaml:"autogroup,omitempty"`
	Params    map[string]interface{} `yaml:"params,omitempty"`
	Grouped   []VertexDump           `yaml:"grouped,omitempty"`
}

// EdgeDump is the structure used to serialize an edge to YAML.
type EdgeDump struct {
	From   string `yaml:"from"`
	To     string `yaml:"to"`
	Name   string `yaml:"name"`
	Notify bool   `yaml:"notify"`
}

// Dump serializes the active graph to a YAML file at the specified path.
func (obj *Engine) Dump(path string) error {
	return DumpGraph(obj.graph, path)
}

// DumpGraph serializes a pgraph.Graph to a YAML file at the specified path.
func DumpGraph(g *pgraph.Graph, path string) error {
	if g == nil {
		return fmt.Errorf("cannot dump nil graph")
	}

	dump := GraphDump{
		Name: g.Name,
	}

	for _, v := range g.VerticesSorted() {
		res, ok := v.(engine.Res)
		if !ok {
			return fmt.Errorf("vertex %s is not a Res", v)
		}
		// Skip resources that are grouped inside others, they will be dumped recursively.
		if gr, ok := res.(engine.GroupableRes); ok && gr.IsGrouped() {
			continue
		}

		vd, err := resourceToVertexDump(res)
		if err != nil {
			return errwrap.Wrapf(err, "failed to dump resource %s", res)
		}
		dump.Vertices = append(dump.Vertices, vd)
	}

	// We need deterministic edges too.
	// Since Edges() is random, we'll iterate over VerticesSorted.
	for _, v1 := range g.VerticesSorted() {
		targets := []pgraph.Vertex{}
		for v2 := range g.Adjacency()[v1] {
			targets = append(targets, v2)
		}
		for _, v2 := range pgraph.Sort(targets) {
			edge := g.Adjacency()[v1][v2]
			e, ok := edge.(*engine.Edge)
			if !ok {
				return fmt.Errorf("edge %s is not an engine.Edge", edge)
			}
			dump.Edges = append(dump.Edges, EdgeDump{
				From:   v1.String(),
				To:     v2.String(),
				Name:   e.Name,
				Notify: e.Notify,
			})
		}
	}

	out, err := yaml.Marshal(dump)
	if err != nil {
		return errwrap.Wrapf(err, "failed to marshal graph dump")
	}

	return os.WriteFile(path, out, 0644)
}

func resourceToVertexDump(res engine.Res) (VertexDump, error) {
	vd := VertexDump{
		Kind: res.Kind(),
		Name: res.Name(),
	}

	if meta := res.MetaParams(); meta != nil {
		vd.Meta = meta
	}

	if gr, ok := res.(engine.GroupableRes); ok {
		if agm := gr.AutoGroupMeta(); agm != nil {
			vd.AutoGroup = agm
		}
		for _, sub := range gr.GetGroup() {
			svd, err := resourceToVertexDump(sub)
			if err != nil {
				return VertexDump{}, err
			}
			vd.Grouped = append(vd.Grouped, svd)
		}
	}

	params, err := engineUtil.ResToParamValues(res)
	if err != nil {
		return VertexDump{}, err
	}

	if len(params) > 0 {
		mapping, err := engineUtil.LangFieldNameToStructFieldName(res.Kind())
		if err != nil {
			return VertexDump{}, err
		}
		// reverse mapping: Go field name -> language name
		rev := make(map[string]string)
		for k, v := range mapping {
			rev[v] = k
		}

		vd.Params = make(map[string]interface{})
		for k, v := range params {
			langName, exists := rev[k]
			if !exists {
				langName = k // fallback
			}
			vd.Params[langName] = v.Value()
		}
	}

	return vd, nil
}

// LoadGraph deserializes a graph from a YAML file.
func LoadGraph(path string) (*pgraph.Graph, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, errwrap.Wrapf(err, "failed to read graph dump file")
	}

	var dump GraphDump
	if err := yaml.Unmarshal(data, &dump); err != nil {
		return nil, errwrap.Wrapf(err, "failed to unmarshal graph dump")
	}

	g, err := pgraph.NewGraph(dump.Name)
	if err != nil {
		return nil, err
	}

	vm := make(map[string]pgraph.Vertex)

	for _, vd := range dump.Vertices {
		if err := loadVertexRecursive(g, vd, nil, vm); err != nil {
			return nil, err
		}
	}

	for _, ed := range dump.Edges {
		from, ok1 := vm[ed.From]
		to, ok2 := vm[ed.To]
		if !ok1 || !ok2 {
			return nil, fmt.Errorf("edge refers to missing vertex: %s -> %s", ed.From, ed.To)
		}
		g.AddEdge(from, to, &engine.Edge{
			Name:   ed.Name,
			Notify: ed.Notify,
		})
	}

	return g, nil
}

func loadVertexRecursive(g *pgraph.Graph, vd VertexDump, parent engine.GroupableRes, vm map[string]pgraph.Vertex) error {
	res, err := vertexDumpToResource(vd)
	if err != nil {
		return errwrap.Wrapf(err, "failed to load resource %s[%s]", vd.Kind, vd.Name)
	}

	g.AddVertex(res)
	vm[res.String()] = res

	if parent != nil {
		gr, ok := res.(engine.GroupableRes)
		if !ok {
			return fmt.Errorf("resource %s is not groupable but has a parent", res)
		}
		gr.SetParent(parent)
		gr.SetGrouped(true)
		parent.SetGroup(append(parent.GetGroup(), gr))
	}

	gr, ok := res.(engine.GroupableRes)
	if ok {
		for _, svd := range vd.Grouped {
			if err := loadVertexRecursive(g, svd, gr, vm); err != nil {
				return err
			}
		}
	}

	return nil
}

func vertexDumpToResource(vd VertexDump) (engine.Res, error) {
	res, err := engine.NewNamedResource(vd.Kind, vd.Name)
	if err != nil {
		return nil, err
	}

	if vd.Meta != nil {
		res.SetMetaParams(vd.Meta)
	}

	if gr, ok := res.(engine.GroupableRes); ok {
		if vd.AutoGroup != nil {
			gr.SetAutoGroupMeta(vd.AutoGroup)
		}
		// Grouped resources are handled in loadVertexRecursive
	}

	if len(vd.Params) > 0 {
		mapping, err := engineUtil.LangFieldNameToStructFieldName(res.Kind())
		if err != nil {
			return nil, err
		}

		rv := reflect.ValueOf(res).Elem()
		for langName, val := range vd.Params {
			fieldName, exists := mapping[langName]
			if !exists {
				// Try case-insensitive fallback if not found in mapping
				fieldName = langName
			}

			field := rv.FieldByName(fieldName)
			if !field.IsValid() {
				// try searching case-insensitively
				for i := 0; i < rv.NumField(); i++ {
					f := rv.Type().Field(i)
					if f.PkgPath != "" {
						continue
					}
					if strings.EqualFold(f.Name, fieldName) {
						field = rv.Field(i)
						break
					}
				}
			}

			if !field.IsValid() {
				continue // skip unknown fields
			}

			mclVal, err := types.ValueOfGolang(val)
			if err != nil {
				return nil, errwrap.Wrapf(err, "failed to convert parameter %s for %s", langName, res)
			}

			if err := types.Into(mclVal, field); err != nil {
				return nil, errwrap.Wrapf(err, "failed to set parameter %s for %s", langName, res)
			}
		}
	}

	return res, nil
}
