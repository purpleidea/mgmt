// Mgmt
// Copyright (C) 2013-2021+ James Shubin and the project contributors
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

// +build !novirt

package resources

import (
	"fmt"
	"math/rand"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/purpleidea/mgmt/engine"
	"github.com/purpleidea/mgmt/engine/traits"
	engineUtil "github.com/purpleidea/mgmt/engine/util"
	"github.com/purpleidea/mgmt/util"
	"github.com/purpleidea/mgmt/util/errwrap"

	"github.com/libvirt/libvirt-go"
	libvirtxml "github.com/libvirt/libvirt-go-xml"
)

func init() {
	engine.RegisterResource("virt", func() engine.Res { return &VirtRes{} })
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

// VirtRes is a libvirt resource. A transient virt resource, which has its state
// set to `shutoff` is one which does not exist. The parallel equivalent is a
// file resource which removes a particular path.
// TODO: some values inside here should be enum's!
type VirtRes struct {
	traits.Base // add the base methods without re-implementation
	traits.Refreshable

	init *engine.Init

	// URI is the libvirt connection URI, eg: `qemu:///session`.
	URI string `lang:"uri" yaml:"uri"`
	// State is the desired vm state. Possible values include: `running`,
	// `paused` and `shutoff`.
	State string `lang:"state" yaml:"state"`
	// Transient is whether the vm is defined (false) or undefined (true).
	Transient bool `lang:"transient" yaml:"transient"`

	// CPUs is the desired cpu count of the machine.
	CPUs uint `lang:"cpus" yaml:"cpus"`
	// MaxCPUs is the maximum number of cpus allowed in the machine. You
	// need to set this so that on boot the `hardware` knows how many cpu
	// `slots` it might need to make room for.
	MaxCPUs uint `lang:"maxcpus" yaml:"maxcpus"`
	// HotCPUs specifies whether we can hot plug and unplug cpus.
	HotCPUs bool `lang:"hotcpus" yaml:"hotcpus"`
	// Memory is the size in KBytes of memory to include in the machine.
	Memory uint64 `lang:"memory" yaml:"memory"`

	// OSInit is the init used by lxc.
	OSInit string `lang:"osinit" yaml:"osinit"`
	// Boot is the boot order. Values are `fd`, `hd`, `cdrom` and `network`.
	Boot []string `lang:"boot" yaml:"boot"`
	// Disk is the list of disk devices to include.
	Disk []*DiskDevice `lang:"disk" yaml:"disk"`
	// CdRom is the list of cdrom devices to include.
	CDRom []*CDRomDevice `lang:"cdrom" yaml:"cdrom"`
	// Network is the list of network devices to include.
	Network []*NetworkDevice `lang:"network" yaml:"network"`
	// Filesystem is the list of file system devices to include.
	Filesystem []*FilesystemDevice `lang:"filesystem" yaml:"filesystem"`

	// Auth points to the libvirt credentials to use if any are necessary.
	Auth *VirtAuth `lang:"auth" yaml:"auth"`

	// RestartOnDiverge is the restart policy, and can be: `ignore`,
	// `ifneeded` or `error`.
	RestartOnDiverge string `lang:"restartondiverge" yaml:"restartondiverge"`
	// RestartOnRefresh specifies if we restart on refresh signal.
	RestartOnRefresh bool `lang:"restartonrefresh" yaml:"restartonrefresh"`

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

// VirtAuth is used to pass credentials to libvirt.
type VirtAuth struct {
	Username string `lang:"username" yaml:"username"`
	Password string `lang:"password" yaml:"password"`
}

// Cmp compares two VirtAuth structs. It errors if they are not identical.
func (obj *VirtAuth) Cmp(auth *VirtAuth) error {
	if (obj == nil) != (auth == nil) { // xor
		return fmt.Errorf("the VirtAuth differs")
	}
	if obj == nil && auth == nil {
		return nil
	}

	if obj.Username != auth.Username {
		return fmt.Errorf("the Username differs")
	}
	if obj.Password != auth.Password {
		return fmt.Errorf("the Password differs")
	}
	return nil
}

// Default returns some sensible defaults for this resource.
func (obj *VirtRes) Default() engine.Res {
	return &VirtRes{
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
	return nil
}

// Init runs some startup code for this resource.
func (obj *VirtRes) Init(init *engine.Init) error {
	obj.init = init // save for later

	if !libvirtInitialized {
		if err := libvirt.EventRegisterDefaultImpl(); err != nil {
			return errwrap.Wrapf(err, "method EventRegisterDefaultImpl failed")
		}
		libvirtInitialized = true
	}
	var u *url.URL
	var err error
	if u, err = url.Parse(obj.URI); err != nil {
		return errwrap.Wrapf(err, "parsing URI (`%s`) failed", obj.URI)
	}
	switch u.Scheme {
	case "lxc":
		obj.uriScheme = lxcURI
	}

	obj.absent = (obj.Transient && obj.State == "shutoff") // machine shouldn't exist

	obj.conn, err = obj.connect() // gets closed in Close method of Res API
	if err != nil {
		return errwrap.Wrapf(err, "connection to libvirt failed in init")
	}

	// check for hard to change properties
	dom, err := obj.conn.LookupDomainByName(obj.Name())
	if err == nil {
		defer dom.Free()
	} else if !isNotFound(err) {
		return errwrap.Wrapf(err, "could not lookup on init")
	}

	if err == nil {
		// maxCPUs, err := dom.GetMaxVcpus()
		i, err := dom.GetVcpusFlags(libvirt.DOMAIN_VCPU_MAXIMUM)
		if err != nil {
			return errwrap.Wrapf(err, "could not lookup MaxCPUs on init")
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
			return errwrap.Wrapf(err, "could not GetXMLDesc on init")
		}
		domXML := &libvirtxml.Domain{}
		if err := domXML.Unmarshal(xmlDesc); err != nil {
			return errwrap.Wrapf(err, "could not unmarshal XML on init")
		}

		// guest agent: domain->devices->channel->target->state == connected?
		for _, x := range domXML.Devices.Channels {
			if x.Target.VirtIO != nil && strings.HasPrefix(x.Target.VirtIO.Name, "org.qemu.guest_agent.") {
				// last connection found wins (usually 1 anyways)
				obj.guestAgentConnected = (x.Target.VirtIO.State == "connected")
			}
		}
	}
	obj.wg = &sync.WaitGroup{}
	return nil
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
		defer obj.init.Logf("EventRunDefaultImpl exited!")
		for {
			// TODO: can we merge this into our main for loop below?
			select {
			case <-exitChan:
				return
			default:
			}
			//obj.init.Logf("EventRunDefaultImpl started!")
			if err := libvirt.EventRunDefaultImpl(); err != nil {
				select {
				case errorChan <- errwrap.Wrapf(err, "EventRunDefaultImpl failed"):
				case <-exitChan:
					// pass
				}
				return
			}
			//obj.init.Logf("EventRunDefaultImpl looped!")
		}
	}()

	// domain events callback
	domCallback := func(c *libvirt.Connect, d *libvirt.Domain, ev *libvirt.DomainEventLifecycle) {
		domName, _ := d.GetName()
		if domName == obj.Name() {
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
		if domName == obj.Name() {
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

	obj.init.Running() // when started, notify engine that we're running

	var send = false // send event?
	for {
		processExited := false // did the process exit fully (shutdown)?
		select {
		case event := <-domChan:
			// TODO: shouldn't we do these checks in CheckApply ?
			switch event {
			case libvirt.DOMAIN_EVENT_DEFINED:
				if obj.Transient {
					send = true
				}
			case libvirt.DOMAIN_EVENT_UNDEFINED:
				if !obj.Transient {
					send = true
				}
			case libvirt.DOMAIN_EVENT_STARTED:
				fallthrough
			case libvirt.DOMAIN_EVENT_RESUMED:
				if obj.State != "running" {
					send = true
				}
			case libvirt.DOMAIN_EVENT_SUSPENDED:
				if obj.State != "paused" {
					send = true
				}
			case libvirt.DOMAIN_EVENT_STOPPED:
				fallthrough
			case libvirt.DOMAIN_EVENT_SHUTDOWN:
				if obj.State != "shutoff" {
					send = true
				}
				processExited = true

			case libvirt.DOMAIN_EVENT_PMSUSPENDED:
				// FIXME: IIRC, in s3 we can't cold change
				// hardware like cpus but in s4 it's okay?
				// verify, detect and patch appropriately!
				fallthrough
			case libvirt.DOMAIN_EVENT_CRASHED:
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
				send = true
				obj.init.Logf("guest agent connected")

			} else if state == libvirt.CONNECT_DOMAIN_EVENT_AGENT_LIFECYCLE_STATE_DISCONNECTED {
				obj.guestAgentConnected = false
				// ignore CONNECT_DOMAIN_EVENT_AGENT_LIFECYCLE_REASON_DOMAIN_STARTED
				// events because they just tell you that guest agent channel was added
				if reason == libvirt.CONNECT_DOMAIN_EVENT_AGENT_LIFECYCLE_REASON_CHANNEL {
					obj.init.Logf("guest agent disconnected")
				}

			} else {
				return fmt.Errorf("unknown guest agent state: %v", state)
			}

		case err := <-errorChan:
			return errwrap.Wrapf(err, "unknown libvirt error")

		case <-obj.init.Done: // closed by the engine to signal shutdown
			return nil
		}

		// do all our event sending all together to avoid duplicate msgs
		if send {
			send = false
			obj.init.Event() // notify engine of an event (this can block)
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
		obj.init.Logf("transient domain %s", state) // log the state
		return dom, false, nil
	}

	dom, err := obj.conn.DomainDefineXML(obj.getDomainXML())
	if err != nil {
		return dom, false, err // returned dom is invalid
	}
	obj.init.Logf("domain defined")

	if obj.State == "running" {
		if err := dom.Create(); err != nil {
			return dom, false, err
		}
		obj.init.Logf("domain started")
	}

	if obj.State == "paused" {
		if err := dom.CreateWithFlags(libvirt.DOMAIN_START_PAUSED); err != nil {
			return dom, false, err
		}
		obj.init.Logf("domain created paused")
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
			return false, fmt.Errorf("domain is blocked")
		}
		if !apply {
			return false, nil
		}
		if isActive { // domain must be paused ?
			if err := dom.Resume(); err != nil {
				return false, errwrap.Wrapf(err, "domain.Resume failed")
			}
			checkOK = false
			obj.init.Logf("domain resumed")
			break
		}
		if err := dom.Create(); err != nil {
			return false, errwrap.Wrapf(err, "domain.Create failed")
		}
		checkOK = false
		obj.init.Logf("domain created")

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
			obj.init.Logf("domain paused")
			break
		}
		if err := dom.CreateWithFlags(libvirt.DOMAIN_START_PAUSED); err != nil {
			return false, errwrap.Wrapf(err, "domain.CreateWithFlags failed")
		}
		checkOK = false
		obj.init.Logf("domain created paused")

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
		obj.init.Logf("domain destroyed")
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
		obj.init.Logf("memory changed to: %d", obj.Memory)
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
			obj.init.Logf("cpus (hot) changed to: %d", obj.CPUs)

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
				obj.init.Logf("cpus (cold) changed to: %d", obj.CPUs)
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
			obj.init.Logf("cpus (guest) changed to: %d", obj.CPUs)
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
			obj.init.Logf("shutdown")
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
			obj.init.Logf("running shutdown")
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
			return false, fmt.Errorf("didn't shutdown after %d seconds", MaxShutdownDelayTimeout)
		}
	}

	return once, nil
}

// CheckApply checks the resource state and applies the resource if the bool
// input is true. It returns error info and if the state check passed or not.
func (obj *VirtRes) CheckApply(apply bool) (bool, error) {
	if obj.conn == nil { // programming error?
		return false, fmt.Errorf("got called with nil connection")
	}
	// if we do the restart, we must flip the flag back to false as evidence
	var restart bool                                // do we need to do a restart?
	if obj.RestartOnRefresh && obj.init.Refresh() { // a refresh is a restart ask
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

	dom, err := obj.conn.LookupDomainByName(obj.Name())
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

		var c bool                       // = true
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
			obj.init.Logf("domain undefined")
		} else {
			domXML, err := dom.GetXMLDesc(libvirt.DOMAIN_XML_INACTIVE)
			if err != nil {
				return false, errwrap.Wrapf(err, "domain.GetXMLDesc failed")
			}
			if _, err = obj.conn.DomainDefineXML(domXML); err != nil {
				return false, errwrap.Wrapf(err, "conn.DomainDefineXML failed")
			}
			obj.init.Logf("domain defined")
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
		return false, fmt.Errorf("needed restart but didn't! (RestartOnDiverge: %s)", obj.RestartOnDiverge)
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

	b += fmt.Sprintf("<name>%s</name>", obj.Name())
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

// DiskDevice represents a disk that is attached to the virt machine.
type DiskDevice struct {
	Source string `lang:"source" yaml:"source"`
	Type   string `lang:"type" yaml:"type"`
}

// GetXML returns the XML representation of this device.
func (obj *DiskDevice) GetXML(idx int) string {
	source, _ := util.ExpandHome(obj.Source) // TODO: should we handle errors?
	var b string
	b += "<disk type='file' device='disk'>"
	b += fmt.Sprintf("<driver name='qemu' type='%s'/>", obj.Type)
	b += fmt.Sprintf("<source file='%s'/>", source)
	b += fmt.Sprintf("<target dev='vd%s' bus='virtio'/>", util.NumToAlpha(idx))
	b += "</disk>"
	return b
}

// Cmp compares two DiskDevice's and returns an error if they are not
// equivalent.
func (obj *DiskDevice) Cmp(dev *DiskDevice) error {
	if (obj == nil) != (dev == nil) { // xor
		return fmt.Errorf("the DiskDevice differs")
	}
	if obj == nil && dev == nil {
		return nil
	}

	if obj.Source != dev.Source {
		return fmt.Errorf("the Source differs")
	}
	if obj.Type != dev.Type {
		return fmt.Errorf("the Type differs")
	}

	return nil
}

// CDRomDevice represents a CDRom device that is attached to the virt machine.
type CDRomDevice struct {
	Source string `lang:"source" yaml:"source"`
	Type   string `lang:"type" yaml:"type"`
}

// GetXML returns the XML representation of this device.
func (obj *CDRomDevice) GetXML(idx int) string {
	source, _ := util.ExpandHome(obj.Source) // TODO: should we handle errors?
	var b string
	b += "<disk type='file' device='cdrom'>"
	b += fmt.Sprintf("<driver name='qemu' type='%s'/>", obj.Type)
	b += fmt.Sprintf("<source file='%s'/>", source)
	b += fmt.Sprintf("<target dev='hd%s' bus='ide'/>", util.NumToAlpha(idx))
	b += "<readonly/>"
	b += "</disk>"
	return b
}

// Cmp compares two CDRomDevice's and returns an error if they are not
// equivalent.
func (obj *CDRomDevice) Cmp(dev *CDRomDevice) error {
	if (obj == nil) != (dev == nil) { // xor
		return fmt.Errorf("the CDRomDevice differs")
	}
	if obj == nil && dev == nil {
		return nil
	}

	if obj.Source != dev.Source {
		return fmt.Errorf("the Source differs")
	}
	if obj.Type != dev.Type {
		return fmt.Errorf("the Type differs")
	}

	return nil
}

// NetworkDevice represents a network card that is attached to the virt machine.
type NetworkDevice struct {
	Name string `lang:"name" yaml:"name"`
	MAC  string `lang:"mac" yaml:"mac"`
}

// GetXML returns the XML representation of this device.
func (obj *NetworkDevice) GetXML(idx int) string {
	if obj.MAC == "" {
		obj.MAC = randMAC()
	}
	var b string
	b += "<interface type='network'>"
	b += fmt.Sprintf("<mac address='%s'/>", obj.MAC)
	b += fmt.Sprintf("<source network='%s'/>", obj.Name)
	b += "</interface>"
	return b
}

// Cmp compares two NetworkDevice's and returns an error if they are not
// equivalent.
func (obj *NetworkDevice) Cmp(dev *NetworkDevice) error {
	if (obj == nil) != (dev == nil) { // xor
		return fmt.Errorf("the NetworkDevice differs")
	}
	if obj == nil && dev == nil {
		return nil
	}

	if obj.Name != dev.Name {
		return fmt.Errorf("the Name differs")
	}
	if obj.MAC != dev.MAC {
		return fmt.Errorf("the MAC differs")
	}

	return nil
}

// FilesystemDevice represents a filesystem that is attached to the virt
// machine.
type FilesystemDevice struct {
	Access   string `lang:"access" yaml:"access"`
	Source   string `lang:"source" yaml:"source"`
	Target   string `lang:"target" yaml:"target"`
	ReadOnly bool   `lang:"read_only" yaml:"read_only"`
}

// GetXML returns the XML representation of this device.
func (obj *FilesystemDevice) GetXML(idx int) string {
	source, _ := util.ExpandHome(obj.Source) // TODO: should we handle errors?
	var b string
	b += "<filesystem" // open
	if obj.Access != "" {
		b += fmt.Sprintf(" accessmode='%s'", obj.Access)
	}
	b += ">" // close
	b += fmt.Sprintf("<source dir='%s'/>", source)
	b += fmt.Sprintf("<target dir='%s'/>", obj.Target)
	if obj.ReadOnly {
		b += "<readonly/>"
	}
	b += "</filesystem>"
	return b
}

// Cmp compares two FilesystemDevice's and returns an error if they are not
// equivalent.
func (obj *FilesystemDevice) Cmp(dev *FilesystemDevice) error {
	if (obj == nil) != (dev == nil) { // xor
		return fmt.Errorf("the FilesystemDevice differs")
	}
	if obj == nil && dev == nil {
		return nil
	}

	if obj.Access != dev.Access {
		return fmt.Errorf("the Access differs")
	}
	if obj.Source != dev.Source {
		return fmt.Errorf("the Source differs")
	}
	if obj.Target != dev.Target {
		return fmt.Errorf("the Target differs")
	}
	if obj.ReadOnly != dev.ReadOnly {
		return fmt.Errorf("the ReadOnly differs")
	}

	return nil
}

// Cmp compares two resources and returns an error if they are not equivalent.
func (obj *VirtRes) Cmp(r engine.Res) error {
	// we can only compare VirtRes to others of the same resource kind
	res, ok := r.(*VirtRes)
	if !ok {
		return fmt.Errorf("not a %s", obj.Kind())
	}

	if obj.URI != res.URI {
		return fmt.Errorf("the URI differs")
	}
	if obj.State != res.State {
		return fmt.Errorf("the State differs")
	}
	if obj.Transient != res.Transient {
		return fmt.Errorf("the Transient differs")
	}

	if obj.CPUs != res.CPUs {
		return fmt.Errorf("the CPUs differ")
	}
	// we can't change this property while machine is running!
	// we do need to return false, so that a new struct gets built,
	// which will cause at least one Init() & CheckApply() to run.
	if obj.MaxCPUs != res.MaxCPUs {
		return fmt.Errorf("the MaxCPUs differ")
	}
	if obj.HotCPUs != res.HotCPUs {
		return fmt.Errorf("the HotCPUs differ")
	}
	// TODO: can we skip the compare of certain properties such as
	// Memory because this object (but with different memory) can be
	// *converted* into the new version that has more/less memory?
	// We would need to run some sort of "old struct update", to get
	// the new values, but that's easy to add.
	if obj.Memory != res.Memory {
		return fmt.Errorf("the Memory differs")
	}

	if obj.OSInit != res.OSInit {
		return fmt.Errorf("the OSInit differs")
	}
	if err := engineUtil.StrListCmp(obj.Boot, res.Boot); err != nil {
		return errwrap.Wrapf(err, "the Boot differs")
	}

	if len(obj.Disk) != len(res.Disk) {
		return fmt.Errorf("the Disk length differs")
	}
	for i := range obj.Disk {
		if err := obj.Disk[i].Cmp(res.Disk[i]); err != nil {
			return errwrap.Wrapf(err, "the Disk differs")
		}
	}
	if len(obj.CDRom) != len(res.CDRom) {
		return fmt.Errorf("the CDRom length differs")
	}
	for i := range obj.CDRom {
		if err := obj.CDRom[i].Cmp(res.CDRom[i]); err != nil {
			return errwrap.Wrapf(err, "the CDRom differs")
		}
	}
	if len(obj.Network) != len(res.Network) {
		return fmt.Errorf("the Network length differs")
	}
	for i := range obj.Network {
		if err := obj.Network[i].Cmp(res.Network[i]); err != nil {
			return errwrap.Wrapf(err, "the Network differs")
		}
	}
	if len(obj.Filesystem) != len(res.Filesystem) {
		return fmt.Errorf("the Filesystem length differs")
	}
	for i := range obj.Filesystem {
		if err := obj.Filesystem[i].Cmp(res.Filesystem[i]); err != nil {
			return errwrap.Wrapf(err, "the Filesystem differs")
		}
	}

	if err := obj.Auth.Cmp(res.Auth); err != nil {
		return errwrap.Wrapf(err, "the Auth differs")
	}

	if obj.RestartOnDiverge != res.RestartOnDiverge {
		return fmt.Errorf("the RestartOnDiverge differs")
	}
	if obj.RestartOnRefresh != res.RestartOnRefresh {
		return fmt.Errorf("the RestartOnRefresh differs")
	}

	return nil
}

// VirtUID is the UID struct for FileRes.
type VirtUID struct {
	engine.BaseUID
}

// UIDs includes all params to make a unique identification of this object. Most
// resources only return one, although some resources can return multiple.
func (obj *VirtRes) UIDs() []engine.ResUID {
	x := &VirtUID{
		BaseUID: engine.BaseUID{Name: obj.Name(), Kind: obj.Kind()},
		// TODO: add more properties here so we can link to vm dependencies
	}
	return []engine.ResUID{x}
}

// UnmarshalYAML is the custom unmarshal handler for this struct. It is
// primarily useful for setting the defaults.
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
