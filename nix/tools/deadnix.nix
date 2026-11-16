{ lib, deadnix }:
# deadnix has no config file to name its version, so it is declared here.
let
  declared = "1.3.2";
in
lib.throwIf (deadnix.version != declared)
  "nix/tools/deadnix.nix declares deadnix ${declared}, but the locked nixpkgs ships ${deadnix.version}: write ${deadnix.version} there (or update the flake lock)"
  deadnix
