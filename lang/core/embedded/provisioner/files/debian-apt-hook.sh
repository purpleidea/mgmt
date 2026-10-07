#!/bin/sh
# Installed into the debian-installer by the mgmt provisioner. It runs after the
# base system is installed, which is before the repositories get set up. Our
# repository metadata is cached forever, but some of it (security, and updates
# since trixie) has a date after which apt refuses to use it. We'd rather get
# the updates we have, than none of them, so this tells apt to ignore the dates
# during the install. The signatures are still checked. Our post script removes
# this, so the installed machine checks those dates as usual.
mkdir -p /target/etc/apt/apt.conf.d/
cat > /target/etc/apt/apt.conf.d/00mgmt-provisioner << EOF
// Installed by the mgmt provisioner, and removed at the end of the install.
Acquire::Check-Valid-Until "false";
Acquire::Check-Date "false";
EOF
