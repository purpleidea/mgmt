// Mgmt
// Copyright (C) 2013-2018+ James Shubin and the project contributors
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

package main

import (
	"flag"
	"log"
)

var (
	pkg       = flag.String("package", "lang/funcs/core", "path to the package")
	filename  = flag.String("filename", "golang2mgmt.yaml", "path to the config")
	templates = flag.String("templates", "lang/golang2mgmt/templates/*.tpl", "path to the templates")
)

func main() {
	flag.Parse()
	if *pkg == "" {
		log.Fatalf("No package passed!")
	}

	err := parsePkg(*pkg, *filename, *templates)
	if err != nil {
		log.Fatal(err)
	}
}
