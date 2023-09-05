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

package ast

import (
	"fmt"

	"github.com/purpleidea/mgmt/pgraph"
)

func (obj *StmtBind) SetScopeGraphviz(g *pgraph.Graph) {
	g.AddVertex(obj)
}

func (obj *StmtRes) SetScopeGraphviz(g *pgraph.Graph) {
	g.AddVertex(obj)
	obj.Name.SetScopeGraphviz(g)
	g.AddEdge(obj, obj.Name, &pgraph.SimpleEdge{Name: "name"})
	for _, resContents := range obj.Contents {
		switch r := resContents.(type) {
		case *StmtResField:
			r.Value.SetScopeGraphviz(g)
			g.AddEdge(obj, r.Value, &pgraph.SimpleEdge{Name: r.Field})
		}
	}
}

func (obj *StmtEdge) SetScopeGraphviz(g *pgraph.Graph) {
	g.AddVertex(obj)
}

func (obj *StmtIf) SetScopeGraphviz(g *pgraph.Graph) {
	g.AddVertex(obj)
}

func (obj *StmtProg) SetScopeGraphviz(g *pgraph.Graph) {
	g.AddVertex(obj)
	for _, s := range obj.Body {
		s.SetScopeGraphviz(g)
		g.AddEdge(obj, s, &pgraph.SimpleEdge{Name: ""})
	}
}

func (obj *StmtFunc) SetScopeGraphviz(g *pgraph.Graph) {
	g.AddVertex(obj)
}

func (obj *StmtClass) SetScopeGraphviz(g *pgraph.Graph) {
	g.AddVertex(obj)
}

func (obj *StmtInclude) SetScopeGraphviz(g *pgraph.Graph) {
	g.AddVertex(obj)
}

func (obj *StmtImport) SetScopeGraphviz(g *pgraph.Graph) {
	g.AddVertex(obj)
}

func (obj *StmtComment) SetScopeGraphviz(g *pgraph.Graph) {
	g.AddVertex(obj)
}

func (obj *ExprBool) SetScopeGraphviz(g *pgraph.Graph) {
	g.AddVertex(obj)
}

func (obj *ExprStr) SetScopeGraphviz(g *pgraph.Graph) {
	g.AddVertex(obj)
}

func (obj *ExprInt) SetScopeGraphviz(g *pgraph.Graph) {
	g.AddVertex(obj)
}

func (obj *ExprFloat) SetScopeGraphviz(g *pgraph.Graph) {
	g.AddVertex(obj)
}

func (obj *ExprList) SetScopeGraphviz(g *pgraph.Graph) {
	g.AddVertex(obj)
}

func (obj *ExprMap) SetScopeGraphviz(g *pgraph.Graph) {
	g.AddVertex(obj)
}

func (obj *ExprStruct) SetScopeGraphviz(g *pgraph.Graph) {
	g.AddVertex(obj)
}

func (obj *ExprFunc) SetScopeGraphviz(g *pgraph.Graph) {
	g.AddVertex(obj)
	if obj.Body != nil {
		obj.Body.SetScopeGraphviz(g)
		g.AddEdge(obj, obj.Body, &pgraph.SimpleEdge{Name: "body"})
	}
}

func (obj *ExprRecur) SetScopeGraphviz(g *pgraph.Graph) {
	g.AddVertex(obj)
}

func (obj *ExprCall) SetScopeGraphviz(g *pgraph.Graph) {
	g.AddVertex(obj)
	obj.expr.SetScopeGraphviz(g)
	g.AddEdge(obj, obj.expr, &pgraph.SimpleEdge{Name: "expr"})

	for i, arg := range obj.Args {
		arg.SetScopeGraphviz(g)
		g.AddEdge(obj, arg, &pgraph.SimpleEdge{Name: fmt.Sprintf("arg%d", i)})
	}
}

func (obj *ExprVar) SetScopeGraphviz(g *pgraph.Graph) {
	g.AddVertex(obj)
	target := obj.scope.Variables[obj.Name]
	target.SetScopeGraphviz(g)
	g.AddEdge(obj, target, &pgraph.SimpleEdge{Name: "target"})
}

func (obj *ExprParam) SetScopeGraphviz(g *pgraph.Graph) {
	g.AddVertex(obj)
}

func (obj *ExprPoly) SetScopeGraphviz(g *pgraph.Graph) {
	g.AddVertex(obj)
	obj.Definition.SetScopeGraphviz(g)
	g.AddEdge(obj, obj.Definition, &pgraph.SimpleEdge{Name: "def"})
}

func (obj *ExprIf) SetScopeGraphviz(g *pgraph.Graph) {
	g.AddVertex(obj)
}
