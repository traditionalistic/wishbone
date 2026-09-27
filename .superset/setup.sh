#!/bin/sh
# Prepare a fresh Superset workspace: pull in untracked local files, fetch deps.
set -eu

root="${SUPERSET_ROOT_PATH:-}"

# Untracked/ignored files that never come with the worktree. Extend this list.
for f in .env .env.local backend/.env .superset/config.local.json; do
	if [ -n "$root" ] && [ -f "$root/$f" ] && [ ! -e "$f" ]; then
		mkdir -p "$(dirname "$f")"
		cp "$root/$f" "$f"
		echo "copied $f from root"
	fi
done

# Deps, once the dirs stop being placeholders.
[ -f backend/go.mod ] && (cd backend && go mod download)
[ -f android/gradlew ] && ./android/gradlew -p android --quiet help >/dev/null
[ -f ios/Tuist/Config.swift ] && (cd ios && tuist install && tuist generate --no-open)

exit 0
