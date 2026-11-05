# CLI

## Open

```sh
servef                      # current directory in the desktop app (browser if not installed)
servef ~/code/foo           # a directory
servef --path a --path b    # several directories
servef ./browser            # a directory named browser
servef --no-open .          # print the desktop URL, do not open it
```

## Browser

```sh
servef browser .                    # serve on localhost and open the browser
servef browser --no-open .          # serve, do not open the browser
servef browser --port 8080 .        # fixed port (0 asks the OS; default first free of 7971-7980)
servef browser --host 0.0.0.0 .     # bind another address (no authentication)
```

`--host` or `--port` on bare `servef` also serves in the browser.

## Scope

```sh
servef --depth 3 .          # project directory depth (default 5)
servef --scan-depth 4 ~/code   # depth to search for git repositories (default 10)
```

## Remote

```sh
servef ~/code/foo --remote          # on the server: serve over TLS, print a servef connect string
servef ~/code/foo --remote --web    # also print a browser link
servef --remote --host 10.0.0.5     # one address (default every interface)
servef connect <string>             # on your computer: open it in the desktop app
servef connect --host localhost <string>   # through your own ssh -L tunnel
servef connect --no-open <string>   # save it and print the servef:// link
servef remote status                # print the state and the string again
servef remote stop                  # stop serving over TCP now
servef remote rotate                # new identity; old strings stop working
```

Bare `servef` on a machine with no UI prints this advice and exits.

## Walkthroughs

```sh
servef walkthrough publish tour.json     # check and store a walkthrough
cat tour.json | servef walkthrough publish -
servef walkthrough publish --json --no-open tour.json
servef walkthrough schema                # print the JSON Schema
```

## Daemon

```sh
servef daemon status     # running daemon's roots and status (--json)
servef daemon stop       # stop it
```

## Other

```sh
servef --version
servef [command] --help
```
