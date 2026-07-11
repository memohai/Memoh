package contextbudget

import "sort"

type Retention string

const (
	RetentionCandidate Retention = "candidate"
	RetentionRequired  Retention = "required"
	RetentionDrop      Retention = "drop"
)

type DropReason string

const (
	DropBudget DropReason = "budget"
	DropPolicy DropReason = "policy"
)

type Item struct {
	ID string
	// Group is an occurrence-scoped atomic key. Callers must not reuse a raw
	// external tool-call ID as the key across separate exchanges.
	Group string
	// Tokens below zero are treated as zero. The zero Retention value is a
	// normal budget candidate.
	Tokens int
	// DropTier is a protection band: lower tiers drop first; ties drop by the
	// atomic group's last member, oldest first.
	DropTier  int
	Retention Retention
	// PolicyReason is reported only for RetentionDrop. Empty uses DropPolicy.
	PolicyReason DropReason
	// CompactableTokens contributes independently metered raw preselection
	// pressure. Negative values are treated as zero.
	CompactableTokens int
}

type Request struct {
	// SourceLimit is the source-only budget after caller-owned prompt and notice
	// reserves. Nil is unlimited; a non-nil negative value is treated as zero.
	SourceLimit *int
	Items       []Item
}

type Decision struct {
	Index  int
	ID     string
	Reason DropReason
}

type Allocation struct {
	Kept    []Decision
	Dropped []Decision
	// SourceTokens counts every input item before selection.
	SourceTokens int
	// SelectedTokens counts retained source items, excluding caller reserves.
	SelectedTokens int
	// CompactableTokens is raw preselection pressure and does not shrink when
	// items are dropped by policy or budget.
	CompactableTokens    int
	SourceOverflowTokens int
	SourcesFit           bool
	Changed              bool
	BudgetTrimmed        bool
}

type sourceGroup struct {
	tokens       int
	dropTier     int
	lastIndex    int
	required     bool
	policyDrop   bool
	policyReason DropReason
}

func Allocate(request Request) Allocation {
	groups, itemGroups, result := buildGroups(request.Items)
	sourceLimit := 0
	if request.SourceLimit != nil {
		sourceLimit = max(*request.SourceLimit, 0)
	}
	selected := make([]bool, len(groups))
	dropReasons := make([]DropReason, len(groups))
	budgetDrops := make([]bool, len(groups))

	for i := range groups {
		group := &groups[i]
		switch {
		case group.required:
			selected[i] = true
			result.SelectedTokens += group.tokens
		case group.policyDrop:
			dropReasons[i] = group.policyReason
		default:
			selected[i] = true
			result.SelectedTokens += group.tokens
		}
	}

	if request.SourceLimit != nil && result.SelectedTokens > sourceLimit {
		droppable := make([]int, 0, len(groups))
		for i := range groups {
			if selected[i] && !groups[i].required && groups[i].tokens > 0 {
				droppable = append(droppable, i)
			}
		}
		sort.SliceStable(droppable, func(i, j int) bool {
			left := groups[droppable[i]]
			right := groups[droppable[j]]
			if left.dropTier != right.dropTier {
				return left.dropTier < right.dropTier
			}
			return left.lastIndex < right.lastIndex
		})
		for _, groupIndex := range droppable {
			if result.SelectedTokens <= sourceLimit {
				break
			}
			selected[groupIndex] = false
			dropReasons[groupIndex] = DropBudget
			budgetDrops[groupIndex] = true
			result.SelectedTokens -= groups[groupIndex].tokens
		}
	}

	for itemIndex, item := range request.Items {
		decision := Decision{Index: itemIndex, ID: item.ID}
		groupIndex := itemGroups[itemIndex]
		if selected[groupIndex] {
			result.Kept = append(result.Kept, decision)
			continue
		}
		decision.Reason = dropReasons[groupIndex]
		result.Dropped = append(result.Dropped, decision)
		result.BudgetTrimmed = result.BudgetTrimmed || budgetDrops[groupIndex]
	}
	result.Changed = len(result.Dropped) > 0
	result.SourcesFit = request.SourceLimit == nil || result.SelectedTokens <= sourceLimit
	if request.SourceLimit != nil && result.SelectedTokens > sourceLimit {
		result.SourceOverflowTokens = result.SelectedTokens - sourceLimit
	}
	return result
}

func buildGroups(items []Item) ([]sourceGroup, []int, Allocation) {
	groups := make([]sourceGroup, 0, len(items))
	itemGroups := make([]int, len(items))
	groupIndexes := make(map[string]int)
	result := Allocation{}
	for i, item := range items {
		tokens := max(item.Tokens, 0)
		result.SourceTokens += tokens
		result.CompactableTokens += max(item.CompactableTokens, 0)

		groupIndex := -1
		if item.Group != "" {
			if existing, ok := groupIndexes[item.Group]; ok {
				groupIndex = existing
			}
		}
		if groupIndex < 0 {
			groupIndex = len(groups)
			groups = append(groups, sourceGroup{dropTier: item.DropTier, lastIndex: i})
			if item.Group != "" {
				groupIndexes[item.Group] = groupIndex
			}
		}

		group := &groups[groupIndex]
		group.tokens += tokens
		group.dropTier = max(group.dropTier, item.DropTier)
		group.lastIndex = max(group.lastIndex, i)
		group.required = group.required || item.Retention == RetentionRequired
		if item.Retention == RetentionDrop {
			group.policyDrop = true
			if group.policyReason == "" {
				group.policyReason = item.PolicyReason
				if group.policyReason == "" {
					group.policyReason = DropPolicy
				}
			}
		}
		itemGroups[i] = groupIndex
	}
	return groups, itemGroups, result
}
