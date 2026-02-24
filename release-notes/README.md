# Release Notes

Create a markdown file here before tagging each release. The file name must match the tag (e.g. `v0.10.0.md` for tag `v0.10.0`).

## Process

1. Create `release-notes/vX.Y.Z.md` with the features and changes for that version
2. Commit the file
3. Tag: `git tag -a vX.Y.Z -m "vX.Y.Z: brief description"`
4. Push: `git push origin main && git push origin vX.Y.Z`

The GitHub release will use the content of the markdown file as the release body.
