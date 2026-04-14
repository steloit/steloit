#!/bin/bash
# Post-edit hook: auto-format files after Claude edits them
# Triggered by Edit/Write tool calls via PostToolUse hook

case "$CLAUDE_FILE_PATH" in
  *.go)
    if command -v goimports &> /dev/null; then
      goimports -w "$CLAUDE_FILE_PATH" 2>/dev/null
    elif command -v gofmt &> /dev/null; then
      gofmt -w "$CLAUDE_FILE_PATH" 2>/dev/null
    fi
    ;;
  *.ts|*.tsx|*.js|*.jsx)
    if command -v prettier &> /dev/null; then
      prettier --write "$CLAUDE_FILE_PATH" 2>/dev/null
    elif command -v npx &> /dev/null; then
      npx prettier --write "$CLAUDE_FILE_PATH" 2>/dev/null
    fi
    ;;
  *.py)
    if command -v black &> /dev/null; then
      black --quiet "$CLAUDE_FILE_PATH" 2>/dev/null
    fi
    if command -v isort &> /dev/null; then
      isort --quiet "$CLAUDE_FILE_PATH" 2>/dev/null
    fi
    ;;
esac
