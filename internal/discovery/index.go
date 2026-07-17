package discovery

import (
	"cmp"
	"container/heap"
	"slices"
	"sort"

	"github.com/zrma/sema/internal/domain"
)

const (
	skillBandWidth   = 100
	latencyBandWidth = 25
)

type PartitionKey struct {
	PartySize   int
	SkillBand   int
	RoleProfile uint64
	LatencyBand int
}

type IndexStats struct {
	Tickets    int
	Partitions int
}

type partition struct {
	key     PartitionKey
	tickets []domain.MatchTicket
}

// Index partitions one canonical queue while preserving enqueue order inside
// every partition. Selection merges partition heads back into the exact oldest
// fitting prefix, so partitioning does not change fairness or approximation.
type Index struct {
	tickets           []domain.MatchTicket
	partitions        []partition
	partyThresholds   []int
	fittingByPartyMax map[int][]int
}

func BuildIndex(tickets []domain.MatchTicket) Index {
	grouped := make(map[PartitionKey][]domain.MatchTicket)
	partySizes := make(map[int]struct{})
	for _, ticket := range tickets {
		key := partitionKey(ticket)
		grouped[key] = append(grouped[key], ticket)
		partySizes[key.PartySize] = struct{}{}
	}
	keys := make([]PartitionKey, 0, len(grouped))
	for key := range grouped {
		keys = append(keys, key)
	}
	slices.SortFunc(keys, comparePartitionKey)
	partitions := make([]partition, len(keys))
	for index, key := range keys {
		partitions[index] = partition{key: key, tickets: grouped[key]}
	}
	index := Index{tickets: tickets, partitions: partitions}
	// Typical party envelopes have only a handful of sizes. Cache exact queue
	// positions for those thresholds; fall back to partition merge for unusually
	// broad envelopes to keep memory bounded.
	if len(partySizes) <= 16 {
		for size := range partySizes {
			index.partyThresholds = append(index.partyThresholds, size)
		}
		slices.Sort(index.partyThresholds)
		index.fittingByPartyMax = make(map[int][]int, len(index.partyThresholds))
		for _, threshold := range index.partyThresholds {
			positions := make([]int, 0, len(tickets))
			for ticketIndex, ticket := range tickets {
				if len(ticket.Players) <= threshold {
					positions = append(positions, ticketIndex)
				}
			}
			index.fittingByPartyMax[threshold] = positions
		}
	}
	return index
}

func (index Index) Stats() IndexStats {
	return IndexStats{Tickets: len(index.tickets), Partitions: len(index.partitions)}
}

func (index Index) SelectWindow(slots []int, limit int) Window {
	if limit <= 0 {
		return Window{Tickets: index.tickets}
	}
	maxPartySize := 0
	for _, available := range slots {
		maxPartySize = max(maxPartySize, available)
	}
	if len(index.partyThresholds) > 0 {
		thresholdIndex := sort.Search(len(index.partyThresholds), func(position int) bool {
			return index.partyThresholds[position] > maxPartySize
		}) - 1
		if thresholdIndex < 0 {
			return Window{Tickets: make([]domain.MatchTicket, 0)}
		}
		positions := index.fittingByPartyMax[index.partyThresholds[thresholdIndex]]
		count := min(limit, len(positions))
		selected := make([]domain.MatchTicket, count)
		for selectedIndex, ticketIndex := range positions[:count] {
			selected[selectedIndex] = index.tickets[ticketIndex]
		}
		return Window{Tickets: selected, Truncated: len(positions) > limit}
	}
	cursors := make(partitionHeap, 0, len(index.partitions))
	for partitionIndex, partition := range index.partitions {
		if partition.key.PartySize <= maxPartySize && len(partition.tickets) > 0 {
			cursors = append(cursors, partitionCursor{
				partition: partitionIndex,
				ticket:    0,
				value:     partition.tickets[0],
			})
		}
	}
	heap.Init(&cursors)
	selected := make([]domain.MatchTicket, 0, min(limit, len(index.tickets)))
	for cursors.Len() > 0 {
		if len(selected) == limit {
			return Window{Tickets: selected, Truncated: true}
		}
		cursor := heap.Pop(&cursors).(partitionCursor)
		selected = append(selected, cursor.value)
		cursor.ticket++
		partition := index.partitions[cursor.partition]
		if cursor.ticket < len(partition.tickets) {
			cursor.value = partition.tickets[cursor.ticket]
			heap.Push(&cursors, cursor)
		}
	}
	return Window{Tickets: selected}
}

func partitionKey(ticket domain.MatchTicket) PartitionKey {
	totalSkill, maximumLatency := 0, 0
	roleProfile := uint64(0)
	for _, player := range ticket.Players {
		totalSkill += player.Skill
		maximumLatency = max(maximumLatency, player.LatencyMillis)
		roleProfile += stableRoleHash(player.Role)
	}
	averageSkill := 0
	if len(ticket.Players) > 0 {
		averageSkill = totalSkill / len(ticket.Players)
	}
	return PartitionKey{
		PartySize: len(ticket.Players), SkillBand: averageSkill / skillBandWidth,
		RoleProfile: roleProfile, LatencyBand: maximumLatency / latencyBandWidth,
	}
}

func stableRoleHash(role string) uint64 {
	const (
		offset = uint64(14695981039346656037)
		prime  = uint64(1099511628211)
	)
	hash := offset
	for index := 0; index < len(role); index++ {
		hash ^= uint64(role[index])
		hash *= prime
	}
	return hash
}

func comparePartitionKey(left, right PartitionKey) int {
	if result := cmp.Compare(left.PartySize, right.PartySize); result != 0 {
		return result
	}
	if result := cmp.Compare(left.SkillBand, right.SkillBand); result != 0 {
		return result
	}
	if result := cmp.Compare(left.RoleProfile, right.RoleProfile); result != 0 {
		return result
	}
	return cmp.Compare(left.LatencyBand, right.LatencyBand)
}

type partitionCursor struct {
	partition int
	ticket    int
	value     domain.MatchTicket
}

type partitionHeap []partitionCursor

func (items partitionHeap) Len() int { return len(items) }
func (items partitionHeap) Less(left, right int) bool {
	if !items[left].value.EnqueuedAt.Equal(items[right].value.EnqueuedAt) {
		return items[left].value.EnqueuedAt.Before(items[right].value.EnqueuedAt)
	}
	if items[left].value.ID != items[right].value.ID {
		return items[left].value.ID < items[right].value.ID
	}
	return items[left].value.Revision < items[right].value.Revision
}
func (items partitionHeap) Swap(left, right int) {
	items[left], items[right] = items[right], items[left]
}
func (items *partitionHeap) Push(value any) { *items = append(*items, value.(partitionCursor)) }
func (items *partitionHeap) Pop() any {
	previous := *items
	last := len(previous) - 1
	value := previous[last]
	*items = previous[:last]
	return value
}
