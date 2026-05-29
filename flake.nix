{
  description = "Himitsu Bako: encrypted clipboard-backed secret storage";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, flake-utils }:
    let
      mkHimitsuBako = pkgs:
        let
          runtimeDeps =
            [ pkgs.fzf ]
            ++ pkgs.lib.optionals pkgs.stdenv.isLinux [
              pkgs.wl-clipboard
              pkgs.xclip
            ];
        in
        pkgs.buildGoModule {
          pname = "himitsu-bako";
          version = "1.1.0";

          src = self;

          vendorHash = "sha256-jwQvAIS8XCWjZtP9pKt1RRSaKBZ9dgDc2SD6q0K+sEs=";

          ldflags = [ "-s" "-w" ];

          nativeBuildInputs = [ pkgs.makeBinaryWrapper ];

          postInstall = ''
            wrapProgram $out/bin/himitsu-bako \
              --prefix PATH : ${pkgs.lib.makeBinPath runtimeDeps}
          '';

          meta = with pkgs.lib; {
            description = "Encrypted clipboard-backed secret storage using age";
            homepage = "https://github.com/shichirouji21/himitsu-bako";
            license = licenses.bsd2;
            mainProgram = "himitsu-bako";
            platforms = platforms.unix;
          };
        };
    in
    flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = nixpkgs.legacyPackages.${system};
        himitsu-bako = mkHimitsuBako pkgs;
        runtimeDeps =
          [ pkgs.fzf ]
          ++ pkgs.lib.optionals pkgs.stdenv.isLinux [
            pkgs.wl-clipboard
            pkgs.xclip
          ];
      in {
        packages.default = himitsu-bako;
        packages.himitsu-bako = himitsu-bako;

        apps.default = flake-utils.lib.mkApp { drv = himitsu-bako; };

        devShells.default = pkgs.mkShell {
          packages = [ pkgs.go pkgs.gopls ] ++ runtimeDeps;
        };
      }) // {
        overlays.default = final: _prev: {
          himitsu-bako = mkHimitsuBako final;
        };
      };
}
