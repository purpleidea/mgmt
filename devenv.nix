{ pkgs, lib, config, inputs, ... }:
let
  pkgs-unstable = import inputs.nixpkgs-unstable { system = pkgs.stdenv.system; };
in
{
  packages = with pkgs; [
    golint
    mdl
    gdb
    etcd
    pkg-config
    libvirt
    libxml2
    augeas
    nex
    ragel
    which
    graphviz
    graphviz-nox
    gcc
    bash
    inotify-tools
  ];
  languages.go = {
    enable = true;
    package = pkgs-unstable.go;
  };

  shell = lib.mkForce (pkgs.buildFHSUserEnv {
    name = "devenv-shell";
    targetPkgs = _: config.packages;
    runScript = "bash";
    profile = ''
      ${lib.optionalString config.devenv.debug "set -x"}
      ${config.enterShell}
    '' + lib.concatStringsSep "\n" (lib.mapAttrsToList (name: value: ''
      export ${name}=${value}
    '') config.env);
  }).env;
}
