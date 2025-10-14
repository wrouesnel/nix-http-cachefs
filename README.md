# Afero Filesystem for accessing Nix binary caches

This is an afero filesystem for accessing Nix binary caches which are served over
http.

It allows on-disk nix path like access to derivations stored on the remote filesystem.
It can be advisable to use this with the read through cache as the compressed nature
of the store means access can be quite slow.

This was developed to provide a lightweight way to do Nix derivation exploration.
It also obviously enables interesting use cases like mounting a Nix binary cache
as a FUSE filesystem.