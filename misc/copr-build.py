#!/usr/bin/python

# README:
# for initial setup, browse to: https://copr.fedoraproject.org/api/
# and it will have a ~/.config/copr config that you can download.
# happy hacking!

import os
import sys
import copr
import time

COPR = 'mgmt'

if len(sys.argv) != 2:
	print("Usage: %s <srpm url>" % sys.argv[0])
	sys.exit(1)

url = sys.argv[1]

client = copr.CoprClient.create_from_file_config(os.path.expanduser("~/.config/copr"))

result = client.create_new_build(COPR, [url])
if result.output != 'ok':
	print(result.error)
	sys.exit(1)
print(result.message)

# modified from: https://python-copr.readthedocs.org/en/latest/Examples.html#work-with-builds
for bw in result.builds_list:
	print("Build #{}: {}".format(bw.build_id, bw.handle.get_build_details().status))

# cancel all created build
#for bw in result.builds_list:
#	bw.handle.cancel_build()

# get build status for each chroot
#for bw in result.builds_list:
#	print("build: {}".format(bw.build_id))
#	for ch, status in bw.handle.get_build_details().data["chroots"].items():
#		print("\t chroot {}:\t {}".format(ch, status))

# simple build progress:

watched = set(result.builds_list)
done = set()
state = {}
for bw in watched:	# store initial states
	state[bw.build_id] = bw.handle.get_build_details().status

while watched != done:
	for bw in watched:
		if bw in done:
			continue
		status = bw.handle.get_build_details().status
		if status != state.get(bw.build_id):
			print("Build #{}: {}".format(bw.build_id, status))
			state[bw.build_id] = status	# update status

		if status in ['skipped', 'failed', 'succeeded']:
			done.add(bw)

	if watched == done: break	# avoid long while sleep
	else: time.sleep(10)

print 'Done!'
