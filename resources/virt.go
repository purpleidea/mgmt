package resources

import (
	"encoding/gob"
	"log"
	"fmt"
	"strconv"

	"github.com/purpleidea/mgmt/event"
	"github.com/rgbkrk/libvirt-go"
	"math/rand"
	"time"
	"bytes"
)

func init() {
	gob.Register(&VirtRes{})
}

type VirtRes struct {
	BaseRes `yaml:",inline"`
	Cpus int `yaml:"cpus"`
	Ram int `yaml:"ram"`
	State string `yaml:"state"` //running, paused, shutoff
	Transient bool `yaml:"transient"`
	Boot []string `yaml:"boot"` // boot order. values: fd, hd, cdrom, network
	Disk []diskDevice `yaml:"disk"`
	CDRom []cdRomDevice `yaml:"cdrom"`
	Network []networkDevice `yaml:"network"`
	Filesystem []filesystemDevice `yaml:"filesystem"`
	conn libvirt.VirConnection
}

type VirtUUID struct {
	BaseUUID
}

type virtDevice interface {
	GetXML(idx int) string
}

type diskDevice struct {
	Source string `yaml:"source"`
	Type string `yaml:"type"`
}

type cdRomDevice struct {
	Source string `yaml:"source"`
	Type string `yaml:"type"`
}

type networkDevice struct {
	Name string `yaml:"name"`
	MAC string `yaml:"mac"`
}

type filesystemDevice struct {
	Access string `yaml:"access"`
	Source string `yaml:"source"`
	Target string `yaml:"target"`
	ReadOnly bool `yaml:"read_only"`
}

var (
	libvirtInitialized bool = false
)

func NewVirtRes(name string, cpus, ram int, state string, transient bool) *VirtRes {
	obj := &VirtRes{
		BaseRes: BaseRes{
			Name: name,
		},
		Cpus: cpus,
		Ram: ram,
		State: state,
		Transient: transient,
	}
	obj.Init()
	return obj
}

func (obj *VirtRes) Init() error {
	obj.BaseRes.kind = "Virt"

	if !libvirtInitialized {
		libvirt.EventRegisterDefaultImpl()
		// run libvirt event loop
		go func() {
			for {
				libvirt.EventRunDefaultImpl()
			}
		}()
		libvirtInitialized = true
	}

	conn, err := libvirt.NewVirConnection("qemu:///system")
	if err != nil {
		// should we proceed or exit?
		fmt.Println("Connection to libvirt faild")
		return err
	}
	obj.conn = conn

	obj.BaseRes.Init() // call base init, b/c we're overriding

	return nil
}

func (obj *VirtRes) Validate() bool {
	return true
}

func (obj *VirtRes) GetUUIDs() []ResUUID {
	x := &VirtUUID{
		BaseUUID: BaseUUID{name: obj.GetName(), kind: obj.Kind()},
	}
	return []ResUUID{x}
}

func (obj *VirtRes) Watch(processChan chan event.Event) error {
	if obj.IsWatching() {
		return nil
	}
	obj.SetWatching(true)
	defer obj.SetWatching(false)
	cuuid := obj.converger.Register()
	defer cuuid.Unregister()

	ch := make(chan int, 10)
	callback := libvirt.DomainEventCallback(
		func(c *libvirt.VirConnection, d *libvirt.VirDomain, eventDetails interface{}, f func()) int {
			if lifecycleEvent, ok := eventDetails.(libvirt.DomainLifecycleEvent); ok {
				domName, _ := d.GetName()
				if domName == obj.BaseRes.Name {
					ch <- lifecycleEvent.Event
				}
			} else {
				fmt.Println("event details isn't DomainLifecycleEvent")
			}
			return 0
		},
	)
	callbackId := obj.conn.DomainEventRegister(
		libvirt.VirDomain{},
		libvirt.VIR_DOMAIN_EVENT_ID_LIFECYCLE,
		&callback,
		nil,
	)
	defer obj.conn.DomainEventDeregister(callbackId)

	var send = false
	var exit = false
	var dirty = false

	for {
		select {
		case event := <-ch:
			switch event {
			case libvirt.VIR_DOMAIN_EVENT_DEFINED:
				if obj.Transient {
					dirty= true
					send = true
				}
			case libvirt.VIR_DOMAIN_EVENT_UNDEFINED:
				if !obj.Transient {
					dirty= true
					send = true
				}
			case libvirt.VIR_DOMAIN_EVENT_STARTED:
				fallthrough
			case libvirt.VIR_DOMAIN_EVENT_RESUMED:
				if obj.State != "running" {
					dirty= true
					send = true
				}
			case libvirt.VIR_DOMAIN_EVENT_SUSPENDED:
				if obj.State != "paused" {
					dirty= true
					send = true
				}
			case libvirt.VIR_DOMAIN_EVENT_STOPPED:
				fallthrough
			case libvirt.VIR_DOMAIN_EVENT_SHUTDOWN:
				if obj.State != "shutoff" {
					dirty= true
					send = true
				}
			case libvirt.VIR_DOMAIN_EVENT_PMSUSPENDED:
				fallthrough
			case libvirt.VIR_DOMAIN_EVENT_CRASHED:
				dirty= true
				send = true
			}

		case event := <-obj.events:
			cuuid.SetConverged(false)
			if exit, send = obj.ReadEvent(&event); exit {
				return nil// exit
			}

		case _ = <-cuuid.ConvergedTimer():
			cuuid.SetConverged(true) // converged!
			continue
		}

		if send {
			send = false
			if dirty {
				dirty = false
				obj.isStateOK = false // something made state dirty
			}
			resp := event.NewResp()
			processChan <- event.Event{event.EventNil, resp, "", true} // trigger process
			resp.ACKWait()                                 // wait for the ACK()
		}
	}
}

func (obj *VirtRes) CheckApply(apply bool) (bool, error) {
	log.Printf("%v[%v]: CheckApply(%t)", obj.Kind(), obj.GetName(), apply)

	if obj.isStateOK { // cache the state
		return true, nil
	}

	dom, err := obj.conn.LookupDomainByName(obj.BaseRes.Name)
	defer dom.Free()

	// Domain Exists
	if err == nil {
		domInfo, err := dom.GetInfo()
		if err != nil {
			// we don't know if the state is ok
			return false, err
		}
		isPersistent, err := dom.IsPersistent()
		if err != nil {
			// we don't know if the state is ok
			return false, err
		}
		isActive, err := dom.IsActive()
		if err != nil {
			// we don't know if the state is ok
			return false, err
		}

		// Check for persistence
		if isPersistent == obj.Transient {
			if !apply {
				return false, nil
			}
			if isPersistent {
				if err = dom.Undefine(); err != nil {
					return false, err
				}
				log.Printf("%v[%v]: Domain Undefined", obj.Kind(), obj.GetName())
			} else {
				domXML, err := dom.GetXMLDesc(libvirt.VIR_DOMAIN_XML_INACTIVE)
				if err != nil {
					return false, err
				}
				if _, err = obj.conn.DomainDefineXML(domXML); err != nil {
					return false, err
				}
				log.Printf("%v[%v]: Domain Defined", obj.Kind(), obj.GetName())
			}
		}

		// Check for valid state
		domState := domInfo.GetState()
		switch obj.State {
		case "running":
			if domState == libvirt.VIR_DOMAIN_RUNNING {
				break
			}
			if domState == libvirt.VIR_DOMAIN_BLOCKED {
				// TODO: what should happen?
				return false, fmt.Errorf("Domain %s blocked on resources", obj.BaseRes.Name)
			}
			if !apply {
				return false, nil
			}
			if isActive {
				if err = dom.Resume(); err != nil {
					return false, err
				}
				log.Printf("%v[%v]: Domain Resumed", obj.Kind(), obj.GetName())
				break
			}
			if err = dom.Create(); err != nil {
				return false, err
			}
			log.Printf("%v[%v]: Domain Created", obj.Kind(), obj.GetName())
		case "paused":
			if domState == libvirt.VIR_DOMAIN_PAUSED {
				break
			}
			if !apply {
				return false, nil
			}
			if isActive {
				if err = dom.Suspend(); err != nil {
					return false, err
				}
				log.Printf("%v[%v]: Domain Paused", obj.Kind(), obj.GetName())
				break
			}
			if err = dom.CreateWithFlags(libvirt.VIR_DOMAIN_START_PAUSED); err != nil {
				return false, err
			}
			fmt.Println("Domain Created Paused")
			log.Printf("%v[%v]: Domain Created Paused", obj.Kind(), obj.GetName())
		case "shutoff":
			if domState == libvirt.VIR_DOMAIN_SHUTOFF || domState == libvirt.VIR_DOMAIN_SHUTDOWN {
				break
			}
			if !apply {
				return false, nil
			}
			if err = dom.Destroy(); err != nil {
				return false, err
			}
			log.Printf("%v[%v]: Domain Destroyed", obj.Kind(), obj.GetName())
		}

		// Check memory
		mem := uint64(obj.Ram * 1024)
		if domInfo.GetMemory() != mem {
			if !apply {
				return false, nil
			}
			if err = dom.SetMemory(mem); err != nil {
				return false, err
			}
			log.Printf("%v[%v]: Memory Changed", obj.Kind(), obj.GetName())
		}

		// Check cpus
		if domInfo.GetNrVirtCpu() != uint16(obj.Cpus) {
			if !apply {
				return false, nil
			}
			if err = dom.SetVcpus(uint(obj.Cpus)); err != nil {
				return false, err
			}
			log.Printf("%v[%v]: CPUs Changed", obj.Kind(), obj.GetName())
		}
		return true, nil
	}

	// Domain does not exist
	if !apply {
		return false, nil
	}

	// apply portion
	if obj.Transient {
		var flag uint32
		switch obj.State {
		case "running":
			flag = libvirt.VIR_DOMAIN_NONE
		case "paused":
			flag = libvirt.VIR_DOMAIN_START_PAUSED
		case "shutoff":
			// TODO: What do we do? Validate should capture this?
			return false, fmt.Errorf("Invalid combination of transient and shutoff state")
		}
		_, err := obj.conn.DomainCreateXML(obj.getDomainXML(), flag)
		if err != nil {
			return false, err
		}
		return true, nil
	}

	dom, err = obj.conn.DomainDefineXML(obj.getDomainXML())
	if err != nil {
		return false, err
	}

	if obj.State == "running" {
		if err = dom.Create(); err != nil {
			return false, err
		}
	}

	if obj.State == "paused" {
		if err = dom.CreateWithFlags(libvirt.VIR_DOMAIN_START_PAUSED); err != nil {
			return false, err
		}
	}

	// Domain is defined and state is "shutoff"
	return true, nil
}

func (obj *VirtRes) AutoEdges() AutoEdge {
	return nil
}

func (obj *VirtRes) Compare(res Res) bool {
	return true
}

func (obj *VirtRes) CollectPattern(string) {

}

func (obj *VirtRes) GroupCmp(r Res) bool {
	return false
}

func (obj *VirtRes) getDomainXML() string {
	var baf bytes.Buffer

	baf.WriteString(`<domain type='kvm'>
		  <name>` + obj.BaseRes.Name + `</name>
		  <memory unit='KiB'>` + strconv.Itoa(obj.Ram * 1024) + `</memory>
		  <vcpu>` + strconv.Itoa(obj.Cpus) + `</vcpu>
		  <os>
		    <type>hvm</type>`)

	if obj.Boot != nil {
		for _, boot := range obj.Boot {
			baf.WriteString("<boot dev='")
			baf.WriteString(boot)
			baf.WriteString("'/>")
		}
	}
	// TODO: Use capabilities to determine emulator
	baf.WriteString(`</os>
		         <devices>
		           <emulator>/usr/bin/kvm-spice</emulator>`)

	if obj.Disk != nil {
		for i, disk := range obj.Disk {
			baf.WriteString(disk.GetXML(i))
		}
	}

	if obj.CDRom != nil {
		for i, cdrom := range obj.CDRom {
			baf.WriteString(cdrom.GetXML(i))
		}
	}

	if obj.Network != nil {
		for i, net := range obj.Network {
			baf.WriteString(net.GetXML(i))
		}
	}

	if obj.Filesystem != nil {
		for i, fs := range obj.Filesystem {
			baf.WriteString(fs.GetXML(i))
		}
	}

	baf.WriteString(`    <serial type='pty'>
			       <target port='0'/>
			     </serial>
    			     <console type='pty'>
    			       <target type='serial' port='0'/>
    			     </console>
    			   </devices>
    			 </domain>`)

	return baf.String()
}

func (d *diskDevice) GetXML(idx int) string {
	return `<disk type='file' device='disk'>
		  <driver name='qemu' type='` + d.Type + `'/>
		  <source file='` + d.Source + `'/>
		  <target dev='vd` + (string)(idx+97) + `' bus='virtio'/>
		</disk>`
}

func (d *cdRomDevice) GetXML(idx int) string {
	return `<disk type='file' device='cdrom'>
		  <driver name='qemu' type='` + d.Type + `'/>
		  <source file='` + d.Source + `'/>
		  <target dev='hd` + (string)(idx+97) + `' bus='ide'/>
		  <readonly/>
		</disk>`
}

func (d *networkDevice) GetXML(idx int) string {
	if d.MAC == "" {
		d.MAC = randMAC()
	}

	return `<interface type='network'>
		  <mac address='` + d.MAC + `'/>
		  <source network='` + d.Name + `'/>
		</interface>`
}

func (d *filesystemDevice) GetXML(idx int) string {
	var baf bytes.Buffer

	baf.WriteString("<filesystem")
	if d.Access != "" {
		baf.WriteString(" accessmode='" + d.Access + "'")
	}
	baf.WriteString(">")
	baf.WriteString("<source dir='" + d.Source + "'/>")
	baf.WriteString("<target dir='" + d.Target + "'/>")
	if d.ReadOnly {
		baf.WriteString("<readonly/>")
	}
	baf.WriteString("</filesystem>")

	return baf.String()
}

func randMAC() string {
	rand.Seed(time.Now().UnixNano())
	return "52:54:00" + fmt.Sprintf(":%x", rand.Intn(255)) +
		fmt.Sprintf(":%x", rand.Intn(255)) +
		fmt.Sprintf(":%x", rand.Intn(255))
}
