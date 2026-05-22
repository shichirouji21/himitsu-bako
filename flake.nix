{
  description = "Himitsu Bako: encrypted clipboard-backed secret storage";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, flake-utils }:
    flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = nixpkgs.legacyPackages.${system};

        runtimeDeps =
          [ pkgs.fzf ]
          ++ pkgs.lib.optionals pkgs.stdenv.isLinux [
            pkgs.wl-clipboard
            pkgs.xclip
          ];

        himitsu-bako = pkgs.buildGoModule {
          pname = "himitsu-bako";
          version = "1.0.0";

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
      in {
        packages.default = himitsu-bako;
        packages.himitsu-bako = himitsu-bako;

        apps.default = flake-utils.lib.mkApp { drv = himitsu-bako; };

        devShells.default = pkgs.mkShell {
          packages = [ pkgs.go pkgs.gopls ] ++ runtimeDeps;
        };
      });
}
