#!/bin/bash

# install dependencies
APT=$(command -v apt)
YUM=$(command -v yum)

if [ -z "$APT" -a -z "$YUM" ]
then
	# TODO relay this error back to the engine
	exit 1
fi

if [ -n "$APT" ]
then
	apt update
	apt install -y libaugeas0 libvirt0 || exit 1
fi

if [ -n "$YUM" ]
then
	yum install -y augeas-libs libvirt-libs || exit 1
fi

# download the binary and graph
tmpdir=$(mktemp -d)
curl "{{- .Binary -}}" -o "${tmpdir}/mgmt"
chmod +x "${tmpdir}/mgmt"
curl "{{- .RemoteGraph -}}" -o "${tmpdir}/graph.yaml"

# The URLs below retrieve IP addresses from the EC2 internal metadata service.
# http://docs.aws.amazon.com/AWSEC2/latest/UserGuide/ec2-instance-metadata.html
internal=$(curl http://169.254.169.254/latest/meta-data/local-ipv4)
external=$(curl http://169.254.169.254/latest/meta-data/public-ipv4)

# run mgmt and connect back to the seed
${tmpdir}/mgmt run --yaml ${tmpdir}/graph.yaml --no-pgp --tmp-prefix \
--seeds {{.Seed}} \
--client-urls "http://${internal}:{{.CPort}}" \
--server-urls "http://${internal}:{{.SPort}}" \
--advertise-client-urls "http://${external}:{{.CPort}}" \
--advertise-peer-urls "http://${external}:{{.SPort}}"
