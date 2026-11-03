package app

import (
	"errors"

	"github.com/flexdinesh/servef/internal/files"
	"github.com/flexdinesh/servef/internal/projects"
	"github.com/flexdinesh/servef/pkg/contract"
)

// Project resolution messages.
const (
	NoProjectMessage          = "No project was selected."
	ProjectUnavailableMessage = "Project unavailable."
	InsideRepositoryMessage   = "Project unavailable: folders inside a repository open with the repository."
)

// Projects lists the served projects and browses folders within the roots.
type Projects struct{ *state }

// Workspace is the project directory an operation is scoped to: its ID and
// absolute root.
type Workspace struct {
	ID   string
	Root string
}

// Resolve resolves a project ID within a snapshot of the served roots.
func (p *Projects) Resolve(id string) (Workspace, error) {
	return ResolveProject(p.config.Roots.List(), id)
}

// ResolveProject resolves a project ID within roots: an empty ID or one
// outside the roots is invalid; a folder inside a repository or a missing
// project is not found.
func ResolveProject(roots []string, id string) (Workspace, error) {
	if id == "" {
		return Workspace{}, fail(contract.KindInvalid, NoProjectMessage)
	}
	root, err := projects.Resolve(roots, id)
	if errors.Is(err, files.ErrPathEscape) {
		return Workspace{}, fail(contract.KindInvalid, ProjectUnavailableMessage)
	}
	if errors.Is(err, projects.ErrInsideRepository) {
		return Workspace{}, fail(contract.KindNotFound, InsideRepositoryMessage)
	}
	if err != nil {
		return Workspace{}, fail(contract.KindNotFound, ProjectUnavailableMessage)
	}
	return Workspace{ID: id, Root: root}, nil
}

// List discovers the projects within the served roots. A root removed while
// serving is a warning, not a failure for every root.
func (p *Projects) List() contract.ProjectsResponse {
	roots := p.config.Roots.List()
	discovery := projects.Discover(roots, p.config.ScanDepth)
	return contract.ProjectsResponse{
		SessionKey: p.sessionKey(roots),
		Roots:      mapSlice(projects.Roots(roots), rootContract),
		Projects:   mapSlice(discovery.Projects, projectContract),
		Warnings:   warningStrings(append(discovery.Skipped, discovery.Warnings...)),
	}
}

// Directories lists a folder's child directories within the served roots.
func (p *Projects) Directories(path string) (contract.DirectoriesResponse, error) {
	directories, err := projects.Directories(p.config.Roots.List(), path)
	switch {
	case errors.Is(err, files.ErrPathEscape):
		return contract.DirectoriesResponse{}, fail(contract.KindInvalid, "That folder is outside the served folders.")
	case errors.Is(err, projects.ErrInsideRepository):
		return contract.DirectoriesResponse{}, fail(contract.KindInvalid, "Repositories open whole; their folders can't be browsed.")
	case errors.Is(err, projects.ErrUnavailable):
		return contract.DirectoriesResponse{}, fail(contract.KindNotFound, "That folder is unavailable.")
	case err != nil:
		return contract.DirectoriesResponse{}, fail(contract.KindInternal, "Could not read that folder.")
	}
	return contract.DirectoriesResponse{Path: path, Directories: mapSlice(directories, directoryContract)}, nil
}

func rootContract(root projects.Root) contract.Root {
	return contract.Root{Path: root.Path, Name: root.Name, IsRepo: root.IsRepo}
}

func projectContract(project projects.Project) contract.Project {
	return contract.Project{
		ID:         project.ID,
		Name:       project.Name,
		Path:       project.Path,
		Kind:       contract.ProjectKind(project.Kind),
		Group:      project.Group,
		Root:       project.Root,
		Branch:     project.Branch,
		Repository: project.Repository,
		Available:  project.Available,
	}
}

func directoryContract(directory projects.Directory) contract.Directory {
	return contract.Directory{Name: directory.Name, Path: directory.Path, IsProject: directory.IsProject}
}
