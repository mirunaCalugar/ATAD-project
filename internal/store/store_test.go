package store

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
)

func openTemp(t *testing.T) (*Store, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "links.db")
	s, err := Open(context.Background(), path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s, path
}

func TestCreateAssignsIDsFromOne(t *testing.T) {
	s, _ := openTemp(t)
	ctx := context.Background()

	for i, code := range []string{"first", "second"} {
		l, err := s.Create(ctx, code, "https://example.org")
		if err != nil {
			t.Fatalf("Create(%q): %v", code, err)
		}
		if want := int64(i + 1); l.ID != want {
			t.Errorf("Create(%q).ID = %d, want %d", code, l.ID, want)
		}
	}
}

func TestCreateDuplicateCode(t *testing.T) {
	s, _ := openTemp(t)
	ctx := context.Background()

	if _, err := s.Create(ctx, "gofmt", "https://go.dev"); err != nil {
		t.Fatalf("first Create: %v", err)
	}
	if _, err := s.Create(ctx, "gofmt", "https://example.org"); !errors.Is(err, ErrCodeTaken) {
		t.Fatalf("second Create error = %v, want ErrCodeTaken", err)
	}
	all, err := s.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(all) != 1 || all[0].URL != "https://go.dev" {
		t.Errorf("List = %+v, want only the first link", all)
	}
}

func TestConcurrentCreateSameCode(t *testing.T) {
	s, _ := openTemp(t)
	ctx := context.Background()
	const workers = 50

	var (
		wg              sync.WaitGroup
		mu              sync.Mutex
		created, taken  int
		unexpectedError error
	)
	start := make(chan struct{})
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, err := s.Create(ctx, "race", "https://example.org")
			mu.Lock()
			defer mu.Unlock()
			switch {
			case err == nil:
				created++
			case errors.Is(err, ErrCodeTaken):
				taken++
			default:
				unexpectedError = err
			}
		}()
	}
	close(start)
	wg.Wait()

	if unexpectedError != nil {
		t.Fatalf("unexpected error: %v", unexpectedError)
	}
	if created != 1 || taken != workers-1 {
		t.Errorf("created = %d, taken = %d, want 1 and %d", created, taken, workers-1)
	}
}

func TestCreateGeneratedRetriesOnCollision(t *testing.T) {
	s, _ := openTemp(t)
	ctx := context.Background()
	if _, err := s.Create(ctx, "taken01", "https://example.org"); err != nil {
		t.Fatalf("Create: %v", err)
	}

	codes := []string{"taken01", "taken01", "fresh01"}
	s.generate = func() (string, error) {
		c := codes[0]
		codes = codes[1:]
		return c, nil
	}

	l, err := s.CreateGenerated(ctx, "https://go.dev")
	if err != nil {
		t.Fatalf("CreateGenerated: %v", err)
	}
	if l.Code != "fresh01" {
		t.Errorf("CreateGenerated code = %q, want fresh01", l.Code)
	}
}

func TestCreateGeneratedGivesUp(t *testing.T) {
	s, _ := openTemp(t)
	ctx := context.Background()
	if _, err := s.Create(ctx, "taken01", "https://example.org"); err != nil {
		t.Fatalf("Create: %v", err)
	}

	calls := 0
	s.generate = func() (string, error) {
		calls++
		return "taken01", nil
	}

	if _, err := s.CreateGenerated(ctx, "https://go.dev"); !errors.Is(err, ErrNoFreeCode) {
		t.Fatalf("CreateGenerated error = %v, want ErrNoFreeCode", err)
	}
	if calls != maxGenerateAttempts {
		t.Errorf("generate called %d times, want %d", calls, maxGenerateAttempts)
	}
}

func TestStatsDoesNotCountVisits(t *testing.T) {
	s, _ := openTemp(t)
	ctx := context.Background()
	if _, err := s.Create(ctx, "gofmt", "https://go.dev"); err != nil {
		t.Fatalf("Create: %v", err)
	}

	st, err := s.Stats(ctx, "gofmt")
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if st.Visits != 0 || st.LastVisitAt != nil {
		t.Errorf("Stats before visit = %+v, want 0 visits and nil last visit", st)
	}

	url, err := s.Visit(ctx, "gofmt")
	if err != nil {
		t.Fatalf("Visit: %v", err)
	}
	if url != "https://go.dev" {
		t.Errorf("Visit url = %q, want https://go.dev", url)
	}

	for i := range 2 {
		st, err := s.Stats(ctx, "gofmt")
		if err != nil {
			t.Fatalf("Stats read %d: %v", i+1, err)
		}
		if st.Visits != 1 || st.LastVisitAt == nil {
			t.Errorf("Stats read %d = %+v, want 1 visit and a last visit time", i+1, st)
		}
	}
}

func TestConcurrentVisitsAreAllCounted(t *testing.T) {
	s, _ := openTemp(t)
	ctx := context.Background()
	if _, err := s.Create(ctx, "busy", "https://example.org"); err != nil {
		t.Fatalf("Create: %v", err)
	}
	const visits = 50

	var wg sync.WaitGroup
	errs := make([]error, visits)
	for i := range visits {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, errs[i] = s.Visit(ctx, "busy")
		}()
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("Visit %d: %v", i, err)
		}
	}
	st, err := s.Stats(ctx, "busy")
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if st.Visits != visits {
		t.Errorf("Visits = %d, want %d", st.Visits, visits)
	}
}

func TestUnknownCodeIsNotFound(t *testing.T) {
	s, _ := openTemp(t)
	ctx := context.Background()

	if _, err := s.Visit(ctx, "nope"); !errors.Is(err, ErrNotFound) {
		t.Errorf("Visit error = %v, want ErrNotFound", err)
	}
	if _, err := s.Stats(ctx, "nope"); !errors.Is(err, ErrNotFound) {
		t.Errorf("Stats error = %v, want ErrNotFound", err)
	}
	if err := s.Delete(ctx, "nope"); !errors.Is(err, ErrNotFound) {
		t.Errorf("Delete error = %v, want ErrNotFound", err)
	}
}

func TestListNewestFirst(t *testing.T) {
	s, _ := openTemp(t)
	ctx := context.Background()
	for _, code := range []string{"aaaa", "bbbb", "cccc"} {
		if _, err := s.Create(ctx, code, "https://example.org"); err != nil {
			t.Fatalf("Create(%q): %v", code, err)
		}
	}

	all, err := s.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	var got []string
	for _, l := range all {
		got = append(got, l.Code)
	}
	want := []string{"cccc", "bbbb", "aaaa"}
	if len(got) != len(want) {
		t.Fatalf("List codes = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("List codes = %v, want %v", got, want)
		}
	}
}

func TestListEmptyIsNotNil(t *testing.T) {
	s, _ := openTemp(t)
	all, err := s.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if all == nil {
		t.Error("List on an empty store = nil, want an empty slice")
	}
}

func TestDeleteRemovesLink(t *testing.T) {
	s, _ := openTemp(t)
	ctx := context.Background()
	if _, err := s.Create(ctx, "gone", "https://example.org"); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := s.Delete(ctx, "gone"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := s.Visit(ctx, "gone"); !errors.Is(err, ErrNotFound) {
		t.Errorf("Visit after Delete error = %v, want ErrNotFound", err)
	}
}

func TestLinksSurviveReopen(t *testing.T) {
	s, path := openTemp(t)
	ctx := context.Background()
	if _, err := s.Create(ctx, "keep", "https://go.dev"); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := s.Visit(ctx, "keep"); err != nil {
		t.Fatalf("Visit: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	reopened, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer reopened.Close()

	url, err := reopened.Visit(ctx, "keep")
	if err != nil {
		t.Fatalf("Visit after reopen: %v", err)
	}
	if url != "https://go.dev" {
		t.Errorf("Visit after reopen url = %q, want https://go.dev", url)
	}
	st, err := reopened.Stats(ctx, "keep")
	if err != nil {
		t.Fatalf("Stats after reopen: %v", err)
	}
	if st.Visits != 2 {
		t.Errorf("Visits after reopen = %d, want 2", st.Visits)
	}
}

func TestResetClearsAndRestartsIDs(t *testing.T) {
	s, _ := openTemp(t)
	ctx := context.Background()
	for _, code := range []string{"aaaa", "bbbb"} {
		if _, err := s.Create(ctx, code, "https://example.org"); err != nil {
			t.Fatalf("Create(%q): %v", code, err)
		}
	}

	if err := s.Reset(ctx); err != nil {
		t.Fatalf("Reset: %v", err)
	}
	all, err := s.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(all) != 0 {
		t.Errorf("List after Reset = %+v, want empty", all)
	}
	l, err := s.Create(ctx, "aaaa", "https://example.org")
	if err != nil {
		t.Fatalf("Create after Reset: %v", err)
	}
	if l.ID != 1 {
		t.Errorf("ID after Reset = %d, want 1", l.ID)
	}
}
