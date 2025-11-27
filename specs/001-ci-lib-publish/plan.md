# Plan: CI/CD for Shared Library Publishing

## Goal
Implement a GitHub Actions workflow to automatically publish the `@alkem-io/matrix-adapter-go-lib` package to GitHub Packages on tags and relevant PRs.

## Phases

### Phase 1: Workflow Setup
- Create `.github/workflows/publish-lib.yml`.
- Configure permissions for `GITHUB_TOKEN` to write packages.
- Setup `pnpm` and Node.js environment.

### Phase 2: Versioning Logic
- **Tags**: Extract version from git tag (e.g., `v1.0.0` -> `1.0.0`).
- **PRs**: Generate version string `0.0.0-pr-{PR_NUMBER}-{SHORT_SHA}`.
- **Manual**: Generate version string `0.0.0-manual-{SHORT_SHA}`.
- Use `npm version` or `pnpm version` to update `package.json` before publishing.

### Phase 3: Change Detection (PRs)
- For PR triggers, ensure we only publish if `lib/` content has effectively changed.
- *Optimization*: The workflow trigger `paths: ['lib/**', 'pkg/dto/**']` handles the coarse grain.
- We can also do a finer check if needed, but the path trigger is likely sufficient for the MVP.

### Phase 4: Publishing
- Authenticate with GitHub Packages registry.
- Run `pnpm publish --no-git-checks`.
- Use `--tag latest` for tags and `--tag canary` for PRs.

## Risks
- **Version Conflicts**: PR versions must be unique. Including SHA ensures this.
- **Authentication**: Ensure `GITHUB_TOKEN` has `packages: write` permission.
- **Registry URL**: Need to ensure `.npmrc` or `package.json` points to GitHub Packages.

## Exit Criteria
- Pushing a tag `v0.0.1-test` triggers a publish of version `0.0.1-test`.
- Opening a PR with changes to `pkg/dto` triggers a publish of a canary version.
- Manually triggering the workflow publishes a manual snapshot version.
- No publish happens on PRs that don't touch relevant files.
