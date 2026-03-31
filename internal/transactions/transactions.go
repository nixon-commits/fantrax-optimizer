package transactions

import (
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	"github.com/nixon-commits/rosterbot/internal/hkb"
	"github.com/nixon-commits/rosterbot/internal/notify"
	"github.com/pmurley/go-fantrax/models"
)

// TradeClient is the subset of fantrax.Client needed for trade fetching.
type TradeClient interface {
	GetRecentTrades(since time.Time) ([]models.Transaction, error)
}

// TradeSide represents one team's side of a trade.
type TradeSide struct {
	TeamName string
	Players  []TradePlayer
	Total    int
}

// TradePlayer is a player involved in a trade with their HKB value.
type TradePlayer struct {
	Name     string
	Position string
	Value    int  // HKB value (0 if unranked)
	Ranked   bool // true if found in HKB
}

// Trade represents a grouped trade between two teams.
type Trade struct {
	ProcessedDate time.Time
	Sides         [2]TradeSide
}

// CheckTrades fetches recent trades, values them via HKB, and sends a notification.
func CheckTrades(ft TradeClient, cacheDir string, pushoverUserKey, pushoverAPIToken string, dryRun bool) error {
	since := time.Now().Add(-48 * time.Hour) // TODO: revert to -24h after testing
	txs, err := ft.GetRecentTrades(since)
	if err != nil {
		return fmt.Errorf("get recent trades: %w", err)
	}

	if len(txs) == 0 {
		log.Println("No trades in the last 24 hours.")
		return nil
	}

	players, err := hkb.GetPlayers(cacheDir)
	if err != nil {
		return fmt.Errorf("get HKB players: %w", err)
	}
	lookup := buildHKBLookup(players)

	trades := groupTrades(txs, lookup)

	report := formatReport(trades, true)
	fmt.Println(report)

	if dryRun {
		return nil
	}

	if pushoverUserKey == "" || pushoverAPIToken == "" {
		log.Println("Pushover credentials not set, skipping notification.")
		return nil
	}

	plain := formatReport(trades, false)
	if err := notify.SendPushover(pushoverUserKey, pushoverAPIToken, "Trade Alert", plain); err != nil {
		log.Printf("notification failed: %v", err)
	}
	return nil
}

// buildHKBLookup creates a map from normalized player name to HKB player.
func buildHKBLookup(players []hkb.Player) map[string]hkb.Player {
	m := make(map[string]hkb.Player, len(players))
	for _, p := range players {
		m[normalizeName(p.Name)] = p
	}
	return m
}

// groupTrades groups transaction rows by TradeGroupID into Trade structs.
func groupTrades(txs []models.Transaction, lookup map[string]hkb.Player) []Trade {
	groups := make(map[string][]models.Transaction)
	for _, tx := range txs {
		groups[tx.TradeGroupID] = append(groups[tx.TradeGroupID], tx)
	}

	var trades []Trade
	for _, group := range groups {
		t := buildTrade(group, lookup)
		trades = append(trades, t)
	}
	sort.Slice(trades, func(i, j int) bool {
		return trades[i].ProcessedDate.Before(trades[j].ProcessedDate)
	})
	return trades
}

// buildTrade constructs a Trade from a group of transactions sharing the same TradeGroupID.
func buildTrade(group []models.Transaction, lookup map[string]hkb.Player) Trade {
	// Partition by direction: players moving to each team.
	sides := make(map[string]*TradeSide)
	var processedDate time.Time

	for _, tx := range group {
		if tx.ProcessedDate.After(processedDate) {
			processedDate = tx.ProcessedDate
		}

		// ToTeamName receives the player.
		key := tx.ToTeamName
		side, ok := sides[key]
		if !ok {
			side = &TradeSide{TeamName: tx.ToTeamName}
			sides[key] = side
		}

		tp := TradePlayer{
			Name:     tx.PlayerName,
			Position: tx.PlayerPosition,
		}
		if hkbPlayer, found := lookup[normalizeName(tx.PlayerName)]; found {
			tp.Value = hkbPlayer.Value
			tp.Ranked = true
		}
		side.Players = append(side.Players, tp)
		side.Total += tp.Value
	}

	trade := Trade{ProcessedDate: processedDate}
	i := 0
	for _, side := range sides {
		if i < 2 {
			trade.Sides[i] = *side
			i++
		}
	}
	return trade
}

const (
	colorRed   = "\033[31m"
	colorGreen = "\033[32m"
	colorReset = "\033[0m"
)

// formatReport produces a human-readable trade report. When color is true,
// ANSI escape codes highlight the winning (green) and losing (red) side.
func formatReport(trades []Trade, color bool) string {
	var b strings.Builder
	for i, t := range trades {
		if i > 0 {
			b.WriteString("\n")
		}
		fmt.Fprintf(&b, "Trade: %s <-> %s\n", t.Sides[0].TeamName, t.Sides[1].TeamName)
		for si, side := range t.Sides {
			other := t.Sides[1-si]
			diff := side.Total - other.Total
			var clr string
			if color {
				switch {
				case diff > 0:
					clr = colorGreen
				case diff < 0:
					clr = colorRed
				}
			}

			fmt.Fprintf(&b, "  %s receives: ", side.TeamName)
			for j, p := range side.Players {
				if j > 0 {
					b.WriteString(", ")
				}
				if p.Ranked {
					fmt.Fprintf(&b, "%s (%s) %s", p.Name, p.Position, formatValue(p.Value))
				} else {
					fmt.Fprintf(&b, "%s (%s) unranked", p.Name, p.Position)
				}
			}
			diffSign := "+"
			absDiff := diff
			if diff < 0 {
				diffSign = "-"
				absDiff = -diff
			}
			reset := ""
			if clr != "" {
				reset = colorReset
			}
			if diff != 0 {
				fmt.Fprintf(&b, " = %s%s (%s%s)%s\n", clr, formatValue(side.Total), diffSign, formatValue(absDiff), reset)
			} else {
				fmt.Fprintf(&b, " = %s\n", formatValue(side.Total))
			}
		}
	}
	return b.String()
}

// formatValue formats an HKB value integer with comma separators.
func formatValue(v int) string {
	s := fmt.Sprintf("%d", v)
	if len(s) <= 3 {
		return s
	}
	var parts []string
	for len(s) > 3 {
		parts = append([]string{s[len(s)-3:]}, parts...)
		s = s[:len(s)-3]
	}
	parts = append([]string{s}, parts...)
	return strings.Join(parts, ",")
}

// normalizeName lowercases and strips common suffixes for name matching.
func normalizeName(name string) string {
	n := strings.ToLower(strings.TrimSpace(name))
	for _, suffix := range []string{" jr.", " sr.", " iv", " iii", " ii"} {
		n = strings.TrimSuffix(n, suffix)
	}
	return strings.TrimSpace(n)
}
