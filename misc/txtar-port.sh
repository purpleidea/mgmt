#!/usr/bin/env bash
# port tests to txtar
for f in */main.mcl; do
	echo $f == $(dirname $f)
	cat $f | cat - $(dirname $f).txtar | sponge $(dirname $f).txtar
	echo '-- main.mcl --' | cat - $(dirname $f).txtar | sponge $(dirname $f).txtar
done

#for f in *.txtar; do
#	echo $f
#	#cat $f | cat - $(dirname $f).txtar | sponge $(dirname $f).txtar
#	echo '-- OUTPUT --' | cat - $f | sponge $f
#done
