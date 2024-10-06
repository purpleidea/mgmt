package coreos

import (
	"bufio"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/purpleidea/mgmt/lang/types"
)

const (
	VIRTUALIZATION_EC2    = "ec2"
	VIRTUALIZATION_GCE    = "GCE"
	VIRTUALIZATION_DOCKER = "docker"
	VIRTUALIZATION_PODMAN = "podman"
	VIRTUALIZATION_LXC    = "lxc"
	VIRTUALIZATION_NONE   = "none"
)

type (
	fileTargetPredicate    func(fileTarget) bool
	networkTargetPredicate func(networkTarget) bool
)

// TODO fileTarget should probably take a func(string) (bool,error)
type fileTarget struct {
	file       string
	target     string
	existsOnly bool
}

// TODO networkTarget should probably take a func(response) (bool,error)
type networkTarget struct {
	url       string
	headerVal string
	headerKey string
}

type (
	fileTargets    []fileTarget
	networkTargets []networkTarget
)

type VirtualizationPlatform interface {
	checkFiles() bool
	checkNetwork() bool
	evaluate() string
}

type ConcretePlatform struct {
	name           string
	fileTargets    fileTargets
	networkTargets networkTargets
}

var ec2Platform = ConcretePlatform{
	name: VIRTUALIZATION_EC2,
	fileTargets: []fileTarget{
		{file: "/sys/class/dmi/id/board_vendor", target: "Amazon EC2"},
		{file: "/sys/class/dmi/id/sys_vendor", target: "Amazon EC2"},
	},
}

var gcePlatform = ConcretePlatform{
	name: VIRTUALIZATION_GCE,
	networkTargets: []networkTarget{
		{url: "metadata.google.internal", headerKey: "Metadata-Flavor", headerVal: "Google"},
	},
}

var dockerPlatform = ConcretePlatform{
	name: VIRTUALIZATION_DOCKER,
	fileTargets: []fileTarget{
		{file: "/.dockerenv", existsOnly: true},
		{file: "/proc/self/mountinfo", target: "docker/containers"},
	},
}

var podmanPlatform = ConcretePlatform{
	name: VIRTUALIZATION_PODMAN,
	fileTargets: []fileTarget{
		{file: "/.dockerenv", existsOnly: true},
		{file: "/proc/self/mountinfo", target: "docker/containers"},
	},
}

var lxcPlatform = ConcretePlatform{
	name: VIRTUALIZATION_LXC,
	fileTargets: []fileTarget{
		{file: "/proc/1/environ", target: "container=lxc"},
	},
}

func checkInNetwork(networkTarget networkTarget) bool {
	tr := &http.Transport{
		MaxIdleConns:          5,
		ResponseHeaderTimeout: 10 * time.Second,
		IdleConnTimeout:       30 * time.Second,
		DisableCompression:    true,
	}

	client := &http.Client{Transport: tr}

	resp, err := client.Get(networkTarget.url)
	if err != nil {
		return false
	}
	actualHeader := resp.Header.Get(networkTarget.headerKey)
	if actualHeader == networkTarget.headerVal {
		return true
	}
	return false
}

func scanFile(fileTarget fileTarget) bool {
	f, err := os.OpenFile(fileTarget.file, os.O_RDONLY, os.ModePerm)
	if err != nil {
		return false
	}
	defer f.Close()

	if fileTarget.existsOnly {
		return true
	}

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text() // GET the line string
		if strings.Contains(line, fileTarget.target) {
			return true
		}
	}
	if err := sc.Err(); err != nil {
		return false
	}
	return false
}

func (plat *ConcretePlatform) checkFiles() bool {
	for _, targetFile := range plat.fileTargets {
		result := scanFile(targetFile)
		if result {
			return result
		}
	}

	return false
}

func (plat *ConcretePlatform) checkNetwork() bool {
	for _, targetFile := range plat.networkTargets {
		result := checkInNetwork(targetFile)
		if result {
			return result
		}
	}

	return false
}

func (plat *ConcretePlatform) evaluate() string {
	switch true {
	case plat.checkNetwork():
		return plat.name
	case plat.checkFiles():
		return plat.name
	case false:

	}
	return VIRTUALIZATION_NONE
}

func isEc2Instance() string {
	return ec2Platform.evaluate()
}

func isGCEInstance() string {
	return gcePlatform.evaluate()
}

func isDockerContainer() string {
	return dockerPlatform.evaluate()
}

func isLxcContainer() string {
	return lxcPlatform.evaluate()
}

func isPodManContainer() string {
	return lxcPlatform.evaluate()
}

func isContainer() string {
	isContainerFuncs := []func() string{isDockerContainer, isPodManContainer, isLxcContainer}
	for _, isContainerFunc := range isContainerFuncs {

		containerPlatform := isContainerFunc()
		if containerPlatform != VIRTUALIZATION_NONE {
			return containerPlatform
		}

	}

	return VIRTUALIZATION_NONE
}

func IsVirtualized(input []types.Value) (types.Value, error) {
	platform := isContainer()

	virtualizationDetected := platform != VIRTUALIZATION_NONE

	return &types.StructValue{
		T: types.NewType("struct{isVirtualized bool; platform str}"),
		V: map[string]types.Value{
			"isVirtualized": &types.BoolValue{V: virtualizationDetected},
			"platformName":  &types.StrValue{V: platform},
		},
	}, nil
}
