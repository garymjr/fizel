package fizzy

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/gmurray/fizel/internal/config"
	"github.com/gmurray/fizel/internal/model"
)

type Runner func(args []string, env []string) ([]byte, error)

type Tracker struct {
	settings        config.TrackerSettings
	settingsByBoard map[string]config.TrackerSettings
	runner          Runner
}

func NewFromSettings(settings config.TrackerSettings) *Tracker {
	return &Tracker{
		settings: settings,
		settingsByBoard: map[string]config.TrackerSettings{
			settings.BoardID: settings,
		},
		runner: defaultRunner,
	}
}

func NewWithRunner(settings config.TrackerSettings, runner Runner) *Tracker {
	return &Tracker{
		settings: settings,
		settingsByBoard: map[string]config.TrackerSettings{
			settings.BoardID: settings,
		},
		runner: runner,
	}
}

func NewFromMany(settings []config.TrackerSettings) *Tracker {
	if len(settings) == 0 {
		return &Tracker{runner: defaultRunner}
	}
	byBoard := make(map[string]config.TrackerSettings, len(settings))
	for _, entry := range settings {
		byBoard[entry.BoardID] = entry
	}
	return &Tracker{
		settings:        settings[0],
		settingsByBoard: byBoard,
		runner:          defaultRunner,
	}
}

func (t *Tracker) FetchCandidateItems() ([]model.Item, error) {
	var all []model.Item
	for _, settings := range t.uniqueBoards() {
		items, err := t.fetchBoardItems(settings, []string{"card", "list", "--board", settings.BoardID, "--all"})
		if err != nil {
			return nil, err
		}
		all = append(all, items...)
	}
	return dedupe(all), nil
}

func (t *Tracker) FetchItemsByStates(states []string) ([]model.Item, error) {
	requested := make(map[string]struct{}, len(states))
	for _, state := range states {
		requested[normalizeState(state)] = struct{}{}
	}
	var all []model.Item
	if len(requested) == 0 {
		return nil, nil
	}
	for _, settings := range t.uniqueBoards() {
		if includesOpenStates(requested) {
			items, err := t.fetchBoardItems(settings, []string{"card", "list", "--board", settings.BoardID, "--all"})
			if err != nil {
				return nil, err
			}
			all = append(all, filterByStates(items, requested)...)
		}
		if _, ok := requested["done"]; ok {
			items, err := t.fetchBoardItems(settings, []string{"card", "list", "--board", settings.BoardID, "--indexed-by", "closed", "--all"})
			if err != nil {
				return nil, err
			}
			all = append(all, filterByStates(items, requested)...)
		}
		if _, ok := requested["not-now"]; ok {
			items, err := t.fetchBoardItems(settings, []string{"card", "list", "--board", settings.BoardID, "--indexed-by", "not_now", "--all"})
			if err != nil {
				return nil, err
			}
			all = append(all, filterByStates(items, requested)...)
		}
	}
	return dedupe(all), nil
}

func (t *Tracker) FetchItemStatesByIDs(ids []string) ([]model.Item, error) {
	var out []model.Item
	for _, id := range ids {
		boardID, cardNumber, err := parseIssueID(id)
		if err != nil {
			return nil, err
		}
		settings, err := t.settingsForBoard(boardID)
		if err != nil {
			return nil, err
		}
		payload, err := t.run(settings, []string{"card", "show", strconv.Itoa(cardNumber)})
		if err != nil {
			return nil, err
		}
		card, ok := payload.Data.(map[string]any)
		if !ok {
			return nil, errors.New("invalid fizzy card payload")
		}
		item, err := normalizeCard(card, boardID)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, nil
}

func (t *Tracker) CreateComment(id, body string) error {
	_, cardNumber, err := parseIssueID(id)
	if err != nil {
		return err
	}
	settings, err := t.settingsForBoardFromID(id)
	if err != nil {
		return err
	}
	_, err = t.run(settings, []string{"comment", "create", "--card", strconv.Itoa(cardNumber), "--body", body})
	return err
}

func (t *Tracker) UpdateItemState(id, state string) error {
	boardID, cardNumber, err := parseIssueID(id)
	if err != nil {
		return err
	}
	settings, err := t.settingsForBoard(boardID)
	if err != nil {
		return err
	}
	state = normalizeState(state)
	switch state {
	case "done":
		_, err = t.run(settings, []string{"card", "close", strconv.Itoa(cardNumber)})
		return err
	case "not-now":
		_, err = t.run(settings, []string{"card", "postpone", strconv.Itoa(cardNumber)})
		return err
	case "maybe":
		_, err = t.run(settings, []string{"card", "untriage", strconv.Itoa(cardNumber)})
		return err
	default:
		columnID, err := t.resolveColumnID(boardID, state)
		if err != nil {
			return err
		}
		_, err = t.run(settings, []string{"card", "column", strconv.Itoa(cardNumber), "--column", columnID})
		return err
	}
}

type envelope struct {
	OK      bool `json:"ok"`
	Success bool `json:"success"`
	Data    any  `json:"data"`
}

func (t *Tracker) fetchBoardItems(settings config.TrackerSettings, args []string) ([]model.Item, error) {
	payload, err := t.run(settings, args)
	if err != nil {
		return nil, err
	}
	rows, ok := payload.Data.([]any)
	if !ok {
		return nil, errors.New("invalid fizzy list payload")
	}
	items := make([]model.Item, 0, len(rows))
	for _, row := range rows {
		card, ok := row.(map[string]any)
		if !ok {
			continue
		}
		item, err := normalizeCard(card, t.settings.BoardID)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

func (t *Tracker) resolveColumnID(boardID, target string) (string, error) {
	settings, err := t.settingsForBoard(boardID)
	if err != nil {
		return "", err
	}
	payload, err := t.run(settings, []string{"column", "list", "--board", boardID})
	if err != nil {
		return "", err
	}
	rows, ok := payload.Data.([]any)
	if !ok {
		return "", errors.New("invalid fizzy column payload")
	}
	for _, row := range rows {
		column, ok := row.(map[string]any)
		if !ok {
			continue
		}
		name := normalizeState(stringValue(column["name"]))
		id := stringValue(column["id"])
		if name == target || normalizeState(id) == target {
			return id, nil
		}
	}
	return "", fmt.Errorf("fizzy column not found for state %q", target)
}

func (t *Tracker) run(settings config.TrackerSettings, args []string) (envelope, error) {
	env := []string{
		"FIZZY_TOKEN=" + settings.APIKey,
		"FIZZY_API_URL=" + settings.APIURL,
	}
	if settings.Profile != "" {
		env = append(env, "FIZZY_PROFILE="+settings.Profile)
	}
	out, err := t.runner(args, env)
	if err != nil {
		return envelope{}, err
	}
	var payload envelope
	if err := json.Unmarshal(out, &payload); err != nil {
		return envelope{}, fmt.Errorf("decode fizzy json: %w", err)
	}
	return payload, nil
}

func (t *Tracker) RunRaw(args []string) (any, error) {
	return t.run(t.settings, args)
}

func (t *Tracker) uniqueBoards() []config.TrackerSettings {
	if len(t.settingsByBoard) == 0 {
		if strings.TrimSpace(t.settings.BoardID) == "" {
			return nil
		}
		return []config.TrackerSettings{t.settings}
	}
	out := make([]config.TrackerSettings, 0, len(t.settingsByBoard))
	for _, settings := range t.settingsByBoard {
		if strings.TrimSpace(settings.BoardID) == "" {
			continue
		}
		out = append(out, settings)
	}
	return out
}

func (t *Tracker) settingsForBoardFromID(id string) (config.TrackerSettings, error) {
	boardID, _, err := parseIssueID(id)
	if err != nil {
		return config.TrackerSettings{}, err
	}
	return t.settingsForBoard(boardID)
}

func (t *Tracker) settingsForBoard(boardID string) (config.TrackerSettings, error) {
	if settings, ok := t.settingsByBoard[boardID]; ok {
		return settings, nil
	}
	if boardID == t.settings.BoardID {
		return t.settings, nil
	}
	return config.TrackerSettings{}, fmt.Errorf("fizzy settings not found for board %q", boardID)
}

func defaultRunner(args []string, env []string) ([]byte, error) {
	if _, err := exec.LookPath("fizzy"); err != nil {
		return nil, errors.New("fizzy CLI not found on PATH")
	}
	base := append([]string{}, os.Environ()...)
	base = append(base, env...)
	cmdArgs := append([]string{}, args...)
	cmd := exec.Command("fizzy", cmdArgs...)
	cmd.Env = base
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("fizzy command failed: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return out, nil
}

func normalizeCard(card map[string]any, fallbackBoardID string) (model.Item, error) {
	board := nestedMap(card, "board")
	boardID := stringValue(board["id"])
	if boardID == "" {
		boardID = fallbackBoardID
	}
	number, err := intValue(card["number"])
	if err != nil {
		return model.Item{}, fmt.Errorf("invalid card number: %w", err)
	}
	boardName := stringValue(board["name"])
	createdAt := parseTime(stringValue(card["created_at"]))
	updatedAt := parseTime(firstNonEmpty(
		stringValue(card["updated_at"]),
		stringValue(card["last_active_at"]),
	))
	state := inferState(card)

	return model.Item{
		ID:               fmt.Sprintf("%s:%d", boardID, number),
		Identifier:       buildIdentifier(boardName, boardID, number),
		Title:            stringValue(card["title"]),
		Description:      firstNonEmpty(stringValue(card["description"]), nestedString(card, "description", "plain_text")),
		State:            state,
		URL:              stringValue(card["url"]),
		AssigneeID:       firstAssigneeID(card),
		Labels:           extractLabels(card),
		AssignedToWorker: true,
		CreatedAt:        createdAt,
		UpdatedAt:        updatedAt,
	}, nil
}

func inferState(card map[string]any) string {
	if boolValue(card["closed"]) {
		return "done"
	}
	if column := nestedMap(card, "column"); column != nil {
		if name := stringValue(column["name"]); name != "" {
			return name
		}
		if kind := stringValue(column["kind"]); kind != "" {
			return kind
		}
	}
	switch normalizeState(firstNonEmpty(
		stringValue(card["lane"]),
		stringValue(card["indexed_by"]),
		stringValue(card["column_name"]),
		stringValue(card["status"]),
	)) {
	case "not_now", "not-now", "not now":
		return "not-now"
	case "closed", "done":
		return "done"
	case "":
		return "maybe"
	default:
		return firstNonEmpty(
			stringValue(card["column_name"]),
			stringValue(card["status"]),
			"maybe",
		)
	}
}

func parseIssueID(id string) (string, int, error) {
	parts := strings.Split(id, ":")
	if len(parts) != 2 {
		return "", 0, fmt.Errorf("invalid fizzy issue id %q", id)
	}
	number, err := strconv.Atoi(parts[1])
	if err != nil {
		return "", 0, err
	}
	return parts[0], number, nil
}

func normalizeState(v string) string {
	return strings.ToLower(strings.TrimSpace(strings.ReplaceAll(v, "_", "-")))
}

func includesOpenStates(requested map[string]struct{}) bool {
	for state := range requested {
		if state != "done" && state != "not-now" {
			return true
		}
	}
	return false
}

func filterByStates(items []model.Item, requested map[string]struct{}) []model.Item {
	var out []model.Item
	for _, item := range items {
		if _, ok := requested[normalizeState(item.State)]; ok {
			out = append(out, item)
		}
	}
	return out
}

func dedupe(items []model.Item) []model.Item {
	seen := map[string]struct{}{}
	out := make([]model.Item, 0, len(items))
	for _, item := range items {
		if _, ok := seen[item.ID]; ok {
			continue
		}
		seen[item.ID] = struct{}{}
		out = append(out, item)
	}
	return out
}

func buildIdentifier(boardName, boardID string, number int) string {
	base := boardName
	if strings.TrimSpace(base) == "" {
		base = boardID
	}
	base = strings.ToLower(base)
	base = strings.ReplaceAll(base, " ", "-")
	return fmt.Sprintf("%s-%d", base, number)
}

func firstAssigneeID(card map[string]any) string {
	raw, ok := card["assignees"].([]any)
	if !ok || len(raw) == 0 {
		return ""
	}
	assignee, ok := raw[0].(map[string]any)
	if !ok {
		return ""
	}
	return stringValue(assignee["id"])
}

func extractLabels(card map[string]any) []string {
	raw, ok := card["tags"].([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, tag := range raw {
		row, ok := tag.(map[string]any)
		if !ok {
			continue
		}
		title := strings.ToLower(firstNonEmpty(stringValue(row["title"]), stringValue(row["name"])))
		if title != "" {
			out = append(out, title)
		}
	}
	return out
}

func parseTime(v string) time.Time {
	if v == "" {
		return time.Time{}
	}
	ts, err := time.Parse(time.RFC3339, v)
	if err != nil {
		return time.Time{}
	}
	return ts
}

func nestedMap(m map[string]any, key string) map[string]any {
	raw, _ := m[key].(map[string]any)
	return raw
}

func nestedString(m map[string]any, key, subkey string) string {
	return stringValue(nestedMap(m, key)[subkey])
}

func stringValue(v any) string {
	switch value := v.(type) {
	case string:
		return value
	default:
		return ""
	}
}

func intValue(v any) (int, error) {
	switch value := v.(type) {
	case int:
		return value, nil
	case float64:
		return int(value), nil
	case string:
		return strconv.Atoi(value)
	default:
		return 0, fmt.Errorf("unsupported numeric type %T", v)
	}
}

func boolValue(v any) bool {
	value, _ := v.(bool)
	return value
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
