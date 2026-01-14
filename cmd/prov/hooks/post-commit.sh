#!/bin/sh
# AI Provenance post-commit hook
# Automatically correlates commits with AI prompts

# Get the full path to prov binary (injected at install time)
PROV_PATH="{{PROV_PATH}}"

# Get commit information
COMMIT_SHA=$(git rev-parse HEAD)
REPO_PATH=$(git rev-parse --show-toplevel)

# Call prov to correlate this commit with prompts
"$PROV_PATH" correlate-commit "$COMMIT_SHA" "$REPO_PATH" 2>/dev/null || true

# Always exit 0 so commit isn't blocked if correlation fails
exit 0
