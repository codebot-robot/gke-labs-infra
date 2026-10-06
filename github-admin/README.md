# github-admin

`github-admin` manages the GitHub configuration of gke-labs repositories as
code. The desired state of each repository lives in a YAML file under
[`repos/`](repos/), shared rulesets live under [`rulesets/`](rulesets/), and
`github-admin apply` converges GitHub to match.

## Layout

```
github-admin/
  rulesets/            shared default rulesets, one per file
    require-pr-reviews.yaml
    merge-queue.yaml
  repos/gke-labs/      one file per public repository: the source of truth
    <repo>.yaml
```

## Repository config

Every gke-labs repository should require pull request reviews on its default
branch. The minimal config references the shared ruleset by name:

```yaml
owner: gke-labs
name: my-repo
defaultRulesets:
  - require-pr-reviews
```

- `defaultRulesets` lists rulesets from `rulesets/` that are applied verbatim.
  A default ruleset's file name must match its `name`.
- `customRulesets` holds rulesets specific to one repository, written out in
  full. Names must not collide with a referenced default.
- `settings` holds the repository settings that differ from GitHub's defaults
  (issues, wiki and projects on; squash, merge and rebase allowed; auto-merge
  and delete-branch-on-merge off). An omitted setting means its default, and
  `apply` enforces it, so a repository whose wiki was switched off in the UI
  shows up in the plan until `hasWiki: false` is added to its file.
- `description`, `homepage` and `topics` are managed only when present.

Rulesets are matched by name on GitHub and updated in place, so applying is
idempotent. Rules from all active rulesets combine on a branch, which is what
makes the shared/custom split work.

### Default rulesets

- `require-pr-reviews`: pull request with one approving review, no deletion,
  no force push. Repository admins can bypass.
- `merge-queue`: pull requests land through the merge queue. It carries no
  status checks, because those differ per repository.

### Merge queue repositories

A repository using the merge queue adds the `merge-queue` default, enables
auto-merge, and declares its status checks in a custom ruleset:

```yaml
owner: gke-labs
name: my-repo
settings:
  allowAutoMerge: true
defaultRulesets:
  - require-pr-reviews
  - merge-queue
customRulesets:
  - name: required-status-checks
    target: branch
    enforcement: active
    conditions:
      refName:
        include:
          - "~DEFAULT_BRANCH"
    rules:
      requiredStatusChecks:
        contexts:
          - ap-test
          - ap-verify-generate
```

### Deviating from a default

To change one setting for one repository, copy the default into
`customRulesets` under the same name and edit it. The live ruleset is updated
in place, and `export` will show it as custom until it matches a default again.

Rulesets are used rather than legacy branch protection rules because they
layer: a repository can carry the shared rulesets alongside rulesets owned by
the organization or enterprise.

## Private repositories

Only public repositories are managed here. The configuration of a private
repository (its name, settings and rules) would be published by committing it,
so both `apply` and `export` skip private repositories unless
`--include-private` is passed. Do not add private repositories to `repos/`.

## Applying

`apply` works like a plan-and-apply tool. For each repository it fetches the
current state, prints only what differs, and (unless `--dry-run`, the default)
changes only that. Unchanged repositories print `no changes`.

```
gke-labs/kube-etl
  ~ settings.allowAutoMerge: false => true
  ~ ruleset "merge-queue"
        conditions:
          refName:
            include:
            - ~DEFAULT_BRANCH
        enforcement: active
        name: merge-queue
        rules:
          deletion: true
          mergeQueue:
      -     minEntriesToMergeWaitMinutes: 1
      +     minEntriesToMergeWaitMinutes: 5
  + ruleset "required-status-checks"
      + conditions:
      ...

Plan: 1 to add, 1 to change. Dry run; re-run with --dry-run=false to apply.
```

```shell
export GITHUB_TOKEN=$(gh auth token)

# Preview
go run ./github-admin apply --config github-admin/repos/gke-labs

# Apply everything
go run ./github-admin apply --config github-admin/repos/gke-labs --dry-run=false

# Apply a single repository
go run ./github-admin apply --config github-admin/repos/gke-labs/my-repo.yaml --dry-run=false
```

`--config` accepts either a single (possibly multi-document) YAML file or a
directory, in which case every `.yaml`/`.yml` file beneath it is loaded.
`--default-rulesets` points at the shared rulesets directory and defaults to
`github-admin/rulesets`, so run from the repository root.

## Adding a repository

1. Create `repos/gke-labs/<name>.yaml`. The house config for a new gke-labs
   repository is:

   ```yaml
   owner: gke-labs
   name: <name>
   settings:
     hasWiki: false
   defaultRulesets:
     - require-pr-reviews
   ```

   For an existing repository, run `export` for it instead (see below) and
   then add the defaults you want.
2. Run `apply` for that file and check the plan.
3. Send the new file as a pull request so the checked-in state stays accurate.

Settings defaults follow GitHub's, not gke-labs convention, so that a file
describes exactly how a repository differs from a freshly created one.

## Importing an existing repository

`export` reads a repository's live configuration and writes it in the same
form `repos/` uses, omitting anything that is a default. Use it to bootstrap
the file for a repository, or to see how a repository has drifted from its
file (export to a scratch path and diff).

```shell
GITHUB_TOKEN=$(gh auth token) \
  go run ./github-admin export --owner gke-labs --repo my-repo \
    --output 'github-admin/repos/{org}/{repo}.yaml'
```

Live rulesets that exactly match a default are written by name under
`defaultRulesets`; anything else is written in full under `customRulesets`.
Exporting overwrites the file, including any comments, so re-add notes
afterwards. Organization and enterprise rulesets are skipped, as they are not
managed here.
