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
                url = "https://github.com/nxtcoder17/Runfile/releases/download/nightly/run-linux-amd64";
                sha256 = "sha256-H6On0hZrPjm679mSb2gdcj++aQjLPo9Z7F1m/PDsBTo=";
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


