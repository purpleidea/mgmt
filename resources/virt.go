// Mgmt
// Copyright (C) 2013-2017+ James Shubin and the project contributors
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
// +build !novirt

package resources

import (
	"encoding/gob"
	"fmt"
	"log"
	"math/rand"
	"net/url"
	"os/user"
	"path"
	"strings"
	"sync"
	"time"

	multierr "github.com/hashicorp/go-multierror"
	"github.com/libvirt/libvirt-go"
	libvirtxml "github.com/libvirt/libvirt-go-xml"
	errwrap "github.com/pkg/errors"
)

func init() {
	gob.Register(&VirtRes{})
}

const (
	// DefaultMaxCPUs is the default number of possible cpu "slots" used.
	DefaultMaxCPUs = 32

	// MaxShutdownDelayTimeout is the max time we wait for a vm to shutdown.
	MaxShutdownDelayTimeout = 60 * 5 // seconds

	// ShortPollInterval is how often we poll when expecting an event.
	ShortPollInterval = 5 // seconds
)

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
	CPUs       uint               `yaml:"cpus"`
	MaxCPUs    uint               `yaml:"maxcpus"`
	Memory     uint64             `yaml:"memory"` // in KBytes
	OSInit     string             `yaml:"osinit"` // init used by lxc
	Boot       []string           `yaml:"boot"`   // boot order. values: fd, hd, cdrom, network
	Disk       []diskDevice       `yaml:"disk"`
	CDRom      []cdRomDevice      `yaml:"cdrom"`
	Network    []networkDevice    `yaml:"network"`
	Filesystem []filesystemDevice `yaml:"filesystem"`
	Auth       *VirtAuth          `yaml:"auth"`

	HotCPUs bool `yaml:"hotcpus"` // allow hotplug of cpus?
	// FIXME: values here should be enum's!
	RestartOnDiverge string `yaml:"restartondiverge"` // restart policy: "ignore", "ifneeded", "error"
	RestartOnRefresh bool   `yaml:"restartonrefresh"` // restart on refresh?

	wg                  *sync.WaitGroup
	conn                *libvirt.Connect
	version             uint32 // major * 1000000 + minor * 1000 + release
	absent              bool   // cached state
	uriScheme           virtURISchemeType
	processExitWatch    bool // do we want to wait on an explicit process exit?
	processExitChan     chan struct{}
	restartScheduled    bool // do we need to schedule a hard restart?
	guestAgentConnected bool // our tracking of if guest agent is running
}

// Default returns some sensible defaults for this resource.
func (obj *VirtRes) Default() Res {
	return &VirtRes{
		BaseRes: BaseRes{
			MetaParams: DefaultMetaParams, // force a default
		},

		MaxCPUs: DefaultMaxCPUs,
		HotCPUs: true, // we're a dynamic engine, be dynamic by default!

		RestartOnDiverge: "error", // safest default :(
	}
}

// Validate if the params passed in are valid data.
func (obj *VirtRes) Validate() error {
	if obj.CPUs > obj.MaxCPUs {
		return fmt.Errorf("the number of CPUs (%d) must not be greater than MaxCPUs (%d)", obj.CPUs, obj.MaxCPUs)
	}
	return obj.BaseRes.Validate()
}

// Init runs some startup code for this resource.
func (obj *VirtRes) Init() error {
	if !libvirtInitialized {
		if err := libvirt.EventRegisterDefaultImpl(); err != nil {
			return errwrap.Wrapf(err, "method EventRegisterDefaultImpl failed")
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

	obj.conn, err = obj.connect() // gets closed in Close method of Res API
	if err != nil {
		return errwrap.Wrapf(err, "%s[%s]: Connection to libvirt failed in init", obj.Kind(), obj.GetName())
	}

	// check for hard to change properties
	dom, err := obj.conn.LookupDomainByName(obj.GetName())
	if err == nil {
		defer dom.Free()
	} else if !isNotFound(err) {
		return errwrap.Wrapf(err, "%s[%s]: Could not lookup on init", obj.Kind(), obj.GetName())
	}

	if err == nil {
		// maxCPUs, err := dom.GetMaxVcpus()
		i, err := dom.GetVcpusFlags(libvirt.DOMAIN_VCPU_MAXIMUM)
		if err != nil {
			return errwrap.Wrapf(err, "%s[%s]: Could not lookup MaxCPUs on init", obj.Kind(), obj.GetName())
		}
		maxCPUs := uint(i)
		if obj.MaxCPUs != maxCPUs { // max cpu slots is hard to change
			// we'll need to reboot to fix this one...
			obj.restartScheduled = true
		}

		// parse running domain xml to read properties
		// FIXME: should we do this in Watch, after we register the
		// event handlers so that we don't miss any events via race?
		xmlDesc, err := dom.GetXMLDesc(0) // 0 means no flags
		if err != nil {
			return errwrap.Wrapf(err, "%s[%s]: Could not GetXMLDesc on init", obj.Kind(), obj.GetName())
		}
		domXML := &libvirtxml.Domain{}
		if err := domXML.Unmarshal(xmlDesc); err != nil {
			return errwrap.Wrapf(err, "%s[%s]: Could not unmarshal XML on init", obj.Kind(), obj.GetName())
		}

		// guest agent: domain->devices->channel->target->state == connected?
		for _, x := range domXML.Devices.Channels {
			if x.Target.Type == "virtio" && strings.HasPrefix(x.Target.Name, "org.qemu.guest_agent.") {
				// last connection found wins (usually 1 anyways)
				obj.guestAgentConnected = (x.Target.State == "connected")
			}
		}
	}
	obj.wg = &sync.WaitGroup{}
	obj.BaseRes.kind = "Virt"
	return obj.BaseRes.Init() // call base init, b/c we're overriding
}

// Close runs some cleanup code for this resource.
func (obj *VirtRes) Close() error {
	// By the time that this Close method is called, the engine promises
	// that the Watch loop has previously shutdown! (Assuming no bugs!)
	// TODO: As a result, this is an extra check which shouldn't be needed,
	// but which might mask possible engine bugs. Consider removing it!
	obj.wg.Wait()

	// TODO: what is the first int Close return value useful for (if at all)?
	_, err := obj.conn.Close() // close libvirt conn that was opened in Init
	obj.conn = nil             // set to nil to help catch any nil ptr bugs!

	// call base close, b/c we're overriding
	if e := obj.BaseRes.Close(); err == nil {
		err = e
	} else if e != nil {
		err = multierr.Append(err, e) // list of errors
	}
	return err
}

// connect is the connect helper for the libvirt connection. It can handle auth.
func (obj *VirtRes) connect() (conn *libvirt.Connect, err error) {
	if obj.Auth != nil {
		callback := func(creds []*libvirt.ConnectCredential) {
			// Populate credential structs with the
			// prepared username/password values
			for _, cred := range creds {
				if cred.Type == libvirt.CRED_AUTHNAME {
					cred.Result = obj.Auth.Username
					cred.ResultLen = len(cred.Result)
				} else if cred.Type == libvirt.CRED_PASSPHRASE {
					cred.Result = obj.Auth.Password
					cred.ResultLen = len(cred.Result)
				}
			}
		}
		auth := &libvirt.ConnectAuth{
			CredType: []libvirt.ConnectCredentialType{
				libvirt.CRED_AUTHNAME, libvirt.CRED_PASSPHRASE,
			},
			Callback: callback,
		}
		conn, err = libvirt.NewConnectWithAuth(obj.URI, auth, 0)
		if err == nil {
			if version, err := conn.GetLibVersion(); err == nil {
				obj.version = version
			}
		}
	}
	if obj.Auth == nil || err != nil {
		conn, err = libvirt.NewConnect(obj.URI)
		if err == nil {
			if version, err := conn.GetLibVersion(); err == nil {
				obj.version = version
			}
		}
	}
	return
}

// Watch is the primary listener for this resource and it outputs events.
func (obj *VirtRes) Watch() error {
	// FIXME: how will this work if we're polling?
	wg := &sync.WaitGroup{}
	defer wg.Wait()                               // wait until everyone has exited before we exit!
	domChan := make(chan libvirt.DomainEventType) // TODO: do we need to buffer this?
	gaChan := make(chan *libvirt.DomainEventAgentLifecycle)
	errorChan := make(chan error)
	exitChan := make(chan struct{})
	defer close(exitChan)
	obj.wg.Add(1) // don't exit without waiting for EventRunDefaultImpl
	wg.Add(1)

	// run libvirt event loop
	// TODO: *trigger* EventRunDefaultImpl to unblock so it can shut down...
	// at the moment this isn't a major issue because it seems to unblock in
	// bursts every 5 seconds! we can do this by writing to an event handler
	// in the meantime, terminating the program causes it to exit anyways...
	go func() {
		defer obj.wg.Done()
		defer wg.Done()
		defer log.Printf("EventRunDefaultImpl exited!")
		for {
			// TODO: can we merge this into our main for loop below?
			select {
			case <-exitChan:
				return
			default:
			}
			//log.Printf("EventRunDefaultImpl started!")
			if err := libvirt.EventRunDefaultImpl(); err != nil {
				select {
				case errorChan <- errwrap.Wrapf(err, "EventRunDefaultImpl failed"):
				case <-exitChan:
					// pass
				}
				return
			}
			//log.Printf("EventRunDefaultImpl looped!")
		}
	}()

	// domain events callback
	domCallback := func(c *libvirt.Connect, d *libvirt.Domain, ev *libvirt.DomainEventLifecycle) {
		domName, _ := d.GetName()
		if domName == obj.GetName() {
			select {
			case domChan <- ev.Event: // send
			case <-exitChan:
			}
		}
	}
	// if dom is nil, we get events for *all* domains!
	domCallbackID, err := obj.conn.DomainEventLifecycleRegister(nil, domCallback)
	if err != nil {
		return err
	}
	defer obj.conn.DomainEventDeregister(domCallbackID)

	// guest agent events callback
	gaCallback := func(c *libvirt.Connect, d *libvirt.Domain, eva *libvirt.DomainEventAgentLifecycle) {
		domName, _ := d.GetName()
		if domName == obj.GetName() {
			select {
			case gaChan <- eva: // send
			case <-exitChan:
			}
		}
	}
	gaCallbackID, err := obj.conn.DomainEventAgentLifecycleRegister(nil, gaCallback)
	if err != nil {
		return err
	}
	defer obj.conn.DomainEventDeregister(gaCallbackID)

	// notify engine that we're running
	if err := obj.Running(); err != nil {
		return err // bubble up a NACK...
	}

	var send = false
	var exit *error // if ptr exists, that is the exit error to return

	for {
		processExited := false // did the process exit fully (shutdown)?
		select {
		case event := <-domChan:
			// TODO: shouldn't we do these checks in CheckApply ?
			switch event {
			case libvirt.DOMAIN_EVENT_DEFINED:
				if obj.Transient {
					obj.StateOK(false) // dirty
					send = true
				}
			case libvirt.DOMAIN_EVENT_UNDEFINED:
				if !obj.Transient {
					obj.StateOK(false) // dirty
					send = true
				}
			case libvirt.DOMAIN_EVENT_STARTED:
				fallthrough
			case libvirt.DOMAIN_EVENT_RESUMED:
				if obj.State != "running" {
					obj.StateOK(false) // dirty
					send = true
				}
			case libvirt.DOMAIN_EVENT_SUSPENDED:
				if obj.State != "paused" {
					obj.StateOK(false) // dirty
					send = true
				}
			case libvirt.DOMAIN_EVENT_STOPPED:
				fallthrough
			case libvirt.DOMAIN_EVENT_SHUTDOWN:
				if obj.State != "shutoff" {
					obj.StateOK(false) // dirty
					send = true
				}
				processExited = true

			case libvirt.DOMAIN_EVENT_PMSUSPENDED:
				// FIXME: IIRC, in s3 we can't cold change
				// hardware like cpus but in s4 it's okay?
				// verify, detect and patch appropriately!
				fallthrough
			case libvirt.DOMAIN_EVENT_CRASHED:
				obj.StateOK(false) // dirty
				send = true
				processExited = true // FIXME: is this okay for PMSUSPENDED ?
			}

			if obj.processExitWatch && processExited {
				close(obj.processExitChan) // send signal
				obj.processExitWatch = false
			}

		case agentEvent := <-gaChan:
			state, reason := agentEvent.State, agentEvent.Reason

			if state == libvirt.CONNECT_DOMAIN_EVENT_AGENT_LIFECYCLE_STATE_CONNECTED {
				obj.guestAgentConnected = true
				obj.StateOK(false) // dirty
				send = true
				log.Printf("%s[%s]: Guest agent connected", obj.Kind(), obj.GetName())

			} else if state == libvirt.CONNECT_DOMAIN_EVENT_AGENT_LIFECYCLE_STATE_DISCONNECTED {
				obj.guestAgentConnected = false
				// ignore CONNECT_DOMAIN_EVENT_AGENT_LIFECYCLE_REASON_DOMAIN_STARTED
				// events because they just tell you that guest agent channel was added
				if reason == libvirt.CONNECT_DOMAIN_EVENT_AGENT_LIFECYCLE_REASON_CHANNEL {
					log.Printf("%s[%s]: Guest agent disconnected", obj.Kind(), obj.GetName())
				}

			} else {
				return fmt.Errorf("unknown %s[%s] guest agent state: %v", obj.Kind(), obj.GetName(), state)
			}

		case err := <-errorChan:
			return fmt.Errorf("unknown %s[%s] libvirt error: %s", obj.Kind(), obj.GetName(), err)

		case event := <-obj.Events():
			if exit, send = obj.ReadEvent(event); exit != nil {
				return *exit // exit
			}
		}

		if send {
			send = false
			obj.Event()
		}
	}
}

// domainCreate creates a transient or persistent domain in the correct state.
// It doesn't check the state before hand, as it is a simple helper function.
// The caller must run dom.Free() after use, when error was returned as nil.
func (obj *VirtRes) domainCreate() (*libvirt.Domain, bool, error) {

	if obj.Transient {
		var flag libvirt.DomainCreateFlags
		var state string
		switch obj.State {
		case "running":
			flag = libvirt.DOMAIN_NONE
			state = "started"
		case "paused":
			flag = libvirt.DOMAIN_START_PAUSED
			state = "paused"
		case "shutoff":
			// a transient, shutoff machine, means machine is absent
			return nil, true, nil // returned dom is invalid
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
		if err := dom.CreateWithFlags(libvirt.DOMAIN_START_PAUSED); err != nil {
			return dom, false, err
		}
		log.Printf("%s[%s]: Domain created paused", obj.Kind(), obj.GetName())
	}

	return dom, false, nil
}

// stateCheckApply starts, stops, or pauses/unpauses the domain as needed.
func (obj *VirtRes) stateCheckApply(apply bool, dom *libvirt.Domain) (bool, error) {
	var checkOK = true
	domInfo, err := dom.GetInfo()
	if err != nil {
		// we don't know if the state is ok
		return false, errwrap.Wrapf(err, "domain.GetInfo failed")
	}
	isActive, err := dom.IsActive()
	if err != nil {
		// we don't know if the state is ok
		return false, errwrap.Wrapf(err, "domain.IsActive failed")
	}

	// check for valid state
	switch obj.State {
	case "running":
		if domInfo.State == libvirt.DOMAIN_RUNNING {
			break
		}
		if domInfo.State == libvirt.DOMAIN_BLOCKED {
			// TODO: what should happen?
			return false, fmt.Errorf("domain %s is blocked", obj.GetName())
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
		if domInfo.State == libvirt.DOMAIN_PAUSED {
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
		if err := dom.CreateWithFlags(libvirt.DOMAIN_START_PAUSED); err != nil {
			return false, errwrap.Wrapf(err, "domain.CreateWithFlags failed")
		}
		checkOK = false
		log.Printf("%s[%s]: Domain created paused", obj.Kind(), obj.GetName())

	case "shutoff":
		if domInfo.State == libvirt.DOMAIN_SHUTOFF || domInfo.State == libvirt.DOMAIN_SHUTDOWN {
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

	return checkOK, nil
}

// attrCheckApply performs the CheckApply functions for CPU, Memory and others.
// This shouldn't be called when the machine is absent; it won't be found!
func (obj *VirtRes) attrCheckApply(apply bool, dom *libvirt.Domain) (bool, error) {
	var checkOK = true
	domInfo, err := dom.GetInfo()
	if err != nil {
		// we don't know if the state is ok
		return false, errwrap.Wrapf(err, "domain.GetInfo failed")
	}

	// check (balloon) memory
	// FIXME: check that we don't increase past max memory...
	if domInfo.Memory != obj.Memory {
		if !apply {
			return false, nil
		}
		checkOK = false
		if err := dom.SetMemory(obj.Memory); err != nil {
			return false, errwrap.Wrapf(err, "domain.SetMemory failed")
		}
		log.Printf("%s[%s]: Memory changed to %d", obj.Kind(), obj.GetName(), obj.Memory)
	}

	// check cpus
	if domInfo.NrVirtCpu != obj.CPUs {
		if !apply {
			return false, nil
		}

		// unused: DOMAIN_VCPU_CURRENT
		switch domInfo.State {
		case libvirt.DOMAIN_PAUSED:
			// we can queue up the SetVcpus operation,
			// which will be seen once vm is unpaused!
			fallthrough
		case libvirt.DOMAIN_RUNNING:
			// cpu hot*un*plug introduced in 2.2.0
			// 2 * 1000000 + 2 * 1000 + 0 = 2002000
			if obj.HotCPUs && obj.version < 2002000 && domInfo.NrVirtCpu > obj.CPUs {
				return false, fmt.Errorf("libvirt 2.2.0 or greater is required to hotunplug cpus")
			}
			// pkrempa says HOTPLUGGABLE is implied when doing LIVE
			// on running machine, but we add it anyways in case we
			// race and the machine is in shutoff state. We need to
			// specify HOTPLUGGABLE if we add while machine is off!
			// We particularly need to add HOTPLUGGABLE with CONFIG
			flags := libvirt.DOMAIN_VCPU_LIVE
			if !obj.Transient {
				flags |= libvirt.DOMAIN_VCPU_CONFIG
				// hotpluggable flag introduced in 2.4.0
				// 2 * 1000000 + 4 * 1000 + 0 = 2004000
				if obj.version >= 2004000 {
					flags |= libvirt.DOMAIN_VCPU_HOTPLUGGABLE
				}
			}
			if err := dom.SetVcpusFlags(obj.CPUs, flags); err != nil {
				return false, errwrap.Wrapf(err, "domain.SetVcpus failed")
			}
			checkOK = false
			log.Printf("%s[%s]: CPUs (hot) changed to %d", obj.Kind(), obj.GetName(), obj.CPUs)

		case libvirt.DOMAIN_SHUTOFF, libvirt.DOMAIN_SHUTDOWN:
			if !obj.Transient {
				flags := libvirt.DOMAIN_VCPU_CONFIG
				if obj.version >= 2004000 {
					flags |= libvirt.DOMAIN_VCPU_HOTPLUGGABLE
				}
				if err := dom.SetVcpusFlags(obj.CPUs, flags); err != nil {
					return false, errwrap.Wrapf(err, "domain.SetVcpus failed")
				}
				checkOK = false
				log.Printf("%s[%s]: CPUs (cold) changed to %d", obj.Kind(), obj.GetName(), obj.CPUs)
			}

		default:
			// FIXME: is this accurate?
			return false, fmt.Errorf("can't modify cpus when in %v", domInfo.State)
		}
	}

	// modify the online aspect of the cpus with qemu-guest-agent
	if obj.HotCPUs && obj.guestAgentConnected && domInfo.State != libvirt.DOMAIN_PAUSED {

		// if hotplugging a cpu without the guest agent, you might need:
		// manually to: echo 1 > /sys/devices/system/cpu/cpu1/online OR
		// udev (untested) in: /etc/udev/rules.d/99-hotplugCPU.rules
		// SUBSYSTEM=="cpu",ACTION=="add",RUN+="/bin/sh -c '[ ! -e /sys$devpath/online ] || echo 1 > /sys$devpath/online'"

		// how many online cpus are there?
		i, err := dom.GetVcpusFlags(libvirt.DOMAIN_VCPU_GUEST)
		if err != nil {
			return false, errwrap.Wrapf(err, "domain.GetVcpus failed from qemu-guest-agent")
		}
		onlineCPUs := uint(i)
		if onlineCPUs != obj.CPUs {
			if !apply {
				return false, nil
			}
			if err := dom.SetVcpusFlags(obj.CPUs, libvirt.DOMAIN_VCPU_GUEST); err != nil {
				return false, errwrap.Wrapf(err, "domain.SetVcpus failed")
			}
			checkOK = false
			log.Printf("%s[%s]: CPUs (guest) changed to %d", obj.Kind(), obj.GetName(), obj.CPUs)
		}
	}

	return checkOK, nil
}

// domainShutdownSync powers off a domain in a manner which will allow hardware
// to be changed while off. This requires the process to exit so that when it's
// called again, qemu can start up fresh as if we cold swapped in new hardware!
// This method is particularly special because it waits for shutdown to finish.
func (obj *VirtRes) domainShutdownSync(apply bool, dom *libvirt.Domain) (bool, error) {
	// we need to wait for shutdown to be finished before we can restart it
	once := true
	timeout := time.After(time.Duration(MaxShutdownDelayTimeout) * time.Second)

	// wait until shutdown completion...
	for {
		domInfo, err := dom.GetInfo()
		if err != nil {
			// we don't know if the state is ok
			return false, errwrap.Wrapf(err, "domain.GetInfo failed")
		}
		if domInfo.State == libvirt.DOMAIN_SHUTOFF || domInfo.State == libvirt.DOMAIN_SHUTDOWN {
			log.Printf("%s[%s]: Shutdown", obj.Kind(), obj.GetName())
			break
		}

		if once {
			if !apply {
				return false, nil
			}
			obj.processExitWatch = true
			obj.processExitChan = make(chan struct{})
			// if machine shuts down before we call this, we error;
			// this isn't ideal, but it happened due to user error!
			log.Printf("%s[%s]: Running shutdown", obj.Kind(), obj.GetName())
			if err := dom.Shutdown(); err != nil {
				// FIXME: if machine is already shutdown completely, return early
				return false, errwrap.Wrapf(err, "domain.Shutdown failed")
			}
			once = false // we did some work!
		}

		select {
		case <-obj.processExitChan: // should get a close signal...
			// pass
		case <-time.After(time.Duration(ShortPollInterval) * time.Second):
			// poll until timeout in case no event ever arrives
			// this happens when using Meta().Poll for example!

			// FIXME: some domains can reboot when asked to shutdown
			// via the `on_poweroff` xml setting. in this case, we
			// might only exit from here via timeout... avoid this!
			// https://libvirt.org/formatdomain.html#elementsEvents
			continue
		case <-timeout:
			return false, fmt.Errorf("%s[%s]: didn't shutdown after %d seconds", obj.Kind(), obj.GetName(), MaxShutdownDelayTimeout)
		}
	}

	return once, nil
}

// CheckApply checks the resource state and applies the resource if the bool
// input is true. It returns error info and if the state check passed or not.
func (obj *VirtRes) CheckApply(apply bool) (bool, error) {
	if obj.conn == nil {
		panic("virt: CheckApply is being called with nil connection")
	}
	// if we do the restart, we must flip the flag back to false as evidence
	var restart bool                           // do we need to do a restart?
	if obj.RestartOnRefresh && obj.Refresh() { // a refresh is a restart ask
		restart = true
	}

	// we need to restart in all situations except ignore. the "error" case
	// means that if a restart is actually needed, we should return an error
	if obj.restartScheduled && obj.RestartOnDiverge != "ignore" { // "ignore", "ifneeded", "error"
		restart = true
	}
	if !apply {
		restart = false
	}

	var checkOK = true

	dom, err := obj.conn.LookupDomainByName(obj.GetName())
	if err == nil {
		// pass
	} else if isNotFound(err) {
		// domain not found
		if obj.absent {
			// we can ignore the restart var since we're not running
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
	defer dom.Free() // the Free() for two possible domain objects above
	// domain now exists

	isPersistent, err := dom.IsPersistent()
	if err != nil {
		// we don't know if the state is ok
		return false, errwrap.Wrapf(err, "domain.IsPersistent failed")
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
			domXML, err := dom.GetXMLDesc(libvirt.DOMAIN_XML_INACTIVE)
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

	// shutdown here and let the stateCheckApply fix things up...
	// TODO: i think this is the most straight forward process...
	if !obj.absent && restart {
		if c, err := obj.domainShutdownSync(apply, dom); err != nil {
			return false, errwrap.Wrapf(err, "domainShutdownSync failed")
		} else if !c {
			checkOK = false
			restart = false // clear the restart requirement...
		}
	}

	// FIXME: is doing this early check (therefore twice total) a good idea?
	// run additional pre-emptive attr change checks here for hotplug stuff!
	if !obj.absent {
		if c, err := obj.attrCheckApply(apply, dom); err != nil {
			return false, errwrap.Wrapf(err, "early attrCheckApply failed")
		} else if !c {
			checkOK = false
		}
	}
	// TODO: do we need to run again below after we've booted up the domain?

	// apply correct machine state, eg: startup/shutoff/pause as needed
	if c, err := obj.stateCheckApply(apply, dom); err != nil {
		return false, errwrap.Wrapf(err, "stateCheckApply failed")
	} else if !c {
		checkOK = false
	}

	// FIXME: should we wait to ensure machine is booted before continuing?
	// it may be useful to wait for guest agent to hotplug some ram or cpu!

	// mem & cpu checks...
	if !obj.absent {
		if c, err := obj.attrCheckApply(apply, dom); err != nil {
			return false, errwrap.Wrapf(err, "attrCheckApply failed")
		} else if !c {
			checkOK = false
		}
	}

	// we had to do a restart, we didn't, and we should error if it was needed
	if obj.restartScheduled && restart == true && obj.RestartOnDiverge == "error" {
		return false, fmt.Errorf("%s[%s]: needed restart but didn't! (RestartOnDiverge: %v)", obj.Kind(), obj.GetName(), obj.RestartOnDiverge)
	}

	return checkOK, nil // w00t
}

// getDomainType returns the correct domain type based on the uri.
func (obj VirtRes) getDomainType() string {
	switch obj.uriScheme {
	case lxcURI:
		return "<domain type='lxc'>"
	default:
		return "<domain type='kvm'>"
	}
}

// getOSType returns the correct os type based on the uri.
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

// getDomainXML returns the representative XML for a domain struct.
// FIXME: replace this with the libvirt-go-xml package instead!
func (obj *VirtRes) getDomainXML() string {
	var b string
	b += obj.getDomainType() // start domain

	b += fmt.Sprintf("<name>%s</name>", obj.GetName())
	b += fmt.Sprintf("<memory unit='KiB'>%d</memory>", obj.Memory)

	if obj.HotCPUs {
		b += fmt.Sprintf("<vcpu current='%d'>%d</vcpu>", obj.CPUs, obj.MaxCPUs)
		b += fmt.Sprintf("<vcpus>")
		b += fmt.Sprintf("<vcpu id='0' enabled='yes' hotpluggable='no' order='1'/>") // zeroth cpu can't change
		for i := uint(1); i < obj.MaxCPUs; i++ {                                     // skip first entry
			enabled := "no"
			if i < obj.CPUs {
				enabled = "yes"
			}
			b += fmt.Sprintf("<vcpu id='%d' enabled='%s' hotpluggable='yes'/>", i, enabled)
		}
		b += fmt.Sprintf("</vcpus>")
	} else {
		b += fmt.Sprintf("<vcpu>%d</vcpu>", obj.CPUs)
	}

	b += "<os>"
	b += obj.getOSType()
	b += obj.getOSInit()
	if obj.Boot != nil {
		for _, boot := range obj.Boot {
			b += fmt.Sprintf("<boot dev='%s'/>", boot)
		}
	}
	b += fmt.Sprintf("</os>")

	if obj.HotCPUs {
		// acpi is needed for cpu hotplug support
		b += fmt.Sprintf("<features>")
		b += fmt.Sprintf("<acpi/>")
		b += fmt.Sprintf("</features>")
	}

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

	// qemu guest agent is not *required* for hotplugging cpus but
	// it helps because it can ask the host to make them online...
	if obj.HotCPUs {
		// enable qemu guest agent (on the host side)
		b += fmt.Sprintf("<channel type='unix'>")
		b += fmt.Sprintf("<source mode='bind'/>")
		b += fmt.Sprintf("<target type='virtio' name='org.qemu.guest_agent.0'/>")
		b += fmt.Sprintf("</channel>")
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
	source, _ := expandHome(d.Source) // TODO: should we handle errors?
	var b string
	b += "<disk type='file' device='disk'>"
	b += fmt.Sprintf("<driver name='qemu' type='%s'/>", d.Type)
	b += fmt.Sprintf("<source file='%s'/>", source)
	b += fmt.Sprintf("<target dev='vd%s' bus='virtio'/>", (string)(idx+97)) // TODO: 26, 27... should be 'aa', 'ab'...
	b += "</disk>"
	return b
}

func (d *cdRomDevice) GetXML(idx int) string {
	source, _ := expandHome(d.Source) // TODO: should we handle errors?
	var b string
	b += "<disk type='file' device='cdrom'>"
	b += fmt.Sprintf("<driver name='qemu' type='%s'/>", d.Type)
	b += fmt.Sprintf("<source file='%s'/>", source)
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
	source, _ := expandHome(d.Source) // TODO: should we handle errors?
	var b string
	b += "<filesystem" // open
	if d.Access != "" {
		b += fmt.Sprintf(" accessmode='%s'", d.Access)
	}
	b += ">" // close
	b += fmt.Sprintf("<source dir='%s'/>", source)
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

// UIDs includes all params to make a unique identification of this object.
// Most resources only return one, although some resources can return multiple.
func (obj *VirtRes) UIDs() []ResUID {
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
		// we can't change this property while machine is running!
		// we do need to return false, so that a new struct gets built,
		// which will cause at least one Init() & CheckApply() to run.
		if obj.MaxCPUs != res.MaxCPUs {
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

// UnmarshalYAML is the custom unmarshal handler for this struct.
// It is primarily useful for setting the defaults.
func (obj *VirtRes) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type rawRes VirtRes // indirection to avoid infinite recursion

	def := obj.Default()      // get the default
	res, ok := def.(*VirtRes) // put in the right format
	if !ok {
		return fmt.Errorf("could not convert to VirtRes")
	}
	raw := rawRes(*res) // convert; the defaults go here

	if err := unmarshal(&raw); err != nil {
		return err
	}

	*obj = VirtRes(raw) // restore from indirection with type conversion!
	return nil
}

// randMAC returns a random mac address in the libvirt range.
func randMAC() string {
	rand.Seed(time.Now().UnixNano())
	return "52:54:00" +
		fmt.Sprintf(":%x", rand.Intn(255)) +
		fmt.Sprintf(":%x", rand.Intn(255)) +
		fmt.Sprintf(":%x", rand.Intn(255))
}

// isNotFound tells us if this is a domain not found error.
func isNotFound(err error) bool {
	if err == nil {
		return false
	}
	if virErr, ok := err.(libvirt.Error); ok && virErr.Code == libvirt.ERR_NO_DOMAIN {
		// domain not found
		return true
	}
	return false // some other error
}

// expandHome does a simple expansion of the tilde into your $HOME value.
func expandHome(p string) (string, error) {
	// TODO: this doesn't match strings of the form: ~james/...
	if !strings.HasPrefix(p, "~/") {
		return p, nil
	}
	usr, err := user.Current()
	if err != nil {
		return p, fmt.Errorf("can't expand ~ into home directory")
	}
	return path.Join(usr.HomeDir, p[len("~/"):]), nil
}
