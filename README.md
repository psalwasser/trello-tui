# trello-tui

A fast, keyboard-driven terminal interface for Trello. It's meant to replace the web app for day-to-day board work.

```
 Product Roadmap                                                          1 ›

  Backlog                       5   Up Next                       2   In Progress
┏━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━┓ ╭───────────────────────────────╮ ╭──────────────
┃  Feature   Design             ┃ │  Feature   Urgent             │ │  Feature   De
┃ Dark mode for the mobile app  ┃ │ SSO with Okta                 │ │ New billing p
┃ ≡                             ┃ │ ◷ Fri 5pm ≡ ☑ 1/4          DU │ │ ◷ Tomorrow 5p
┗━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━┛ ╰───────────────────────────────╯ ╰──────────────
```

## Why it feels fast

- **Instant startup.** It reopens your last board straight from a local cache (`~/.cache/trello-tui`) and refreshes in the background.
- **Optimistic edits.** Moves, renames, labels, checklist ticks and comments show up immediately. The API calls happen in the background.
- **Ordered writes.** Mutations go through a FIFO queue, so dragging a card five slots with `JJJJJ` lands in the right place. If a call fails, the board resyncs from the server.
- **One request per board.** Lists, cards, labels and members all come back in a single API call.
- **Auto-refresh** every 45s while you're idle, so you see your teammates' changes.

## Install

Requires Go 1.25+ (this repo pins it via `mise.toml`).

```sh
go build -o trello-tui . && mv trello-tui ~/.local/bin/
# or: mise exec -- go build -o trello-tui .
```

Try it without an account:

```sh
trello-tui --demo
```

## Setup

On first launch, `trello-tui` walks you through authentication (re-run it any time with `trello-tui auth`):

1. Create a Power-Up at <https://trello.com/power-ups/admin> (any name), open its **API key** tab and generate a key.
2. Paste the key. The tool opens Trello's authorize page, where you approve access and copy the token.
3. Paste the token. It is checked against the API and then saved to `~/.config/trello-tui/config.json` (mode 0600).

You can also set `TRELLO_API_KEY` and `TRELLO_TOKEN` in the environment.

## Keys

Press `?` in the app for the full list.

| Board | |
|---|---|
| `h j k l` / arrows | navigate lists and cards |
| `enter` | open card |
| `n` / `O` | new card at bottom / top (stays open for rapid entry) |
| `H J K L` | move card between lists / within a list |
| `M` | move card to any list |
| `e` / `E` | rename / edit description in `$EDITOR` |
| `t` / `m` / `space` | labels / members / join or leave |
| `D` | mark done / not done |
| `d` | due date (`fri 3pm`, `+3d`, `10/15`, `tomorrow`…) |
| `x` | archive card |
| `N` / `R` / `X` | new list / rename list / archive list |
| `/` | filter: text, `#label`, `@member`, `due`, `overdue` |
| `z` | compact view |
| `o` / `y` | open in browser / copy link |
| `r` / `b` / `q` | refresh / switch board / quit |

| Card | |
|---|---|
| `j k` + `space` | select and toggle checklist items |
| `a` / `N` / `X` | add item / new checklist / delete item |
| `c` / `C` | comment inline / in `$EDITOR` |
| `D` / `A` | mark done / archive |
| `H L` | move card to previous/next list |
| `esc` | back |

## Development

```sh
go test ./...          # unit tests: API client (fake server), date parsing, UI flows on the demo backend
go run . --demo        # run against in-memory sample data
```

Layout:

- `internal/trello`: REST client, types, and the `Backend` interface
- `internal/demo`: in-memory backend used by `--demo` and the tests
- `internal/config`: credentials, UI state and the on-disk cache
- `internal/ui`: Bubble Tea app (boards picker, board, card detail, modals)
