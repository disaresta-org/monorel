```yaml
# bitbucket-pipelines.yml
image: ghcr.io/disaresta-org/monorel:0.13.0

pipelines:
  branches:
    main:
      - step:
          name: monorel
          script:
            - export GIT_AUTHOR_NAME="monorel-bot[automation]"
            - export GIT_AUTHOR_EMAIL="monorel-bot@users.noreply.example.com"
            - export GIT_COMMITTER_NAME="$GIT_AUTHOR_NAME"
            - export GIT_COMMITTER_EMAIL="$GIT_AUTHOR_EMAIL"
            - if echo "$BITBUCKET_COMMIT_MESSAGE" | grep -q "^chore(release):"; then
                monorel tag &&
                git push --follow-tags "https://${BB_USER}:${BITBUCKET_TOKEN}@bitbucket.org/${BITBUCKET_REPO_FULL_NAME}.git" &&
                monorel publish;
              else
                git fetch origin "$BITBUCKET_BRANCH" &&
                git checkout -B monorel/release "origin/$BITBUCKET_BRANCH" &&
                apply_out=$(monorel apply); echo "$apply_out";
                if ! echo "$apply_out" | grep -qF "Nothing to apply."; then
                  git push -f "https://${BB_USER}:${BITBUCKET_TOKEN}@bitbucket.org/${BITBUCKET_REPO_FULL_NAME}.git" monorel/release &&
                  monorel preview --upsert;
                fi;
              fi
```
