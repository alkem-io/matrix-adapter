# Spec 001: CI/CD for Shared Library Publishing

## Problem
The shared TypeScript library (`lib/`) is generated from Go DTOs and defines the contract between the Matrix Adapter and other services. Currently, there is no automated pipeline to publish this library. This leads to:
- Manual effort to publish updates.
- Risk of drift between the Go service and the TypeScript library.
- Difficulty in testing changes to the contract in consuming services before they are merged/released.

## Outcomes
1.  **Automated Release Publishing**: When a git tag is pushed (e.g., `v1.2.3`), the pipeline automatically builds and publishes the library with the corresponding version.
2.  **PR/Canary Publishing**: When a PR is open and changes the generated library code, the pipeline publishes a pre-release version (e.g., `0.0.0-pr-123.commitHash`) to allow testing in other services.
3.  **Manual Publishing**: Developers can manually trigger the workflow from the GitHub UI to publish a snapshot version (e.g., `0.0.0-manual-{SHORT_SHA}`) from any branch.
4.  **Change Detection**: The PR workflow only publishes if there are actual changes in the `lib/` directory or the source DTOs that generate it.

## Constraints
- **Registry**: GitHub Packages (npm).
- **Versioning**:
    - Tags: Use the git tag (stripped of `v` prefix if necessary).
    - PRs: Use a format like `0.0.0-pr-{PR_NUMBER}-{SHORT_SHA}` or similar to ensure uniqueness and ordering.
    - Manual: Use a format like `0.0.0-manual-{SHORT_SHA}`.
- **Tooling**: GitHub Actions, `pnpm` (as seen in `lib/`).
- **Secrets**: `GITHUB_TOKEN` should be sufficient for GitHub Packages.

## Open Questions
- Should we run `make generate` in the CI to ensure `lib/` is up to date, or fail if it's not?
    - *Decision*: CI should run generation. If it differs from committed code in a PR, it might be better to fail and ask user to commit changes, OR commit and push (but that's complex). For publishing, we assume the code in the repo is what we want to publish, but we should verify it matches the Go definitions.
    - *Refinement*: The user asked "if generated library differs from latest tag". This implies we might need to check if the content changed.
- How to handle version bumps in `package.json`?
    - For tags: The tag itself is the source of truth. We can override the version in `package.json` during the build.
    - For PRs: We generate a dynamic version.

## Plan Link
See `plan.md` (to be created).
