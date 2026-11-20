package main

import (
	"encoding/json"
	"io"
	"strings"
	"time"
)

type jsonColumn struct {
	ID    string   `json:"id"`
	Head  string   `json:"head"`
	Value *float64 `json:"value"`
	State string   `json:"state"`
}

type jsonOnce struct {
	Time     string       `json:"time"`
	ExitCode int          `json:"exit_code"`
	Columns  []jsonColumn `json:"columns"`
}

func jsonState(r reading) string {
	switch {
	case r.blank:
		return "blank"
	case r.v < 0:
		return "no_sensor"
	case r.v == 999:
		return "no_reply"
	}
	switch {
	case r.severity() >= 1:
		return "critical"
	case r.severity() >= 0.5:
		return "warning"
	}
	return "ok"
}

func exitCodeFor(rs []reading) int {
	code := 0
	for _, r := range rs {
		if r.blank || r.v < 0 {
			continue
		}
		if r.v == 999 {
			code = 2
			continue
		}
		switch {
		case r.severity() >= 1:
			code = 2
		case r.severity() >= 0.5 && code < 1:
			code = 1
		}
	}
	return code
}

func writeJSONOnce(w io.Writer, rs []reading) error {
	out := jsonOnce{Time: time.Now().UTC().Format(time.RFC3339), ExitCode: exitCodeFor(rs)}
	out.Columns = make([]jsonColumn, 0, len(rs))
	for _, r := range rs {
		c := jsonColumn{ID: r.id, Head: strings.TrimSpace(r.head), State: jsonState(r)}
		if !r.blank && r.v >= 0 && r.v != 999 {
			v := float64(r.v)
			c.Value = &v
		}
		out.Columns = append(out.Columns, c)
	}
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	return enc.Encode(out)
}

func writeJSONError(w io.Writer, err error) {
	_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
}
