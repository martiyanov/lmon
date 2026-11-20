package main

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestExitCodeFor(t *testing.T) {
	ok := []reading{{id: "N", head: "NET", v: 10, green: 50, yellow: 100, red: 200}}
	if got := exitCodeFor(ok); got != 0 {
		t.Fatalf("ok code = %d", got)
	}
	warn := []reading{{id: "T", head: " C°", v: 90, green: 60, yellow: 90, red: 100}}
	if got := exitCodeFor(warn); got != 1 {
		t.Fatalf("warn code = %d", got)
	}
	bad := []reading{{id: "G", head: "GTW", v: 999, green: 10, yellow: 50, red: 100}}
	if got := exitCodeFor(bad); got != 2 {
		t.Fatalf("bad code = %d", got)
	}
	noSensor := []reading{{id: "T", head: " C°", v: -1, green: 60, yellow: 75, red: 90}}
	if got := exitCodeFor(noSensor); got != 0 {
		t.Fatalf("no sensor code = %d", got)
	}
}

func TestJSONOnce(t *testing.T) {
	var buf bytes.Buffer
	rs := []reading{
		{id: "N", head: "NET", v: 12, green: 50, yellow: 150, red: 300},
		{id: "T", head: " C°", v: -1, green: 60, yellow: 75, red: 90},
		{id: "G", head: "GTW", v: 999, green: 10, yellow: 50, red: 100},
	}
	if err := writeJSONOnce(&buf, rs); err != nil {
		t.Fatal(err)
	}
	var got jsonOnce
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Columns) != 3 || got.ExitCode != 2 {
		t.Fatalf("json = %+v", got)
	}
	if got.Columns[0].Value == nil || *got.Columns[0].Value != 12 || got.Columns[0].State != "ok" {
		t.Fatalf("first = %+v", got.Columns[0])
	}
	if got.Columns[1].Value != nil || got.Columns[1].State != "no_sensor" {
		t.Fatalf("sensor = %+v", got.Columns[1])
	}
	if got.Columns[2].Value != nil || got.Columns[2].State != "no_reply" {
		t.Fatalf("reply = %+v", got.Columns[2])
	}
}
