{
  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixos-25.05";
  inputs.nixpkgs-unstable.url = "github:NixOS/nixpkgs/nixos-unstable";

  outputs =
    {
      self,
      nixpkgs,
      nixpkgs-unstable,
    }:
    let
      supportedSystems = [ "x86_64-linux" ];
      forEachSupportedSystem =
        f:
        nixpkgs.lib.genAttrs supportedSystems (
          system:
          f {
            pkgs = import nixpkgs { inherit system; };
            pkgs-unstable = import nixpkgs-unstable { inherit system; };
            inherit system;
          }
        );
    in
    {
      devShells = forEachSupportedSystem (
        {
          pkgs,
          system,
          pkgs-unstable,
        }:
        {
          default = pkgs.mkShell {
            packages = with pkgs; [
              # Go & Go Tooling
              pkgs-unstable.go
              pkgs-unstable.delve
              pkgs-unstable.golangci-lint
              pkgs-unstable.gotools
              ginkgo

              # Codegen for the spec protos
              protobuf
              buf
              protoc-gen-go

              # Commands
              just

              # Misc
              pkg-config
              jq
            ];

            env = {
              GOROOT = "${pkgs-unstable.go}/share/go";
            };

            hardeningDisable = [ "fortify" ];
          };
        }
      );
    };
}
