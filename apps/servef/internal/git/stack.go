package git

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/go-git/go-git/v5/plumbing"
)

// stackCandidateLimit caps the local branches Stack considers; a variable
// so tests can lower it.
var stackCandidateLimit = 100

// StackLayer is one branch of a stack: a local branch whose base..tip range
// holds HEAD or lies within HEAD's, or HEAD itself.
type StackLayer struct {
	// Name is the branch's short name; empty for a detached HEAD.
	Name string
	// Aliases are the other local branches at the same tip, sorted.
	Aliases []string
	// Head is the layer's tip.
	Head Hash
	// Parent names the nearest layer below, or the base branch.
	Parent string
	// Ahead counts the commits in the layer's range and not its parent's.
	Ahead int
	// Current marks HEAD's layer.
	Current bool
}

// Stack is HEAD's branch stack against the base branch.
type Stack struct {
	// Base is the base branch's short name, or empty when none was given and
	// none exists.
	Base string
	// MergeBase and Head follow BranchCompare.
	MergeBase Hash
	Head      Hash
	// Branch is HEAD's branch; empty when detached.
	Branch string
	// Layers run from the farthest from the base to the nearest.
	Layers []StackLayer
	// Truncated reports that the candidate limit or the walk bounds left
	// branches unconsidered.
	Truncated bool
}

// stackLayer is a layer and the commits of its base..tip range.
type stackLayer struct {
	StackLayer
	commits map[plumbing.Hash]bool
}

// Stack finds the local branches stacked with HEAD on the base branch. A
// branch is a layer when its tip is in base..HEAD (below HEAD) or HEAD is in
// base..tip (above HEAD); tips reachable from the base are not. HEAD's own
// layer is always listed. Branches at one tip share a layer: HEAD's, else
// the first by name. A layer's parent is the layer below it with the largest
// range, or the base. Without a base, with an unborn HEAD, or with nothing
// ahead, there are no layers. Every walk shares Log's bounds; past them, or
// past stackCandidateLimit branches, the rest are skipped and Truncated is
// set. base follows BranchCompare.
func (r *Repository) Stack(ctx context.Context, base string) (Stack, error) {
	baseName, baseHash, err := r.base(base)
	if err != nil {
		return Stack{}, err
	}
	headHash, err := r.head("")
	if err != nil {
		return Stack{}, err
	}
	result := Stack{Base: baseName, Head: Hash(headHash), Branch: r.Branch(), Layers: []StackLayer{}}
	if baseName == "" || headHash.IsZero() {
		return result, nil
	}
	w, err := r.newWalker(ctx)
	if err != nil {
		return Stack{}, err
	}
	branch, boundary, err := w.branch(headHash, baseHash)
	if err == nil && len(branch) > 0 {
		var mergeBase plumbing.Hash
		mergeBase, err = w.mergeBase(boundary)
		result.MergeBase = Hash(mergeBase)
	}
	if errors.Is(err, ErrRangeTooLarge) {
		return Stack{}, rangeTooLarge{base: baseName}
	}
	if err != nil {
		return Stack{}, err
	}
	if len(branch) == 0 {
		return result, nil
	}

	current := stackLayer{StackLayer: StackLayer{Name: result.Branch, Head: result.Head, Current: true}, commits: hashSet(branch)}
	layers := []*stackLayer{&current}
	// byTip maps each walked tip to its layer, or nil when it is none.
	byTip := map[plumbing.Hash]*stackLayer{headHash: &current}
	candidates, truncated, err := r.stackCandidates(result.Branch, baseName)
	if err != nil {
		return Stack{}, err
	}
	result.Truncated = truncated
	for _, candidate := range candidates {
		if shared, walked := byTip[candidate.hash]; walked {
			if shared != nil {
				shared.Aliases = append(shared.Aliases, candidate.name)
			}
			continue
		}
		commits, err := w.branchSet(candidate.hash, baseHash)
		if errors.Is(err, ErrRangeTooLarge) {
			result.Truncated = true
			break
		}
		if err != nil {
			return Stack{}, err
		}
		if len(commits) == 0 || (!current.commits[candidate.hash] && !commits[headHash]) {
			byTip[candidate.hash] = nil
			continue
		}
		layer := &stackLayer{StackLayer: StackLayer{Name: candidate.name, Head: Hash(candidate.hash)}, commits: commits}
		layers = append(layers, layer)
		byTip[candidate.hash] = layer
	}

	for _, layer := range layers {
		var parent *stackLayer
		for _, below := range layers {
			if below == layer || (below.Current && below.Name == "") || !layer.commits[plumbing.Hash(below.Head)] {
				continue
			}
			if parent == nil || len(below.commits) > len(parent.commits) {
				parent = below
			}
		}
		layer.Parent, layer.Ahead = baseName, len(layer.commits)
		if parent != nil {
			layer.Parent, layer.Ahead = parent.Name, 0
			for hash := range layer.commits {
				if !parent.commits[hash] {
					layer.Ahead++
				}
			}
		}
	}
	sort.SliceStable(layers, func(a, b int) bool {
		if len(layers[a].commits) != len(layers[b].commits) {
			return len(layers[a].commits) > len(layers[b].commits)
		}
		return layers[a].Name < layers[b].Name
	})
	for _, layer := range layers {
		if layer.Aliases == nil {
			layer.Aliases = []string{}
		}
		sort.Strings(layer.Aliases)
		result.Layers = append(result.Layers, layer.StackLayer)
	}
	return result, nil
}

// stackCandidate is a local branch Stack may list.
type stackCandidate struct {
	name string
	hash plumbing.Hash
}

// stackCandidates lists the local branches other than HEAD's and the one
// named like the base, sorted by name, keeping the first
// stackCandidateLimit.
func (r *Repository) stackCandidates(headBranch, baseName string) ([]stackCandidate, bool, error) {
	refs, err := r.storage.IterReferences()
	if err != nil {
		return nil, false, fmt.Errorf("read refs: %w", err)
	}
	defer refs.Close()
	var candidates []stackCandidate
	err = refs.ForEach(func(ref *plumbing.Reference) error {
		if !listedBranch(ref) || !ref.Name().IsBranch() {
			return nil
		}
		name := strings.TrimPrefix(ref.Name().String(), branchPrefix)
		if name != headBranch && name != baseName {
			candidates = append(candidates, stackCandidate{name: name, hash: ref.Hash()})
		}
		return nil
	})
	if err != nil {
		return nil, false, fmt.Errorf("read refs: %w", err)
	}
	sort.Slice(candidates, func(a, b int) bool { return candidates[a].name < candidates[b].name })
	if len(candidates) > stackCandidateLimit {
		return candidates[:stackCandidateLimit], true, nil
	}
	return candidates, false, nil
}

// branchSet is the set of commits branch lists for head against base.
func (w *walker) branchSet(head, base plumbing.Hash) (map[plumbing.Hash]bool, error) {
	commits, _, err := w.branch(head, base)
	if err != nil {
		return nil, err
	}
	return hashSet(commits), nil
}

func hashSet(hashes []plumbing.Hash) map[plumbing.Hash]bool {
	set := make(map[plumbing.Hash]bool, len(hashes))
	for _, hash := range hashes {
		set[hash] = true
	}
	return set
}
