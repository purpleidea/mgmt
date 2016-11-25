// Mgmt
// Copyright (C) 2013-2016+ James Shubin and the project contributors
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
	"encoding/gob"
	"fmt"
	"log"
	"math/rand"
	"net/url"
	"time"

	"github.com/purpleidea/mgmt/event"
	"github.com/purpleidea/mgmt/global"

	errwrap "github.com/pkg/errors"
	"github.com/rgbkrk/libvirt-go"
)

func init() {
	gob.Register(&VirtRes{})
}

var (
	libvirtInitialized = false
)

type virtURISchemeType int

const (
	defaultURI virtURISchemeType = iota
	lxcURI
)

// VirtAuth is used to pass credentials to libvirt.
type VirtAuth struct {
	Username string `yaml:"username"`
	Password string `yaml:"password"`
}

// VirtRes is a libvirt resource. A transient virt resource, which has its state
// set to `shutoff` is one which does not exist. The parallel equivalent is a
// file resource which removes a particular path.
type VirtRes struct {
	BaseRes    `yaml:",inline"`
	URI        string             `yaml:"uri"`       // connection uri, eg: qemu:///session
	State      string             `yaml:"state"`     // running, paused, shutoff
	Transient  bool               `yaml:"transient"` // defined (false) or undefined (true)
	CPUs       uint16             `yaml:"cpus"`
	Memory     uint64             `yaml:"memory"` // in KBytes
	OSInit     string             `yaml:"osinit"` // init used by lxc
	Boot       []string           `yaml:"boot"`   // boot order. values: fd, hd, cdrom, network
	Disk       []diskDevice       `yaml:"disk"`
	CDRom      []cdRomDevice      `yaml:"cdrom"`
	Network    []networkDevice    `yaml:"network"`
	Filesystem []filesystemDevice `yaml:"filesystem"`
	Auth       *VirtAuth          `yaml:"auth"`

	conn      libvirt.VirConnection
	absent    bool // cached state
	uriScheme virtURISchemeType
}

// NewVirtRes is a constructor for this resource. It also calls Init() for you.
func NewVirtRes(name string, uri, state string, transient bool, cpus uint16, memory uint64, osinit string) (*VirtRes, error) {
	obj := &VirtRes{
		BaseRes: BaseRes{
			Name: name,
		},
		URI:       uri,
		State:     state,
		Transient: transient,
		CPUs:      cpus,
		Memory:    memory,
		OSInit:    osinit,
	}
	return obj, obj.Init()
}

// Init runs some startup code for this resource.
func (obj *VirtRes) Init() error {
	if !libvirtInitialized {
		if err := libvirt.EventRegisterDefaultImpl(); err != nil {
			return errwrap.Wrapf(err, "EventRegisterDefaultImpl failed")
		}
		libvirtInitialized = true
	}
	var u *url.URL
	var err error
	if u, err = url.Parse(obj.URI); err != nil {
		return errwrap.Wrapf(err, "%s[%s]: Parsing URI failed: %s", obj.Kind(), obj.GetName(), obj.URI)
	}
	switch u.Scheme {
	case "lxc":
		obj.uriScheme = lxcURI
	}

	obj.absent = (obj.Transient && obj.State == "shutoff") // machine shouldn't exist

	obj.BaseRes.kind = "Virt"
	return obj.BaseRes.Init() // call base init, b/c we're overriding
}

// Validate if the params passed in are valid data.
func (obj *VirtRes) Validate() error {
	return nil
}

func (obj *VirtRes) connect() (conn libvirt.VirConnection, err error) {
	if obj.Auth != nil {
		conn, err = libvirt.NewVirConnectionWithAuth(obj.URI, obj.Auth.Username, obj.Auth.Password)
	}
	if obj.Auth == nil || err != nil {
		conn, err = libvirt.NewVirConnection(obj.URI)
	}
	return
}

// Watch is the primary listener for this resource and it outputs events.
func (obj *VirtRes) Watch(processChan chan event.Event) error {
	if obj.IsWatching() {
		return nil
	}
	obj.SetWatching(true)
	defer obj.SetWatching(false)
	cuid := obj.converger.Register()
	defer cuid.Unregister()

	var startup bool
	Startup := func(block bool) <-chan time.Time {
		if block {
			return nil // blocks forever
			//return make(chan time.Time) // blocks forever
		}
		return time.After(time.Duration(500) * time.Millisecond) // 1/2 the resolution of converged timeout
	}

	conn, err := obj.connect()
	if err != nil {
		return fmt.Errorf("Connection to libvirt failed with: %s", err)
	}

	eventChan := make(chan int) // TODO: do we need to buffer this?
	errorChan := make(chan error)
	exitChan := make(chan struct{})
	defer close(exitChan)

	// run libvirt event loop
	// TODO: *trigger* EventRunDefaultImpl to unblock so it can shut down...
	// at the moment this isn't a major issue because it seems to unblock in
	// bursts every 5 seconds! we can do this by writing to an event handler
	// in the meantime, terminating the program causes it to exit anyways...
	go func() {
		for {
			// TODO: can we merge this into our main for loop below?
			select {
			case <-exitChan:
				log.Printf("EventRunDefaultImpl exited!")
				return
			default:
			}
			//log.Printf("EventRunDefaultImpl started!")
			if err := libvirt.EventRunDefaultImpl(); err != nil {
				errorChan <- errwrap.Wrapf(err, "EventRunDefaultImpl failed")
				return
			}
			//log.Printf("EventRunDefaultImpl looped!")
		}
	}()

	callback := libvirt.DomainEventCallback(
		func(c *libvirt.VirConnection, d *libvirt.VirDomain, eventDetails interface{}, f func()) int {
			if lifecycleEvent, ok := eventDetails.(libvirt.DomainLifecycleEvent); ok {
				domName, _ := d.GetName()
				if domName == obj.GetName() {
					eventChan <- lifecycleEvent.Event
				}
			} else if global.DEBUG {
				log.Printf("%s[%s]: Event details isn't DomainLifecycleEvent", obj.Kind(), obj.GetName())
			}
			return 0
		},
	)
	callbackID := conn.DomainEventRegister(
		libvirt.VirDomain{},
		libvirt.VIR_DOMAIN_EVENT_ID_LIFECYCLE,
		&callback,
		nil,
	)
	defer conn.DomainEventDeregister(callbackID)

	var send = false
	var exit = false

	for {
		select {
		case event := <-eventChan:
			// TODO: shouldn't we do these checks in CheckApply ?
			switch event {
			case libvirt.VIR_DOMAIN_EVENT_DEFINED:
				if obj.Transient {
					obj.StateOK(false) // dirty
					send = true
				}
			case libvirt.VIR_DOMAIN_EVENT_UNDEFINED:
				if !obj.Transient {
					obj.StateOK(false) // dirty
					send = true
				}
			case libvirt.VIR_DOMAIN_EVENT_STARTED:
				fallthrough
			case libvirt.VIR_DOMAIN_EVENT_RESUMED:
				if obj.State != "running" {
					obj.StateOK(false) // dirty
					send = true
				}
			case libvirt.VIR_DOMAIN_EVENT_SUSPENDED:
				if obj.State != "paused" {
					obj.StateOK(false) // dirty
					send = true
				}
			case libvirt.VIR_DOMAIN_EVENT_STOPPED:
				fallthrough
			case libvirt.VIR_DOMAIN_EVENT_SHUTDOWN:
				if obj.State != "shutoff" {
					obj.StateOK(false) // dirty
					send = true
				}
			case libvirt.VIR_DOMAIN_EVENT_PMSUSPENDED:
				fallthrough
			case libvirt.VIR_DOMAIN_EVENT_CRASHED:
				obj.StateOK(false) // dirty
				send = true
			}

		case err := <-errorChan:
			cuid.SetConverged(false)
			return fmt.Errorf("Unknown %s[%s] libvirt error: %s", obj.Kind(), obj.GetName(), err)

		case event := <-obj.Events():
			cuid.SetConverged(false)
			if exit, send = obj.ReadEvent(&event); exit {
				return nil // exit
			}

		case <-cuid.ConvergedTimer():
			cuid.SetConverged(true) // converged!
			continue

		case <-Startup(startup):
			cuid.SetConverged(false)
			send = true
			obj.StateOK(false) // dirty
		}

		if send {
			startup = true // startup finished
			send = false
			if exit, err := obj.DoSend(processChan, ""); exit || err != nil {
				return err // we exit or bubble up a NACK...
			}
		}
	}
}

// attrCheckApply performs the CheckApply functions for CPU, Memory and others.
// This shouldn't be called when the machine is absent; it won't be found!
func (obj *VirtRes) attrCheckApply(apply bool) (bool, error) {
	var checkOK = true

	dom, err := obj.conn.LookupDomainByName(obj.GetName())
	if err != nil {
		return false, errwrap.Wrapf(err, "conn.LookupDomainByName failed")
	}

	domInfo, err := dom.GetInfo()
	if err != nil {
		// we don't know if the state is ok
		return false, errwrap.Wrapf(err, "domain.GetInfo failed")
	}

	// check memory
	if domInfo.GetMemory() != obj.Memory {
		checkOK = false
		if !apply {
			return false, nil
		}
		if err := dom.SetMemory(obj.Memory); err != nil {
			return false, errwrap.Wrapf(err, "domain.SetMemory failed")
		}
		log.Printf("%s[%s]: Memory changed", obj.Kind(), obj.GetName())
	}

	// check cpus
	if domInfo.GetNrVirtCpu() != obj.CPUs {
		checkOK = false
		if !apply {
			return false, nil
		}
		if err := dom.SetVcpus(obj.CPUs); err != nil {
			return false, errwrap.Wrapf(err, "domain.SetVcpus failed")
		}
		log.Printf("%s[%s]: CPUs changed", obj.Kind(), obj.GetName())
	}

	return checkOK, nil
}

// domainCreate creates a transient or persistent domain in the correct state. It
// doesn't check the state before hand, as it is a simple helper function.
func (obj *VirtRes) domainCreate() (libvirt.VirDomain, bool, error) {

	if obj.Transient {
		var flag uint32
		var state string
		switch obj.State {
		case "running":
			flag = libvirt.VIR_DOMAIN_NONE
			state = "started"
		case "paused":
			flag = libvirt.VIR_DOMAIN_START_PAUSED
			state = "paused"
		case "shutoff":
			// a transient, shutoff machine, means machine is absent
			return libvirt.VirDomain{}, true, nil // returned dom is invalid
		}
		dom, err := obj.conn.DomainCreateXML(obj.getDomainXML(), flag)
		if err != nil {
			return dom, false, err // returned dom is invalid
		}
		log.Printf("%s[%s]: Domain transient %s", state, obj.Kind(), obj.GetName())
		return dom, false, nil
	}

	dom, err := obj.conn.DomainDefineXML(obj.getDomainXML())
	if err != nil {
		return dom, false, err // returned dom is invalid
	}
	log.Printf("%s[%s]: Domain defined", obj.Kind(), obj.GetName())

	if obj.State == "running" {
		if err := dom.Create(); err != nil {
			return dom, false, err
		}
		log.Printf("%s[%s]: Domain started", obj.Kind(), obj.GetName())
	}

	if obj.State == "paused" {
		if err := dom.CreateWithFlags(libvirt.VIR_DOMAIN_START_PAUSED); err != nil {
			return dom, false, err
		}
		log.Printf("%s[%s]: Domain created paused", obj.Kind(), obj.GetName())
	}

	return dom, false, nil
}

// CheckApply checks the resource state and applies the resource if the bool
// input is true. It returns error info and if the state check passed or not.
func (obj *VirtRes) CheckApply(apply bool) (bool, error) {
	var err error
	obj.conn, err = obj.connect()
	if err != nil {
		return false, fmt.Errorf("Connection to libvirt failed with: %s", err)
	}

	var checkOK = true

	dom, err := obj.conn.LookupDomainByName(obj.GetName())
	if err == nil {
		// pass
	} else if virErr, ok := err.(libvirt.VirError); ok && virErr.Code == libvirt.VIR_ERR_NO_DOMAIN {
		// domain not found
		if obj.absent {
			return true, nil
		}

		if !apply {
			return false, nil
		}

		var c = true
		dom, c, err = obj.domainCreate() // create the domain
		if err != nil {
			return false, errwrap.Wrapf(err, "domainCreate failed")
		} else if !c {
			checkOK = false
		}

	} else {
		return false, errwrap.Wrapf(err, "LookupDomainByName failed")
	}
	defer dom.Free()
	// domain exists

	domInfo, err := dom.GetInfo()
	if err != nil {
		// we don't know if the state is ok
		return false, errwrap.Wrapf(err, "domain.GetInfo failed")
	}
	isPersistent, err := dom.IsPersistent()
	if err != nil {
		// we don't know if the state is ok
		return false, errwrap.Wrapf(err, "domain.IsPersistent failed")
	}
	isActive, err := dom.IsActive()
	if err != nil {
		// we don't know if the state is ok
		return false, errwrap.Wrapf(err, "domain.IsActive failed")
	}

	// check for persistence
	if isPersistent == obj.Transient { // if they're different!
		if !apply {
			return false, nil
		}
		if isPersistent {
			if err := dom.Undefine(); err != nil {
				return false, errwrap.Wrapf(err, "domain.Undefine failed")
			}
			log.Printf("%s[%s]: Domain undefined", obj.Kind(), obj.GetName())
		} else {
			domXML, err := dom.GetXMLDesc(libvirt.VIR_DOMAIN_XML_INACTIVE)
			if err != nil {
				return false, errwrap.Wrapf(err, "domain.GetXMLDesc failed")
			}
			if _, err = obj.conn.DomainDefineXML(domXML); err != nil {
				return false, errwrap.Wrapf(err, "conn.DomainDefineXML failed")
			}
			log.Printf("%s[%s]: Domain defined", obj.Kind(), obj.GetName())
		}
		checkOK = false
	}

	// check for valid state
	domState := domInfo.GetState()
	switch obj.State {
	case "running":
		if domState == libvirt.VIR_DOMAIN_RUNNING {
			break
		}
		if domState == libvirt.VIR_DOMAIN_BLOCKED {
			// TODO: what should happen?
			return false, fmt.Errorf("Domain %s is blocked!", obj.GetName())
		}
		if !apply {
			return false, nil
		}
		if isActive { // domain must be paused ?
			if err := dom.Resume(); err != nil {
				return false, errwrap.Wrapf(err, "domain.Resume failed")
			}
			checkOK = false
			log.Printf("%s[%s]: Domain resumed", obj.Kind(), obj.GetName())
			break
		}
		if err := dom.Create(); err != nil {
			return false, errwrap.Wrapf(err, "domain.Create failed")
		}
		checkOK = false
		log.Printf("%s[%s]: Domain created", obj.Kind(), obj.GetName())

	case "paused":
		if domState == libvirt.VIR_DOMAIN_PAUSED {
			break
		}
		if !apply {
			return false, nil
		}
		if isActive { // domain must be running ?
			if err := dom.Suspend(); err != nil {
				return false, errwrap.Wrapf(err, "domain.Suspend failed")
			}
			checkOK = false
			log.Printf("%s[%s]: Domain paused", obj.Kind(), obj.GetName())
			break
		}
		if err := dom.CreateWithFlags(libvirt.VIR_DOMAIN_START_PAUSED); err != nil {
			return false, errwrap.Wrapf(err, "domain.CreateWithFlags failed")
		}
		checkOK = false
		log.Printf("%s[%s]: Domain created paused", obj.Kind(), obj.GetName())

	case "shutoff":
		if domState == libvirt.VIR_DOMAIN_SHUTOFF || domState == libvirt.VIR_DOMAIN_SHUTDOWN {
			break
		}
		if !apply {
			return false, nil
		}

		if err := dom.Destroy(); err != nil {
			return false, errwrap.Wrapf(err, "domain.Destroy failed")
		}
		checkOK = false
		log.Printf("%s[%s]: Domain destroyed", obj.Kind(), obj.GetName())
	}

	if !apply {
		return false, nil
	}
	// remaining apply portion

	// mem & cpu checks...
	if !obj.absent {
		if c, err := obj.attrCheckApply(apply); err != nil {
			return false, errwrap.Wrapf(err, "attrCheckApply failed")
		} else if !c {
			checkOK = false
		}
	}

	return checkOK, nil // w00t
}

// Return the correct domain type based on the uri
func (obj VirtRes) getDomainType() string {
	switch obj.uriScheme {
	case lxcURI:
		return "<domain type='lxc'>"
	default:
		return "<domain type='kvm'>"
	}
}

// Return the correct os type based on the uri
func (obj VirtRes) getOSType() string {
	switch obj.uriScheme {
	case lxcURI:
		return "<type>exe</type>"
	default:
		return "<type>hvm</type>"
	}
}

func (obj VirtRes) getOSInit() string {
	switch obj.uriScheme {
	case lxcURI:
		return fmt.Sprintf("<init>%s</init>", obj.OSInit)
	default:
		return ""
	}
}

func (obj *VirtRes) getDomainXML() string {
	var b string
	b += obj.getDomainType() // start domain

	b += fmt.Sprintf("<name>%s</name>", obj.GetName())
	b += fmt.Sprintf("<memory unit='KiB'>%d</memory>", obj.Memory)
	b += fmt.Sprintf("<vcpu>%d</vcpu>", obj.CPUs)

	b += "<os>"
	b += obj.getOSType()
	b += obj.getOSInit()
	if obj.Boot != nil {
		for _, boot := range obj.Boot {
			b += fmt.Sprintf("<boot dev='%s'/>", boot)
		}
	}
	b += fmt.Sprintf("</os>")

	b += fmt.Sprintf("<devices>") // start devices

	if obj.Disk != nil {
		for i, disk := range obj.Disk {
			b += fmt.Sprintf(disk.GetXML(i))
		}
	}

	if obj.CDRom != nil {
		for i, cdrom := range obj.CDRom {
			b += fmt.Sprintf(cdrom.GetXML(i))
		}
	}

	if obj.Network != nil {
		for i, net := range obj.Network {
			b += fmt.Sprintf(net.GetXML(i))
		}
	}

	if obj.Filesystem != nil {
		for i, fs := range obj.Filesystem {
			b += fmt.Sprintf(fs.GetXML(i))
		}
	}

	b += "<serial type='pty'><target port='0'/></serial>"
	b += "<console type='pty'><target type='serial' port='0'/></console>"
	b += "</devices>" // end devices
	b += "</domain>"  // end domain
	return b
}

type virtDevice interface {
	GetXML(idx int) string
}

type diskDevice struct {
	Source string `yaml:"source"`
	Type   string `yaml:"type"`
}

type cdRomDevice struct {
	Source string `yaml:"source"`
	Type   string `yaml:"type"`
}

type networkDevice struct {
	Name string `yaml:"name"`
	MAC  string `yaml:"mac"`
}

type filesystemDevice struct {
	Access   string `yaml:"access"`
	Source   string `yaml:"source"`
	Target   string `yaml:"target"`
	ReadOnly bool   `yaml:"read_only"`
}

func (d *diskDevice) GetXML(idx int) string {
	var b string
	b += "<disk type='file' device='disk'>"
	b += fmt.Sprintf("<driver name='qemu' type='%s'/>", d.Type)
	b += fmt.Sprintf("<source file='%s'/>", d.Source)
	b += fmt.Sprintf("<target dev='vd%s' bus='virtio'/>", (string)(idx+97)) // TODO: 26, 27... should be 'aa', 'ab'...
	b += "</disk>"
	return b
}

func (d *cdRomDevice) GetXML(idx int) string {
	var b string
	b += "<disk type='file' device='cdrom'>"
	b += fmt.Sprintf("<driver name='qemu' type='%s'/>", d.Type)
	b += fmt.Sprintf("<source file='%s'/>", d.Source)
	b += fmt.Sprintf("<target dev='hd%s' bus='ide'/>", (string)(idx+97)) // TODO: 26, 27... should be 'aa', 'ab'...
	b += "<readonly/>"
	b += "</disk>"
	return b
}

func (d *networkDevice) GetXML(idx int) string {
	if d.MAC == "" {
		d.MAC = randMAC()
	}
	var b string
	b += "<interface type='network'>"
	b += fmt.Sprintf("<mac address='%s'/>", d.MAC)
	b += fmt.Sprintf("<source network='%s'/>", d.Name)
	b += "</interface>"
	return b
}

func (d *filesystemDevice) GetXML(idx int) string {
	var b string
	b += "<filesystem" // open
	if d.Access != "" {
		b += fmt.Sprintf(" accessmode='%s'", d.Access)
	}
	b += ">" // close
	b += fmt.Sprintf("<source dir='%s'/>", d.Source)
	b += fmt.Sprintf("<target dir='%s'/>", d.Target)
	if d.ReadOnly {
		b += "<readonly/>"
	}
	b += "</filesystem>"
	return b
}

// VirtUID is the UID struct for FileRes.
type VirtUID struct {
	BaseUID
}

// GetUIDs includes all params to make a unique identification of this object.
// Most resources only return one, although some resources can return multiple.
func (obj *VirtRes) GetUIDs() []ResUID {
	x := &VirtUID{
		BaseUID: BaseUID{name: obj.GetName(), kind: obj.Kind()},
		// TODO: add more properties here so we can link to vm dependencies
	}
	return []ResUID{x}
}

// GroupCmp returns whether two resources can be grouped together or not.
func (obj *VirtRes) GroupCmp(r Res) bool {
	_, ok := r.(*VirtRes)
	if !ok {
		return false
	}
	return false // not possible atm
}

// AutoEdges returns the AutoEdge interface. In this case no autoedges are used.
func (obj *VirtRes) AutoEdges() AutoEdge {
	return nil
}

// Compare two resources and return if they are equivalent.
func (obj *VirtRes) Compare(res Res) bool {
	switch res.(type) {
	case *VirtRes:
		res := res.(*VirtRes)
		if !obj.BaseRes.Compare(res) { // call base Compare
			return false
		}

		if obj.Name != res.Name {
			return false
		}
		if obj.URI != res.URI {
			return false
		}
		if obj.State != res.State {
			return false
		}
		if obj.Transient != res.Transient {
			return false
		}
		if obj.CPUs != res.CPUs {
			return false
		}
		// TODO: can we skip the compare of certain properties such as
		// Memory because this object (but with different memory) can be
		// *converted* into the new version that has more/less memory?
		// We would need to run some sort of "old struct update", to get
		// the new values, but that's easy to add.
		if obj.Memory != res.Memory {
			return false
		}
		// TODO:
		//if obj.Boot != res.Boot {
		//	return false
		//}
		//if obj.Disk != res.Disk {
		//	return false
		//}
		//if obj.CDRom != res.CDRom {
		//	return false
		//}
		//if obj.Network != res.Network {
		//	return false
		//}
		//if obj.Filesystem != res.Filesystem {
		//	return false
		//}
	default:
		return false
	}
	return true
}

// CollectPattern applies the pattern for collection resources.
func (obj *VirtRes) CollectPattern(string) {
}

// randMAC returns a random mac address in the libvirt range.
func randMAC() string {
	rand.Seed(time.Now().UnixNano())
	return "52:54:00" +
		fmt.Sprintf(":%x", rand.Intn(255)) +
		fmt.Sprintf(":%x", rand.Intn(255)) +
		fmt.Sprintf(":%x", rand.Intn(255))
}
