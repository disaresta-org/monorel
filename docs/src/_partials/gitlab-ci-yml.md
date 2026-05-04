```yaml
default:
  image: ghcr.io/disaresta-org/monorel:1.0.0

monorel:
  rules:
    - if: $CI_COMMIT_BRANCH == $CI_DEFAULT_BRANCH
  variables:
    GITLAB_TOKEN: $MONOREL_GITLAB_TOKEN
  script:
    - git config user.name  "monorel-bot[automation]"
    - git config user.email "monorel-bot@users.noreply.example.com"
    - monorel auto
```
