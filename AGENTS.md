# Repository Instructions

## GitHub Releases

- Whenever the user asks to push changes to GitHub, create a new GitHub Release after the push succeeds.
- Use the next semantic version unless the user requests a specific version. Update all version metadata and `CHANGELOG.md` before committing.
- Create a new annotated tag and push both the branch and the tag. Never reuse or move an existing release tag.
- Publish a non-draft release with concise release notes and attach the current build artifacts from `dist/`, including `SHA256SUMS.txt`.
- Verify that the remote branch, tag, release, and uploaded assets all point to the same commit.
