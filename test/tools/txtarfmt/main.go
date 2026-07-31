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

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/purpleidea/mgmt/lang/format"
	"github.com/purpleidea/mgmt/lang/format/astfmt"
	"github.com/purpleidea/mgmt/lang/parser"

	"golang.org/x/tools/txtar"
)

// main formats all txtar archives below the current working directory.
func main() {
	if len(os.Args) != 1 {
		fmt.Fprintf(os.Stderr, "usage: ./txtarfmt\n")
		os.Exit(2)
	}

	formatter := &format.Formatter{
		LexParser:    parser.LexParseWithComments,
		ASTFormatter: astfmt.Format,
	}
	formatter.Init()

	result, err := run(context.Background(), ".", formatter)
	if err != nil {
		fmt.Fprintf(os.Stderr, "txtarfmt: %+v\n", err)
		os.Exit(1)
	}

	fmt.Printf("txtarfmt: archives=%d formatted=%d nofmt=%d skipped=%d\n", result.Archives, result.Formatted, result.NoFmt, result.Skipped)
}

// config is the txtar CONFIG data relevant to the formatter.
type config struct {
	NoFmt bool `json:"nofmt"`
}

// result summarizes what happened while walking the txtar archives.
type result struct {
	Archives  int
	Formatted int
	NoFmt     int
	Skipped   int
}

// run walks root and formats every txtar archive it finds.
func run(ctx context.Context, root string, formatter *format.Formatter) (*result, error) {
	result := &result{}

	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".txtar") {
			return nil
		}

		formatted, markedNoFmt, skipped, err := formatTxtar(ctx, path, formatter)
		if err != nil {
			return err
		}
		if skipped {
			result.Skipped++
			return nil
		}

		result.Archives++
		if markedNoFmt {
			result.NoFmt++
		} else if formatted {
			result.Formatted++
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	return result, nil
}

// formatTxtar formats all mcl files in a single txtar archive.
func formatTxtar(ctx context.Context, path string, formatter *format.Formatter) (bool, bool, bool, error) {
	archive, err := txtar.ParseFile(path)
	if err != nil {
		return false, false, false, fmt.Errorf("err parsing txtar(%s): %w", path, err)
	}

	config, err := parseConfig(archive)
	if err != nil {
		return false, false, false, fmt.Errorf("err parsing txtar(%s) config: %w", path, err)
	}
	if config.NoFmt {
		return false, false, true, nil
	}

	formattedFiles := map[string][]byte{}
	var hasMCL bool
	var formatErr error
	for _, file := range archive.Files {
		if !strings.HasSuffix(file.Name, ".mcl") {
			continue
		}

		hasMCL = true
		output, err := formatter.FormatData(ctx, bytes.NewReader(file.Data))
		if err != nil {
			formatErr = err
			break
		}
		formattedFiles[file.Name] = output
	}
	if !hasMCL {
		return false, false, true, nil
	}

	if formatErr != nil {
		changed, err := setNoFmt(archive)
		if err != nil {
			return false, false, false, fmt.Errorf("err setting nofmt in txtar(%s): %w", path, err)
		}
		if !changed {
			return false, false, false, nil
		}
		if err := os.WriteFile(path, txtar.Format(archive), 0600); err != nil {
			return false, false, false, err
		}
		return false, true, false, nil
	}

	var changed bool
	for i := range archive.Files {
		output, exists := formattedFiles[archive.Files[i].Name]
		if !exists {
			continue
		}
		if bytes.Equal(output, archive.Files[i].Data) {
			continue
		}

		archive.Files[i].Data = output
		changed = true
	}
	if !changed {
		return false, false, false, nil
	}
	if err := os.WriteFile(path, txtar.Format(archive), 0600); err != nil {
		return false, false, false, err
	}
	return true, false, false, nil
}

// parseConfig returns the formatter-relevant CONFIG values from archive.
func parseConfig(archive *txtar.Archive) (*config, error) {
	config := &config{}
	for _, file := range archive.Files {
		if file.Name != "CONFIG" {
			continue
		}
		if err := json.Unmarshal(file.Data, config); err != nil {
			return nil, err
		}
		break
	}
	return config, nil
}

// setNoFmt adds or updates the archive CONFIG with nofmt enabled.
func setNoFmt(archive *txtar.Archive) (bool, error) {
	for i := range archive.Files {
		if archive.Files[i].Name != "CONFIG" {
			continue
		}

		config := map[string]any{}
		if err := json.Unmarshal(archive.Files[i].Data, &config); err != nil {
			return false, err
		}
		if nofmt, ok := config["nofmt"].(bool); ok && nofmt {
			return false, nil
		}

		config["nofmt"] = true
		data, err := json.MarshalIndent(config, "", "\t")
		if err != nil {
			return false, err
		}
		archive.Files[i].Data = append(data, '\n')
		return true, nil
	}

	config := map[string]any{
		"nofmt": true,
	}
	data, err := json.MarshalIndent(config, "", "\t")
	if err != nil {
		return false, err
	}

	archive.Files = append([]txtar.File{{
		Name: "CONFIG",
		Data: append(data, '\n'),
	}}, archive.Files...)
	return true, nil
}
