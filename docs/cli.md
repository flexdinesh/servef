# CLI

## Open

```sh
srcz                      # current directory in the desktop app (browser if not installed)
srcz ~/code/foo           # a directory
srcz --path a --path b    # several directories
srcz ./browser            # a directory named browser
srcz --no-open .          # print the desktop URL, do not open it
```

## Browser

```sh
srcz browser .                    # serve on localhost and open the browser
srcz browser --no-open .          # serve, do not open the browser
srcz browser --port 8080 .        # fixed port (0 asks the OS; default first free of 7971-7980)
srcz browser --host 0.0.0.0 .     # bind another address (no authentication)
```

`--host` or `--port` on bare `srcz` also serves in the browser.

## Scope

```sh
srcz --depth 3 .          # project directory depth (default 5)
srcz --scan-depth 4 ~/code   # depth to search for git repositories (default 10)
```

## Remote

```sh
srcz ~/code/foo --remote          # on the server: serve over TLS, print a srcz connect string
srcz ~/code/foo --remote --web    # also print a browser link
srcz --remote --host 10.0.0.5     # one address (default every interface)
srcz connect <string>             # on your computer: open it in the desktop app
srcz connect --host localhost <string>   # through your own ssh -L tunnel
srcz connect --no-open <string>   # save it and print the srcz:// link
srcz remote status                # print the state and the string again
srcz remote stop                  # stop serving over TCP now
srcz remote rotate                # new identity; old strings stop working
```

Bare `srcz` on a machine with no UI prints this advice and exits.

## Walkthroughs

```sh
srcz walkthrough publish tour.json     # check and store a walkthrough
cat tour.json | srcz walkthrough publish -
srcz walkthrough publish --json --no-open tour.json
srcz walkthrough schema                # print the JSON Schema
```

## Daemon

```sh
srcz daemon status     # running daemon's roots and status (--json)
srcz daemon stop       # stop it
```

## Other

```sh
srcz --version
srcz [command] --help
```
