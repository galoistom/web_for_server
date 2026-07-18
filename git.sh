#!/bin/bash
git add .

msg="${1:-"Auto-commit: $(date +'%Y-%m-%d %H:%M:%S')"}"

git commit -m "$msg" && git push
