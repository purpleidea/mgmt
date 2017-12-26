#!/bin/bash
set -eEu
set -o pipefail
. test/util.sh

################################################################################
# check for any bash files that aren't properly formatted
# TODO: this is hardly exhaustive
################################################################################

ROOT=$(dirname "${BASH_SOURCE}")/..

cd "${ROOT}"

declare -i RC=0

is_shell() {
	declare -r file="$1"
	file --mime --dereference "${file}" | grep 'shellscript' &> /dev/null
}

while IFS='' read -r -d '' file; do
	if ! is_shell "${file}"; then
		continue
	fi

	if [[ "${file}" =~ misc/delta-cpu.sh ]]; then
		# TODO: Don't skip this file.
		continue
	fi

	if grep -q '^  ' "$file"; then
		# Search for more than one leading space to ensure we use tabs.
		err "$file should use tabs, not spaces, for indentation."
		RC=1
	fi

	if ! [[ $(sed -n '1p' "$file") =~ ^\#!/bin/bash$ ]]; then
		# This is arbitrary to promote consistency.
		err "$file should have '#!/bin/bash' as the shebang."
		RC=1
	fi

	if ! [[ $(sed -n '2p' "$file") =~ set\ -eEu ]]; then
		# set -e (errexit) so we exit on error
		# set -E (errtrace) is needed for traps
		# set -u to avoid unbound variables
		err "$file needs \"set -eEu\""
		RC=1
	fi

	if ! [[ $(sed -n '3p' "$file") =~ set\ -o\ pipefail ]]; then
		# set -o pipeline means every command in pipeline must exit 0
		# and forces us to write robust scripts.
		err "$file needs \"set -o pipefail\""
		RC=1
	fi

	if [[ "${file}" =~ test/ ]] && ! [[ $(sed -n '4p' "$file") =~ ^.\ test/util.sh$ ]]; then
		# All test scripts should source test/util.sh
		err "$file needs \". test/util.sh\""
		RC=1
	fi

	output="$(git grep '[i]f .*\! -n' || :)"
	if [[ -n "${output}" ]]; then
		# Avoid double-negatives.
		err "Use '-z' instead of '! -n'"
		indent "${output}"
		RC=1
	fi

	output="$(git grep '[i]f .*\! -z' || :)"
	if [[ -n "${output}" ]]; then
		# Avoid double-negatives.
		err "Use '-n' instead of '! -z'"
		indent "${output}"
		RC=1
	fi
done < <(git ls-files -z)
exit $RC
