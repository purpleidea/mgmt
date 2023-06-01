#!/bin/bash
# port tests to txtar
for f in */main.mcl; do
	echo $f == $(dirname $f)
	cat $f | cat - $(dirname $f).txtar | sponge $(dirname $f).txtar
	echo '-- main.mcl --' | cat - $(dirname $f).txtar | sponge $(dirname $f).txtar
done
