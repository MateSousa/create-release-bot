# Create Release Bot GitHub Action

The Create Release Bot GitHub Action is used to manage releases for your repository. This action automates the process of creating a release, updating the changelog, merging pull requests, and more.

## Advice

The best way to use this action for now is combine with [Create Release Action](https://github.com/MateSousa/create-release), create release action will create a PR for release and this action will update the PR, merge and create a release.

## Inputs

The following inputs are required for the GitHub Action:

- `repo_owner`: The owner of the repository.
- `repo_name`: The name of the repository.
- `base_branch`: The base branch to create the pull request against.
- `target_branch`: The target branch for the pull request.
- `github_token`: GitHub token with repo access.
- `github_event`: GitHub event (default: `github.event`).
## Usage

To use the Create Release Bot and Create Release, add the following code to your workflow file:

```yaml
name: Create Release

on:
  push:
    branches:
      - master
  pull_request:
    types: [closed]
  issue_comment:
    types: [created]
jobs:
  release:
    name: Release
    runs-on: ubuntu-latest
    permissions:
      contents: write
      pull-requests: write
    steps:
      - name: Checkout code
        uses: actions/checkout@v2
      
      - name: Create Release PR
        if: github.event_name == 'push'
        uses: MateSousa/create-release@v0.0.1 # v0.0.1 is the latest version
        with:
          repo_owner: ${{ secrets.REPO_OWNER }}}
          repo_name: ${{ secrets.REPO_NAME }}}
          base_branch: ${{ secrets.BASE_BRANCH }}}
          target_branch: ${{ secrets.TARGET_BRANCH }}}
          github_token: ${{ secrets.GITHUB_TOKEN }}
      
      - name: Bot running
        if: github.event_name == 'pull_request' || github.event_name == 'issue_comment'
        uses: MateSousa/create-release-bot@v0.0.1 # v0.0.1 is the latest version
        with:
          repo_owner: ${{ secrets.REPO_OWNER }}}
          repo_name: ${{ secrets.REPO_NAME }}}
          base_branch: ${{ secrets.BASE_BRANCH }}}
          target_branch: ${{ secrets.TARGET_BRANCH }}}
          github_token: ${{ secrets.GITHUB_TOKEN }}
          github_event: ${{ toJson(github.event) }}
```

You will also need to habilitate in Action tab the following options:
    - Allow github actions to create and approve pull requests

## Commands

The following commands are available for the Create Release Bot GitHub Action:

- `/merge`: When you put this command in a comment, the action will create or update the base branch with a changelog.md file and merge the pull request into the target branch, creating a new tag and release.