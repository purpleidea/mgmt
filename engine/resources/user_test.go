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

//go:build !root

package resources

import (
	"context"
	"os/user"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/purpleidea/mgmt/engine"
)

// fakeUserCmd records one RunCmd invocation.
type fakeUserCmd struct {
	Name string
	Args []string
}

// fakeUserFuncs serves user/group lookups from in-memory tables and records
// every RunCmd call. Lookup tables are not mutated by RunCmd; the goal is to
// assert what command would be issued, given a particular system state.
type fakeUserFuncs struct {
	users     map[string]*user.User
	usersByID map[string]*user.User
	groups    map[string]*user.Group
	userGIDs  map[string][]string // includes the user's primary GID first
	shells    map[string]string

	cmds []fakeUserCmd
}

func (obj *fakeUserFuncs) lookup(name string) (*user.User, error) {
	if u, ok := obj.users[name]; ok {
		return u, nil
	}
	return nil, user.UnknownUserError(name)
}

//nolint:revive // Matches os/user.LookupId.
func (obj *fakeUserFuncs) lookupId(uid string) (*user.User, error) {
	if u, ok := obj.usersByID[uid]; ok {
		return u, nil
	}
	n, _ := strconv.Atoi(uid)
	return nil, user.UnknownUserIdError(n)
}

//nolint:revive // Matches os/user.LookupGroupId.
func (obj *fakeUserFuncs) lookupGroupId(gid string) (*user.Group, error) {
	if g, ok := obj.groups[gid]; ok {
		return g, nil
	}
	return nil, user.UnknownGroupIdError(gid)
}

func (obj *fakeUserFuncs) groupIds(u *user.User) ([]string, error) {
	if gids, ok := obj.userGIDs[u.Username]; ok {
		return gids, nil
	}
	return []string{u.Gid}, nil
}

func (obj *fakeUserFuncs) shell(_ context.Context, name string) (string, error) {
	return obj.shells[name], nil
}

func (obj *fakeUserFuncs) runCmd(_ context.Context, name string, args []string) error {
	obj.cmds = append(obj.cmds, fakeUserCmd{Name: name, Args: append([]string(nil), args...)})
	return nil
}

// install replaces defaultUserFuncs with the fake for the duration of the test;
// the original is restored via t.Cleanup.
func (obj *fakeUserFuncs) install(t *testing.T) {
	t.Helper()
	orig := defaultUserFuncs
	defaultUserFuncs = userFuncs{
		Lookup:        obj.lookup,
		LookupId:      obj.lookupId,
		LookupGroupId: obj.lookupGroupId,
		GroupIds:      obj.groupIds,
		Shell:         obj.shell,
		RunCmd:        obj.runCmd,
	}
	t.Cleanup(func() { defaultUserFuncs = orig })
}

// addUser registers a user. supplGIDs lists supplemental groups (excluding the
// primary group). shell is the user's login shell.
func (obj *fakeUserFuncs) addUser(u *user.User, supplGIDs []string, shell string) {
	if obj.users == nil {
		obj.users = map[string]*user.User{}
		obj.usersByID = map[string]*user.User{}
		obj.userGIDs = map[string][]string{}
		obj.shells = map[string]string{}
	}
	obj.users[u.Username] = u
	obj.usersByID[u.Uid] = u
	gids := append([]string{u.Gid}, supplGIDs...)
	obj.userGIDs[u.Username] = gids
	obj.shells[u.Username] = shell
}

func (obj *fakeUserFuncs) addGroup(g *user.Group) {
	if obj.groups == nil {
		obj.groups = map[string]*user.Group{}
	}
	obj.groups[g.Gid] = g
}

// fakeUserInit returns the minimal *engine.Init needed for UserRes.CheckApply.
func fakeUserInit(t *testing.T) *engine.Init {
	return &engine.Init{
		Debug: testing.Verbose(),
		Logf:  func(format string, v ...interface{}) { t.Logf("user: "+format, v...) },
	}
}

// newTestUser is a small constructor for *user.User with stringified ids.
func newTestUser(name string, uid, gid uint32, home string) *user.User {
	return &user.User{
		Username: name,
		Uid:      strconv.Itoa(int(uid)),
		Gid:      strconv.Itoa(int(gid)),
		HomeDir:  home,
	}
}

func strPtr(s string) *string { return &s }
func u32Ptr(v uint32) *uint32 { return &v }

// loadEtc populates a fakeUserFuncs from /etc/passwd and /etc/group style data.
//
// passwd lines: username:password:uid:gid:gecos:home:shell
//
// group lines: groupname:password:gid:user1,user2,...
//
// Empty strings yield an empty fake (no users, no groups).
func loadEtc(t *testing.T, passwd, group string) *fakeUserFuncs {
	t.Helper()
	f := &fakeUserFuncs{}

	type grp struct {
		name, gid string
		members   []string
	}
	var grps []grp

	for _, line := range splitLines(group) {
		parts := strings.Split(line, ":")
		if len(parts) < 4 {
			t.Fatalf("bad /etc/group line: %q", line)
		}
		var members []string
		if parts[3] != "" {
			members = strings.Split(parts[3], ",")
		}
		grps = append(grps, grp{name: parts[0], gid: parts[2], members: members})
		f.addGroup(&user.Group{Gid: parts[2], Name: parts[0]})
	}

	for _, line := range splitLines(passwd) {
		parts := strings.Split(line, ":")
		if len(parts) < 7 {
			t.Fatalf("bad /etc/passwd line: %q", line)
		}
		username, uid, gid, home, shell := parts[0], parts[2], parts[3], parts[5], parts[6]
		u := &user.User{Username: username, Uid: uid, Gid: gid, HomeDir: home}

		var suppl []string
		for _, g := range grps {
			if g.gid == gid {
				continue // primary group: not supplemental
			}
			for _, m := range g.members {
				if m == username {
					suppl = append(suppl, g.gid)
					break
				}
			}
		}
		f.addUser(u, suppl, shell)
	}
	return f
}

func splitLines(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

// mkUser builds a *UserRes with name and state pre-set, plus any per-test
// tweaks applied by opts.
func mkUser(name, state string, opts ...func(*UserRes)) *UserRes {
	r := &UserRes{State: state}
	r.SetName(name)
	r.SetKind("user")
	for _, o := range opts {
		o(r)
	}
	return r
}

func TestUserCheckApply_ExistsNoOp(t *testing.T) {
	f := &fakeUserFuncs{}
	f.addUser(newTestUser("james", 1000, 1000, "/home/james/"), nil, "/bin/bash")
	f.addGroup(&user.Group{Gid: "1000", Name: "james"})
	f.install(t)

	res := &UserRes{State: "exists"}
	res.SetName("james")
	res.SetKind("user")
	if err := res.Init(fakeUserInit(t)); err != nil {
		t.Fatal(err)
	}

	checkOK, err := res.CheckApply(context.Background(), true)
	if err != nil {
		t.Fatalf("func CheckApply: %v", err)
	}
	if !checkOK {
		t.Errorf("expected no-op (checkOK=true), got false")
	}
	if len(f.cmds) != 0 {
		t.Errorf("expected no commands, got %v", f.cmds)
	}
}

func TestUserCheckApply_AbsentAlready(t *testing.T) {
	f := &fakeUserFuncs{}
	f.install(t)

	res := &UserRes{State: "absent"}
	res.SetName("ghost")
	res.SetKind("user")
	if err := res.Init(fakeUserInit(t)); err != nil {
		t.Fatal(err)
	}

	checkOK, err := res.CheckApply(context.Background(), true)
	if err != nil {
		t.Fatalf("func CheckApply: %v", err)
	}
	if !checkOK {
		t.Errorf("expected checkOK=true for already-absent user")
	}
	if len(f.cmds) != 0 {
		t.Errorf("expected no commands, got %v", f.cmds)
	}
}

func TestUserCheckApply_CreateNoApply(t *testing.T) {
	f := &fakeUserFuncs{}
	f.install(t)

	res := &UserRes{State: "exists", UID: u32Ptr(2000)}
	res.SetName("brian")
	res.SetKind("user")
	if err := res.Init(fakeUserInit(t)); err != nil {
		t.Fatal(err)
	}

	checkOK, err := res.CheckApply(context.Background(), false)
	if err != nil {
		t.Fatalf("func CheckApply: %v", err)
	}
	if checkOK {
		t.Errorf("expected checkOK=false (would need creation)")
	}
	if len(f.cmds) != 0 {
		t.Errorf("apply=false must not run commands; got %v", f.cmds)
	}
}

func TestUserCheckApply_CreateApply(t *testing.T) {
	f := &fakeUserFuncs{}
	f.install(t)

	res := &UserRes{
		State:   "exists",
		UID:     u32Ptr(2000),
		HomeDir: strPtr("/home/james/"),
		Shell:   strPtr("/bin/bash"),
	}
	res.SetName("james")
	res.SetKind("user")
	if err := res.Init(fakeUserInit(t)); err != nil {
		t.Fatal(err)
	}

	checkOK, err := res.CheckApply(context.Background(), true)
	if err != nil {
		t.Fatalf("func CheckApply: %v", err)
	}
	if checkOK {
		t.Errorf("expected checkOK=false on create")
	}
	if len(f.cmds) != 1 {
		t.Fatalf("expected one command, got %v", f.cmds)
	}
	got := f.cmds[0]
	want := fakeUserCmd{
		Name: "useradd",
		Args: []string{"--uid", "2000", "--home", "/home/james", "--shell", "/bin/bash", "james"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("command mismatch:\n got: %+v\nwant: %+v", got, want)
	}
}

func TestUserCheckApply_ModifyShell(t *testing.T) {
	f := &fakeUserFuncs{}
	f.addUser(newTestUser("james", 1000, 1000, "/home/james/"), nil, "/bin/bash")
	f.addGroup(&user.Group{Gid: "1000", Name: "james"})
	f.install(t)

	res := &UserRes{State: "exists", Shell: strPtr("/bin/msh")}
	res.SetName("james")
	res.SetKind("user")
	if err := res.Init(fakeUserInit(t)); err != nil {
		t.Fatal(err)
	}

	checkOK, err := res.CheckApply(context.Background(), true)
	if err != nil {
		t.Fatalf("func CheckApply: %v", err)
	}
	if checkOK {
		t.Errorf("expected checkOK=false (shell differs)")
	}
	if len(f.cmds) != 1 || f.cmds[0].Name != "usermod" {
		t.Fatalf("expected one usermod, got %v", f.cmds)
	}
	want := []string{"--shell", "/bin/msh", "james"}
	if !reflect.DeepEqual(f.cmds[0].Args, want) {
		t.Errorf("args mismatch:\n got: %v\nwant: %v", f.cmds[0].Args, want)
	}
}

func TestUserCheckApply_Delete(t *testing.T) {
	f := &fakeUserFuncs{}
	f.addUser(newTestUser("james", 1000, 1000, "/home/james/"), nil, "/bin/bash")
	f.install(t)

	res := &UserRes{State: "absent"}
	res.SetName("james")
	res.SetKind("user")
	if err := res.Init(fakeUserInit(t)); err != nil {
		t.Fatal(err)
	}

	checkOK, err := res.CheckApply(context.Background(), true)
	if err != nil {
		t.Fatalf("func CheckApply: %v", err)
	}
	if checkOK {
		t.Errorf("expected checkOK=false on delete")
	}
	want := fakeUserCmd{Name: "userdel", Args: []string{"james"}}
	if len(f.cmds) != 1 || !reflect.DeepEqual(f.cmds[0], want) {
		t.Errorf("expected userdel james, got %v", f.cmds)
	}
}

func TestUserCheckApply_UIDConflict(t *testing.T) {
	f := &fakeUserFuncs{}
	f.addUser(newTestUser("james", 1000, 1000, "/home/james/"), nil, "/bin/bash")
	f.install(t)

	// brian requests UID 1000 which is already taken by james
	res := &UserRes{State: "exists", UID: u32Ptr(1000)}
	res.SetName("brian")
	res.SetKind("user")
	if err := res.Init(fakeUserInit(t)); err != nil {
		t.Fatal(err)
	}

	checkOK, err := res.CheckApply(context.Background(), true)
	if err == nil {
		t.Errorf("expected error for taken UID, got nil")
	}
	if checkOK {
		t.Errorf("expected checkOK=false on UID conflict, got true")
	}
	if len(f.cmds) != 0 {
		t.Errorf("expected no commands on UID conflict, got %v", f.cmds)
	}
}

func TestUserCheckApply_HomeDirTrailingSlash(t *testing.T) {
	// A stored HomeDir of /home/james should compare equal to /home/james/.
	// XXX: Is this what we want? Does it really matter?
	f := &fakeUserFuncs{}
	f.addUser(newTestUser("james", 1000, 1000, "/home/james"), nil, "/bin/bash")
	f.addGroup(&user.Group{Gid: "1000", Name: "james"})
	f.install(t)

	res := &UserRes{State: "exists", HomeDir: strPtr("/home/james/")}
	res.SetName("james")
	res.SetKind("user")
	if err := res.Init(fakeUserInit(t)); err != nil {
		t.Fatal(err)
	}

	checkOK, err := res.CheckApply(context.Background(), true)
	if err != nil {
		t.Fatalf("func CheckApply: %v", err)
	}
	if !checkOK {
		t.Errorf("expected checkOK=true (trailing-slash equivalence)")
	}
	if len(f.cmds) != 0 {
		t.Errorf("expected no commands, got %v", f.cmds)
	}
}

// TestUserCheckApply_Issue842 reproduces github.com/purpleidea/mgmt/issues/842
//
// mcl `user "mgmttest" { state => "exists" }` with no other fields, where the
// user already exists. The user has a primary group (mgmttest) and one
// supplemental group it is naturally a member of. Without the primary-GID skip
// in CheckApply, the loop that collects supplemental group names includes the
// primary group's name in `groups`. With `obj.Groups` set to nothing or to a
// value that doesn't include the primary group, cmpGroups then claims a
// mismatch and the apply branch runs `usermod LOGIN` with no other args,
// producing "usermod: no options". This test pins the no-op behavior.
func TestUserCheckApply_Issue842(t *testing.T) {
	f := loadEtc(t,
		"mgmttest:x:5000:5000::/home/mgmttest/:/bin/bash",
		strings.Join([]string{
			"mgmttest:x:5000:",       // primary group
			"extras:x:6000:mgmttest", // supplemental
		}, "\n"),
	)
	f.install(t)

	// Resource asks for exactly the supplemental group the user already has.
	res := mkUser("mgmttest", "exists", func(r *UserRes) {
		r.Groups = []string{"extras"}
	})
	if err := res.Init(fakeUserInit(t)); err != nil {
		t.Fatal(err)
	}

	checkOK, err := res.CheckApply(context.Background(), true)
	if err != nil {
		t.Fatalf("func CheckApply: unexpected error: %v", err)
	}
	if !checkOK {
		t.Errorf("expected no-op (checkOK=true); got false")
	}
	if len(f.cmds) != 0 {
		t.Errorf("expected no commands; got %v", f.cmds)
	}
}

// TestUserCheckApply_EmptyGroupsClears asserts the semantic distinction between
// a nil Groups field and an empty-but-non-nil Groups field. nil means "do not
// manage supplemental groups" (no-op even if the user has some), while
// []string{} means "the user should have zero supplemental groups" and must
// fire `usermod --groups "" LOGIN` to clear any current memberships.
func TestUserCheckApply_EmptyGroupsClears(t *testing.T) {
	f := loadEtc(t,
		"james:x:1000:1000::/home/james/:/bin/bash",
		strings.Join([]string{
			"james:x:1000:",
			"wheel:x:10:james",
			"extras:x:20:james",
		}, "\n"),
	)
	f.install(t)

	res := mkUser("james", "exists", func(r *UserRes) { r.Groups = []string{} })
	if err := res.Init(fakeUserInit(t)); err != nil {
		t.Fatal(err)
	}

	checkOK, err := res.CheckApply(context.Background(), true)
	if err != nil {
		t.Fatalf("func CheckApply: unexpected error: %v", err)
	}
	if checkOK {
		t.Errorf("expected checkOK=false (supplementals need clearing)")
	}
	want := fakeUserCmd{Name: "usermod", Args: []string{"--groups", "", "james"}}
	if len(f.cmds) != 1 || !reflect.DeepEqual(f.cmds[0], want) {
		t.Errorf("expected one %v; got %v", want, f.cmds)
	}
}

// TestUserCheckApply_EmptyGroupsNoopWhenAlreadyEmpty is the other half of the
// empty-vs-nil semantic: if the user is already in no supplemental groups,
// asking for [] is a no-op.
func TestUserCheckApply_EmptyGroupsNoopWhenAlreadyEmpty(t *testing.T) {
	f := loadEtc(t,
		"james:x:1000:1000::/home/james/:/bin/bash",
		"james:x:1000:",
	)
	f.install(t)

	res := mkUser("james", "exists", func(r *UserRes) { r.Groups = []string{} })
	if err := res.Init(fakeUserInit(t)); err != nil {
		t.Fatal(err)
	}

	checkOK, err := res.CheckApply(context.Background(), true)
	if err != nil {
		t.Fatalf("func CheckApply: unexpected error: %v", err)
	}
	if !checkOK {
		t.Errorf("expected checkOK=true (already no supplementals)")
	}
	if len(f.cmds) != 0 {
		t.Errorf("expected no commands; got %v", f.cmds)
	}
}

// TestUserCheckApply_NilGroupsIgnoresExisting is the nil counterpart: a nil
// Groups field means mgmt should not touch supplemental group memberships at
// all, so an existing user with supplementals is a no-op.
func TestUserCheckApply_NilGroupsIgnoresExisting(t *testing.T) {
	f := loadEtc(t,
		"james:x:1000:1000::/home/james/:/bin/bash",
		strings.Join([]string{
			"james:x:1000:",
			"wheel:x:10:james",
		}, "\n"),
	)
	f.install(t)

	// Groups left unset (nil).
	res := mkUser("james", "exists")
	if err := res.Init(fakeUserInit(t)); err != nil {
		t.Fatal(err)
	}

	checkOK, err := res.CheckApply(context.Background(), true)
	if err != nil {
		t.Fatalf("func CheckApply: unexpected error: %v", err)
	}
	if !checkOK {
		t.Errorf("expected checkOK=true (nil Groups ignores existing supplementals)")
	}
	if len(f.cmds) != 0 {
		t.Errorf("expected no commands; got %v", f.cmds)
	}
}

// TestUserValidate_GroupInGroups asserts that listing the primary Group inside
// the supplemental Groups list is rejected. AutoEdges would emit duplicate
// edges and useradd/usermod treat the primary specially, so this combination is
// always a config error.
func TestUserValidate_GroupInGroups(t *testing.T) {
	res := mkUser("james", "exists", func(r *UserRes) {
		r.Group = strPtr("wheel")
		r.Groups = []string{"wheel", "devs"}
	})
	if err := res.Validate(); err == nil {
		t.Error("expected error when primary Group is also in Groups; got nil")
	}
}

// TestUserValidate_NameInGroups asserts that the user's own name (which is the
// conventional primary-group name on most distros) is rejected from the
// supplemental Groups list.
func TestUserValidate_NameInGroups(t *testing.T) {
	res := mkUser("james", "exists", func(r *UserRes) {
		r.Groups = []string{"james", "wheel"}
	})
	if err := res.Validate(); err == nil {
		t.Error("expected error when user name appears in Groups; got nil")
	}
}

// TestUserCheckApply_AbsentSkipsUIDConflict guards against the duplicate-UID
// check firing for users we want absent. If the resource asks for `ghost` to be
// absent and ghost doesn't exist, the result should be no-op (true, nil) --
// even if obj.UID is set and some other user happens to hold that UID.
// Previously the dup-UID check ran first and would error with "the requested
// UID is already taken", masking the intended absent semantics.
func TestUserCheckApply_AbsentSkipsUIDConflict(t *testing.T) {
	f := loadEtc(t,
		"james:x:1000:1000::/home/james/:/bin/bash",
		"james:x:1000:",
	)
	f.install(t)

	res := mkUser("ghost", "absent", func(r *UserRes) { r.UID = u32Ptr(1000) })
	if err := res.Init(fakeUserInit(t)); err != nil {
		t.Fatal(err)
	}

	checkOK, err := res.CheckApply(context.Background(), true)
	if err != nil {
		t.Fatalf("func CheckApply: unexpected error: %v", err)
	}
	if !checkOK {
		t.Errorf("expected no-op (checkOK=true) for absent missing user; got false")
	}
	if len(f.cmds) != 0 {
		t.Errorf("expected no commands; got %v", f.cmds)
	}
}

// TestUserCmp_DoesNotMutateGroups guards against Cmp() sorting the caller's
// Groups slices in place. The function compares group membership in any order,
// but it must do so without disturbing the input. A slice-header copy like eg:
// (`x := obj.Groups`) shares the backing array, so a naive sort.Strings on the
// copy would reorder the caller's data too.
func TestUserCmp_DoesNotMutateGroups(t *testing.T) {
	a := mkUser("james", "exists", func(r *UserRes) {
		r.Groups = []string{"elephant", "flower", "peach"}
	})
	b := mkUser("james", "exists", func(r *UserRes) {
		r.Groups = []string{"peach", "flower", "elephant"}
	})
	aWant := []string{"elephant", "flower", "peach"}
	bWant := []string{"peach", "flower", "elephant"}

	if err := a.Cmp(b); err != nil {
		t.Fatalf("cmp: unexpected error: %v", err)
	}
	if !reflect.DeepEqual(a.Groups, aWant) {
		t.Errorf("a.Groups mutated by Cmp:\n got: %v\nwant: %v", a.Groups, aWant)
	}
	if !reflect.DeepEqual(b.Groups, bWant) {
		t.Errorf("b.Groups mutated by Cmp:\n got: %v\nwant: %v", b.Groups, bWant)
	}
}

// TestUserCheckApplyTable walks a table of (system state, resource params)
// pairs and asserts the CheckApply return values plus the exact command (if
// any) that the fake recorded. Each row stands on its own; system state lives
// in passwd/group text strings that mirror /etc/passwd and /etc/group.
func TestUserCheckApplyTable(t *testing.T) {
	tests := []struct {
		name    string
		passwd  string
		group   string
		res     *UserRes
		apply   bool // the `apply` arg passed to CheckApply
		wantOK  bool
		wantErr bool
		wantCmd *fakeUserCmd // nil = no command should be recorded
	}{
		{
			name:   "exists-matches-noop",
			passwd: "james:x:1000:1000::/home/james/:/bin/bash",
			group:  "james:x:1000:",
			res:    mkUser("james", "exists"),
			apply:  true,
			wantOK: true,
		},
		{
			name:   "absent-already",
			res:    mkUser("ghost", "absent"),
			apply:  true,
			wantOK: true,
		},
		{
			name:  "create-no-apply",
			res:   mkUser("brian", "exists", func(r *UserRes) { r.UID = u32Ptr(2000) }),
			apply: false,
		},
		{
			name: "create-apply",
			res: mkUser("brian", "exists", func(r *UserRes) {
				r.UID = u32Ptr(2000)
				r.HomeDir = strPtr("/home/brian/")
				r.Shell = strPtr("/bin/bash")
			}),
			apply: true,
			wantCmd: &fakeUserCmd{
				Name: "useradd",
				Args: []string{"--uid", "2000", "--home", "/home/brian", "--shell", "/bin/bash", "brian"},
			},
		},
		{
			name:   "modify-shell",
			passwd: "james:x:1000:1000::/home/james/:/bin/bash",
			group:  "james:x:1000:",
			res:    mkUser("james", "exists", func(r *UserRes) { r.Shell = strPtr("/bin/msh") }),
			apply:  true,
			wantCmd: &fakeUserCmd{
				Name: "usermod",
				Args: []string{"--shell", "/bin/msh", "james"},
			},
		},
		{
			name:   "modify-homedir",
			passwd: "james:x:1000:1000::/home/james/:/bin/bash",
			group:  "james:x:1000:",
			res:    mkUser("james", "exists", func(r *UserRes) { r.HomeDir = strPtr("/srv/james/") }),
			apply:  true,
			wantCmd: &fakeUserCmd{
				Name: "usermod",
				Args: []string{"--home", "/srv/james", "james"},
			},
		},
		{
			name:   "homedir-trailing-slash-equivalent1",
			passwd: "james:x:1000:1000::/home/james:/bin/bash",
			group:  "james:x:1000:",
			res:    mkUser("james", "exists", func(r *UserRes) { r.HomeDir = strPtr("/home/james/") }),
			apply:  true,
			wantOK: true,
		},
		{
			name:   "homedir-trailing-slash-equivalent2",
			passwd: "james:x:1000:1000::/home/james/:/bin/bash",
			group:  "james:x:1000:",
			res:    mkUser("james", "exists", func(r *UserRes) { r.HomeDir = strPtr("/home/james") }),
			apply:  true,
			wantOK: true,
		},
		{
			name: "modify-supplemental-groups",
			passwd: strings.Join([]string{
				"james:x:1000:1000::/home/james/:/bin/bash",
			}, "\n"),
			group: strings.Join([]string{
				"james:x:1000:",
				"wheel:x:10:james",
				"devs:x:20:",
			}, "\n"),
			res:   mkUser("james", "exists", func(r *UserRes) { r.Groups = []string{"devs"} }),
			apply: true,
			wantCmd: &fakeUserCmd{
				Name: "usermod",
				Args: []string{"--groups", "devs", "james"},
			},
		},
		{
			name: "primary-group-by-name-differs",
			passwd: strings.Join([]string{
				"james:x:1000:1000::/home/james/:/bin/bash",
			}, "\n"),
			group: strings.Join([]string{
				"james:x:1000:",
				"staff:x:50:",
			}, "\n"),
			res:   mkUser("james", "exists", func(r *UserRes) { r.Group = strPtr("staff") }),
			apply: true,
			wantCmd: &fakeUserCmd{
				Name: "usermod",
				Args: []string{"--gid", "staff", "james"},
			},
		},
		{
			name:    "uid-conflict-errors",
			passwd:  "james:x:1000:1000::/home/james/:/bin/bash",
			group:   "james:x:1000:",
			res:     mkUser("brian", "exists", func(r *UserRes) { r.UID = u32Ptr(1000) }),
			apply:   true,
			wantErr: true,
		},
		{
			name:   "delete-existing",
			passwd: "james:x:1000:1000::/home/james/:/bin/bash",
			group:  "james:x:1000:",
			res:    mkUser("james", "absent"),
			apply:  true,
			wantCmd: &fakeUserCmd{
				Name: "userdel",
				Args: []string{"james"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := loadEtc(t, tt.passwd, tt.group)
			f.install(t)

			if err := tt.res.Init(fakeUserInit(t)); err != nil {
				t.Fatalf("func Init: %v", err)
			}

			checkOK, err := tt.res.CheckApply(context.Background(), tt.apply)

			if tt.wantErr {
				if err == nil {
					t.Fatalf("func CheckApply: expected error, got nil")
				}
			} else if err != nil {
				t.Fatalf("func CheckApply: unexpected error: %v", err)
			}

			if checkOK != tt.wantOK {
				t.Errorf("checkOK = %v, want %v", checkOK, tt.wantOK)
			}

			if tt.wantCmd == nil {
				if len(f.cmds) != 0 {
					t.Errorf("expected no command, got %v", f.cmds)
				}
				return
			}

			if len(f.cmds) != 1 {
				t.Fatalf("expected exactly one command, got %v", f.cmds)
			}
			if !reflect.DeepEqual(f.cmds[0], *tt.wantCmd) {
				t.Errorf("command mismatch:\n got: %+v\nwant: %+v", f.cmds[0], *tt.wantCmd)
			}
		})
	}
}
