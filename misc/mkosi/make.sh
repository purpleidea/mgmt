#!/bin/bash -e

echo running "$0"
set -o errexit
set -o pipefail

# Run it the directory this script is in.
ROOT=$(dirname "${BASH_SOURCE}")
cd "${ROOT}"
#pwd

if [ "$2" = "" ]; then
	# output should be an absolute path
	echo "Usage: ./$0 <distro> <output>"
	exit 1
fi
distro="$1"	# eg: fedora-29
output="$2"	# an absolute dir path

# Check that the "default" mkosi distro file exists.
mkosi_default="mkosi.default.${distro}"	# eg: mkosi.default.fedora-29
if [ ! -e "${mkosi_default}" ]; then
	echo "Error: mkosi distro file doesn't exist."
	exit 1
fi

# Make sure we're on a tagged commit.
TAG=$(git tag -l --points-at HEAD)
if [ "$TAG" == "" ]; then
	echo "Error: fpm cannot handle an untagged commit."
	exit 1
fi

mkdir -p mkosi.{cache,builddir,output}

# Speed up builds significantly.
if mountpoint mkosi.output/; then
	echo "The output directory is already a mountpoint."
	exit 1
fi
echo "Mounting 5G tmpfs in 3 seconds, press ^C to cancel if you are low on RAM."
sleep 3s
sudo mount -t tmpfs -o size=5g tmpfs mkosi.output/	# zoom!
trap 'echo Unmounting tmpfs... && sudo umount mkosi.output/' EXIT	# Unmount on script exit.

echo "Running mkosi (requires root)..."
# Passing env vars doesn't work, so use: https://github.com/systemd/mkosi/pull/367
time sudo mkosi --default="${mkosi_default}" build	# Test with `summary` instead of `build`.

# FIXME: workaround bug: https://github.com/systemd/mkosi/issues/366
u=$(id --name --user)
g=$(id --name --group)
echo "Running chown (requires root)..."
sudo chown -R $u:$g mkosi.{cache,builddir}

# Move packaged build artifact into our releases/ directory.
mv mkosi.builddir/${distro}/ "${output}"	# mv mkosi.builddir/fedora-29/ /.../releases/$(VERSION)/

echo "Done $0 run!"
