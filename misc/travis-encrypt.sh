#!/usr/bin/env bash
# modified slightly, originally from:
# https://raw.githubusercontent.com/dlenski/travis-encrypt-sh/master/travis-encrypt

if [[ $# < 2 ]]; then
	p="$(basename $0)"
	here=$(mktemp)
	git remote -v 2>/dev/null | grep -oP '(?<=github.com[:/])([^/]+/[^/]+?)(?=\.git| )' > "$here"
	IFS=/ read user repo < "$here"
else
	user="$1"
	repo="$2"
	shift 2
fi

if [[ -z "$user" || -z "$repo" ]]; then
	echo "usage: $p [user] [repository] [value to encrypt]"
	echo
	echo "e.g.: $p 'P@ssw0rd' (only inside a repo with a github remote)"
	echo "or $p ${user:-jsmith} ${repo:-MyRepo} 'VAR=\"s3cret\"'"
	echo "or $p ${user:-jsmith} ${repo:-MyRepo} 'P@ssw0rd'"
	exit 1
fi >&2

value="$1"

# Fetch key
keyurl="https://api.travis-ci.org/repos/$user/$repo/key"
echo "Fetching key from $keyurl ..." >&2
keyfile=$(mktemp)
curl -s "$keyurl" > "$keyfile" || {
	echo "Couldn't fetch key from $keyurl!" >&2
	exit 1
}

# (Exceptionally poor)-man's JSON-to-PEM
# Some Travis-CI pubkeys have " RSA PUBLIC KEY", where others have the standard " PUBLIC KEY".
sed -i 's|\\n|\n|g; s|"|\n|g; s/ RSA PUBLIC KEY/ PUBLIC KEY/g' "$keyfile"
grep -q "BEGIN PUBLIC KEY" "$keyfile" || {
	echo "Key file from $keyurl seems malformed: $keyfile" >&2
	exit 1
}

if [[ -z "$value" ]]; then
	read -p "Value to encrypt? " value
fi

echo "Encrypting with openssl rsautl ..." >&2

set -o pipefail
echo -n "$value" | openssl rsautl -encrypt -inkey "$keyfile" -pubin -pkcs | base64 -w0 || {
	echo "Error in openssl rsautl." >&2
	exit 1
}
echo $'\nSuccess.' >&2
