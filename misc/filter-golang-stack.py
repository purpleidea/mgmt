#!/usr/bin/env python3
# -*- coding: utf-8 -*-
# Copyright (C) 2019+ James Shubin
# Written by James Shubin <james@shubin.ca>
#
# This program is free software: you can redistribute it and/or modify
# it under the terms of the GNU General Public License as published by
# the Free Software Foundation, either version 3 of the License, or
# (at your option) any later version.
#
# This program is distributed in the hope that it will be useful,
# but WITHOUT ANY WARRANTY; without even the implied warranty of
# MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
# GNU General Public License for more details.
#
# You should have received a copy of the GNU General Public License
# along with this program.  If not, see <http://www.gnu.org/licenses/>.

# This program filters golang stack traces. Modify the filter by editing the
# filter_chunk function. This is a quick hack to help me get work done. It's
# written in python to encourage some golang enthusiast to rewrite it in golang
# and to expand the functionality.

import sys

lines = sys.stdin.readlines()

print("read: %d lines" % len(lines))

# find program start
for i in range(len(lines)):
	line = lines[i]
	if line.startswith("PC="):
		start=i
		break

print("starts at line: %d" % (start+1)) # +1 because we're zero based

def is_chunk(line):
	if line == "main.main()" or line.startswith("goroutine"):
		return True

	return False

def is_end(line):
	# register junk as seen at the end
	rx = ["rax", "rbx", "rcx", "rdx", "rdi", "rsi", "rbp", "rsp", "r8", "r9", "r10", "r11", "r12", "r13", "r14", "r15", "rip", "rflags", "cs", "fs", "gs"]
	for x in rx:
		if line.startswith(x + " "):
			return True

	return False

def filter_chunk(chunk):
	lines = chunk.split("\n")
	if len(lines) < 2:
		return False

	package_line = lines[1]
	if package_line.startswith("github.com/purpleidea/mgmt/vendor/"):
		return False

	return True

chunks = []
for i in range(start+1, len(lines)):
	line = lines[i]

	# found a chunk
	if is_chunk(line):
		chunk = line
		for j in range(i+1, len(lines)):
			jline = lines[j]
			if is_chunk(jline) or is_end(jline):
				chunks.append(chunk)
				i = j
				if is_end(jline):
					end = j
				break
			# else
			chunk = chunk + jline


print("found %d chunks" % len(chunks))

count = 0
for i in range(len(chunks)):
	chunk = chunks[i]
	if not filter_chunk(chunk):
		continue

	print(">>> chunk %d:" % i)
	print(chunk)
	print("")
	count = count + 1

print("got %d filtered chunks" % count)

print("ends at line: %d" % (end+1)) # +1 because we're zero based

for i in range(end, len(lines)):
	line = lines[i]
	print("%s" % line, end='')	# already comes with a newline
