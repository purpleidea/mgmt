# Legacy lib examples

These programs demonstrate historical versions of the lib and GAPI APIs. They
do not compile against the current APIs and are retained as reference material.

Each source file has a `//go:build ignore` constraint so that the golang tool
does not treat this directory as a package. To restore an example, update it to
the current APIs and remove the constraint; `test/test-examples.sh` will then
compile it.
