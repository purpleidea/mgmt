#!/usr/bin/env bash
# Make a new "baddev" branch, start on whatever branch you want to base it on.
git checkout -B baddev # scary -B reset the branch
make
git add lang/parser/lexer.nn.go --force
git add lang/parser/y.go --force
git add lang/interpolate/parse.generated.go --force
git add lang/core/generated_funcs.go --force
git add engine/resources/http_server_ui/main.wasm --force
git commit -m 'lang: Commit generated code'
git push origin baddev --force
echo 'run `make baddev` to build a binary!'
