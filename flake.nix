{
  description = "kloudlite plugin helm charts dev environment";

  inputs = {
    nixpkgs.url = "github:nixos/nixpkgs/nixos-unstable";
  };

  outputs = { self, nixpkgs, flake-utils }:
    flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = import nixpkgs { inherit system; };
      in
      {
        devShells.default = pkgs.mkShell {
          # hardeningDisable = [ "all" ];

          buildInputs = with pkgs; [
            # cli tools
            jq
            yq
            go-task

            # source version control
            git
            pre-commit

            # programming tools
            go
            kubebuilder

            upx

            (stdenv.mkDerivation {
              name = "run";
              pname = "run";
              src = fetchurl {
                url = "https://github.com/nxtcoder17/Runfile/releases/download/v1.1.2/run-linux-amd64";
                sha256 = "sha256-vAnENb2BhIKHSkx19pTytlv03RIOzUxoxn2nRrk6Zd8=";
              };
              unpackPhase = ":";
              buildInputs = [ curl ];
              nativeBuildInputs = [ coreutils makeWrapper ];
              installPhase = ''
                mkdir -p $out/bin
                # ls -al $src/
                cp $src $out/bin/run
                chmod +x $out/bin/run
              '';
            })
          ];

          shellHook = ''
          '';
        };
      }
    );
}


