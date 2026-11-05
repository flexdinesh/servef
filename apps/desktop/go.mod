module github.com/flexdinesh/servef/apps/desktop

go 1.27.3

require (
	github.com/egoist/mygo v0.7.4
	github.com/flexdinesh/servef v0.0.0
)

require (
	github.com/ebitengine/purego v0.11.1 // indirect
	golang.org/x/net v0.59.0 // indirect
	golang.org/x/text v0.43.0 // indirect
)

replace github.com/flexdinesh/servef => ../servef

tool github.com/egoist/mygo/cmd/mygo
