package main

import "testing"

func TestFilteredMenuItems(t *testing.T) {
	items := []menuItem{
		{title: "guanyu", details: "root@guanyu.babudiu.com:22"},
		{title: "ingeek-jps", details: "likun.zhang@10.200.4.188:2222 [default]"},
	}

	filtered, indexes := filteredMenuItems(items, "ingeek")
	if len(filtered) != 1 {
		t.Fatalf("expected 1 filtered item, got %d", len(filtered))
	}
	if filtered[0].title != "ingeek-jps" || len(indexes) != 1 || indexes[0] != 1 {
		t.Fatalf("unexpected filtered result: %#v %#v", filtered, indexes)
	}

	filtered, indexes = filteredMenuItems(items, "root@guanyu")
	if len(filtered) != 1 || indexes[0] != 0 {
		t.Fatalf("expected detail search to match first item, got %#v %#v", filtered, indexes)
	}
}
