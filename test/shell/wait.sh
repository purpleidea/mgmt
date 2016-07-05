# NOTE: boiler plate to wait on mgmt; source with: . wait.sh; should NOT be +x
while test "`jobs -p`" != ""
do
	for j in `jobs -p`
	do
		wait $j || continue	# wait for mgmt job $j
	done
done
