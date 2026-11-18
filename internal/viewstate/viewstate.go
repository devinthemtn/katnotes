// Package viewstate keeps how you like to look at your notes, shared by the
// TUI and the GUI: each tab's sort order and the saved searches. It lives in
// <config dir>/view.json.
package viewstate

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/devinthemtn/katnotes/internal/config"
)

// FileName is the state file inside the config directory.
const FileName = "view.json"

// Search is a saved search.
type Search struct {
	Name string `json:"name"`
	Mode string `json:"mode"` // notes.SearchMode
	Text string `json:"text"`
}

// State is the whole file.
type State struct {
	Sort     map[string]string `json:"sort"` // kind -> notes.SortBy
	Searches []Search          `json:"searches"`
}

// Path returns the state file's location.
func Path() (string, error) {
	d, err := config.Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, FileName), nil
}

// Load reads the state; a missing or unreadable file is empty.
func Load(path string) State {
	st := State{Sort: map[string]string{}, Searches: []Search{}}
	data, err := os.ReadFile(path)
	if err != nil {
		return st
	}
	if json.Unmarshal(data, &st) != nil {
		return State{Sort: map[string]string{}, Searches: []Search{}}
	}
	if st.Sort == nil {
		st.Sort = map[string]string{}
	}
	if st.Searches == nil {
		st.Searches = []Search{}
	}
	return st
}

func update(path string, f func(*State) error) (State, error) {
	st := Load(path)
	if path == "" {
		return st, nil
	}
	if err := f(&st); err != nil {
		return st, err
	}
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return st, err
	}
	return st, config.WriteFile(path, append(data, '\n'))
}

// SetSort remembers the sort order for a kind's list.
func SetSort(path, kind, sortBy string) (State, error) {
	return update(path, func(st *State) error {
		st.Sort[kind] = sortBy
		return nil
	})
}

// SaveSearch adds or replaces the saved search called s.Name.
func SaveSearch(path string, s Search) (State, error) {
	s.Name = strings.Join(strings.Fields(s.Name), " ")
	if s.Name == "" {
		return Load(path), fmt.Errorf("a saved search needs a name")
	}
	if strings.TrimSpace(s.Text) == "" {
		return Load(path), fmt.Errorf("nothing to save: search for something first")
	}
	return update(path, func(st *State) error {
		for i, x := range st.Searches {
			if strings.EqualFold(x.Name, s.Name) {
				st.Searches[i] = s
				return nil
			}
		}
		st.Searches = append(st.Searches, s)
		sort.SliceStable(st.Searches, func(i, j int) bool {
			return strings.ToLower(st.Searches[i].Name) < strings.ToLower(st.Searches[j].Name)
		})
		return nil
	})
}

// DeleteSearch removes a saved search by name (case-insensitive).
func DeleteSearch(path, name string) (State, error) {
	return update(path, func(st *State) error {
		for i, x := range st.Searches {
			if strings.EqualFold(x.Name, strings.TrimSpace(name)) {
				st.Searches = append(st.Searches[:i], st.Searches[i+1:]...)
				return nil
			}
		}
		return fmt.Errorf("no saved search %q", name)
	})
}

// FindSearch returns the saved search called name (case-insensitive).
func (st State) FindSearch(name string) (Search, bool) {
	for _, x := range st.Searches {
		if strings.EqualFold(x.Name, strings.TrimSpace(name)) {
			return x, true
		}
	}
	return Search{}, false
}
