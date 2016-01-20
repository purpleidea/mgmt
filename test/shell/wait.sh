# NOTE: boiler plate to wait on mgmt; source with: . wait.sh; should NOT be +x
for j in `jobs -p`
do
	[ $j -eq `pidof etcd` ] && continue	# don't wait for etcd
	wait $j	# wait for mgmt job $j
done
