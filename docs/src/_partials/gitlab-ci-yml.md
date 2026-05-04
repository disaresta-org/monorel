```yaml
default:
  # entrypoint: [""] override is required: the published image's
  # entrypoint is `monorel` (entrypoint-binary container), and
  # GitLab's docker+machine executor wraps every script as
  # `sh -c '...'`. Without the override the runner invokes
  # `monorel sh -c '...'` and fails with `unknown command "sh"`.
  image:
    name: ghcr.io/disaresta-org/monorel:1.0.0
    entrypoint: [""]

monorel:
  rules:
    - if: $CI_COMMIT_BRANCH == $CI_DEFAULT_BRANCH
  variables:
    GITLAB_TOKEN: $MONOREL_GITLAB_TOKEN
  script:
    - git config user.name  "monorel-bot[automation]"
    - git config user.email "monorel-bot@users.noreply.example.com"
    # The runner's auto-cloned remote uses CI_JOB_TOKEN, which is
    # read-only by default. Replace it with MONOREL_GITLAB_TOKEN so
    # `monorel auto`'s git push to monorel/release can write.
    - git remote set-url origin "https://oauth2:${MONOREL_GITLAB_TOKEN}@${CI_SERVER_HOST}/${CI_PROJECT_PATH}.git"
    - monorel auto
```
