#!/usr/bin/env bash
# Checks that every copy of the model slug rule agrees with data/slug_cases.tsv.
set -euo pipefail
cd "$(dirname "$0")/.."
fail=0
check() { # impl name want got
  if [[ "$4" != "$3" ]]; then echo "$1: slug($2) = '$4', want '$3'"; fail=1; fi
}

eval "$(grep '^slugify()' scripts/setup.sh)"
js_fn="$(sed -n '/^function slugify/,/^}/p' vscode-extension/extension.js)"
py_fn="$(sed -n '/^def slugify/,/^$/p' scripts/export_model.py)"

while IFS=$'\t' read -r name want; do
  check setup.sh "$name" "$want" "$(slugify "$name")"
  check extension.js "$name" "$want" "$(node -e "$js_fn; process.stdout.write(slugify(process.argv[1]))" "$name")"
  check export_model.py "$name" "$want" "$(python3 -c "$py_fn
import sys; sys.stdout.write(slugify(sys.argv[1]))" "$name")"
done < data/slug_cases.tsv
exit $fail
