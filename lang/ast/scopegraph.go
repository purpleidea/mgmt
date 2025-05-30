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

package ast

import (
	"fmt"

	"github.com/purpleidea/mgmt/lang/interfaces"
	"github.com/purpleidea/mgmt/pgraph"
)

// ScopeGraph adds nodes and vertices to the supplied graph.
func (obj *StmtBind) ScopeGraph(g *pgraph.Graph) {
	g.AddVertex(obj)
}

// ScopeGraph adds nodes and vertices to the supplied graph.
func (obj *StmtRes) ScopeGraph(g *pgraph.Graph) {
	g.AddVertex(obj)
	name, ok := obj.Name.(interfaces.ScopeGrapher)
	if !ok {
		panic("can't graph scope") // programming error
	}
	name.ScopeGraph(g)
	g.AddEdge(obj, obj.Name, &pgraph.SimpleEdge{Name: "name"})
	for _, resContents := range obj.Contents {
		switch r := resContents.(type) {
		case *StmtResField:
			rValue, ok := r.Value.(interfaces.ScopeGrapher)
			if !ok {
				panic("can't graph scope") // programming error
			}
			rValue.ScopeGraph(g)
			g.AddEdge(obj, r.Value, &pgraph.SimpleEdge{Name: r.Field})
		}
	}
}

// ScopeGraph adds nodes and vertices to the supplied graph.
func (obj *StmtEdge) ScopeGraph(g *pgraph.Graph) {
	g.AddVertex(obj)
}

// ScopeGraph adds nodes and vertices to the supplied graph.
func (obj *StmtIf) ScopeGraph(g *pgraph.Graph) {
	g.AddVertex(obj)
}

// ScopeGraph adds nodes and vertices to the supplied graph.
func (obj *StmtFor) ScopeGraph(g *pgraph.Graph) {
	g.AddVertex(obj)
}

// ScopeGraph adds nodes and vertices to the supplied graph.
func (obj *StmtProg) ScopeGraph(g *pgraph.Graph) {
	g.AddVertex(obj)
	for _, stmt := range obj.Body {
		stmt, ok := stmt.(interfaces.ScopeGrapher)
		if !ok {
			panic("can't graph scope") // programming error
		}
		stmt.ScopeGraph(g)
		g.AddEdge(obj, stmt, &pgraph.SimpleEdge{Name: ""})
	}
}

// ScopeGraph adds nodes and vertices to the supplied graph.
func (obj *StmtFunc) ScopeGraph(g *pgraph.Graph) {
	g.AddVertex(obj)
}

// ScopeGraph adds nodes and vertices to the supplied graph.
func (obj *StmtClass) ScopeGraph(g *pgraph.Graph) {
	g.AddVertex(obj)
}

// ScopeGraph adds nodes and vertices to the supplied graph.
func (obj *StmtInclude) ScopeGraph(g *pgraph.Graph) {
	g.AddVertex(obj)
}

// ScopeGraph adds nodes and vertices to the supplied graph.
func (obj *StmtImport) ScopeGraph(g *pgraph.Graph) {
	g.AddVertex(obj)
}

// ScopeGraph adds nodes and vertices to the supplied graph.
func (obj *StmtComment) ScopeGraph(g *pgraph.Graph) {
	g.AddVertex(obj)
}

// ScopeGraph adds nodes and vertices to the supplied graph.
func (obj *ExprBool) ScopeGraph(g *pgraph.Graph) {
	g.AddVertex(obj)
}

// ScopeGraph adds nodes and vertices to the supplied graph.
func (obj *ExprStr) ScopeGraph(g *pgraph.Graph) {
	g.AddVertex(obj)
}

// ScopeGraph adds nodes and vertices to the supplied graph.
func (obj *ExprInt) ScopeGraph(g *pgraph.Graph) {
	g.AddVertex(obj)
}

// ScopeGraph adds nodes and vertices to the supplied graph.
func (obj *ExprFloat) ScopeGraph(g *pgraph.Graph) {
	g.AddVertex(obj)
}

// ScopeGraph adds nodes and vertices to the supplied graph.
func (obj *ExprList) ScopeGraph(g *pgraph.Graph) {
	g.AddVertex(obj)
}

// ScopeGraph adds nodes and vertices to the supplied graph.
func (obj *ExprMap) ScopeGraph(g *pgraph.Graph) {
	g.AddVertex(obj)
}

// ScopeGraph adds nodes and vertices to the supplied graph.
func (obj *ExprStruct) ScopeGraph(g *pgraph.Graph) {
	g.AddVertex(obj)
}

// ScopeGraph adds nodes and vertices to the supplied graph.
func (obj *ExprFunc) ScopeGraph(g *pgraph.Graph) {
	g.AddVertex(obj)
	if obj.Body != nil {
		body, ok := obj.Body.(interfaces.ScopeGrapher)
		if !ok {
			panic("can't graph scope") // programming error
		}
		body.ScopeGraph(g)
		g.AddEdge(obj, obj.Body, &pgraph.SimpleEdge{Name: "body"})
	}
}

// ScopeGraph adds nodes and vertices to the supplied graph.
func (obj *ExprCall) ScopeGraph(g *pgraph.Graph) {
	g.AddVertex(obj)
	expr, ok := obj.expr.(interfaces.ScopeGrapher)
	if !ok {
		panic("can't graph scope") // programming error
	}
	expr.ScopeGraph(g)
	g.AddEdge(obj, obj.expr, &pgraph.SimpleEdge{Name: "expr"})

	for i, arg := range obj.Args {
		arg, ok := arg.(interfaces.ScopeGrapher)
		if !ok {
			panic("can't graph scope") // programming error
		}
		arg.ScopeGraph(g)
		g.AddEdge(obj, arg, &pgraph.SimpleEdge{Name: fmt.Sprintf("arg%d", i)})
	}
}

// ScopeGraph adds nodes and vertices to the supplied graph.
func (obj *ExprVar) ScopeGraph(g *pgraph.Graph) {
	g.AddVertex(obj)
	target := obj.scope.Variables[obj.Name]
	newTarget, ok := target.(interfaces.ScopeGrapher)
	if !ok {
		panic("can't graph scope") // programming error
	}
	newTarget.ScopeGraph(g)
	g.AddEdge(obj, target, &pgraph.SimpleEdge{Name: "target"})
}

// ScopeGraph adds nodes and vertices to the supplied graph.
func (obj *ExprParam) ScopeGraph(g *pgraph.Graph) {
	g.AddVertex(obj)
}

// ScopeGraph adds nodes and vertices to the supplied graph.
func (obj *ExprIterated) ScopeGraph(g *pgraph.Graph) {
	g.AddVertex(obj)
	definition, ok := obj.Definition.(interfaces.ScopeGrapher)
	if !ok {
		panic("can't graph scope") // programming error
	}
	definition.ScopeGraph(g)
	g.AddEdge(obj, obj.Definition, &pgraph.SimpleEdge{Name: "def"})
}

// ScopeGraph adds nodes and vertices to the supplied graph.
func (obj *ExprPoly) ScopeGraph(g *pgraph.Graph) {
	g.AddVertex(obj)
	definition, ok := obj.Definition.(interfaces.ScopeGrapher)
	if !ok {
		panic("can't graph scope") // programming error
	}
	definition.ScopeGraph(g)
	g.AddEdge(obj, obj.Definition, &pgraph.SimpleEdge{Name: "def"})
}

// ScopeGraph adds nodes and vertices to the supplied graph.
func (obj *ExprTopLevel) ScopeGraph(g *pgraph.Graph) {
	g.AddVertex(obj)
	definition, ok := obj.Definition.(interfaces.ScopeGrapher)
	if !ok {
		panic("can't graph scope") // programming error
	}
	definition.ScopeGraph(g)
	g.AddEdge(obj, obj.Definition, &pgraph.SimpleEdge{Name: "def"})
}

// ScopeGraph adds nodes and vertices to the supplied graph.
func (obj *ExprSingleton) ScopeGraph(g *pgraph.Graph) {
	g.AddVertex(obj)
	definition, ok := obj.Definition.(interfaces.ScopeGrapher)
	if !ok {
		panic("can't graph scope") // programming error
	}
	definition.ScopeGraph(g)
	g.AddEdge(obj, obj.Definition, &pgraph.SimpleEdge{Name: "def"})
}

// ScopeGraph adds nodes and vertices to the supplied graph.
func (obj *ExprIf) ScopeGraph(g *pgraph.Graph) {
	g.AddVertex(obj)
}
