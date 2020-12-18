module github.com/purpleidea/mgmt

go 1.15

require (
	docker.io/go-docker v1.0.0
	github.com/Microsoft/go-winio v0.4.16 // indirect
	github.com/aws/aws-sdk-go v1.36.10
	github.com/coredhcp/coredhcp v0.0.0-20200809170558-a9aa31766d13
	github.com/coreos/go-systemd/v22 v22.1.0
	github.com/cyphar/filepath-securejoin v0.2.2
	github.com/davecgh/go-spew v1.1.1
	github.com/deniswernert/go-fstab v0.0.0-20141204152952-eb4090f26517
	github.com/docker/distribution v2.7.1+incompatible // indirect
	github.com/docker/docker/internal/testutil v0.0.0-00010101000000-000000000000 // indirect
	github.com/docker/go-connections v0.4.0
	github.com/docker/go-units v0.4.0 // indirect
	github.com/godbus/dbus/v5 v5.0.3
	github.com/hashicorp/consul/api v1.8.1
	github.com/hashicorp/go-multierror v1.1.0
	github.com/hashicorp/hil v0.0.0-20201113172851-43f73a9c7007
	github.com/iancoleman/strcase v0.1.2
	github.com/insomniacslk/dhcp v0.0.0-20201112113307-4de412bc85d8
	github.com/kylelemons/godebug v1.1.0
	github.com/libvirt/libvirt-go v6.10.0+incompatible
	github.com/libvirt/libvirt-go-xml v6.10.0+incompatible
	github.com/opencontainers/go-digest v1.0.0 // indirect
	github.com/opencontainers/image-spec v1.0.1 // indirect
	github.com/pborman/uuid v1.2.1
	github.com/pin/tftp v0.0.0-20200229063000-e4f073737eb2
	github.com/pkg/errors v0.9.1
	github.com/prometheus/client_golang v1.9.0
	github.com/spf13/afero v1.5.1
	github.com/urfave/cli/v2 v2.3.0
	github.com/vishvananda/netlink v1.1.0
	go.etcd.io/etcd v0.0.0-20191023171146-3cf2f69b5738
	golang.org/x/crypto v0.0.0-20201217014255-9d1352758620
	golang.org/x/net v0.0.0-20201216054612-986b41b23924 // indirect
	golang.org/x/sys v0.0.0-20201214210602-f9fddec55a1e
	golang.org/x/time v0.0.0-20201208040808-7e3f01d25324
	golang.org/x/tools v0.0.0-20201217235154-5b06639e575e // indirect
	gopkg.in/fsnotify.v1 v1.4.7
	gopkg.in/src-d/go-git.v4 v4.13.1
	gopkg.in/yaml.v2 v2.4.0
	honnef.co/go/augeas v0.0.0-20161110001225-ca62e35ed6b8
)

replace github.com/docker/docker/internal/testutil => gotest.tools/v3 v3.0.3
