package ace

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
)

type Playbook struct {
	Version     string     `json:"version,omitempty"`
	LastUpdated string     `json:"last_updated,omitempty"`
	KeyPoints   []KeyPoint `json:"key_points,omitempty"`
}

type KeyPoint struct {
	Name            string   `json:"name,omitempty"`
	Text            string   `json:"text,omitempty"`
	Tags            []string `json:"tags,omitempty"`
	Score           int      `json:"score,omitempty"`
	Pending         bool     `json:"pending,omitempty"`
	EffectRating    *float64 `json:"effect_rating,omitempty"`
	RiskLevel       *float64 `json:"risk_level,omitempty"`
	InnovationLevel *float64 `json:"innovation_level,omitempty"`
}

// UnmarshalJSON keeps compatibility with existing ACE playbooks where
// `key_points` may contain strings, objects, or divider entries.
func (p *Playbook) UnmarshalJSON(bts []byte) error {
	type rawPlaybook struct {
		Version     string          `json:"version,omitempty"`
		LastUpdated string          `json:"last_updated,omitempty"`
		KeyPoints   json.RawMessage `json:"key_points,omitempty"`
	}
	var raw rawPlaybook
	if err := json.Unmarshal(bts, &raw); err != nil {
		return err
	}

	p.Version = raw.Version
	p.LastUpdated = raw.LastUpdated
	if p.Version == "" {
		p.Version = "1.0"
	}

	if len(bytes.TrimSpace(raw.KeyPoints)) == 0 {
		p.KeyPoints = []KeyPoint{}
		return nil
	}

	var arr []any
	if err := json.Unmarshal(raw.KeyPoints, &arr); err != nil {
		return err
	}

	keyPoints := make([]KeyPoint, 0, len(arr))
	for _, item := range arr {
		switch v := item.(type) {
		case string:
			text := strings.TrimSpace(v)
			if text == "" {
				continue
			}
			keyPoints = append(keyPoints, KeyPoint{Text: text})
		case map[string]any:
			// Skip divider-like entries.
			if isTruthy(v["divider"]) {
				continue
			}

			kp, ok := parseKeyPointObject(v)
			if !ok {
				continue
			}
			keyPoints = append(keyPoints, kp)
		default:
			continue
		}
	}

	p.KeyPoints = keyPoints
	return nil
}

func parseKeyPointObject(obj map[string]any) (KeyPoint, bool) {
	var kp KeyPoint

	if name, ok := obj["name"].(string); ok {
		kp.Name = strings.TrimSpace(name)
	}
	if text, ok := obj["text"].(string); ok {
		kp.Text = strings.TrimSpace(text)
	}
	if kp.Text == "" {
		return KeyPoint{}, false
	}

	if tagsAny, ok := obj["tags"]; ok {
		switch t := tagsAny.(type) {
		case []any:
			var tags []string
			for _, it := range t {
				if s, ok := it.(string); ok {
					tags = append(tags, s)
				}
			}
			kp.Tags = normalizeTags(tags)
		case []string:
			kp.Tags = normalizeTags(t)
		}
	}

	if pending, ok := obj["pending"].(bool); ok {
		kp.Pending = pending
	}

	if scoreAny, ok := obj["score"]; ok {
		if score, ok := toInt(scoreAny); ok {
			kp.Score = score
		}
	}

	if v, ok := obj["effect_rating"]; ok {
		if f, ok := toFloat(v); ok {
			if 0 <= f && f <= 1 {
				kp.EffectRating = &f
			}
		}
	}
	if v, ok := obj["risk_level"]; ok {
		if f, ok := toFloat(v); ok {
			if -1 <= f && f <= 1 {
				kp.RiskLevel = &f
			}
		}
	}
	if v, ok := obj["innovation_level"]; ok {
		if f, ok := toFloat(v); ok {
			if 0 <= f && f <= 1 {
				kp.InnovationLevel = &f
			}
		}
	}

	return kp, true
}

func toInt(v any) (int, bool) {
	switch n := v.(type) {
	case int:
		return n, true
	case int32:
		return int(n), true
	case int64:
		return int(n), true
	case float64:
		return int(n), true
	case json.Number:
		i, err := n.Int64()
		if err == nil {
			return int(i), true
		}
		f, err := n.Float64()
		if err == nil {
			return int(f), true
		}
		return 0, false
	case string:
		i, err := strconv.Atoi(strings.TrimSpace(n))
		return i, err == nil
	default:
		return 0, false
	}
}

func toFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int32:
		return float64(n), true
	case int64:
		return float64(n), true
	case json.Number:
		f, err := n.Float64()
		return f, err == nil
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(n), 64)
		return f, err == nil
	default:
		return 0, false
	}
}

func isTruthy(v any) bool {
	switch t := v.(type) {
	case bool:
		return t
	case string:
		return strings.EqualFold(strings.TrimSpace(t), "true")
	default:
		return false
	}
}

func (p Playbook) Validate() error {
	if p.KeyPoints == nil {
		return errors.New("playbook key_points must not be null")
	}
	for _, kp := range p.KeyPoints {
		if strings.TrimSpace(kp.Text) == "" {
			return fmt.Errorf("key point has empty text: %q", kp.Name)
		}
	}
	return nil
}

func (p *Playbook) Normalize() {
	if p.Version == "" {
		p.Version = "1.0"
	}
	if p.KeyPoints == nil {
		p.KeyPoints = []KeyPoint{}
	}
	for i := range p.KeyPoints {
		p.KeyPoints[i].Text = strings.TrimSpace(p.KeyPoints[i].Text)
		p.KeyPoints[i].Name = strings.TrimSpace(p.KeyPoints[i].Name)
		p.KeyPoints[i].Tags = normalizeTags(p.KeyPoints[i].Tags)

		if p.KeyPoints[i].EffectRating == nil {
			v := inferEffectRatingFromScore(p.KeyPoints[i].Score)
			p.KeyPoints[i].EffectRating = &v
		}
		if p.KeyPoints[i].RiskLevel == nil {
			v := inferRiskFromText(p.KeyPoints[i].Text)
			p.KeyPoints[i].RiskLevel = &v
		}
		if p.KeyPoints[i].InnovationLevel == nil {
			v := inferInnovationFromText(p.KeyPoints[i].Text)
			p.KeyPoints[i].InnovationLevel = &v
		}
	}
}

func (p *Playbook) Sort() {
	slices.SortFunc(p.KeyPoints, func(a, b KeyPoint) int {
		if a.Score != b.Score {
			if a.Score > b.Score {
				return -1
			}
			return 1
		}
		return strings.Compare(a.Name, b.Name)
	})
}
