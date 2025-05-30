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

// Package util contains utility functions that are specific to the resource
// engine.
package util

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/gob"
	"fmt"
	"io"
	"os"
	"os/user"
	"reflect"
	"strconv"
	"strings"

	"github.com/purpleidea/mgmt/engine"
	"github.com/purpleidea/mgmt/lang/types"
	"github.com/purpleidea/mgmt/util/errwrap"

	"github.com/godbus/dbus/v5"
)

const (
	// DBusInterface is the dbus interface that contains general methods.
	DBusInterface = "org.freedesktop.DBus"
	// DBusAddMatch is the dbus method to receive a subset of dbus broadcast
	// signals.
	DBusAddMatch = DBusInterface + ".AddMatch"
	// DBusRemoveMatch is the dbus method to remove a previously defined
	// AddMatch rule.
	DBusRemoveMatch = DBusInterface + ".RemoveMatch"
	// DBusSystemd1Path is the base systemd1 path.
	DBusSystemd1Path = "/org/freedesktop/systemd1"
	// DBusSystemd1Iface is the base systemd1 interface.
	DBusSystemd1Iface = "org.freedesktop.systemd1"
	// DBusSystemd1ManagerIface is the systemd manager interface used for
	// interfacing with systemd units.
	DBusSystemd1ManagerIface = DBusSystemd1Iface + ".Manager"
	// DBusRestartUnit is the dbus method for restarting systemd units.
	DBusRestartUnit = DBusSystemd1ManagerIface + ".RestartUnit"
	// DBusStopUnit is the dbus method for stopping systemd units.
	DBusStopUnit = DBusSystemd1ManagerIface + ".StopUnit"
	// DBusSignalJobRemoved is the name of the dbus signal that produces a
	// message when a dbus job is done (or has errored.)
	DBusSignalJobRemoved = "JobRemoved"
)

// ResPathUID returns a unique resource UID based on its name and kind. It's
// safe to use as a token in a path, and as a result has no slashes in it.
func ResPathUID(res engine.Res) string {
	// res.Name() is NOT sufficiently unique to use as a UID here, because:
	// a name of: /tmp/mgmt/foo is /tmp-mgmt-foo and
	// a name of: /tmp/mgmt-foo -> /tmp-mgmt-foo if we replace slashes.
	// As a result, we base64 encode (but without slashes).
	name := strings.ReplaceAll(res.Name(), "/", "-")
	if os.PathSeparator != '/' { // lol windows?
		name = strings.ReplaceAll(name, string(os.PathSeparator), "-")
	}
	b := []byte(res.Name())
	encoded := base64.URLEncoding.EncodeToString(b)
	// Add the safe name on so that it's easier to identify by name...
	return fmt.Sprintf("%s-%s+%s", res.Kind(), encoded, name)
}

// ResToB64 encodes a resource to a base64 encoded string (after serialization).
func ResToB64(res engine.Res) (string, error) {
	b := bytes.Buffer{}
	e := gob.NewEncoder(&b)
	err := e.Encode(&res) // pass with &
	if err != nil {
		return "", errwrap.Wrapf(err, "gob failed to encode")
	}
	return base64.StdEncoding.EncodeToString(b.Bytes()), nil
}

// B64ToRes decodes a resource from a base64 encoded string (after
// deserialization).
func B64ToRes(str string) (engine.Res, error) {
	var output interface{}
	bb, err := base64.StdEncoding.DecodeString(str)
	if err != nil {
		return nil, errwrap.Wrapf(err, "base64 failed to decode")
	}
	b := bytes.NewBuffer(bb)
	d := gob.NewDecoder(b)
	if err := d.Decode(&output); err != nil { // pass with &
		return nil, errwrap.Wrapf(err, "gob failed to decode")
	}
	res, ok := output.(engine.Res)
	if !ok {
		return nil, fmt.Errorf("output `%v` is not a Res", output)
	}
	return res, nil
}

// StructTagToFieldName returns a mapping from recommended alias to actual field
// name. It returns an error if it finds a collision. It uses the `lang` tags.
// It must be passed a ptr to a struct or it will error.
func StructTagToFieldName(stptr interface{}) (map[string]string, error) {
	// TODO: fallback to looking up yaml tags, although harder to parse
	result := make(map[string]string) // `lang` field tag -> field name
	if stptr == nil {
		return nil, fmt.Errorf("got nil input instead of ptr to struct")
	}
	typ := reflect.TypeOf(stptr)
	if k := typ.Kind(); k != reflect.Ptr { // we only look at *Struct's
		return nil, fmt.Errorf("input is not a ptr, got: %+v", k)
	}
	st := typ.Elem()                         // elem for ptr to struct (dereference the pointer)
	if k := st.Kind(); k != reflect.Struct { // this should be a struct now
		return nil, fmt.Errorf("input doesn't point to a struct, got: %+v", k)
	}

	for i := 0; i < st.NumField(); i++ {
		field := st.Field(i)
		name := field.Name
		// if !ok, then nothing is found
		if alias, ok := field.Tag.Lookup(types.StructTag); ok { // golang 1.7+
			if val, exists := result[alias]; exists {
				return nil, fmt.Errorf("field `%s` uses the same key `%s` as field `%s`", name, alias, val)
			}
			// empty string ("") is a valid value
			if alias != "" {
				result[alias] = name
			}
		}
	}
	return result, nil
}

// StructFieldCompat returns whether a send struct and key is compatible with a
// recv struct and key. This inputs must both be a ptr to a string, and a valid
// key that can be found in the struct tag. The (1) first values are for send,
// and the (2) second values are for recv.
// TODO: add a bool to decide if *string to string or string to *string is okay.
func StructFieldCompat(st1 interface{}, key1 string, st2 interface{}, key2 string) error {
	m1, err := StructTagToFieldName(st1)
	if err != nil {
		return err
	}
	k1, exists := m1[key1]
	if !exists {
		return fmt.Errorf("key not found in send struct")
	}

	m2, err := StructTagToFieldName(st2)
	if err != nil {
		return err
	}
	k2, exists := m2[key2]
	if !exists {
		return fmt.Errorf("key not found in recv struct")
	}

	obj1 := reflect.Indirect(reflect.ValueOf(st1))
	//type1 := obj1.Type()
	value1 := obj1.FieldByName(k1)
	kind1 := value1.Kind()

	obj2 := reflect.Indirect(reflect.ValueOf(st2))
	//type2 := obj2.Type()
	value2 := obj2.FieldByName(k2)
	kind2 := value2.Kind()

	if !value2.CanSet() { // if we can't set, then this is pointless!
		return fmt.Errorf("can't set")
	}

	// if we can't interface, we can't compare...
	if !value1.CanInterface() {
		return fmt.Errorf("can't interface the send")
	}
	if !value2.CanInterface() {
		return fmt.Errorf("can't interface the recv")
	}

	// If we're sending _from_ an interface... (value res `any` field)
	if kind1 == reflect.Interface || kind1 == reflect.Ptr {
		// TODO: Can we do more checks instead of only returning early?
		return nil
	}
	// If we're sending _to_ an interface... (value res `any` field)
	if kind2 == reflect.Interface || kind2 == reflect.Ptr {
		// TODO: Can we do more checks instead of only returning early?
		return nil
	}

	if kind1 != kind2 {
		return fmt.Errorf("field kind mismatch between %s and %s", kind1, kind2)
	}

	if t1, t2 := value1.Type(), value2.Type(); t1 != t2 {
		return fmt.Errorf("type mismatch between %s and %s", t1, t2)
	}

	return nil
}

// LowerStructFieldNameToFieldName returns a mapping from the lower case version
// of each field name to the actual field name. It only returns public fields.
// It returns an error if it finds a collision.
func LowerStructFieldNameToFieldName(res engine.Res) (map[string]string, error) {
	result := make(map[string]string) // lower field name -> field name
	st := reflect.TypeOf(res).Elem()  // elem for ptr to res
	for i := 0; i < st.NumField(); i++ {
		field := st.Field(i)
		name := field.Name

		if strings.Title(name) != name { // must have been a priv field
			continue
		}

		if alias := strings.ToLower(name); alias != "" {
			if val, exists := result[alias]; exists {
				return nil, fmt.Errorf("field `%s` uses the same key `%s` as field `%s`", name, alias, val)
			}
			result[alias] = name
		}
	}
	return result, nil
}

// LangFieldNameToStructFieldName returns the mapping from lang (AST) field
// names to field name as used in the struct. The logic here is a bit strange;
// if the resource has struct tags, then it uses those, otherwise it falls back
// to using the lower case versions of things. It might be clever to combine the
// two so that tagged fields are used as such, and others are used in lowercase,
// but this is currently not implemented.
// TODO: should this behaviour be changed?
func LangFieldNameToStructFieldName(kind string) (map[string]string, error) {
	res, err := engine.NewResource(kind)
	if err != nil {
		return nil, err
	}
	mapping, err := StructTagToFieldName(res)
	if err != nil {
		return nil, errwrap.Wrapf(err, "resource kind `%s` has bad field mapping", kind)
	}
	if len(mapping) == 0 { // if no `lang` tags exist, get them automatically
		mapping, err = LowerStructFieldNameToFieldName(res)
		if err != nil {
			return nil, errwrap.Wrapf(err, "resource kind `%s` has bad automatic field mapping", kind)
		}
	}

	return mapping, nil // lang field name -> field name
}

// LangFieldNameToStructType returns the mapping from lang (AST) field names,
// and the expected type in our type system for each.
// XXX: Should this return unification variables instead of variant types?
func LangFieldNameToStructType(kind string) (map[string]*types.Type, error) {
	res, err := engine.NewResource(kind)
	if err != nil {
		return nil, err
	}

	gtyp := reflect.TypeOf(res)
	st, err := types.ResTypeOf(gtyp)
	if err != nil {
		return nil, err
	}
	if st == nil {
		return nil, fmt.Errorf("got empty type")
	}
	if st.Kind != types.KindStruct {
		return nil, fmt.Errorf("not a struct kind")
	}
	// unpack the top-level struct, it should have the field names matching
	// the parameters of the struct.

	return st.Map, nil
}

// ResToParamValues returns a list of field names and their corresponding values
// if they are non-zero. This is meant for testing, and should be improved for
// robustness or with tests if it's ever used for value extraction. This also
// contains a hack to specifically print from resources that contain interface
// fields too. Consider moving that into types.ValueOf after testing if that
// doesn't break other code paths.
func ResToParamValues(res engine.Res) (map[string]types.Value, error) {

	ret := make(map[string]types.Value)
	st := reflect.ValueOf(res).Elem() // pointer to struct, then struct
	tt := reflect.TypeOf(res).Elem()  // pointer to struct, then struct

	fields := []string{}
	// TODO: private fields inside of a struct are still printed
	vf := reflect.VisibleFields(tt) // []reflect.StructField
	for _, field := range vf {
		if field.Tag == "" {
			continue // skip
		}
		if _, ok := field.Tag.Lookup(types.StructTag); !ok {
			continue
		}

		fields = append(fields, field.Name)
	}

	for _, name := range fields {
		rval := st.FieldByName(name) // exported field type

		// TODO: zero fields inside of a struct are still printed
		if rval.IsZero() {
			continue // skip zero values
		}

		// TODO: consider turning this into types.ConfigurableValueOf
		// and allowing the `kind == reflect.Interface` option?
		val, err := types.ValueOf(rval)
		if err != nil {
			// This can happen for bad fields like "Base" and so on.
			// They are supposed to be skipped by the struct tag,
			// but if this changes and we need to label them, then
			// we can improve our above heuristic.
			return nil, fmt.Errorf("field `%s` does not have a valid value: %+v", name, err)
		}

		ret[name] = val
	}

	return ret, nil
}

// GetUID returns the UID of a user. It supports a UID or a username. Caller
// should first check user is not empty. It will return an error if it can't
// lookup the UID or username.
func GetUID(username string) (int, error) {
	userObj, err := user.LookupId(username)
	if err == nil {
		return strconv.Atoi(userObj.Uid)
	}

	userObj, err = user.Lookup(username)
	if err == nil {
		return strconv.Atoi(userObj.Uid)
	}

	return -1, errwrap.Wrapf(err, "user lookup error (%s)", username)
}

// GetGID returns the GID of a group. It supports a GID or a group name. Caller
// should first check group is not empty. It will return an error if it can't
// lookup the GID or group name.
func GetGID(group string) (int, error) {
	groupObj, err := user.LookupGroupId(group)
	if err == nil {
		return strconv.Atoi(groupObj.Gid)
	}

	groupObj, err = user.LookupGroup(group)
	if err == nil {
		return strconv.Atoi(groupObj.Gid)
	}

	return -1, errwrap.Wrapf(err, "group lookup error (%s)", group)
}

// GetUIDGID is a small helper function to return the current uid and gid of the
// user running this program. If invoked under `sudo` it works as expected to
// return the root uid and gid.
func GetUIDGID() (int, int, error) {
	currentUser, err := user.Current()
	if err != nil {
		return -1, -1, err
	}

	uid, err := strconv.Atoi(currentUser.Uid)
	if err != nil {
		return -1, -1, err
	}

	gid, err := strconv.Atoi(currentUser.Gid)
	if err != nil {
		return -1, -1, err
	}

	return uid, gid, nil
}

// ReadData is a helper to read data, usually from our vardir directory. If the
// data is nil, this means the file didn't exist.
func ReadData(p string) (*string, error) {
	file, err := os.Open(p) // open a handle to read the file
	if os.IsNotExist(err) {
		return nil, nil // no file
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()
	data, err := io.ReadAll(file)
	if err != nil {
		return nil, err
	}
	s := string(data)
	return &s, nil
}

// WriteData is a helper to write data, usually into our vardir directory. If
// the data is nil, this counts as a request to delete the file.
func WriteData(p string, data *string) (int, error) {
	if data == nil {
		err := os.Remove(p)
		if os.IsNotExist(err) {
			return 0, nil // no file
		}
		return -1, err
	}

	uid, gid, err := GetUIDGID()
	if err != nil {
		return -1, err
	}

	// Chmod it before we write the secret data.
	file, err := os.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	//file, err := os.Create(p) // open a handle to create the file
	if err != nil {
		return -1, err
	}
	defer file.Close()

	// Chown it before we write the secret data.
	if err := file.Chown(uid, gid); err != nil {
		return -1, err
	}

	c, err := file.Write([]byte(*data))
	if err != nil {
		return c, errwrap.Wrapf(err, "can't write file")
	}
	return c, file.Sync()
}

// RestartUnit restarts the given dbus unit and waits for it to finish starting.
func RestartUnit(ctx context.Context, conn *dbus.Conn, unit string) error {
	return unitStateAction(ctx, conn, unit, DBusRestartUnit)
}

// StopUnit stops the given dbus unit and waits for it to finish stopping.
func StopUnit(ctx context.Context, conn *dbus.Conn, unit string) error {
	return unitStateAction(ctx, conn, unit, DBusStopUnit)
}

// unitStateAction is a helper function to perform state actions on systemd
// units. It waits for the requested job to be complete before it returns.
func unitStateAction(ctx context.Context, conn *dbus.Conn, unit, action string) error {
	// Add a dbus rule to watch the systemd1 JobRemoved signal, used to wait
	// until the job completes.
	args := []string{
		"type='signal'",
		fmt.Sprintf("path='%s'", DBusSystemd1Path),
		fmt.Sprintf("interface='%s'", DBusSystemd1ManagerIface),
		fmt.Sprintf("member='%s'", DBusSignalJobRemoved),
		fmt.Sprintf("arg2='%s'", unit),
	}
	// match dbus messages
	if call := conn.BusObject().Call(DBusAddMatch, 0, strings.Join(args, ",")); call.Err != nil {
		return errwrap.Wrapf(call.Err, "error creating dbus call")
	}
	defer conn.BusObject().Call(DBusRemoveMatch, 0, args) // ignore the error

	// channel for godbus signal
	ch := make(chan *dbus.Signal)
	defer close(ch)
	// subscribe the channel to the signal
	conn.Signal(ch)
	defer conn.RemoveSignal(ch)

	// perform requested action on specified unit
	sd1 := conn.Object(DBusSystemd1Iface, dbus.ObjectPath(DBusSystemd1Path))
	if call := sd1.Call(action, 0, unit, "fail"); call.Err != nil {
		return errwrap.Wrapf(call.Err, "error stopping unit: %s", unit)
	}

	// wait for the job to be removed, indicating completion
	select {
	case event, ok := <-ch:
		if !ok {
			return fmt.Errorf("channel closed unexpectedly")
		}
		if event.Body[3] != "done" {
			return fmt.Errorf("unexpected job status: %s", event.Body[3])
		}
	case <-ctx.Done():
		return fmt.Errorf("action %s on %s failed due to context timeout", action, unit)
	}
	return nil
}

// autoEdgeCombiner holds the state of the auto edge generator.
type autoEdgeCombiner struct {
	ae  []engine.AutoEdge
	ptr int
}

// Next returns the next automatic edge.
func (obj *autoEdgeCombiner) Next() []engine.ResUID {
	if len(obj.ae) <= obj.ptr {
		panic("shouldn't be called anymore!")
	}
	return obj.ae[obj.ptr].Next() // return the next edge
}

// Test takes the output of the last call to Next() and outputs true if we
// should continue.
func (obj *autoEdgeCombiner) Test(input []bool) bool {
	if !obj.ae[obj.ptr].Test(input) {
		obj.ptr++ // match found, on to the next
	}
	return len(obj.ae) > obj.ptr // are there any auto edges left?
}

// AutoEdgeCombiner takes any number of AutoEdge structs, and combines them into
// a single one, so that the logic from each one can be built separately, and
// then combined using this utility. This makes implementing different AutoEdge
// generators much easier. This respects the Next() and Test() API, and ratchets
// through each AutoEdge entry until they have all run their course.
func AutoEdgeCombiner(ae ...engine.AutoEdge) (engine.AutoEdge, error) {
	return &autoEdgeCombiner{
		ae: ae,
	}, nil
}

// CleanError takes the engine errors and prints them on a single line.
// TODO: maybe we can improve this here, it's a bit ugly.
func CleanError(err error) string {
	if err == nil {
		return fmt.Sprintf("%v", err)
	}
	return strings.ReplaceAll(err.Error(), "\n", " ")
}

// DebugStructFields returns a pretty string display of struct fields (like a
// resource) for debugging. The output is not guaranteed to be stable.
func DebugStructFields(st interface{}) string {
	s := ""
	v := reflect.ValueOf(st)
	t := reflect.TypeOf(st)

	// if it's a pointer, get the element it points to
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
		t = t.Elem()
	}

	for i := 0; i < v.NumField(); i++ {
		field := t.Field(i)
		value := v.Field(i)

		// only print exported (public) fields
		if field.PkgPath != "" {
			continue
		}
		if value.IsZero() {
			s += fmt.Sprintf("(%s): %v\n", field.Name, "<nil>")
		} else {
			s += fmt.Sprintf("(%s): %v\n", field.Name, value.Elem().Interface())
		}
	}

	return s
}
