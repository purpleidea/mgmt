#!/bin/bash

# use generic change
set -euo pipefail

misc/changelog-from-git.sh > debian/changelog
