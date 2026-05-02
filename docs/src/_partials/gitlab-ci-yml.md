```yaml
# .gitlab-ci.yml
default:
  image: ghcr.io/disaresta-org/monorel:0.6.0

stages:
  - release-pr
  - release

# Maintain the always-open release MR. Fires on every push to the
# default branch except the chore(release): merge commit.
release-pr:
  stage: release-pr
  rules:
    - if: $CI_COMMIT_BRANCH == $CI_DEFAULT_BRANCH && $CI_COMMIT_TITLE !~ /^chore\(release\):/
  variables:
    GITLAB_TOKEN: $MONOREL_GITLAB_TOKEN
  script:
    - git config user.name  "monorel-bot[automation]"
    - git config user.email "monorel-bot@users.noreply.example.com"
    - git fetch origin "$CI_DEFAULT_BRANCH"
    - git checkout -B monorel/release "origin/$CI_DEFAULT_BRANCH"
    - apply_out=$(monorel apply); echo "$apply_out"
    - |
      if echo "$apply_out" | grep -qF "Nothing to apply."; then exit 0; fi
    - git push -f "https://oauth2:${MONOREL_GITLAB_TOKEN}@${CI_SERVER_HOST}/${CI_PROJECT_PATH}.git" monorel/release
    - monorel preview --upsert

# Cut the release after the always-open MR is merged.
release:
  stage: release
  rules:
    - if: $CI_COMMIT_BRANCH == $CI_DEFAULT_BRANCH && $CI_COMMIT_TITLE =~ /^chore\(release\):/
  variables:
    GITLAB_TOKEN: $MONOREL_GITLAB_TOKEN
  script:
    - git config user.name  "monorel-bot[automation]"
    - git config user.email "monorel-bot@users.noreply.example.com"
    - monorel tag
    - git push --follow-tags "https://oauth2:${MONOREL_GITLAB_TOKEN}@${CI_SERVER_HOST}/${CI_PROJECT_PATH}.git"
    - monorel publish
```
