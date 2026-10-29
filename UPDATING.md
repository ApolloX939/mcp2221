# Updating mcp2221

How to move to a newer release of what [INSTALL.md](INSTALL.md) set up: the
`claudematrix` program, the Claude Code plugins and the Go library. Each part
updates on its own, but a plugin and the program it runs are best kept
together, because a new plugin setting needs a `claudematrix` that understands
it.

## Contents

- [Update everything](#update-everything)
- [The claudematrix program](#the-claudematrix-program)
- [Claude Code plugins](#claude-code-plugins)
- [Settings after an update](#settings-after-an-update)
- [Go library](#go-library)
- [Linux permissions](#linux-permissions)
- [Troubleshooting](#troubleshooting)

## Update everything

```sh
go install github.com/ApolloX939/mcp2221/cmd/claudematrix@latest
claude plugin marketplace update mcp2221
claude plugin update led-matrix@mcp2221 --scope user
claude plugin update progress-pie@mcp2221 --scope user
```

Skip the plugin line you never installed, and use the `--scope` you installed
with. Then restart Claude Code, or run `/reload-plugins` in a running session.

## The claudematrix program

Installing again fetches the newest release and replaces the old binary:

```sh
go install github.com/ApolloX939/mcp2221/cmd/claudematrix@latest
```

If Go answers with an old version, its module proxy may still be caching the
last one. Ask for the exact release instead, such as `@v1.2.3`, or bypass the
proxy for one run:

```sh
GOFLAGS=-mod=mod GOPROXY=direct go install github.com/ApolloX939/mcp2221/cmd/claudematrix@latest
```

`claudematrix -h` lists the flags this build knows. A flag missing there means
the program is older than the plugin asking for it.

A running matrix keeps the old program until it stops. The next prompt in
Claude Code starts the new one.

## Claude Code plugins

Plugins come from the `mcp2221` marketplace, which Claude Code keeps a local
copy of. Refresh that copy, then update the plugins from it:

```sh
claude plugin marketplace update mcp2221
claude plugin update led-matrix@mcp2221 --scope user
claude plugin update progress-pie@mcp2221 --scope user
```

You can do the same inside Claude Code with `/plugin`: open the **Marketplaces**
tab to update `mcp2221`, and the **Installed** tab to update each plugin. Then
run `/reload-plugins`, or start a new session.

A plugin only counts as newer when its `version` in
`.claude-plugin/plugin.json` went up. If you update and nothing changes, the
release did not raise the version; reinstall to force it:

```sh
claude plugin uninstall led-matrix@mcp2221 --scope user
claude plugin install led-matrix@mcp2221 --scope user
```

Reinstalling keeps nothing that [Settings after an update](#settings-after-an-update)
does not cover, so check your settings afterwards.

## Settings after an update

Settings you saved with `/plugin configure` carry over. A release can add new
ones, and they start at their default, so nothing changes until you set them.
The full list is in [Settings](INSTALL.md#settings).

To review or change them:

```
/plugin configure led-matrix@mcp2221
/reload-plugins
```

Settings added to the led-matrix plugin so far, and what they do:

| Setting | Use |
| --- | --- |
| `rotate` | Turn the display by 0, 90, 180 or 270 degrees. |
| `mirror` | Flip the display left to right, if digits and their order come out backwards. |
| `clockX`, `clockY` | Shift the countdown. |

Find good values for all of them at once with `claudematrix -align`; see
[Aligning the display](INSTALL.md#aligning-the-display).

## Go library

Programs that import the library update with the Go tool:

```sh
go get github.com/ApolloX939/mcp2221@latest
go mod tidy
```

`@v1.2.3` picks an exact release. `go list -m -versions
github.com/ApolloX939/mcp2221` shows which exist.

A local checkout of this repo updates with `git pull`; build `claudematrix`
from it again afterwards with `go install ./cmd/claudematrix`.

## Linux permissions

The udev rules in [Linux permissions](INSTALL.md#linux-permissions) rarely
change. If a release says they did, replace the rule files, then run
`sudo udevadm control --reload-rules` and replug the adapter.

## Troubleshooting

- **The plugin did not change.** Run `claude plugin marketplace update mcp2221`
  first; without it `plugin update` compares against the old local copy. Then see
  the reinstall steps under [Claude Code plugins](#claude-code-plugins).
- **A new setting has no effect.** The program is older than the plugin. Run
  `go install` again and check `claudematrix -h` for the flag.
- **`command not found` after `go install`.** Go's install folder is not on
  `PATH`; the plugins look there themselves, so this only matters at the
  terminal. See [Quick start](INSTALL.md#quick-start-the-led-matrix-in-claude-code).
- **Matrix is blank after updating.** Restart Claude Code so the plugins reload,
  and make sure no old `claudematrix` still holds the adapter: `pkill claudematrix`.

To go back, install the earlier release the same way, with its version in place
of `@latest`.
