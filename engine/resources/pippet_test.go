// Mgmt
// Copyright (C) 2013-2020+ James Shubin and the project contributors
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

// +build !root

package resources

import (
	"io"
	"testing"
)

type nullWriteCloser struct {
}

type fakePippetReceiver struct {
	stdin  nullWriteCloser
	stdout *io.PipeReader
	Locked bool
}

func (obj nullWriteCloser) Write(data []byte) (int, error) {
	return len(data), nil
}

func (obj nullWriteCloser) Close() error {
	return nil
}

func (obj *fakePippetReceiver) LockApply() {
	obj.Locked = true
}

func (obj *fakePippetReceiver) UnlockApply() {
	obj.Locked = false
}

func (obj *fakePippetReceiver) InputStream() io.WriteCloser {
	return obj.stdin
}

func (obj *fakePippetReceiver) OutputStream() io.ReadCloser {
	return obj.stdout
}

func newFakePippetReceiver(jsonTestOutput string) *fakePippetReceiver {
	output, input := io.Pipe()

	result := &fakePippetReceiver{
		stdout: output,
	}

	go func() {
		// this will appear on the fake stdout
		input.Write([]byte(jsonTestOutput))
	}()

	return result
}

var pippetTestRes = &PippetRes{
	Type:   "notify",
	Title:  "testmessage",
	Params: `{msg: "This is a test"}`,
}

func TestNormalPuppetOutput(t *testing.T) {
	r := newFakePippetReceiver(`{"resource":"Notify[test]","failed":false,"changed":true,"noop":false,"error":false,"exception":null}`)
	changed, err := applyPippetRes(r, pippetTestRes)

	if err != nil {
		t.Errorf("normal Puppet output led to an apply error: %v", err)
	}

	if !changed {
		t.Errorf("return values of applyPippetRes did not reflect the changed state")
	}
}

func TestUnchangedPuppetOutput(t *testing.T) {
	r := newFakePippetReceiver(`{"resource":"Notify[test]","failed":false,"changed":false,"noop":false,"error":false,"exception":null}`)
	changed, err := applyPippetRes(r, pippetTestRes)

	if err != nil {
		t.Errorf("normal Puppet output led to an apply error: %v", err)
	}

	if changed {
		t.Errorf("return values of applyPippetRes did not reflect the changed state")
	}
}

func TestFailingPuppetOutput(t *testing.T) {
	r := newFakePippetReceiver(`{"resource":"Notify[test]","failed":false,"changed":false,"noop":false,"error":true,"exception":"I failed!"}`)
	_, err := applyPippetRes(r, pippetTestRes)

	if err == nil {
		t.Errorf("failing Puppet output led to an apply error: %v", err)
	}
}

func TestEmptyPuppetOutput(t *testing.T) {
	t.Skip("empty output will currently make the application (and the test) hang")
}

func TestPartialPuppetOutput(t *testing.T) {
	r := newFakePippetReceiver(`{"resource":"Notify[test]","failed":false,"changed":true}`)
	_, err := applyPippetRes(r, pippetTestRes)

	if err == nil {
		t.Errorf("partial Puppet output did not lead to an apply error")
	}
}

func TestMalformedPuppetOutput(t *testing.T) {
	r := newFakePippetReceiver(`oops something went wrong!!1!eleven`)
	_, err := applyPippetRes(r, pippetTestRes)

	if err == nil {
		t.Errorf("malformed Puppet output did not lead to an apply error")
	}
}
