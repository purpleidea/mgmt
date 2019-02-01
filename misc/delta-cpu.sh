#!/bin/bash
# shitty cpu count control, useful for live demos

minimum=1	# don't decrease below this number of cpus
maximum=8	# don't increase above this number of cpus
count=1		# initial count
factor=3
function output() {
count=$1	# arg!
cat << EOF > /tmp/cpu-count
$count
EOF
}
#tput cuu 1 && tput el	# remove last line
str=''
tnuoc=$((maximum-count))	# backwards count
count2=$((count * factor))
tnuoc2=$((tnuoc * factor))
left=`yes '>' | head -$count2 | paste -s -d '' -`
right=`yes ' ' | head -$tnuoc2 | paste -s -d '' -`
str="${left}${right}"
_min=$((minimum-1))
_max=$((maximum+1))
reset	# clean up once...
output $count	# call function
while true; do

	read -n1 -r -s -p "CPUs count is: $count; ${str} Press +/- key to adjust." key
	if [ "$key" = "q" ] || [ "$key" = "Q" ]; then
		echo	# newline
		exit
	fi
	if [ ! "$key" = "+" ] && [ ! "$key" = "-" ] && [ ! "$key" = "=" ] && [ ! "$key" = "_" ]; then	# wrong key
		reset	# woops, reset it all...
		continue
	fi
	if [ "$key" == "+" ] || [ "$key" == "=" ]; then
		count=$((count+1))
	fi
	if [ "$key" == "-" ] || [ "$key" == "_" ]; then
		count=$((count-1))
	fi
	if [ $count -eq $_min ]; then	# min
		count=$minimum
	fi
	if [ $count -eq $_max ]; then	# max
		count=$maximum
	fi

	tnuoc=$((maximum-count))	# backwards count
	#echo "count is: $count"
	#echo "tnuoc is: $tnuoc"
	count2=$((count * factor))
	tnuoc2=$((tnuoc * factor))
	left=`yes '>' | head -$count2 | paste -s -d '' -`
	right=`yes ' ' | head -$tnuoc2 | paste -s -d '' -`
	str="${left}${right}"
	#echo "str is: $str"
	echo -ne '\r'	# backup
	output $count	# call function
done
