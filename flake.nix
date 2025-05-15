{
  description = "kloudlite plugin helm charts dev workspace";

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
                url = "https://github.com/nxtcoder17/Runfile/releases/download/v1.5.1/run-linux-amd64";
                sha256 = "sha256-eR/j8+nqoo0khCnBaZg+kqNgnWRTFQDJ7jkRQuo/9Hs=";
              };
              unpackPhase = ":";
              buildInputs = [ ];
              nativeBuildInputs = [ ];
              installPhase = ''
                mkdir -p $out/bin
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


