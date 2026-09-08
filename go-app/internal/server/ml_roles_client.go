package server

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/umayangag/cric-flow/go-app/internal/auction"
	"github.com/umayangag/cric-flow/go-app/internal/services/predictteam"
)

// Wire shapes for POST /xi/player-roles (app/models/xi.py: PlayerRolesRequest /
// PlayerRolesResponse, P3-1).

type mlPlayerRolesRequest struct {
	Format    string   `json:"format"`
	PlayerIDs []string `json:"player_ids"`
}

type mlPlayerRoles struct {
	PlayerID string   `json:"player_id"`
	Known    bool     `json:"known"`
	Roles    []string `json:"roles"`
}

type mlPlayerRolesResponse struct {
	Format           string          `json:"format"`
	Players          []mlPlayerRoles `json:"players"`
	UnknownPlayerIDs []string        `json:"unknown_player_ids"`
	ServedRatings    mlServedRatings `json:"served_ratings"`
}

// PlayerRolesResult is the role read as this service consumes it: the roles keyed by the
// registry id they were asked under, and the rating state that answered.
type PlayerRolesResult struct {
	Roles  map[string]auction.PlayerRoles
	Served predictteam.ServedRatings
}

// PlayerRoles calls POST /xi/player-roles: what the served as-of vectors say about a list
// of players.
//
// It is the one ml-service call the auction module makes, and it is deliberately not a
// selection: the endpoint evaluates no objective, orders nothing and returns no
// probability. The module never calls `/xi/optimize`, because the record says optimised
// selection in domestic T20 is indistinguishable from rating order (plan §8.8) and the IPL
// is domestic T20 — a test asserts through this client that no auction request reaches it.
//
// An empty id list is answered without a request. ml-service requires at least one id, so
// asking it about nobody would be a 422 for a question with an obvious answer, and an
// auction with an empty list is the state every auction starts in.
func (c *MLClient) PlayerRoles(
	ctx context.Context,
	format string,
	playerKeys []string,
) (*PlayerRolesResult, error) {
	if len(playerKeys) == 0 {
		return &PlayerRolesResult{Roles: map[string]auction.PlayerRoles{}}, nil
	}
	payload, err := json.Marshal(mlPlayerRolesRequest{Format: format, PlayerIDs: playerKeys})
	if err != nil {
		return nil, err
	}
	var out mlPlayerRolesResponse
	if err := c.postJSON(ctx, "/xi/player-roles", payload, &out); err != nil {
		return nil, err
	}
	return &PlayerRolesResult{
		Roles:  playerRolesByKey(out.Players),
		Served: out.ServedRatings.served(),
	}, nil
}

// playerRolesByKey turns ml-service's role names into the two predicates the auction
// module reads.
//
// The names are matched rather than counted: a role this service does not recognise is
// left out of both predicates instead of being folded into one of them, which would be a
// role the model reported and this service silently renamed. The vocabulary is declared
// once in contracts/ops-console.contract.json and asserted from every side (H-24), so a
// value arriving here that matches neither is a contract violation and not a surprise.
func playerRolesByKey(players []mlPlayerRoles) map[string]auction.PlayerRoles {
	roles := make(map[string]auction.PlayerRoles, len(players))
	for _, player := range players {
		read := auction.PlayerRoles{Known: player.Known}
		for _, role := range player.Roles {
			switch role {
			case predictteam.RoleKeeper:
				read.Keeper = true
			case predictteam.RoleBowlingOption:
				read.BowlingOption = true
			}
		}
		roles[player.PlayerID] = read
	}
	return roles
}

// rolesRefusal is why a role read did not happen, as the answer carries it (§8.7).
//
// A refused role read does not refuse the auction: the record is facts the operator
// entered and needs no model to be read back. What it does refuse is every number that
// comes off the model — the pool's distribution and the open slots by role — and this is
// the block that says so by name, so a surface can print the refusal instead of a zero.
type rolesRefusal struct {
	Code    string
	Message string
	Hint    string
}

// refusalFrom names why the role read failed, keeping ml-service's own code where it gave
// one (`RATINGS_STALE`, `XI_MODEL_UNAVAILABLE`, `ML_UNREACHABLE`).
func refusalFrom(err error) rolesRefusal {
	var mlErr *mlServiceError
	if errors.As(err, &mlErr) {
		return rolesRefusal{Code: mlErr.Code, Message: mlErr.Message, Hint: mlErr.Hint}
	}
	return rolesRefusal{
		Code:    "ROLE_READ_FAILED",
		Message: err.Error(),
		Hint:    "the auction record is unaffected; the roles and the counts read off them are not shown",
	}
}
