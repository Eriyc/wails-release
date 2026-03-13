{
  description = "my-product";

  inputs = {
    nixpkgs.url = "github:nixos/nixpkgs?ref=nixos-unstable";
    repo-lib.url = "git+https://git.dgren.dev/eric/nix-flake-lib?ref=v3.0.0";
    repo-lib.inputs.nixpkgs.follows = "nixpkgs";
  };

  outputs =
    {
      self,
      nixpkgs,
      repo-lib,
      ...
    }:
    repo-lib.lib.mkRepo {
      inherit self nixpkgs;
      src = ./.;
      systems = repo-lib.lib.systems.default;

      config = {
        shell = {
          env = {
          };
          extraShellText = ''
            export BUN_INSTALL="''${BUN_INSTALL:-$HOME/.bun}"
            export PATH="$BUN_INSTALL/bin:$PATH"
          '';
        };

        formatting = {
          programs = {
            shfmt.enable = true;
          };

          settings = {
            shfmt.options = [
              "-i"
              "2"
              "-s"
              "-w"
            ];
          };
        };

        release = {
          steps = [ ];
        };
      };

      perSystem =
        {
          pkgs,
          system,
          ...
        }:
        let

          wails3 = pkgs.buildGoModule {
            pname = "wails3";
            version = "3.0.0-alpha.74";
            src = pkgs.fetchFromGitHub {
              owner = "wailsapp";
              repo = "wails";
              rev = "v3.0.0-alpha.74";
              hash = "sha256-7cRtJdv7UXi8JEJDC9f6WHrVhU7nDVk+dBUhRenZBc4=";
            };
            modRoot = "v3";
            subPackages = [ "cmd/wails3" ];
            vendorHash = "sha256-WdragX08M/Fmw/IB6Atw27b1PPrQPNo2i19ykQLo8O0=";
            go = pkgs.go_1_26;
            meta.mainProgram = "wails3";
          };
        in
        {
          tools = [
            (repo-lib.lib.tools.fromPackage {
              name = "Bun";
              package = pkgs.bun;
              version.args = [ "--version" ];
              banner.color = "YELLOW";
            })
            (repo-lib.lib.tools.fromPackage {
              name = "Go";
              package = pkgs.go_1_26;
              version.args = [ "version" ];
              banner.color = "CYAN";
            })
            (repo-lib.lib.tools.fromPackage {
              name = "Wails";
              package = wails3;
              version.args = [ "version" ];
              banner.color = "MAGENTA";
            })
          ];

          shell = {
            packages = [
              self.packages.${system}.release
              pkgs.go_1_26
              pkgs.gopls
              pkgs.gotools
              pkgs.bazel-buildtools
              pkgs.bazel-watcher
              pkgs.oxfmt
              pkgs.oxlint
            ];
            extraShellText = "";
          };

          packages.wails3 = wails3;
        };
    };
}
