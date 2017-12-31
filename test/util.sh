# common settings and functions for test scripts

. vendor/ansi/ansi

if [[ $(uname) == "Darwin" ]] ; then
	export timeout="gtimeout"
else
	export timeout="timeout"
fi

fail_test() {
	fail "$0 $*"
	exit 1
}

pass_test() {
	pass "$0"
	exit 0
}

indent() {
	while IFS='' read -r line; do
		echo -e "\\t${line}"
	done < <(echo "$*")
}

err() {
	ansi --red --bold --newline "[ERROR] $*"
}

fail() {
	ansi --red --bold --newline "[FAIL] $*"
}

info() {
	ansi --white --faint --newline "[INFO] $*"
}

pass() {
	ansi --green --bold --newline "[PASS] $*"
}

smitty() {
  info "$*"
  "$@"
}

warn() {
	ansi --yellow --bold --newline "[WARN] $*"
}

is_git_dirty() {
	! [[ "$(git status 2>&1)" =~ working\ (directory|tree)\ clean ]]
}

finish() {
	declare -ri RC=$?

	if [ ${RC} -eq 0 ]; then
		pass_test
	else
		fail_test "failed with exit code ${RC}"
	fi
}

handle_err() {
	declare -ri RC=$?

	# $BASH_COMMAND contains the command that was being executed at the time of the trap
	# ${BASH_LINENO[0]} contains the line number in the script of that command
	err "exit code ${RC} from \"${BASH_COMMAND}\" on line ${BASH_LINENO[0]} of $0"
}

handle_int() {
	echo
	info "$0 interrupted"
	exit 1
}

# Traps.
# NOTE: In POSIX, beside signals, only EXIT is valid as an event.
#       You must use bash to use ERR.
#       If you "set -e" (errexit), you must also "set -E" (errtrace).
trap finish EXIT
trap handle_err ERR
trap handle_int SIGINT

info "Running $0"

# WARN: "failures" is global scope.
failures=''
run-test() {
	$@ || failures=$( [ -n "$failures" ] && echo -e "$failures\\n$*" || echo "$@" )
}
