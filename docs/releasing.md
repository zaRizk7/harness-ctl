# Maintainer publishing

The public destination is [zaRizk7/harness-ctl](https://github.com/zaRizk7/harness-ctl).
The repository, Wiki and functional binary bootstrap may be published. The first
public binary release was v0.1.0. The current patch release is v0.1.1. Both
architecture CI jobs and the actual public Apple Silicon installer smoke passed.
See the [checkpoint](implementation.md)
for exact proof. Signing/notarization remain unconfigured. Do not describe an
unpublished draft or CI artifact as an installed, signed or notarized release.
(User, 2026; Local workspace, 2026)

## Binary releases

1. **A1 Verify.** Review the [request ledger](requests.md) and [limitations](limitations.md).
   Run `make check`, `make coverage` and all hooks using the declared Go version.
   Choose a version after review, then create and push its `vMAJOR.MINOR.PATCH`
   tag using the normal hooks.
2. **A2 Draft.** The tag workflow reuses CI, verifies both macOS architectures,
   builds versioned binaries and creates a draft with `install.sh` and
   `SHA256SUMS`. The installer is fetched from the same release tag. Action
   dependencies are pinned to commit IDs. Only the draft job has repository
   write permission.
3. **A3 Publish.** Review the draft's artifacts and checksums, document concrete
   changes and limitations, and smoke-test installation on disposable machines.
   The user authorized publication after verification. Binary signing and
   notarization are not configured.

(Local workspace, 2026)

Published assets are `harness-ctl-darwin-arm64`, `harness-ctl-darwin-amd64`,
`install.sh` and `SHA256SUMS`. The bootstrap resolves the architecture and checksum
from the selected release, then verifies the binary before executing setup.
Explicit HTTPS asset URLs and trusted SHA-256 overrides remain available. A
release checksum does not authenticate a compromised publisher account. Pin both
the installer source and binary release for repeatability. (Local workspace, 2026)

## Wiki sources

Operator tutorials live in `docs/wiki/`. Publish these reviewed Markdown sources
to the repository's separate Wiki Git repository. Initialize the first page
through GitHub if cloning a fresh Wiki fails. (GitHub, 2026)

```sh
git clone https://github.com/zaRizk7/harness-ctl.wiki.git ../harness-ctl-wiki
cp docs/wiki/*.md ../harness-ctl-wiki/
git -C ../harness-ctl-wiki add -- '*.md'
git -C ../harness-ctl-wiki diff --cached --check
git -C ../harness-ctl-wiki diff --cached
git -C ../harness-ctl-wiki commit -m 'docs: publish reviewed operator tutorials'
git -C ../harness-ctl-wiki push
```

Use normal fast-forward pushes and preserve Wiki history. References use
absolute repository URLs so they remain valid after publication. Keep the source
and published Wiki aligned after every tutorial change. (Project policy, 2026)

## References

- User (2026). Public repository and Wiki authorization in the implementation conversation.
- Local workspace (2026). [CI](../.github/workflows/ci.yml),
  [draft release workflow](../.github/workflows/release.yml), [bootstrap](../scripts/install.sh).
- GitHub (2026). [Adding or editing Wiki pages](https://docs.github.com/en/communities/documenting-your-project-with-wikis/adding-or-editing-wiki-pages).
