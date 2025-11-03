#!/bin/bash

# Do all the gettext stuff.

# Must run from parent dir!
ROOT=$(dirname "${BASH_SOURCE}")/..
cd "${ROOT}"

# First extract all the strings to translate and put them in the po-template.
# We use "--language=C++" because xgettext doesn't support golang yet.
JOIN_EXISTING="--join-existing"
if [ ! -e "data/default.pot" ]; then
	JOIN_EXISTING=""
fi
find . -name '*.go' -print0 | sort -z | xargs -0 xgettext \
	--language=C++ \
	--keyword=G \
	--from-code=UTF-8 \
	$JOIN_EXISTING \
	--no-wrap \
	--omit-header \
	-o data/default.pot

# XXX: No idea why it shows the string location twice in the comment. Remove it?
#awk '$1=="#:" && NF>2 && $(NF)==$(NF-1){NF--}1' data/default.pot | sponge data/default.pot

# Merge in anything new into the translated default.po files.
find . -name 'default.po' -print0 | sort -z | xargs -0 -I {} msgmerge --sort-by-file --update --backup=none "{}" data/default.pot

# Convert from .po to .mo which are more efficient.
#find . -type f -name '*.po' -print0 | xargs -0 -I {} sh -c 'msgfmt -o "{}.mo" "{}"' && \
#	find . -type f -name '*.po.mo' -exec sh -c 'mv "{}" "$(echo "{}" | sed s/\\.po\\.mo/.mo/)"' \;
