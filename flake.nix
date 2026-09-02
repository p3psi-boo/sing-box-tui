{
  description = "sing-box TUI — terminal UI for sing-box API";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, flake-utils }:
    flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = import nixpkgs { inherit system; };
      in {
        devShells.default = pkgs.mkShell {
          packages = with pkgs; [
            go
            buf
            protobuf
            protoc-gen-go
            protoc-gen-go-grpc
            golangci-lint
            gopls
          ];

          shellHook = ''
            export GOPATH="$PWD/.go"
            export GOBIN="$GOPATH/bin"
            export PATH="$GOBIN:$PATH"
            echo "sing-box-tui devShell"
            echo "  go build ./cmd/sing-box-tui"
            echo "  buf generate"
          '';
        };
      });
}
