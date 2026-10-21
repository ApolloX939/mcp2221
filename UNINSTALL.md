# Uninstalling mcp2221

How to remove what [INSTALL.md](INSTALL.md) set up: the Claude Code plugins, the
marketplace, the `claudematrix` program, the Linux permissions and the Go
library. Each step is independent, so do only the ones you need.

## Contents

- [Claude Code plugins](#claude-code-plugins)
- [The claudematrix program](#the-claudematrix-program)
- [Leftover files](#leftover-files)
- [Linux permissions](#linux-permissions)
- [Go library](#go-library)

## Claude Code plugins

Remove the plugins, then the marketplace they came from:

```sh
claude plugin uninstall led-matrix@mcp2221 --scope user
claude plugin uninstall progress-pie@mcp2221 --scope user
claude plugin marketplace remove mcp2221
```

Skip the line for a plugin you never installed. Use the same `--scope` you
installed with. Then start a new Claude Code session, or run `/reload-plugins`
in a running one.

Settings saved with `/plugin configure` go with the plugin.

## The claudematrix program

`go install` put it in Go's install folder, `$GOBIN`, or else `$GOPATH/bin`,
or else `~/go/bin`. Delete it from there:

```sh
rm "$(go env GOBIN)/claudematrix" 2>/dev/null || rm "$(go env GOPATH)/bin/claudematrix"
```

If you installed it some other way, `command -v claudematrix` shows where it is.

Remove the plugins first. While they are installed, the matrix stops working
once the program is gone.

## Leftover files

The plugins write two small files under `~/.claude`. They are harmless, and can
be deleted:

```sh
rm -f ~/.claude/usage-limit ~/.claude/progress-pie
```

If you set `limitFile` or `progressFile` to another place, delete those files
instead.

## Linux permissions

Remove the udev rules you created, then reload:

```sh
sudo rm -f /etc/udev/rules.d/99-mcp2221.rules /etc/udev/rules.d/99-ft232h.rules
sudo udevadm control --reload-rules
```

Replug the adapter. Only root can open it again. If you added yourself to the
`plugdev` group just for this, you can leave it with `sudo gpasswd -d "$USER"
plugdev`, but other devices may use that group too.

Windows and macOS had no setup, so there is nothing to remove.

## Go library

Drop the import from your code, then tidy the module:

```sh
go mod tidy
```

To remove the requirement by hand instead:

```sh
go get github.com/ApolloX939/mcp2221@none
```

A local checkout of this repo can simply be deleted.
