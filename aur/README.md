# AUR source package

`PKGBUILD` + `.SRCINFO` for the **`devspanner`** AUR package — it compiles from the
release source tarball on the user's machine. (The `devspanner-bin` binary package is
published automatically by GoReleaser; see [`../RELEASING.md`](../RELEASING.md).)

These files are **not** consumed by the build here — they live in the repo for
version control. The AUR wants its own git repo containing only `PKGBUILD` and
`.SRCINFO` at the root.

## Publishing (one-time)

1. Create an account at <https://aur.archlinux.org> and add your SSH public key
   (Account → My Account → SSH Public Key).
2. Clone the (empty) AUR repo — it's created on first push:
   ```bash
   git clone ssh://aur@aur.archlinux.org/devspanner.git aur-devspanner
   ```
3. Copy these two files in and verify the package builds:
   ```bash
   cp PKGBUILD .SRCINFO aur-devspanner/
   cd aur-devspanner
   updpkgsums                         # pin sha256 to the real tarball (needs the tag pushed)
   makepkg --printsrcinfo > .SRCINFO  # regenerate after any PKGBUILD edit
   makepkg -si                        # local build + install smoke test
   ```
4. Commit and push:
   ```bash
   git add PKGBUILD .SRCINFO
   git commit -m "Initial import: devspanner 0.1.0"
   git push
   ```

Users can then `yay -S devspanner`.

## On each new release

Bump `pkgver` (reset `pkgrel=1`), then:

```bash
updpkgsums
makepkg --printsrcinfo > .SRCINFO
git commit -am "Upgrade to <version>" && git push
```

Keep the copy here in sync so the repo stays the source of truth.
