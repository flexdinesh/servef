package client_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/url"
	"strings"
	"testing"

	"github.com/flexdinesh/servef/pkg/client"
	"github.com/flexdinesh/servef/pkg/contract"
)

func TestWalkthroughParityAcrossTransports(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	draft := func(id string, lines ...int) contract.WalkthroughDraft {
		return contract.WalkthroughDraft{Project: f.root, ID: id, Title: "Tour " + id, Summary: "Start **here**.", Steps: []contract.WalkthroughDraftStep{
			{Title: "Intro", Body: "See [the readme](README.md)."},
			{Title: "Readme", File: &contract.WalkthroughFileTarget{Path: "README.md", Lines: lines}},
		}}
	}

	schema := f.same(t, "WalkthroughSchema", func(c client.Client) outcome { return result(c.WalkthroughSchema(ctx)) })
	if got, ok := schema.Value.([]byte); !ok || !json.Valid(got) || !bytes.Contains(got, []byte(`"steps"`)) {
		t.Errorf("WalkthroughSchema = %+v", schema)
	}

	failures := []struct {
		name    string
		draft   contract.WalkthroughDraft
		code    string
		message string
	}{
		{"no steps", contract.WalkthroughDraft{Project: f.root, Title: "T"}, contract.CodeInvalidWalkthrough, "steps: needs at least one step"},
		{"lines not a pair", draft("bad", 1, 2, 3), contract.CodeInvalidWalkthrough, "invalid walkthrough JSON: lines must be [start, end], got 3 numbers"},
		{"past the end", draft("bad", 1, 9), contract.CodeInvalidWalkthrough, "steps[1].file.lines: end 9 is past the file's last line, 1"},
		{"outside the roots", contract.WalkthroughDraft{Project: "/", Title: "T", Steps: []contract.WalkthroughDraftStep{{Title: "s"}}}, contract.CodeInvalidWalkthrough, "project: "},
	}
	for _, failure := range failures {
		got := f.same(t, "PublishWalkthrough "+failure.name, func(c client.Client) outcome { return result(c.PublishWalkthrough(ctx, failure.draft)) })
		if kindOf(got.Err) != contract.KindInvalid || codeOf(got.Err) != failure.code || !strings.Contains(got.Err.Error(), failure.message) {
			t.Errorf("PublishWalkthrough %s error = %v, want %s containing %q", failure.name, got.Err, failure.code, failure.message)
		}
	}

	// Each transport publishes its own walkthrough and hears it announced.
	var daemon client.Client
	for _, transport := range f.transports {
		if transport.daemon {
			daemon = transport.client
		}
	}
	events, err := daemon.DaemonEvents(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer events.Close()
	if first, err := events.Next(); err != nil || first.Name != contract.DaemonEventStatus {
		t.Fatalf("first control event = %+v, %v", first, err)
	}
	for _, transport := range f.transports {
		id := strings.ReplaceAll(transport.name, "-", "")
		published, err := transport.client.PublishWalkthrough(ctx, draft(id, 1, 1))
		view := "/view?" + url.Values{"project": {f.root}, "wt": {id}}.Encode()
		if err != nil || published.ID != id || !published.Created || !strings.HasSuffix(published.URL, view) || (transport.name == "in-process") != (published.URL == view) {
			t.Errorf("PublishWalkthrough over %s = %+v, %v; want %s", transport.name, published, err, view)
		}
		again, err := transport.client.PublishWalkthrough(ctx, draft(id, 1, 1))
		if err != nil || again.Created {
			t.Errorf("republish over %s = %+v, %v; want replaced", transport.name, again, err)
		}
		for _, created := range []bool{true, false} {
			event, err := events.Next()
			want := contract.WalkthroughEvent{Project: f.root, ID: id, Title: "Tour " + id, Created: created}
			if err != nil || event.Name != contract.DaemonEventWalkthrough || event.Walkthrough == nil || *event.Walkthrough != want {
				t.Errorf("announcement of %s = %+v, %v; want %+v", transport.name, event, err, want)
			}
		}
	}

	list := f.same(t, "Walkthroughs", func(c client.Client) outcome { return result(c.Walkthroughs(ctx, f.root)) })
	if got, ok := list.Value.([]contract.WalkthroughSummary); !ok || len(got) != len(f.transports) || got[0].StepCount != 2 {
		t.Errorf("Walkthroughs = %+v", list)
	}
	read := f.same(t, "Walkthrough", func(c client.Client) outcome { return result(c.Walkthrough(ctx, f.root, "tcp")) })
	if got, ok := read.Value.(contract.Walkthrough); !ok || !strings.Contains(got.SummaryHTML, "<strong>here</strong>") || len(got.Steps) != 2 || got.Steps[1].File == nil || got.Steps[1].File.Expect != "# Hello" || got.CreatedAt == "" {
		t.Errorf("Walkthrough = %+v", read)
	}
	missing := f.same(t, "Walkthrough missing", func(c client.Client) outcome { return result(c.Walkthrough(ctx, f.root, "gone")) })
	if kindOf(missing.Err) != contract.KindNotFound || codeOf(missing.Err) != contract.CodeWalkthroughUnavailable {
		t.Errorf("Walkthrough missing error = %v", missing.Err)
	}
	f.same(t, "Walkthroughs without a project", func(c client.Client) outcome { return result(c.Walkthroughs(ctx, "")) })

	for _, transport := range f.transports {
		id := strings.ReplaceAll(transport.name, "-", "")
		if err := transport.client.DeleteWalkthrough(ctx, f.root, id); err != nil {
			t.Errorf("DeleteWalkthrough over %s = %v", transport.name, err)
		}
	}
	gone := f.same(t, "DeleteWalkthrough missing", func(c client.Client) outcome {
		return outcome{Err: c.DeleteWalkthrough(ctx, f.root, "tcp")}
	})
	if codeOf(gone.Err) != contract.CodeWalkthroughUnavailable {
		t.Errorf("DeleteWalkthrough missing error = %v", gone.Err)
	}
	f.same(t, "Walkthroughs after delete", func(c client.Client) outcome { return result(c.Walkthroughs(ctx, f.root)) })
}
