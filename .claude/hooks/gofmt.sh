#!/bin/sh
# PostToolUse hook: gofmt the Go file Claude just edited.
file=$(jq -r '.tool_input.file_path // empty')

case "$file" in
*.go) [ -f "$file" ] && gofmt -w "$file" ;;
esac

exit 0
