# estui — Everything Search TUI

## Overview

A terminal UI wrapper around [Everything Search](https://www.voidtools.com/) (`es` CLI) that provides fast, interactive file search with a polished interface.

## Background

Windows' built-in file search is slow because it traverses the directory tree recursively at query time. **Everything** by voidtools solves this by:

- Reading the NTFS Master File Table (MFT) at startup to build a complete file index in RAM
- Subscribing to the USN Change Journal for real-time incremental updates
- Performing in-memory string matching at query time — results in under a millisecond

The `es` CLI exposes this same speed programmatically. This project wraps it in an interactive TUI.

## Tech Stack

| Layer | Tool |
|---|---|
| Language | Go |
| TUI framework | [Bubble Tea](https://github.com/charmbracelet/bubbletea) (Charm) |
| Components | [Bubbles](https://github.com/charmbracelet/bubbles) — list, text input, paginator, spinner |
| Styling | [Lip Gloss](https://github.com/charmbracelet/lipgloss) |
| Search backend | `es` (Everything Search CLI) |

**Why Go?**
- Compiles to a single self-contained native binary
- GC-managed memory safety — no manual allocation bugs
- Tiny syntax, excellent toolchain
- Easy cross-compilation

**Why Bubble Tea?**
- Elm Architecture (Model / Update / View) — clean and learnable
- Pre-built `bubbles/list` component maps directly onto search results
- Animations, pagination, and keyboard nav out of the box
- The Charm ecosystem produces genuinely polished terminal UIs

## Planned Features

- Live search as you type, querying `es` on each keystroke
- Paginated, scrollable results list
- Keyboard navigation (arrows, page up/down, enter to act on selection)
- File metadata display (size, modified date, path)
- Copy path to clipboard on selection
- Open file/folder in Explorer on enter

## Architecture

```
TextInput (search query)
    │
    ▼
os/exec → es <query>        (subprocess call)
    │
    ▼
[]string (parsed stdout)
    │
    ▼
bubbles/list component      (paginated, filterable)
    │
    ▼
Lip Gloss styled viewport
```

The core loop follows Bubble Tea's Elm Architecture:

- **Model** — search query, results slice, list state, loading flag
- **Update** — handles keypresses, dispatches `es` subprocess as a `tea.Cmd`
- **View** — renders search box + results list using Lip Gloss
