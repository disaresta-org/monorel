```yaml
# bitbucket-pipelines.yml
image: ghcr.io/disaresta-org/monorel:1.0.0

pipelines:
  branches:
    main:
      - step:
          name: monorel
          script:
            - git config user.name  "monorel-bot[automation]"
            - git config user.email "monorel-bot@users.noreply.example.com"
            - monorel auto
```
