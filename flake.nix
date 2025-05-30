{
  description = "kloudlite plugin helm charts dev workspace";

  inputs = {
    nixpkgs.url = "github:nixos/nixpkgs/nixos-unstable";
  };

  outputs = { self, nixpkgs, flake-utils }:
    flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = import nixpkgs { inherit system; };
        arch =
          if pkgs.stdenv.isAarch64 then "arm64"
          else if pkgs.stdenv.isx86_64 then "amd64"
          else throw "Unsupported architecture";

      in
      {
        devShells.default = pkgs.mkShell {
          # hardeningDisable = [ "all" ];

          buildInputs = with pkgs; [
            bash

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
                url = "https://github.com/nxtcoder17/Runfile/releases/download/v1.5.3/run-linux-${arch}";
                sha256 =
                  if arch == "amd64" then "sha256-BRTgIIg1D+Q4nYN4Z5LoHv+NKamT34qOZZDUxpZkBa0=" else "sha256-wz0ReA/yvZ1ktMGkLc/vMe/gTDpeI6clL+IBYCUo+Yo=
";
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

        packages.helm-job-runner = pkgs.stdenv.mkDerivation {
          name = "helm-job-runner";
          src = pkgs.buildEnv {
            name = "helm-job-runner";
            paths = with pkgs;
              [
                bash
                kubernetes-helm
                curl
                kubectl
                envsubst
                jq
                busybox
              ];
          };
          installPhase = "cp -r $src $out/";
        };
      }
    );
}


