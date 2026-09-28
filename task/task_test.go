package task

import (
	"math/big"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestUnmarshal(t *testing.T) {
	contents := "Title: A task\n" +
		"Created: 1437509667\n" +
		"Task-Id: 6159523072535192340592\n" +
		"Expires: 2030-01-02\n" +
		"Tags: feature bug\n" +
		"Not-Before: 2029-12-31\n" +
		"Display-Frequency: 7\n" +
		"--- 1538856865\n" +
		"First note.\n" +
		"--- 1538856900\n" +
		"Second note,\n" +
		"on two lines.\n"

	task, err := Unmarshal([]byte(contents))
	if err != nil {
		t.Fatalf("Unmarshal returned error: %v", err)
	}

	if task.Title != "A task" {
		t.Errorf("Title = %q", task.Title)
	}
	if got := task.Created.Unix(); got != 1437509667 {
		t.Errorf("Created = %d", got)
	}
	// The IDs outgrew 64 bits long ago, so they are kept as big integers.
	if want, _ := new(big.Int).SetString("6159523072535192340592", 10); task.ID.Cmp(want) != 0 {
		t.Errorf("ID = %s, want %s", task.ID, want)
	}
	if got := task.Expires.String(); got != "2030-01-02" {
		t.Errorf("Expires = %q", got)
	}
	if got := task.NotBefore.String(); got != "2029-12-31" {
		t.Errorf("NotBefore = %q", got)
	}
	if task.DisplayFrequency != 7 {
		t.Errorf("DisplayFrequency = %d", task.DisplayFrequency)
	}
	if want := []string{"bug", "feature"}; !slices.Equal(task.Tags, want) {
		t.Errorf("Tags = %v, want %v", task.Tags, want)
	}

	if len(task.Notes) != 2 {
		t.Fatalf("len(Notes) = %d, want 2", len(task.Notes))
	}
	if task.Notes[0].Text != "First note." {
		t.Errorf("Notes[0].Text = %q", task.Notes[0].Text)
	}
	if task.Notes[1].Text != "Second note,\non two lines." {
		t.Errorf("Notes[1].Text = %q", task.Notes[1].Text)
	}
	if got := task.Notes[1].Date.Unix(); got != 1538856900 {
		t.Errorf("Notes[1].Date = %d", got)
	}
}

// Files written by the Raku implementation carried no version marker; a file
// with no Task-Id is a version 1 file and gets one on read.
func TestUnmarshalVersionOne(t *testing.T) {
	task, err := Unmarshal([]byte("Title: Old\nCreated: 1437509667\n"))
	if err != nil {
		t.Fatalf("Unmarshal returned error: %v", err)
	}

	if task.Version != 1 {
		t.Errorf("Version = %d, want 1", task.Version)
	}
	if task.ID == nil || task.ID.Sign() == 0 {
		t.Error("no task ID was generated")
	}
}

func TestUnmarshalRejectsBadFiles(t *testing.T) {
	tests := []struct {
		name     string
		contents string
	}{
		{"no title", "Created: 1437509667\n"},
		{"no created", "Title: A task\n"},
		{"unknown header", "Title: A task\nCreated: 1\nWhat: huh\n"},
		{"unparsable header", "Title: A task\nCreated: 1\nnonsense\n"},
		{"bad timestamp", "Title: A task\nCreated: yesterday\n"},
		{"bad date", "Title: A task\nCreated: 1\nExpires: soon\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := Unmarshal([]byte(tt.contents)); err == nil {
				t.Error("Unmarshal returned no error, want one")
			}
		})
	}
}

func TestMarshalRoundTrip(t *testing.T) {
	original := New(1, "A task")
	original.Expires = Date{2030, time.January, 2}
	original.NotBefore = Date{2029, time.December, 31}
	original.DisplayFrequency = 7
	original.Tags = []string{"bug", "feature"}
	original.TrelloID = "abc123"
	original.Notes = []Note{
		{Date: time.Unix(1538856865, 0), Text: "First note."},
		{Date: time.Unix(1538856900, 0), Text: "Trailing blank line follows.\n"},
	}

	parsed, err := Unmarshal(original.Marshal())
	if err != nil {
		t.Fatalf("Unmarshal returned error: %v", err)
	}

	if parsed.Title != original.Title || parsed.TrelloID != original.TrelloID {
		t.Errorf("headers did not survive the round trip: %+v", parsed)
	}
	if parsed.Created.Unix() != original.Created.Unix() {
		t.Errorf("Created = %d, want %d", parsed.Created.Unix(), original.Created.Unix())
	}
	if parsed.ID.Cmp(original.ID) != 0 {
		t.Errorf("ID = %s, want %s", parsed.ID, original.ID)
	}
	if parsed.Expires != original.Expires || parsed.NotBefore != original.NotBefore {
		t.Errorf("dates did not survive the round trip: %+v", parsed)
	}
	if len(parsed.Notes) != len(original.Notes) {
		t.Fatalf("len(Notes) = %d, want %d", len(parsed.Notes), len(original.Notes))
	}
	for i, note := range parsed.Notes {
		if note.Text != original.Notes[i].Text {
			t.Errorf("Notes[%d].Text = %q, want %q", i, note.Text, original.Notes[i].Text)
		}
	}
}

func TestAddNoteTrimsOneTrailingNewline(t *testing.T) {
	task := New(1, "A task")
	if err := task.AddNote("text\n"); err != nil {
		t.Fatal(err)
	}

	if task.Notes[0].Text != "text" {
		t.Errorf("Notes[0].Text = %q, want %q", task.Notes[0].Text, "text")
	}
}

func TestTags(t *testing.T) {
	task := New(1, "A task")

	if added, _ := task.AddTag("beta"); !added {
		t.Error("AddTag reported no change for a new tag")
	}
	if added, _ := task.AddTag("beta"); added {
		t.Error("AddTag reported a change for a duplicate tag")
	}
	if _, err := task.AddTag("alpha"); err != nil {
		t.Fatal(err)
	}

	// Tags are kept sorted so that a task file does not churn.
	if want := []string{"alpha", "beta"}; !slices.Equal(task.Tags, want) {
		t.Errorf("Tags = %v, want %v", task.Tags, want)
	}
	if !task.HasTag("alpha") || task.HasTag("gamma") {
		t.Errorf("HasTag is wrong for %v", task.Tags)
	}

	if removed, _ := task.RemoveTag("gamma"); removed {
		t.Error("RemoveTag reported a change for a tag that was not set")
	}
	if removed, _ := task.RemoveTag("alpha"); !removed {
		t.Error("RemoveTag reported no change for a tag that was set")
	}
	if want := []string{"beta"}; !slices.Equal(task.Tags, want) {
		t.Errorf("Tags = %v, want %v", task.Tags, want)
	}
}

// A task mirrored from Trello is replaced on the next sync, so editing it
// locally would only lose the edit.
func TestTrelloTasksRejectEdits(t *testing.T) {
	task := New(1, "A card")
	task.TrelloID = "abc123"

	if err := task.AddNote("note"); err == nil {
		t.Error("AddNote returned no error, want one")
	}
	if err := task.SetTitle("other"); err == nil {
		t.Error("SetTitle returned no error, want one")
	}
	if _, err := task.AddTag("tag"); err == nil {
		t.Error("AddTag returned no error, want one")
	}
}

func TestIsMature(t *testing.T) {
	task := New(1, "A task")
	if !task.IsMature() {
		t.Error("a task with no maturity date is not mature")
	}

	task.NotBefore = Today()
	if !task.IsMature() {
		t.Error("a task maturing today is not mature")
	}

	task.NotBefore = Today().AddDays(1)
	if task.IsMature() {
		t.Error("a task maturing tomorrow is already mature")
	}

	task.NotBefore = Today().AddDays(-1)
	if !task.IsMature() {
		t.Error("a task that matured yesterday is not mature")
	}
}

func TestDisplayToday(t *testing.T) {
	task := New(1, "A task")
	if !task.DisplayToday() {
		t.Error("a task with no display frequency is held back")
	}

	task.DisplayFrequency = 1
	if !task.DisplayToday() {
		t.Error("a task displayed every day is held back")
	}

	// A frequency this large will not come due for a very long time.
	task.DisplayFrequency = 1_000_000_000_000
	if task.DisplayToday() {
		t.Error("a task with a huge display frequency came due today")
	}

	// Whatever the ID, some day out of every seven is the day.
	task.DisplayFrequency = 7
	displayed := 0
	for i := range 7 {
		task.ID = big.NewInt(int64(i))
		if task.DisplayToday() {
			displayed++
		}
	}
	if displayed != 1 {
		t.Errorf("%d of 7 IDs came due today, want 1", displayed)
	}
}

func TestNewIDIsUnique(t *testing.T) {
	seen := map[string]bool{}
	for range 1000 {
		id := NewID().String()
		if seen[id] {
			t.Fatalf("duplicate task ID %s", id)
		}
		seen[id] = true
	}
}

func TestParseDate(t *testing.T) {
	day, err := ParseDate("2026-02-28")
	if err != nil {
		t.Fatalf("ParseDate returned error: %v", err)
	}
	if want := (Date{2026, time.February, 28}); day != want {
		t.Errorf("ParseDate = %v, want %v", day, want)
	}

	for _, bad := range []string{"", "2026-2-28", "28-02-2026", "tomorrow", "2026-13-01"} {
		if _, err := ParseDate(bad); err == nil {
			t.Errorf("ParseDate(%q) returned no error, want one", bad)
		}
	}
}

func TestDateArithmetic(t *testing.T) {
	day := Date{2026, time.February, 28}

	if got := day.AddDays(1); got != (Date{2026, time.March, 1}) {
		t.Errorf("AddDays(1) = %v", got)
	}
	if got := day.AddDays(-1); got != (Date{2026, time.February, 27}) {
		t.Errorf("AddDays(-1) = %v", got)
	}
	if !day.Before(day.AddDays(1)) || !day.After(day.AddDays(-1)) {
		t.Error("date comparison is wrong")
	}
	if got := day.AddDays(1).DayCount() - day.DayCount(); got != 1 {
		t.Errorf("one day apart counted as %d days", got)
	}
	// The day numbering is the Modified Julian Day, the same numbering the
	// Raku implementation used to schedule tasks with a frequency.
	if got := (Date{1970, time.January, 1}).DayCount(); got != 40587 {
		t.Errorf("DayCount of the Unix epoch = %d, want 40587", got)
	}
	if got := day.Pretty(); !strings.HasSuffix(got, "23:59:59 2026") {
		t.Errorf("Pretty = %q", got)
	}
}
