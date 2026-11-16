{ lib, nixfmt }:
# nixfmt has no config file to name its version, so it is declared here. The
# devshell, the flake's fmt check and `nix fmt` (CI's lint-nix) all run this
# one; a flake lock that moves nixfmt fails here, not as reformatted files.
let
  declared = "1.5.0";
in
lib.throwIf (nixfmt.version != declared)
  "nix/tools/nixfmt.nix declares nixfmt ${declared}, but the locked nixpkgs ships ${nixfmt.version}: write ${nixfmt.version} there and run `task fmt` (or update the flake lock)"
  nixfmt
