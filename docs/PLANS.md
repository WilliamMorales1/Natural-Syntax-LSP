# Open work

Only unfinished items are listed. Shipped behavior and measurements live in `docs/models.md`, `docs/settings.md` and `docs/adding-a-language.md`.

## Order of work

After finishing each item: run `go test ./...`, commit it, then start the next one without waiting. Drop the item from this file in the same commit.

Blocked-looking items are not blocked: Windows is checked by the GitHub Actions workflow (`.github/workflows/ci.yml`: `setup.ps1`, language scripts, models, tokenizer, server smoke test; green on the `multilingual` branch), heavy exports run on Colab, VS Code is installed.

No open items.
