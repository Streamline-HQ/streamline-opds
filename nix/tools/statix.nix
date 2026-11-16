{ lib, statix }:
# statix has no tagged release since 0.5.8: nixpkgs builds a snapshot of the
# molybdenumsoftware fork, which the original's README now points to. The
# snapshot's version is what is declared.
let
  declared = "0.5.8-unstable-2026-07-17";
in
lib.throwIf (statix.version != declared)
  "nix/tools/statix.nix declares statix ${declared}, but the locked nixpkgs ships ${statix.version}: write ${statix.version} there (or update the flake lock)"
  statix
