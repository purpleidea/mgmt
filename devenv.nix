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
}
