# Discord Username Checker

[![Go Report Card](https://goreportcard.com/badge/github.com/y0f/discord-user-checker)](https://goreportcard.com/report/github.com/y0f/discord-user-checker)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

A fast, multithreaded Discord username availability checker written in Go. Supports multiple tokens, automatic rate-limit handling, and bulk wordlist processing.

## Requirements

- Go 1.21 or higher

## Installation

```bash
git clone https://github.com/y0f/discord-user-checker.git
cd discord-user-checker
go build -o checker .
```

## Setup

1. Add your Discord token(s) to `tokens.txt` (one per line).
2. Add usernames to check in `listtocheck.txt` (one per line).
3. Set the check method in `config.json`:

```json
{
  "method": "me"
}
```

| Method    | Description                              |
|-----------|------------------------------------------|
| `me`      | Uses the `PATCH /users/@me` endpoint     |
| `friends` | Uses the `pomelo-attempt` endpoint       |

## Usage

```bash
# Check usernames (single thread)
./checker

# Check usernames with 4 threads
./checker -t 4

# Run the wordlist helper utility
./checker -helper
```

### Results

| File       | Contents                    |
|------------|-----------------------------|
| `good.txt` | Available usernames         |
| `bad.txt`  | Taken usernames             |

### Wordlist Helper

The built-in helper (`-helper`) provides three options for processing wordlists in the `wordlists/` directory:

1. **Split lines** -- Remove non-alphabetic characters and split into separate lines.
2. **Filter by length** -- Keep only words of a specific length.
3. **Save by length** -- Split a wordlist into separate files by word length (e.g. `4char.txt`, `5char.txt`).

## License

This project is licensed under the [MIT License](LICENSE).

## Disclaimer

This tool is for educational purposes only. Use it at your own risk.
