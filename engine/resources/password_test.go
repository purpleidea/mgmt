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

package resources

import (
	"context"
	"testing"

	"github.com/purpleidea/mgmt/engine"
	"github.com/purpleidea/mgmt/engine/local"
)

// newTestPasswordRes builds an initialized password resource which uses dir as
// its VarDir, so that two instances sharing a dir simulate an mgmt restart.
func newTestPasswordRes(t *testing.T, dir string, write bool) *PasswordRes {
	t.Helper()
	res := &PasswordRes{
		Length: 32,
		Write:  write,
	}
	res.SetKind("password")
	res.SetName("test")

	api := (&local.API{
		Prefix: t.TempDir(),
		Logf:   func(string, ...interface{}) {},
	}).Init()

	init := &engine.Init{
		Send:    engine.GenerateSendFunc(res),
		Refresh: func() bool { return false },
		VarDir:  func(string) (string, error) { return dir, nil },
		Local:   api,
		Logf:    func(string, ...interface{}) {},
	}
	if err := res.Init(init); err != nil {
		t.Fatalf("func Init failed: %v", err)
	}
	return res
}

// checkApplyPassword runs CheckApply and returns the password that was sent.
func checkApplyPassword(t *testing.T, res *PasswordRes) string {
	t.Helper()
	if _, err := res.CheckApply(context.Background(), true); err != nil {
		t.Fatalf("func CheckApply failed: %v", err)
	}
	sends, ok := res.Sent().(*PasswordSends)
	if !ok || sends.Password == nil {
		t.Fatalf("func CheckApply did not send a password")
	}
	return *sends.Password
}

// TestPasswordEphemeralStable checks that an ephemeral password is generated
// once and then kept for the lifetime of the resource. It used to be that each
// CheckApply would generate a brand new password.
func TestPasswordEphemeralStable(t *testing.T) {
	res := newTestPasswordRes(t, t.TempDir(), false)
	defer res.Cleanup()

	p1 := checkApplyPassword(t, res)
	checkOK, err := res.CheckApply(context.Background(), true)
	if err != nil {
		t.Fatalf("func CheckApply failed: %v", err)
	}
	if !checkOK {
		t.Errorf("func CheckApply returned false on the second run")
	}
	p2 := checkApplyPassword(t, res)
	if p1 != p2 {
		t.Errorf("password changed from %q to %q", p1, p2)
	}
}

// TestPasswordWriteStable checks that a written password survives a restart of
// the resource, as would happen when mgmt is restarted with the same prefix.
func TestPasswordWriteStable(t *testing.T) {
	dir := t.TempDir()

	res1 := newTestPasswordRes(t, dir, true)
	p1 := checkApplyPassword(t, res1)
	if err := res1.Cleanup(); err != nil {
		t.Fatalf("func Cleanup failed: %v", err)
	}

	res2 := newTestPasswordRes(t, dir, true)
	defer res2.Cleanup()
	p2 := checkApplyPassword(t, res2)
	if p1 != p2 {
		t.Errorf("password changed across restart from %q to %q", p1, p2)
	}
}
