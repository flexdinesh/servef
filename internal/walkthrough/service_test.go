package walkthrough

import (
	"context"
	"errors"
	"regexp"
	"testing"
	"time"
)

// fakeTargets canonicalizes the project and fills one Expect, as a host does.
type fakeTargets struct {
	project string
	err     error
	checked [][]Step
}

func (f *fakeTargets) Check(_ context.Context, _ string, steps []Step) (string, []Step, error) {
	f.checked = append(f.checked, steps)
	if f.err != nil {
		return "", nil, f.err
	}
	checked := append([]Step(nil), steps...)
	for i, step := range checked {
		if step.File != nil && step.File.Expect == "" {
			file := *step.File
			file.Expect = "captured"
			checked[i].File = &file
		}
	}
	return f.project, checked, nil
}

func TestServicePublishesWithCanonicalProjectAndGeneratedID(t *testing.T) {
	targets := &fakeTargets{project: "/canonical"}
	service := NewService(NewStore(t.TempDir()), targets)
	clock := time.Date(2026, 10, 26, 1, 2, 3, 0, time.UTC)
	service.now = func() time.Time { return clock }
	project := absoluteProject(t)

	published, err := service.Publish(context.Background(), Draft{Project: project, Title: "How Login Works", Steps: []Step{{Title: "Intro"}, {Title: "Code", File: &FileTarget{Path: "a.go", Lines: &Lines{1, 2}}}}})
	if err != nil {
		t.Fatal(err)
	}
	w := published.Walkthrough
	if !published.Created || w.Project != "/canonical" || !regexp.MustCompile(`^how-login-works-[0-9a-f]{6}$`).MatchString(w.ID) {
		t.Fatalf("published = %+v", published)
	}
	if w.Steps[1].File.Expect != "captured" || !w.CreatedAt.Equal(clock) {
		t.Errorf("walkthrough = %+v", w)
	}
	got, err := service.Get("/canonical", w.ID)
	if err != nil || got.Title != "How Login Works" {
		t.Fatalf("Get = %+v, %v", got, err)
	}

	// Republishing an ID keeps its creation time.
	clock = clock.Add(time.Hour)
	again, err := service.Publish(context.Background(), Draft{Project: project, ID: w.ID, Title: "Updated", Steps: []Step{{Title: "Only"}}})
	if err != nil {
		t.Fatal(err)
	}
	if again.Created || !again.Walkthrough.CreatedAt.Equal(w.CreatedAt) || !again.Walkthrough.UpdatedAt.Equal(clock) {
		t.Errorf("republished = %+v", again)
	}
	list, err := service.List("/canonical")
	if err != nil || len(list) != 1 || list[0].Title != "Updated" {
		t.Errorf("List = %+v, %v", list, err)
	}
	if err := service.Delete("/canonical", w.ID); err != nil {
		t.Fatal(err)
	}
}

func TestServiceRejectsBeforeCheckingTargets(t *testing.T) {
	targets := &fakeTargets{project: "/canonical"}
	service := NewService(NewStore(t.TempDir()), targets)
	var problems Problems
	if _, err := service.Publish(context.Background(), Draft{Title: "t"}); !errors.As(err, &problems) {
		t.Fatalf("error = %v, want Problems", err)
	}
	if len(targets.checked) != 0 {
		t.Errorf("targets checked an invalid draft")
	}

	targets.err = Problems{{Field: "steps[0].file.path", Message: "is not a file in the project"}}
	if _, err := service.Publish(context.Background(), Draft{Project: absoluteProject(t), Title: "t", Steps: []Step{{Title: "s", File: &FileTarget{Path: "a"}}}}); !errors.As(err, &problems) || problems[0].Field != "steps[0].file.path" {
		t.Fatalf("target error = %v", err)
	}
	if list, _ := service.List("/canonical"); len(list) != 0 {
		t.Errorf("rejected draft stored: %+v", list)
	}
}
