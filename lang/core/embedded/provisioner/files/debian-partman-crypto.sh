#!/bin/sh
# Installed into the debian-installer partitioner (partman) by the mgmt
# provisioner. The automatic partitioner can only encrypt with lvm on top, but
# we don't ever want lvm. Instead, our recipe marks the root partition with the
# crypto method, and then this sets up the encryption and puts the filesystem
# directly on the encrypted device. This is what autopartition-crypto does, but
# without any of the lvm parts. It runs after the automatic partitioning, which
# is the 10initial_auto display.d script, and before the 80manual_partitioning
# one. The partman base dir moved from /lib/partman to /usr/lib/partman after
# trixie, so we look for both of them.

P=/lib/partman
if [ ! -f "$P/lib/base.sh" ]; then
	P=/usr/lib/partman
fi

. $P/lib/base.sh

# Only run once, and only after the automatic partitioning has run.
if [ ! -f /var/lib/partman/initial_auto ] || [ -f /var/lib/partman/mgmt_crypto ]; then
	exit 0
fi
>/var/lib/partman/mgmt_crypto

if [ ! -f $P/lib/crypto-base.sh ]; then
	logger -t mgmt "partman-crypto is not loaded"
	exit 1
fi
. $P/lib/crypto-base.sh

# Find the partitions which the recipe marked for encryption.
found=""
for dev in $DEVICES/*; do
	[ -d "$dev" ] || continue
	cd $dev

	partitions=""
	open_dialog PARTITIONS
	while { read_line num id size type fs path name; [ "$id" ]; }; do
		[ "$fs" != free ] || continue
		partitions="$partitions $id"
	done
	close_dialog

	for id in $partitions; do
		[ -f $id/method ] || continue
		[ "$(cat $id/method)" = crypto ] || continue
		[ ! -f $id/crypt_active ] || continue

		echo dm-crypt >$id/crypto_type
		crypto_prepare_method "$dev/$id" dm-crypt || exit 1
		# It's a fresh install, so don't spend hours erasing the disk.
		>$id/skip_erase
		found="$found $dev/$id"
	done
done

if [ -z "$found" ]; then
	exit 0 # nothing to encrypt
fi

# This writes the partition table, runs luksFormat with the preseeded
# passphrase, opens the device, and then restarts partman to detect it.
crypto_check_setup || exit 1
crypto_setup no || exit 1

# Put the root filesystem on each encrypted device. (There is only one.)
db_get partman/default_filesystem
filesystem="$RET"
for part in $found; do
	if [ ! -f "$part/crypt_active" ]; then
		logger -t mgmt "encrypted device was not set up for: $part"
		exit 1
	fi
	cryptdev=$(cat "$part/crypt_active") # eg: /dev/mapper/sda3_crypt

	for dev in $DEVICES/*; do
		[ -f "$dev/device" ] || continue
		[ "$(cat "$dev/device")" = "$cryptdev" ] || continue
		cd $dev

		partitions=""
		open_dialog PARTITIONS
		while { read_line num id size type fs path name; [ "$id" ]; }; do
			[ "$fs" != free ] || continue
			partitions="$partitions $id"
		done
		close_dialog

		# These are the same files a recipe would make for the root.
		for id in $partitions; do
			echo format >$id/method
			>$id/format
			>$id/use_filesystem
			echo "$filesystem" >$id/filesystem
			echo / >$id/mountpoint
			update_partition $dev $id
		done

		# Like autopartition-crypto, tell partman this virtual device
		# doesn't have a changed partition table to write.
		open_dialog DISK_UNCHANGED
		close_dialog
	done
done

exit 0
