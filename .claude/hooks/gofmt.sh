#!/bin/sh
# PostToolUse hook: gofmt the Go file Claude just edited.
if ! command -v jq >/dev/null 2>&1; then
	echo "gofmt hook: jq is not installed; skipping formatting" >&2
	exit 0
fi
file=$(jq -r '.tool_input.file_path // empty')

case "$file" in
*.go) [ -f "$file" ] && gofmt -w "$file" ;;
esac

exit 0
