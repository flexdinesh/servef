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
ssh host 'servef ~/code/foo'   # no-UI machine: serves on its address, prints web: and desktop: links
```

Desktop app: File > Connect... opens folders on ssh hosts from `~/.ssh/config`.

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
