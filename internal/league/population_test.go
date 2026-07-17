package league

import (
	"reflect"
	"testing"
)

func TestDefaultPopulationStartsAsOneThousandPlayerMixedPartyLeague(t *testing.T) {
	population, err := New(DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	stats := population.Stats()
	if stats.Players != 1000 || stats.Parties != 600 {
		t.Fatalf("population = %d players / %d parties; want 1000 / 600", stats.Players, stats.Parties)
	}
	if stats.Minimum != 1500 || stats.Median != 1500 || stats.Maximum != 1500 || stats.StdDev != 0 {
		t.Fatalf("initial rating distribution = %#v", stats)
	}
	if stats.CenteredHistogram[4] != 1000 {
		t.Fatalf("centered rating histogram = %#v", stats.CenteredHistogram)
	}
	parties := population.Parties()
	for index, expected := range []int{2, 1, 1, 1, 3, 2} {
		if len(parties[index].Players) != expected {
			t.Fatalf("party %d size = %d; want %d", index, len(parties[index].Players), expected)
		}
	}
}

func TestMatchResultAndRatingEvolutionAreDeterministicAndZeroSum(t *testing.T) {
	configuration := DefaultConfig()
	configuration.PopulationSize = 10
	first, err := New(configuration)
	if err != nil {
		t.Fatal(err)
	}
	second, err := New(configuration)
	if err != nil {
		t.Fatal(err)
	}
	teams := [2][]string{
		{"flow-ticket-0001", "flow-ticket-0002", "flow-ticket-0003", "flow-ticket-0004"},
		{"flow-ticket-0005", "flow-ticket-0006"},
	}
	left, err := first.Play("match-1", teams)
	if err != nil {
		t.Fatal(err)
	}
	right, err := second.Play("match-1", teams)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(left, right) {
		t.Fatalf("deterministic results differ:\nleft=%#v\nright=%#v", left, right)
	}
	if left.RatingDelta[0]+left.RatingDelta[1] != 0 || left.RatingDelta[left.WinnerTeam] <= 0 {
		t.Fatalf("rating delta = %#v, winner = %d", left.RatingDelta, left.WinnerTeam)
	}
	stats := first.Stats()
	if stats.GamesPlayed != 1 || stats.Minimum >= stats.Maximum || stats.StdDev == 0 {
		t.Fatalf("rating did not spread after a game: %#v", stats)
	}
	if stats.CenteredHistogram[3] != 5 || stats.CenteredHistogram[5] != 5 {
		t.Fatalf("rating did not spread symmetrically around 1500: %#v", stats.CenteredHistogram)
	}
	for _, party := range first.Parties() {
		if party.Revision != 2 {
			t.Fatalf("party %s revision = %d; want 2", party.ID, party.Revision)
		}
		for _, player := range party.Players {
			if player.Games != 1 {
				t.Fatalf("player %s games = %d; want 1", player.ID, player.Games)
			}
		}
	}
}

func TestPlayRejectsIncompleteTeam(t *testing.T) {
	configuration := DefaultConfig()
	configuration.PopulationSize = 10
	population, err := New(configuration)
	if err != nil {
		t.Fatal(err)
	}
	_, err = population.Play("bad-match", [2][]string{{"flow-ticket-0001"}, {"flow-ticket-0005", "flow-ticket-0006"}})
	if err == nil {
		t.Fatal("incomplete team was accepted")
	}
}

func TestWinProbabilityFavorsTheStrongerTrueSkillTeam(t *testing.T) {
	if probability := winProbability(1500, 1500); probability != 0.5 {
		t.Fatalf("equal-skill probability = %f; want 0.5", probability)
	}
	if stronger, weaker := winProbability(1700, 1300), winProbability(1300, 1700); stronger <= 0.9 || weaker >= 0.1 {
		t.Fatalf("skill-sensitive probabilities = stronger %f, weaker %f", stronger, weaker)
	}
}

func TestRatingDeltaPreservesBoundsAndZeroSum(t *testing.T) {
	teams := [2][]*playerState{
		{{Player: Player{Rating: maximumRating}}},
		{{Player: Player{Rating: minimumRating}}},
	}
	if delta := boundedRatingDelta(16, teams); delta != 0 {
		t.Fatalf("bounded delta = %d; want 0", delta)
	}
	teams[0][0].Rating = 2995
	teams[1][0].Rating = 105
	if delta := boundedRatingDelta(16, teams); delta != 5 {
		t.Fatalf("bounded delta = %d; want 5", delta)
	}
}
