package blizzard

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

// characterPath builds /profile/wow/character/{realm}/{name}{suffix}.
func characterPath(realm, name, suffix string) (string, error) {
	if realm == "" || name == "" {
		return "", fmt.Errorf("realm and character name are required")
	}
	return fmt.Sprintf("/profile/wow/character/%s/%s%s",
		url.PathEscape(RealmSlug(realm)), url.PathEscape(strings.ToLower(name)), suffix), nil
}

func isNotFound(err error) bool {
	var apiErr *APIError
	return errors.As(err, &apiErr) && apiErr.Status == http.StatusNotFound
}

func formatMillis(ms int64) string {
	if ms <= 0 {
		return ""
	}
	return time.UnixMilli(ms).UTC().Format("2006-01-02 15:04 UTC")
}

// ---- Dungeon and raid encounters ------------------------------------------

// EncounterProgress is a condensed view of a character's dungeon or raid kills.
type EncounterProgress struct {
	Character  string              `json:"character"`
	Kind       string              `json:"kind"`
	Expansions []ExpansionProgress `json:"expansions"`
	Note       string              `json:"note,omitempty"`
}

// ExpansionProgress groups instance progress by expansion.
type ExpansionProgress struct {
	ID        int                `json:"id"`
	Name      string             `json:"name"`
	Instances []InstanceProgress `json:"instances"`
}

// InstanceProgress is one dungeon or raid with progress per difficulty.
type InstanceProgress struct {
	ID    int            `json:"id"`
	Name  string         `json:"name"`
	Modes []ModeProgress `json:"modes"`
}

// ModeProgress is progress on one difficulty of an instance.
type ModeProgress struct {
	Difficulty string     `json:"difficulty"`
	Status     string     `json:"status"`
	Progress   string     `json:"progress"` // "3/7" bosses
	Bosses     []BossKill `json:"bosses,omitempty"`
}

// BossKill is a boss's kill count and most recent kill.
type BossKill struct {
	Name     string `json:"name"`
	Kills    int    `json:"kills"`
	LastKill string `json:"last_kill,omitempty"`
}

// Encounters returns a character's dungeon (kind "dungeons") or raid
// (kind "raids") progress. expansion, if set, keeps only expansions whose name
// contains it (case-insensitive, e.g. "midnight" or "current season"); boss-level
// detail is included only when filtering, to keep the full history small.
func (c *Client) Encounters(ctx context.Context, realm, name, kind, expansion string) (EncounterProgress, error) {
	kind = strings.ToLower(strings.TrimSpace(kind))
	switch kind {
	case "", "dungeon", "dungeons":
		kind = "dungeons"
	case "raid", "raids":
		kind = "raids"
	default:
		return EncounterProgress{}, fmt.Errorf("kind must be dungeons or raids, got %q", kind)
	}
	path, err := characterPath(realm, name, "/encounters/"+kind)
	if err != nil {
		return EncounterProgress{}, err
	}
	body, err := c.Get(ctx, path, Profile, nil)
	if err != nil {
		return EncounterProgress{}, err
	}
	var res struct {
		Character struct {
			Name string `json:"name"`
		} `json:"character"`
		Expansions []struct {
			Expansion struct {
				ID   int    `json:"id"`
				Name string `json:"name"`
			} `json:"expansion"`
			Instances []struct {
				Instance struct {
					ID   int    `json:"id"`
					Name string `json:"name"`
				} `json:"instance"`
				Modes []struct {
					Difficulty struct {
						Name string `json:"name"`
					} `json:"difficulty"`
					Status struct {
						Name string `json:"name"`
					} `json:"status"`
					Progress struct {
						CompletedCount int `json:"completed_count"`
						TotalCount     int `json:"total_count"`
						Encounters     []struct {
							CompletedCount int `json:"completed_count"`
							Encounter      struct {
								Name string `json:"name"`
							} `json:"encounter"`
							LastKillTimestamp int64 `json:"last_kill_timestamp"`
						} `json:"encounters"`
					} `json:"progress"`
				} `json:"modes"`
			} `json:"instances"`
		} `json:"expansions"`
	}
	if err := json.Unmarshal(body, &res); err != nil {
		return EncounterProgress{}, fmt.Errorf("encounters response: %w", err)
	}

	filter := strings.ToLower(strings.TrimSpace(expansion))
	out := EncounterProgress{Character: res.Character.Name, Kind: kind, Expansions: []ExpansionProgress{}}
	if out.Character == "" {
		out.Character = name // the dungeons response omits the character block
	}
	var available []string
	for _, e := range res.Expansions {
		available = append(available, e.Expansion.Name)
		if filter != "" && !strings.Contains(strings.ToLower(e.Expansion.Name), filter) {
			continue
		}
		ep := ExpansionProgress{ID: e.Expansion.ID, Name: e.Expansion.Name}
		for _, in := range e.Instances {
			ip := InstanceProgress{ID: in.Instance.ID, Name: in.Instance.Name}
			for _, m := range in.Modes {
				if m.Difficulty.Name == "" {
					continue // the API repeats some modes without a difficulty
				}
				mp := ModeProgress{
					Difficulty: m.Difficulty.Name,
					Status:     m.Status.Name,
					Progress:   fmt.Sprintf("%d/%d", m.Progress.CompletedCount, m.Progress.TotalCount),
				}
				if filter != "" {
					for _, b := range m.Progress.Encounters {
						mp.Bosses = append(mp.Bosses, BossKill{Name: b.Encounter.Name, Kills: b.CompletedCount, LastKill: formatMillis(b.LastKillTimestamp)})
					}
				}
				ip.Modes = append(ip.Modes, mp)
			}
			ep.Instances = append(ep.Instances, ip)
		}
		out.Expansions = append(out.Expansions, ep)
	}
	// Newest expansions (highest IDs) first.
	sort.SliceStable(out.Expansions, func(i, j int) bool { return out.Expansions[i].ID > out.Expansions[j].ID })
	switch {
	case filter != "" && len(out.Expansions) == 0:
		out.Note = fmt.Sprintf("no %s progress matches %q; expansions with progress: %s", kind, expansion, strings.Join(available, ", "))
	case filter == "":
		out.Note = "Boss-level detail omitted; filter by expansion to see individual bosses and kill dates."
	}
	return out, nil
}

// ---- Mythic+ seasons --------------------------------------------------------

// MythicSeason is a character's Mythic+ summary for one season.
type MythicSeason struct {
	Character  string      `json:"character"`
	SeasonID   int         `json:"season_id"`
	SeasonName string      `json:"season_name,omitempty"`
	Started    string      `json:"season_started,omitempty"`
	Ended      string      `json:"season_ended,omitempty"`
	Rating     float64     `json:"mythic_rating"`
	Runs       []MythicRun `json:"best_runs"`
	ThisWeek   []MythicRun `json:"this_week_best_runs,omitempty"`
	Note       string      `json:"note,omitempty"`
}

// MythicRun is one best run.
type MythicRun struct {
	Dungeon   string   `json:"dungeon"`
	Level     int      `json:"keystone_level"`
	Timed     bool     `json:"timed"`
	Duration  string   `json:"duration"`
	Rating    float64  `json:"rating"`
	Affixes   []string `json:"affixes,omitempty"`
	Completed string   `json:"completed"`
	Party     []string `json:"party,omitempty"` // "Name (Spec, ilvl)"
}

type apiRun struct {
	CompletedTimestamp int64 `json:"completed_timestamp"`
	Dungeon            struct {
		Name string `json:"name"`
	} `json:"dungeon"`
	Duration      int64 `json:"duration"`
	Timed         bool  `json:"is_completed_within_time"`
	KeystoneLevel int   `json:"keystone_level"`
	Affixes       []struct {
		Name string `json:"name"`
	} `json:"keystone_affixes"`
	MapRating struct {
		Rating float64 `json:"rating"`
	} `json:"map_rating"`
	Members []struct {
		Character struct {
			Name string `json:"name"`
		} `json:"character"`
		Spec struct {
			Name string `json:"name"`
		} `json:"specialization"`
		ItemLevel int `json:"equipped_item_level"`
	} `json:"members"`
}

func (r apiRun) condense() MythicRun {
	d := time.Duration(r.Duration) * time.Millisecond
	run := MythicRun{
		Dungeon:   r.Dungeon.Name,
		Level:     r.KeystoneLevel,
		Timed:     r.Timed,
		Duration:  fmt.Sprintf("%d:%02d", int(d.Minutes()), int(d.Seconds())%60),
		Rating:    round1(r.MapRating.Rating),
		Completed: formatMillis(r.CompletedTimestamp),
	}
	for _, a := range r.Affixes {
		run.Affixes = append(run.Affixes, a.Name)
	}
	for _, m := range r.Members {
		run.Party = append(run.Party, fmt.Sprintf("%s (%s, %d)", m.Character.Name, m.Spec.Name, m.ItemLevel))
	}
	return run
}

func condenseRuns(runs []apiRun) []MythicRun {
	out := make([]MythicRun, 0, len(runs))
	for _, r := range runs {
		out = append(out, r.condense())
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Rating > out[j].Rating })
	return out
}

func round1(f float64) float64 { return float64(int(f*10+0.5)) / 10 }

// MythicKeystoneSeason returns a character's Mythic+ rating and best runs for
// a season (seasonID 0 means the current season), plus the season's name and
// dates. A character with no runs gets an empty result, not an error.
func (c *Client) MythicKeystoneSeason(ctx context.Context, realm, name string, seasonID int) (MythicSeason, error) {
	base, err := characterPath(realm, name, "/mythic-keystone-profile")
	if err != nil {
		return MythicSeason{}, err
	}
	// The profile 404s only if the character itself is missing.
	body, err := c.Get(ctx, base, Profile, nil)
	if err != nil {
		return MythicSeason{}, err
	}
	var prof struct {
		Character struct {
			Name string `json:"name"`
		} `json:"character"`
		CurrentPeriod struct {
			BestRuns []apiRun `json:"best_runs"`
		} `json:"current_period"`
	}
	if err := json.Unmarshal(body, &prof); err != nil {
		return MythicSeason{}, fmt.Errorf("mythic keystone profile: %w", err)
	}
	out := MythicSeason{Character: prof.Character.Name, Runs: []MythicRun{}}

	current := false
	if seasonID <= 0 {
		idx, err := c.get(ctx, "/data/wow/mythic-keystone/season/index", Dynamic, nil, time.Hour)
		if err != nil {
			return MythicSeason{}, err
		}
		var si struct {
			Current struct {
				ID int `json:"id"`
			} `json:"current_season"`
		}
		if err := json.Unmarshal(idx, &si); err != nil || si.Current.ID == 0 {
			return MythicSeason{}, fmt.Errorf("could not determine the current Mythic+ season")
		}
		seasonID, current = si.Current.ID, true
	}
	out.SeasonID = seasonID

	// Season metadata (name, dates) is game data and exists even with no runs.
	if sb, err := c.get(ctx, "/data/wow/mythic-keystone/season/"+strconv.Itoa(seasonID), Dynamic, nil, time.Hour); err == nil {
		var s struct {
			Name  string `json:"season_name"`
			Start int64  `json:"start_timestamp"`
			End   int64  `json:"end_timestamp"`
		}
		if json.Unmarshal(sb, &s) == nil {
			out.SeasonName, out.Started, out.Ended = s.Name, formatMillis(s.Start), formatMillis(s.End)
		}
	} else if isNotFound(err) {
		return MythicSeason{}, fmt.Errorf("Mythic+ season %d does not exist", seasonID)
	}
	if current {
		out.ThisWeek = condenseRuns(prof.CurrentPeriod.BestRuns)
	}

	sb, err := c.Get(ctx, fmt.Sprintf("%s/season/%d", base, seasonID), Profile, nil)
	if isNotFound(err) {
		out.Note = fmt.Sprintf("%s has no Mythic+ runs recorded in season %d.", out.Character, seasonID)
		return out, nil
	}
	if err != nil {
		return MythicSeason{}, err
	}
	var season struct {
		BestRuns []apiRun `json:"best_runs"`
		Rating   struct {
			Rating float64 `json:"rating"`
		} `json:"mythic_rating"`
	}
	if err := json.Unmarshal(sb, &season); err != nil {
		return MythicSeason{}, fmt.Errorf("mythic keystone season: %w", err)
	}
	out.Rating = round1(season.Rating.Rating)
	out.Runs = condenseRuns(season.BestRuns)
	return out, nil
}

// ---- Profession recipes known vs. missing ----------------------------------

// RecipeProgress compares a character's known recipes with every recipe in one
// profession skill tier (e.g. Midnight Tailoring).
type RecipeProgress struct {
	Character  string           `json:"character"`
	Profession string           `json:"profession"`
	Tier       string           `json:"tier"`
	TierID     int              `json:"tier_id"`
	Skill      string           `json:"skill"`
	Known      int              `json:"known"`
	Total      int              `json:"total"`
	Missing    []RecipeCategory `json:"missing"`
	KnownList  []RecipeCategory `json:"known_recipes,omitempty"`
	OtherTiers []string         `json:"other_tiers,omitempty"`
}

// RecipeCategory is a recipe category with the recipes in it.
type RecipeCategory struct {
	Category string      `json:"category"`
	Recipes  []RecipeRef `json:"recipes"`
}

// RecipeRef is a recipe's ID and name.
type RecipeRef struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// ProfessionRecipes reports which recipes of a profession tier a character
// knows and which are missing. profession is a name ("tailoring") or numeric
// ID; tierID 0 picks the character's newest tier of that profession.
// includeKnown adds the known recipes grouped by category.
func (c *Client) ProfessionRecipes(ctx context.Context, realm, name, profession string, tierID int, includeKnown bool) (RecipeProgress, error) {
	path, err := characterPath(realm, name, "/professions")
	if err != nil {
		return RecipeProgress{}, err
	}
	body, err := c.Get(ctx, path, Profile, nil)
	if err != nil {
		return RecipeProgress{}, err
	}
	type tier struct {
		Tier struct {
			ID   int    `json:"id"`
			Name string `json:"name"`
		} `json:"tier"`
		SkillPoints  int         `json:"skill_points"`
		MaxSkill     int         `json:"max_skill_points"`
		KnownRecipes []RecipeRef `json:"known_recipes"`
	}
	type prof struct {
		Profession struct {
			ID   int    `json:"id"`
			Name string `json:"name"`
		} `json:"profession"`
		Tiers []tier `json:"tiers"`
	}
	var res struct {
		Character struct {
			Name string `json:"name"`
		} `json:"character"`
		Primaries   []prof `json:"primaries"`
		Secondaries []prof `json:"secondaries"`
	}
	if err := json.Unmarshal(body, &res); err != nil {
		return RecipeProgress{}, fmt.Errorf("professions response: %w", err)
	}

	want := strings.ToLower(strings.TrimSpace(profession))
	wantID, _ := strconv.Atoi(want)
	var match *prof
	var have []string
	for _, p := range append(res.Primaries, res.Secondaries...) {
		if len(p.Tiers) == 0 {
			continue // e.g. Archaeology has no recipe tiers
		}
		have = append(have, p.Profession.Name)
		if (wantID != 0 && p.Profession.ID == wantID) || (wantID == 0 && strings.ToLower(p.Profession.Name) == want) {
			match = &p
			break
		}
	}
	if match == nil {
		return RecipeProgress{}, fmt.Errorf("%s has no %q profession with recipes (has: %s)", res.Character.Name, profession, strings.Join(have, ", "))
	}

	var t *tier
	var others []string
	for i := range match.Tiers {
		mt := &match.Tiers[i]
		if (tierID != 0 && mt.Tier.ID == tierID) || (tierID == 0 && (t == nil || mt.Tier.ID > t.Tier.ID)) {
			t = mt
		}
	}
	if t == nil {
		for _, mt := range match.Tiers {
			others = append(others, fmt.Sprintf("%s (%d)", mt.Tier.Name, mt.Tier.ID))
		}
		return RecipeProgress{}, fmt.Errorf("%s has not learned skill tier %d of %s (has: %s)", res.Character.Name, tierID, match.Profession.Name, strings.Join(others, ", "))
	}
	for _, mt := range match.Tiers {
		if mt.Tier.ID != t.Tier.ID {
			others = append(others, fmt.Sprintf("%s (%d)", mt.Tier.Name, mt.Tier.ID))
		}
	}

	tb, err := c.Profession(ctx, match.Profession.ID, t.Tier.ID)
	if err != nil {
		return RecipeProgress{}, err
	}
	var st struct {
		Categories []struct {
			Name    string      `json:"name"`
			Recipes []RecipeRef `json:"recipes"`
		} `json:"categories"`
	}
	if err := json.Unmarshal(tb, &st); err != nil {
		return RecipeProgress{}, fmt.Errorf("skill tier response: %w", err)
	}

	known := make(map[int]bool, len(t.KnownRecipes))
	for _, r := range t.KnownRecipes {
		known[r.ID] = true
	}
	out := RecipeProgress{
		Character:  res.Character.Name,
		Profession: match.Profession.Name,
		Tier:       t.Tier.Name,
		TierID:     t.Tier.ID,
		Skill:      fmt.Sprintf("%d/%d", t.SkillPoints, t.MaxSkill),
		Missing:    []RecipeCategory{},
		OtherTiers: others,
	}
	for _, cat := range st.Categories {
		var miss, got []RecipeRef
		for _, r := range cat.Recipes {
			out.Total++
			if known[r.ID] {
				out.Known++
				got = append(got, r)
			} else {
				miss = append(miss, r)
			}
		}
		if len(miss) > 0 {
			out.Missing = append(out.Missing, RecipeCategory{Category: cat.Name, Recipes: miss})
		}
		if includeKnown && len(got) > 0 {
			out.KnownList = append(out.KnownList, RecipeCategory{Category: cat.Name, Recipes: got})
		}
	}
	return out, nil
}
