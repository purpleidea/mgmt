# wasm

Running `make` builds `main.wasm` into this directory, from the code in the
parent directory. The whole directory gets embedded, and this file makes sure
it always exists, so that we can still compile without the `main.wasm` file,
such as for the tests.
