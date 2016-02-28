# NOTE: boiler plate to wait on mgmt; source with: . wait.sh; should NOT be +x
while test "`jobs -p`" != "" && test "`jobs -p`" != "`pidof etcd`"
do
	for j in `jobs -p`
	do
		[ "$j" = "`pidof etcd`" ] && continue	# don't wait for etcd
		wait $j || continue	# wait for mgmt job $j
	done
done
