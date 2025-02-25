{
  description = "Next generation distributed, event-driven, parallel config management!";
  inputs = {
    flake-utils.url = "github:numtide/flake-utils";
    nixpkgs.url = "github:urandom2/nixpkgs/mgmt";
  };
  outputs = {
    flake-utils,
    nixpkgs,
    self,
  }:
    flake-utils.lib.eachDefaultSystem (system: let
      buildInputs = with pkgs; [
        augeas
        libvirt
        libxml2
      ];
      nativeBuildInputs = with pkgs; [
        gotools
        nex
        pkg-config
        ragel
      ];
      pkgs = nixpkgs.legacyPackages.${system};
    in {
      devShells.default = pkgs.mkShell {
        inherit buildInputs nativeBuildInputs;
        packages = [pkgs.go];
      };
      packages.default = let
        pname = "mgmt";
        version = "0.0.22";
      in
        pkgs.buildGoModule {
          inherit pname version buildInputs nativeBuildInputs;
          ldflags = [
            "-X main.program=${pname}"
            "-X main.version=${version}"
          ];
          meta = {
            description = "Next generation distributed, event-driven, parallel config management!";
            homepage = "https://mgmtconfig.com";
            license = pkgs.lib.licenses.gpl3;
          };
          preBuild = ''
            substituteInPlace Makefile --replace "/usr/bin/env " ""
            substituteInPlace lang/Makefile --replace "/usr/bin/env " ""
            substituteInPlace lang/types/Makefile --replace "/usr/bin/env " ""
            substituteInPlace lang/types/Makefile --replace "unset GOCACHE &&" ""
            patchShebangs .
            make lang funcgen
          '';
          src = ./.;
          subPackages = ["."];
          vendorHash = "sha256-Dtqy4TILN+7JXiHKHDdjzRTsT8jZYG5sPudxhd8znXY=";
        };
    });
}
