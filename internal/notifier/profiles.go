package notifier

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/araujofrancisco/certwatch/internal/config"
)

var sendAtRE = regexp.MustCompile(`^([01]\d|2[0-3]):[0-5]\d$`)

func ValidateProfiles(profiles []config.ProfileConfig) error {
	seen := make(map[string]bool)
	for _, p := range profiles {
		if p.Name == "" {
			return fmt.Errorf("profile name is required")
		}
		if seen[p.Name] {
			return fmt.Errorf("duplicate profile name: %s", p.Name)
		}
		seen[p.Name] = true

		if len(p.Recipients) == 0 {
			return fmt.Errorf("profile %q: at least one recipient required", p.Name)
		}
		for _, r := range p.Recipients {
			if !strings.Contains(r, "@") {
				return fmt.Errorf("profile %q: invalid recipient email: %s", p.Name, r)
			}
		}

		switch p.Type {
		case "immediate":
			if err := validateThresholds(p.Thresholds); err != nil {
				return fmt.Errorf("profile %q: %w", p.Name, err)
			}
		case "daily-digest":
			if p.SendAt == "" {
				return fmt.Errorf("profile %q: send_at required for daily-digest", p.Name)
			}
			if !sendAtRE.MatchString(p.SendAt) {
				return fmt.Errorf("profile %q: invalid send_at format %q (expected HH:MM)", p.Name, p.SendAt)
			}
		case "weekly-digest":
			if p.SendAt == "" {
				return fmt.Errorf("profile %q: send_at required for weekly-digest", p.Name)
			}
			if !sendAtRE.MatchString(p.SendAt) {
				return fmt.Errorf("profile %q: invalid send_at format %q (expected HH:MM)", p.Name, p.SendAt)
			}
			if p.Day == "" {
				return fmt.Errorf("profile %q: day required for weekly-digest", p.Name)
			}
		default:
			return fmt.Errorf("profile %q: unknown type %q", p.Name, p.Type)
		}
	}
	return nil
}

func validateThresholds(thresholds []int) error {
	if len(thresholds) == 0 {
		return fmt.Errorf("thresholds required for immediate profile")
	}
	for i, t := range thresholds {
		if t <= 0 {
			return fmt.Errorf("threshold %d: must be positive", t)
		}
		if i > 0 && t >= thresholds[i-1] {
			return fmt.Errorf("thresholds must be descending, got %d after %d", t, thresholds[i-1])
		}
	}
	return nil
}

func DefaultCron(profile config.ProfileConfig) string {
	if profile.Cron != "" {
		return profile.Cron
	}
	switch profile.Type {
	case "daily-digest":
		if profile.SendAt != "" {
			parts := strings.SplitN(profile.SendAt, ":", 2)
			return fmt.Sprintf("%s %s * * *", parts[1], parts[0])
		}
		return "0 8 * * *"
	case "weekly-digest":
		dayNum := weekdayNumber(profile.Day)
		if profile.SendAt != "" {
			parts := strings.SplitN(profile.SendAt, ":", 2)
			return fmt.Sprintf("%s %s * * %d", parts[1], parts[0], dayNum)
		}
		return fmt.Sprintf("0 9 * * %d", dayNum)
	default:
		return ""
	}
}

func weekdayNumber(day string) int {
	days := map[string]int{
		"sunday": 0, "monday": 1, "tuesday": 2, "wednesday": 3,
		"thursday": 4, "friday": 5, "saturday": 6,
	}
	if v, ok := days[strings.ToLower(day)]; ok {
		return v
	}
	return 1
}

func FilterEnabled(profiles []config.ProfileConfig) []config.ProfileConfig {
	var enabled []config.ProfileConfig
	for _, p := range profiles {
		if p.Enabled {
			enabled = append(enabled, p)
		}
	}
	return enabled
}

func ImmediateThresholds() []int {
	return []int{30, 14, 7, 3, 1}
}

func SortThresholds(thresholds []int) {
	sort.Sort(sort.Reverse(sort.IntSlice(thresholds)))
}
