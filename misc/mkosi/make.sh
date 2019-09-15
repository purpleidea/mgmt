#!/bin/bash -e

echo running "$0"
set -o errexit
set -o pipefail

# Run it the directory this script is in.
ROOT=$(dirname "${BASH_SOURCE}")
cd "${ROOT}"
#pwd

if [ "$3" = "" ]; then
	# output should be an absolute path
	echo "Usage: ./$0 <type> <input> <output>"
	exit 1
fi

# The type should be one of these.
if [ "$1" != "rpm" ] && [ "$1" != "deb" ] && [ "$1" != "pacman" ]; then
	echo "Error: build type sanity check failure."
	exit 1
fi

# The input should start with this format string.
if [[ $2 != mkosi.default.* ]]; then
	echo "Error: build input sanity check failure."
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
time sudo mkosi --default="$2" build	# Test with `summary` instead of `build`.

# FIXME: workaround bug: https://github.com/systemd/mkosi/issues/366
u=$(id --name --user)
g=$(id --name --group)
echo "Running chown (requires root)..."
sudo chown -R $u:$g mkosi.{cache,builddir}

# Move packaged build artifact into our releases/ directory.
mv mkosi.builddir/${1}/ "$3"	# mv mkosi.builddir/rpm/ /.../releases/$(VERSION)/

echo "Done $0 run!"
