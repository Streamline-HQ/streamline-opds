{
  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixpkgs-unstable";
    flake-parts.url = "github:hercules-ci/flake-parts";
    treefmt-nix = {
      url = "github:numtide/treefmt-nix";
      inputs.nixpkgs.follows = "nixpkgs";
    };
    devshell = {
      url = "github:numtide/devshell";
      inputs.nixpkgs.follows = "nixpkgs";
    };
  };

  outputs =
    inputs@{ flake-parts, ... }:
    flake-parts.lib.mkFlake { inherit inputs; } {
      imports = [
        inputs.devshell.flakeModule
        inputs.treefmt-nix.flakeModule
      ];

      systems = [
        "x86_64-linux"
        "aarch64-linux"
      ];
      perSystem =
        { config, pkgs, ... }:
        let
          go = pkgs.callPackage ./nix/go.nix { };
          nixfmt = pkgs.callPackage ./nix/tools/nixfmt.nix { };
          statix = pkgs.callPackage ./nix/tools/statix.nix { };
          deadnix = pkgs.callPackage ./nix/tools/deadnix.nix { };

          # The golangci-lint e2e/go.mod pins, so its gofumpt and golines are the
          # ones `task lint` checks with; nixpkgs' build would be another version.
          golangciFmt = pkgs.writeShellApplication {
            name = "golangci-lint-fmt";
            runtimeInputs = [ go ];
            text = ''
              export GOWORK=off GOTOOLCHAIN=local CGO_ENABLED=0
              exec "$(go -C e2e tool -n golangci-lint)" fmt "$@"
            '';
          };
        in
        {
          # `nix fmt`, `treefmt` in the devshell and `task fmt`.
          treefmt = {
            projectRootFile = "flake.nix";
            # The Go formatter builds golangci-lint from the module cache, which
            # the sandbox of a flake check cannot reach; CI runs `nix fmt -- --ci`.
            flakeCheck = false;
            programs.nixfmt = {
              enable = true;
              package = nixfmt;
            };
            settings.formatter.golangci-lint = {
              command = "${golangciFmt}/bin/golangci-lint-fmt";
              includes = [ "*.go" ];
            };
          };

          checks.nix-lint =
            pkgs.runCommandLocal "check-nix-lint"
              {
                nativeBuildInputs = [
                  statix
                  deadnix
                ];
              }
              ''
                cd ${./.}
                statix check .
                deadnix --fail .
                touch $out
              '';

          devshells.default = {
            env = [
              {
                name = "CGO_ENABLED";
                value = 0;
              }
              # The default, `auto`, downloads whatever toolchain go.mod asks
              # for and would hide a mismatch with nix/go.nix.
              {
                name = "GOTOOLCHAIN";
                value = "local";
              }
            ];
            packages = with pkgs; [
              # The let-bound pin from nix/go.nix, not `pkgs.go`.
              go
              gopls
              go-task
              nixfmt
              statix
              deadnix
              config.treefmt.build.wrapper
              jq
            ];
          };
        };
    };
}
