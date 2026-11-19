package content

import "testing"

func TestQueryNormalize(t *testing.T) {
	query, err := (Query{}).Normalize()
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	if query.Page != 1 || query.PageSize != DefaultPageSize || query.Sort != SortLatest {
		t.Fatalf("unexpected defaults: %+v", query)
	}

	query, err = (Query{Page: 3, PageSize: 999, Sort: SortOldest,
		Tags: []string{" 摄影 ", "摄影", "", "旅记"}}).Normalize()
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	if query.PageSize != MaxPageSize {
		t.Fatalf("pageSize = %d, want %d", query.PageSize, MaxPageSize)
	}
	if len(query.Tags) != 2 || query.Tags[0] != "摄影" || query.Tags[1] != "旅记" {
		t.Fatalf("tags = %v", query.Tags)
	}

	if _, err := (Query{Sort: "random"}).Normalize(); err == nil {
		t.Fatal("invalid sort accepted")
	}
	if _, err := (Query{DateFrom: "2026-13-01"}).Normalize(); err == nil {
		t.Fatal("invalid date accepted")
	}
	if _, err := (Query{DateFrom: "2026-10-05", DateTo: "2026-10-01"}).Normalize(); err == nil {
		t.Fatal("reversed date range accepted")
	}
}
