// Package league models a deterministic closed population that repeatedly plays rated matches.
package league

import (
	"fmt"
	"math"
	"slices"
)

const (
	defaultPopulationSize  = 1000
	defaultInitialRating   = 1500
	defaultRatingK         = 32
	minimumRating          = 100
	maximumRating          = 3000
	ratingHistogramCenter  = 1500
	ratingHistogramStep    = 25
	ratingHistogramMiddle  = 56
	RatingHistogramBuckets = 117
)

var partyPattern = [...]int{2, 1, 1, 1, 3, 2}

// Config controls deterministic population generation and rating updates.
type Config struct {
	Seed           int64
	PopulationSize int
	InitialRating  int
	RatingK        int
}

// DefaultConfig returns the reference 1,000-player league configuration.
func DefaultConfig() Config {
	return Config{
		Seed:           42,
		PopulationSize: defaultPopulationSize,
		InitialRating:  defaultInitialRating,
		RatingK:        defaultRatingK,
	}
}

// Player is the public simulation read model. Hidden true skill is intentionally omitted.
type Player struct {
	ID     string
	Rating int
	Games  int
	Wins   int
}

// RatingHistogram stores exact-center and 25-point rating bands from 100 through 3000.
// Index 56 is rating 1500, lower indices descend from 1475-1499, and higher indices
// ascend from 1501-1525.
type RatingHistogram [RatingHistogramBuckets]int

// Party is a stable group that owns one versioned matchmaking ticket.
type Party struct {
	ID       string
	Revision uint64
	Players  []Player
}

// Stats summarizes the visible rating distribution across the entire population.
type Stats struct {
	Players      int
	Parties      int
	GamesPlayed  int
	Minimum      int
	Percentile10 int
	Median       int
	Percentile90 int
	Maximum      int
	Mean         int
	StdDev       int
	Histogram    [9]int
	// CenteredHistogram preserves a dedicated 1500 bucket for symmetric TUI history.
	CenteredHistogram [9]int
	// RatingHistogram supports a dynamically scaled TUI density axis without exposing player identity.
	RatingHistogram RatingHistogram
}

// Result records one simulated outcome and the per-player Elo movement for each team.
type Result struct {
	MatchID           string
	WinnerTeam        int
	WinnerProbability float64
	TeamRatingBefore  [2]int
	RatingDelta       [2]int
}

type playerState struct {
	Player
	trueSkill int
}

type partyState struct {
	id        string
	revision  uint64
	playerIDs []string
}

// Population owns stable player identity, hidden skill, visible rating, and party membership.
type Population struct {
	configuration Config
	random        generator
	players       map[string]*playerState
	parties       map[string]*partyState
	partyOrder    []string
	gamesPlayed   int
}

// New creates a deterministic population partitioned into the reference mixed-party pattern.
func New(configuration Config) (*Population, error) {
	configuration = normalize(configuration)
	if configuration.Seed < 0 || configuration.PopulationSize < 10 || configuration.InitialRating < minimumRating ||
		configuration.InitialRating > maximumRating || configuration.RatingK <= 0 {
		return nil, fmt.Errorf("league seed, population size, initial rating, or rating K is invalid")
	}
	population := &Population{
		configuration: configuration,
		random:        newGenerator(uint64(configuration.Seed)),
		players:       make(map[string]*playerState, configuration.PopulationSize),
		parties:       make(map[string]*partyState),
	}

	remaining := configuration.PopulationSize
	playerOrdinal := 0
	partyOrdinal := 0
	for remaining > 0 {
		partySize := min(partyPattern[partyOrdinal%len(partyPattern)], remaining)
		partyOrdinal++
		partyID := fmt.Sprintf("flow-ticket-%04d", partyOrdinal)
		party := &partyState{id: partyID, revision: 1, playerIDs: make([]string, 0, partySize)}
		for range partySize {
			playerOrdinal++
			playerID := fmt.Sprintf("flow-player-%05d", playerOrdinal)
			population.players[playerID] = &playerState{
				Player:    Player{ID: playerID, Rating: configuration.InitialRating},
				trueSkill: population.hiddenSkill(),
			}
			party.playerIDs = append(party.playerIDs, playerID)
		}
		population.parties[partyID] = party
		population.partyOrder = append(population.partyOrder, partyID)
		remaining -= partySize
	}
	return population, nil
}

func normalize(configuration Config) Config {
	defaults := DefaultConfig()
	if configuration.PopulationSize == 0 {
		configuration.PopulationSize = defaults.PopulationSize
	}
	if configuration.InitialRating == 0 {
		configuration.InitialRating = defaults.InitialRating
	}
	if configuration.RatingK == 0 {
		configuration.RatingK = defaults.RatingK
	}
	return configuration
}

func (population *Population) hiddenSkill() int {
	// The sum of twelve uniform values is a deterministic normal-like distribution.
	value := 0.0
	for range 12 {
		value += population.random.Float64()
	}
	return clamp(int(math.Round(1500+280*(value-6))), 600, 2400)
}

// Parties returns defensive snapshots in stable ticket order.
func (population *Population) Parties() []Party {
	parties := make([]Party, 0, len(population.partyOrder))
	for _, identifier := range population.partyOrder {
		party, _ := population.Party(identifier)
		parties = append(parties, party)
	}
	return parties
}

// Party returns a defensive snapshot of one stable party.
func (population *Population) Party(identifier string) (Party, bool) {
	state, exists := population.parties[identifier]
	if !exists {
		return Party{}, false
	}
	party := Party{ID: state.id, Revision: state.revision, Players: make([]Player, 0, len(state.playerIDs))}
	for _, playerID := range state.playerIDs {
		party.Players = append(party.Players, population.players[playerID].Player)
	}
	return party, true
}

// Play resolves one 5v5 match from party ticket IDs and updates every participating player.
func (population *Population) Play(matchID string, teams [2][]string) (Result, error) {
	if matchID == "" {
		return Result{}, fmt.Errorf("match identity is required")
	}
	teamPlayers := [2][]*playerState{}
	seen := make(map[string]struct{})
	for teamIndex, partyIDs := range teams {
		for _, partyID := range partyIDs {
			party, exists := population.parties[partyID]
			if !exists {
				return Result{}, fmt.Errorf("party %q does not exist", partyID)
			}
			if _, duplicated := seen[partyID]; duplicated {
				return Result{}, fmt.Errorf("party %q appears more than once", partyID)
			}
			seen[partyID] = struct{}{}
			for _, playerID := range party.playerIDs {
				teamPlayers[teamIndex] = append(teamPlayers[teamIndex], population.players[playerID])
			}
		}
		if len(teamPlayers[teamIndex]) != 5 {
			return Result{}, fmt.Errorf("team %d has %d players; want 5", teamIndex, len(teamPlayers[teamIndex]))
		}
	}

	result := Result{MatchID: matchID}
	teamTrueSkill := [2]int{}
	for teamIndex := range teamPlayers {
		for _, player := range teamPlayers[teamIndex] {
			teamTrueSkill[teamIndex] += player.trueSkill
			result.TeamRatingBefore[teamIndex] += player.Rating
		}
		teamTrueSkill[teamIndex] /= len(teamPlayers[teamIndex])
		result.TeamRatingBefore[teamIndex] /= len(teamPlayers[teamIndex])
	}
	teamZeroWinProbability := winProbability(teamTrueSkill[0], teamTrueSkill[1])
	if population.random.Float64() < teamZeroWinProbability {
		result.WinnerTeam = 0
		result.WinnerProbability = teamZeroWinProbability
	} else {
		result.WinnerTeam = 1
		result.WinnerProbability = 1 - teamZeroWinProbability
	}

	expected := winProbability(result.TeamRatingBefore[0], result.TeamRatingBefore[1])
	actual := 0.0
	if result.WinnerTeam == 0 {
		actual = 1
	}
	delta := int(math.Round(float64(population.configuration.RatingK) * (actual - expected)))
	delta = boundedRatingDelta(delta, teamPlayers)
	result.RatingDelta = [2]int{delta, -delta}
	for teamIndex := range teamPlayers {
		for _, player := range teamPlayers[teamIndex] {
			player.Rating += result.RatingDelta[teamIndex]
			player.Games++
			if teamIndex == result.WinnerTeam {
				player.Wins++
			}
		}
	}
	for partyID := range seen {
		population.parties[partyID].revision++
	}
	population.gamesPlayed++
	return result, nil
}

func boundedRatingDelta(delta int, teams [2][]*playerState) int {
	if delta == 0 {
		return 0
	}
	magnitude := max(delta, -delta)
	for teamIndex, players := range teams {
		increases := (teamIndex == 0 && delta > 0) || (teamIndex == 1 && delta < 0)
		for _, player := range players {
			room := player.Rating - minimumRating
			if increases {
				room = maximumRating - player.Rating
			}
			magnitude = min(magnitude, room)
		}
	}
	if delta < 0 {
		return -magnitude
	}
	return magnitude
}

func winProbability(left, right int) float64 {
	return 1 / (1 + math.Pow(10, float64(right-left)/400))
}

// Stats returns visible distribution evidence without exposing hidden true skill.
func (population *Population) Stats() Stats {
	ratings := make([]int, 0, len(population.players))
	total := 0
	for _, player := range population.players {
		ratings = append(ratings, player.Rating)
		total += player.Rating
	}
	slices.Sort(ratings)
	stats := Stats{
		Players:      len(ratings),
		Parties:      len(population.parties),
		GamesPlayed:  population.gamesPlayed,
		Minimum:      ratings[0],
		Median:       percentile(ratings, 50),
		Maximum:      ratings[len(ratings)-1],
		Mean:         total / len(ratings),
		Percentile10: percentile(ratings, 10),
		Percentile90: percentile(ratings, 90),
	}
	variance := 0.0
	for _, rating := range ratings {
		difference := float64(rating - stats.Mean)
		variance += difference * difference
		stats.Histogram[ratingBucket(rating)]++
		stats.CenteredHistogram[centeredRatingBucket(rating)]++
		stats.RatingHistogram[RatingHistogramBucket(rating)]++
	}
	stats.StdDev = int(math.Round(math.Sqrt(variance / float64(len(ratings)))))
	return stats
}

func percentile(sorted []int, percentage int) int {
	index := (len(sorted) - 1) * percentage / 100
	return sorted[index]
}

func ratingBucket(rating int) int {
	for index, upper := range [...]int{1100, 1250, 1400, 1500, 1600, 1750, 1900, 2100} {
		if rating < upper {
			return index
		}
	}
	return 8
}

func centeredRatingBucket(rating int) int {
	for index, upper := range [...]int{1400, 1450, 1475, 1500, 1501, 1526, 1551, 1601} {
		if rating < upper {
			return index
		}
	}
	return 8
}

// RatingHistogramBucket maps a visible rating into the fine-grained histogram.
func RatingHistogramBucket(rating int) int {
	rating = clamp(rating, minimumRating, maximumRating)
	if rating == ratingHistogramCenter {
		return ratingHistogramMiddle
	}
	if rating < ratingHistogramCenter {
		distance := (ratingHistogramCenter - rating + ratingHistogramStep - 1) / ratingHistogramStep
		return ratingHistogramMiddle - distance
	}
	distance := (rating - ratingHistogramCenter + ratingHistogramStep - 1) / ratingHistogramStep
	return ratingHistogramMiddle + distance
}

// RatingHistogramBounds returns the inclusive rating range represented by a bucket.
func RatingHistogramBounds(index int) (int, int) {
	index = clamp(index, 0, RatingHistogramBuckets-1)
	if index == ratingHistogramMiddle {
		return ratingHistogramCenter, ratingHistogramCenter
	}
	if index < ratingHistogramMiddle {
		distance := ratingHistogramMiddle - index
		lower := ratingHistogramCenter - distance*ratingHistogramStep
		upper := ratingHistogramCenter - (distance-1)*ratingHistogramStep - 1
		return max(minimumRating, lower), upper
	}
	distance := index - ratingHistogramMiddle
	lower := ratingHistogramCenter + (distance-1)*ratingHistogramStep + 1
	upper := ratingHistogramCenter + distance*ratingHistogramStep
	return lower, min(maximumRating, upper)
}

func clamp(value, minimum, maximum int) int {
	return min(maximum, max(minimum, value))
}

type generator struct {
	state uint64
}

func newGenerator(seed uint64) generator {
	return generator{state: seed ^ 0x9e3779b97f4a7c15}
}

func (generator *generator) Uint64() uint64 {
	generator.state += 0x9e3779b97f4a7c15
	value := generator.state
	value = (value ^ (value >> 30)) * 0xbf58476d1ce4e5b9
	value = (value ^ (value >> 27)) * 0x94d049bb133111eb
	return value ^ (value >> 31)
}

func (generator *generator) Float64() float64 {
	return float64(generator.Uint64()>>11) * (1.0 / (1 << 53))
}
