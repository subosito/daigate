{ pkgs, ... }: {
  languages.go.enable = true;
  packages = [ pkgs.just pkgs.openssl ];
}
