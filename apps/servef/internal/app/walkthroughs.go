package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"sync"
	"time"

	"github.com/flexdinesh/servef/internal/document"
	"github.com/flexdinesh/servef/internal/walkthrough"
	"github.com/flexdinesh/servef/pkg/contract"
)

// Walkthroughs (decision 0047) publishes, lists, reads, and deletes
// agent-authored walkthroughs, checking drafts against the served projects
// (NewWalkthroughTargets), and announces each publish to its subscribers
// (decision 0066).
type Walkthroughs struct {
	*state
	service *walkthrough.Service
	notices noticeHub
}

// MaxWalkthroughBytes bounds one encoded draft.
const MaxWalkthroughBytes = walkthrough.MaxRequestBytes

// WalkthroughUnavailableMessage answers a walkthrough the store lacks.
const WalkthroughUnavailableMessage = "That walkthrough is unavailable."

// publishTimeout bounds checking and storing one published walkthrough.
const publishTimeout = 20 * time.Second

// noticeBuffer is how many notices one subscriber holds before the oldest
// is dropped.
const noticeBuffer = 16

// DecodeWalkthroughDraft reads one strict JSON draft (walkthrough.Decode):
// unknown fields, trailing data, and lines that are not [start, end] fail.
// Errors wrap the reader's, so a transport can tell a body over its limit.
func DecodeWalkthroughDraft(r io.Reader) (contract.WalkthroughDraft, error) {
	draft, err := walkthrough.Decode(r)
	if err != nil {
		return contract.WalkthroughDraft{}, err
	}
	steps := make([]contract.WalkthroughDraftStep, len(draft.Steps))
	for i, step := range draft.Steps {
		steps[i] = contract.WalkthroughDraftStep{Title: step.Title, Body: step.Body, File: fileTargetContract(step.File), Diff: diffTargetContract(step.Diff)}
	}
	if draft.Steps == nil {
		steps = nil
	}
	return contract.WalkthroughDraft{Project: draft.Project, ID: draft.ID, Title: draft.Title, Summary: draft.Summary, Steps: steps, Open: draft.Open}, nil
}

// Schema is the JSON Schema of a draft.
func (w *Walkthroughs) Schema() []byte { return walkthrough.Schema }

// Publish validates a draft, checks its targets, stores it, and announces
// it. URL is the walkthrough's view path and query; Open is the
// transport's to honor.
func (w *Walkthroughs) Publish(ctx context.Context, draft contract.WalkthroughDraft) (contract.WalkthroughPublished, error) {
	model, err := draftModel(draft)
	if err != nil {
		return contract.WalkthroughPublished{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, publishTimeout)
	defer cancel()
	published, err := w.service.Publish(ctx, model)
	var problems walkthrough.Problems
	switch {
	case errors.As(err, &problems):
		return contract.WalkthroughPublished{}, &Error{Kind: contract.KindInvalid, Code: contract.CodeInvalidWalkthrough, Message: problems.Error()}
	case err != nil:
		return contract.WalkthroughPublished{}, fail(contract.KindInternal, "Couldn't publish the walkthrough: "+err.Error())
	}
	stored := published.Walkthrough
	w.notices.announce(contract.WalkthroughEvent{Project: stored.Project, ID: stored.ID, Title: stored.Title, Created: published.Created})
	query := url.Values{"project": {stored.Project}, "wt": {stored.ID}}
	return contract.WalkthroughPublished{ID: stored.ID, URL: "/view?" + query.Encode(), Created: published.Created}, nil
}

// List returns a project's walkthroughs, most recently updated first.
func (w *Walkthroughs) List(project string) ([]contract.WalkthroughSummary, error) {
	resolved, err := w.resolve(project)
	if err != nil {
		return nil, err
	}
	summaries, err := w.service.List(resolved.Root)
	if err != nil {
		return nil, fail(contract.KindInternal, "Couldn't list walkthroughs: "+err.Error())
	}
	return mapSlice(summaries, func(summary walkthrough.Summary) contract.WalkthroughSummary {
		return contract.WalkthroughSummary{ID: summary.ID, Title: summary.Title, StepCount: summary.StepCount, UpdatedAt: formatTime(summary.UpdatedAt)}
	}), nil
}

// Get returns a project's walkthrough with its Markdown rendered; narration
// links resolve from the project root, like a root README's.
func (w *Walkthroughs) Get(project, id string) (contract.Walkthrough, error) {
	resolved, err := w.resolve(project)
	if err != nil {
		return contract.Walkthrough{}, err
	}
	stored, err := w.service.Get(resolved.Root, id)
	if errors.Is(err, walkthrough.ErrNotFound) {
		return contract.Walkthrough{}, unavailableWalkthrough()
	}
	if err != nil {
		return contract.Walkthrough{}, fail(contract.KindInternal, "Couldn't read the walkthrough: "+err.Error())
	}
	context := document.Context{Project: stored.Project}
	render := func(source string) string {
		if source == "" {
			return ""
		}
		rendered, err := document.RenderMarkdown([]byte(source), context)
		if err != nil {
			return ""
		}
		return rendered
	}
	steps := mapSlice(stored.Steps, func(step walkthrough.Step) contract.WalkthroughStep {
		return contract.WalkthroughStep{Title: step.Title, Body: step.Body, File: fileTargetContract(step.File), Diff: diffTargetContract(step.Diff), BodyHTML: render(step.Body)}
	})
	return contract.Walkthrough{ID: stored.ID, Project: stored.Project, Title: stored.Title, Summary: stored.Summary, SummaryHTML: render(stored.Summary), Steps: steps, CreatedAt: formatTime(stored.CreatedAt), UpdatedAt: formatTime(stored.UpdatedAt)}, nil
}

// Delete removes a project's walkthrough.
func (w *Walkthroughs) Delete(project, id string) error {
	resolved, err := w.resolve(project)
	if err != nil {
		return err
	}
	err = w.service.Delete(resolved.Root, id)
	if errors.Is(err, walkthrough.ErrNotFound) {
		return unavailableWalkthrough()
	}
	if err != nil {
		return fail(contract.KindInternal, "Couldn't delete the walkthrough: "+err.Error())
	}
	return nil
}

// Announce tells every subscriber that project's walkthrough id was
// published elsewhere (the CLI writes the same store). It fails with
// CodeProjectUnavailable when no served root resolves project and
// CodeWalkthroughUnavailable when the store lacks the walkthrough.
func (w *Walkthroughs) Announce(project, id string, created bool) (contract.WalkthroughEvent, error) {
	resolved, err := w.resolve(project)
	if err != nil {
		return contract.WalkthroughEvent{}, &Error{Kind: contract.KindNotFound, Code: contract.CodeProjectUnavailable, Message: ProjectUnavailableMessage}
	}
	stored, err := w.service.Get(resolved.Root, id)
	if errors.Is(err, walkthrough.ErrNotFound) {
		return contract.WalkthroughEvent{}, unavailableWalkthrough()
	}
	if err != nil {
		return contract.WalkthroughEvent{}, fail(contract.KindInternal, fmt.Sprintf("walkthrough %s: %v", id, err))
	}
	event := contract.WalkthroughEvent{Project: stored.Project, ID: stored.ID, Title: stored.Title, Created: created}
	w.notices.announce(event)
	return event, nil
}

// Notices subscribes to walkthrough announcements. Each subscriber buffers
// 16 and drops the oldest when full. The channel closes after cancel or
// CloseNotices.
func (w *Walkthroughs) Notices() (notices <-chan contract.WalkthroughEvent, cancel func()) {
	return w.notices.subscribe()
}

// CloseNotices ends every notice subscription; later ones start closed.
func (w *Walkthroughs) CloseNotices() { w.notices.close() }

func (w *Walkthroughs) resolve(project string) (Workspace, error) {
	return ResolveProject(w.config.Roots.List(), project)
}

func unavailableWalkthrough() *Error {
	return &Error{Kind: contract.KindNotFound, Code: contract.CodeWalkthroughUnavailable, Message: WalkthroughUnavailableMessage}
}

// formatTime encodes t as encoding/json encodes a time.Time.
func formatTime(t time.Time) string { return t.Format(time.RFC3339Nano) }

func fileTargetContract(file *walkthrough.FileTarget) *contract.WalkthroughFileTarget {
	if file == nil {
		return nil
	}
	return &contract.WalkthroughFileTarget{Path: file.Path, Lines: linesContract(file.Lines), At: file.At, Expect: file.Expect}
}

func diffTargetContract(diff *walkthrough.DiffTarget) *contract.WalkthroughDiffTarget {
	if diff == nil {
		return nil
	}
	compare := diff.Compare
	return &contract.WalkthroughDiffTarget{
		Compare: contract.WalkthroughCompare{Kind: contract.WalkthroughCompareKind(compare.Kind), Base: compare.Base, Head: compare.Head, Commit: compare.Commit, From: compare.From, To: compare.To},
		Path:    diff.Path,
		Side:    contract.WalkthroughSide(diff.Side),
		Lines:   linesContract(diff.Lines),
		Expect:  diff.Expect,
	}
}

func linesContract(lines *walkthrough.Lines) []int {
	if lines == nil {
		return nil
	}
	return []int{lines[0], lines[1]}
}

// draftModel is a draft as walkthrough.Service takes it. Lines that are not
// [start, end] fail as walkthrough.Decode fails them.
func draftModel(draft contract.WalkthroughDraft) (walkthrough.Draft, error) {
	var steps []walkthrough.Step
	if draft.Steps != nil {
		steps = make([]walkthrough.Step, len(draft.Steps))
	}
	for i, step := range draft.Steps {
		model := walkthrough.Step{Title: step.Title, Body: step.Body}
		if file := step.File; file != nil {
			lines, err := linesModel(file.Lines)
			if err != nil {
				return walkthrough.Draft{}, err
			}
			model.File = &walkthrough.FileTarget{Path: file.Path, Lines: lines, At: file.At, Expect: file.Expect}
		}
		if diff := step.Diff; diff != nil {
			lines, err := linesModel(diff.Lines)
			if err != nil {
				return walkthrough.Draft{}, err
			}
			compare := diff.Compare
			model.Diff = &walkthrough.DiffTarget{
				Compare: walkthrough.Compare{Kind: string(compare.Kind), Base: compare.Base, Head: compare.Head, Commit: compare.Commit, From: compare.From, To: compare.To},
				Path:    diff.Path,
				Side:    string(diff.Side),
				Lines:   lines,
				Expect:  diff.Expect,
			}
		}
		steps[i] = model
	}
	return walkthrough.Draft{Project: draft.Project, ID: draft.ID, Title: draft.Title, Summary: draft.Summary, Steps: steps, Open: draft.Open}, nil
}

func linesModel(lines []int) (*walkthrough.Lines, error) {
	if lines == nil {
		return nil, nil
	}
	if len(lines) != 2 {
		message := fmt.Sprintf("%v: lines must be [start, end], got %d numbers", walkthrough.ErrInvalidJSON, len(lines))
		return nil, &Error{Kind: contract.KindInvalid, Code: contract.CodeInvalidWalkthrough, Message: message}
	}
	return &walkthrough.Lines{lines[0], lines[1]}, nil
}

// noticeHub fans announcements out to subscribers without ever blocking the
// announcer. Its zero value is ready to use.
type noticeHub struct {
	mu          sync.Mutex
	closed      bool
	subscribers map[chan contract.WalkthroughEvent]struct{}
}

func (h *noticeHub) subscribe() (<-chan contract.WalkthroughEvent, func()) {
	h.mu.Lock()
	defer h.mu.Unlock()
	notices := make(chan contract.WalkthroughEvent, noticeBuffer)
	if h.closed {
		close(notices)
		return notices, func() {}
	}
	if h.subscribers == nil {
		h.subscribers = map[chan contract.WalkthroughEvent]struct{}{}
	}
	h.subscribers[notices] = struct{}{}
	return notices, func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		if _, ok := h.subscribers[notices]; ok {
			delete(h.subscribers, notices)
			close(notices)
		}
	}
}

// announce sends notice to every subscriber, dropping a full subscriber's
// oldest notice to make room.
func (h *noticeHub) announce(notice contract.WalkthroughEvent) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for notices := range h.subscribers {
		for sent := false; !sent; {
			select {
			case notices <- notice:
				sent = true
			default:
				// Full: drop the oldest, unless the reader just took it.
				select {
				case <-notices:
				default:
				}
			}
		}
	}
}

// close ends every subscription; later subscriptions start closed.
func (h *noticeHub) close() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.closed = true
	for notices := range h.subscribers {
		close(notices)
	}
	h.subscribers = nil
}
