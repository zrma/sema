// Package constraint evaluates matchmaking rules that cannot be relaxed by scoring.
package constraint

import "sema/internal/domain"

// TicketAllowed checks party capacity and the absolute latency cap.
func TicketAllowed(ticket domain.MatchTicket, maxPartySize int, maxLatencyMillis int) bool {
	if len(ticket.Players) > maxPartySize {
		return false
	}
	for _, player := range ticket.Players {
		if player.LatencyMillis > maxLatencyMillis {
			return false
		}
	}
	return true
}

// HardViolation reports placement rules that no relaxation step may waive.
func HardViolation(
	teams [][]domain.MatchTicket,
	policy domain.MatchmakingPolicy,
	kind domain.ProposalKind,
) bool {
	hasHardRole := false
	if kind == domain.ProposalNewMatch {
		for _, requirement := range policy.RoleRequirements {
			if requirement.Hard {
				hasHardRole = true
				break
			}
		}
	}
	for _, tickets := range teams {
		var roleCounts map[string]int
		if hasHardRole {
			roleCounts = make(map[string]int)
		}
		for _, ticket := range tickets {
			for _, player := range ticket.Players {
				if player.LatencyMillis > policy.MaxLatencyMillis {
					return true
				}
				if roleCounts != nil {
					roleCounts[player.Role]++
				}
			}
		}
		if !hasHardRole {
			continue
		}
		for _, requirement := range policy.RoleRequirements {
			if requirement.Hard && roleCounts[requirement.Role] < requirement.MinPerTeam {
				return true
			}
		}
	}
	return false
}
