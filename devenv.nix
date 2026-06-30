{ pkgs, ... }: {
  languages.go.enable = true;

  enterShell = ''
    if [[ -f go.local.mod ]]; then
      export GOFLAGS="''${GOFLAGS:+$GOFLAGS }-modfile=$PWD/go.local.mod"
    fi
  '';

  packages = [ pkgs.just pkgs.openssl ];
}
