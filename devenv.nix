{ pkgs, lib, config, inputs, ... }:
let
  pkgs-unstable = import inputs.nixpkgs-unstable { system = pkgs.stdenv.system; };
in
{
  packages = with pkgs; [
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
  ];
  languages.go = {
    enable = true;
    package = pkgs-unstable.go;
  };
}
