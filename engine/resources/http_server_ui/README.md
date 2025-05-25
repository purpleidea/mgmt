This directory contains the golang wasm source for the `http_server_ui`
resource. It gets built automatically when you run `make` from the main project
root directory.

After it gets built, the compiled artifact gets bundled into the main project
binary via go embed.

It is not a normal package that should get built with everything else.
