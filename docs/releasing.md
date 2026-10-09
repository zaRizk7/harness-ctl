# Maintainer publishing

The public destination is [zaRizk7/harness-ctl](https://github.com/zaRizk7/harness-ctl).
The repository and Wiki may be published. No first binary release version has
been selected. Source builds are available immediately. Do not describe an
unpublished draft or CI artifact as an installed, signed or notarized release.
(User, 2026; Local workspace, 2026)

## Binary releases

1. **A1 Verify.** Review the [request ledger](requests.md) and [limitations](limitations.md).
   Run `make check`, `make coverage` and all hooks using the declared Go version.
   Choose a version after review, then create and push its `vMAJOR.MINOR.PATCH`
   tag using the normal hooks.
2. **A2 Draft.** The tag workflow reuses CI, verifies both macOS architectures,
   builds versioned binaries and creates a draft with `SHA256SUMS`. Action
   dependencies are pinned to commit IDs. Only the draft job has repository
   write permission.
3. **A3 Publish.** Review the draft's artifacts and checksums, document concrete
   changes and limitations, and smoke-test installation on disposable machines.
   Publish the draft only after the release is approved. Binary signing and
   notarization are not configured.

(Local workspace, 2026)

Published asset names are `harness-ctl-darwin-arm64` and
`harness-ctl-darwin-amd64`. A release checksum verifies bytes against its metadata.
It does not authenticate a compromised publisher account. The bootstrap
requires an explicit HTTPS asset URL and trusted SHA-256. Pin both the installer
source and the release rather than silently following `latest`. (Local workspace, 2026)

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
