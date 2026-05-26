package render

import (
	"encoding/json"
	"io"
)

type jsonRenderer struct {
	w io.Writer
}

func (r *jsonRenderer) Init(path string, created []string) error {
	return r.emit(map[string]any{
		"event":   "init",
		"path":    path,
		"created": created,
	})
}

func (r *jsonRenderer) Env(name string) error {
	return r.emit(map[string]any{
		"event":    "env",
		"ambiente": name,
	})
}

func (r *jsonRenderer) Status(s StatusInfo) error {
	return r.emit(map[string]any{
		"event":  "status",
		"status": s,
	})
}

func (r *jsonRenderer) Emit(info EmitInfo) error {
	return r.emit(map[string]any{
		"event": "emit",
		"emit":  info,
	})
}

func (r *jsonRenderer) Query(info QueryInfo) error {
	return r.emit(map[string]any{
		"event": "query",
		"query": info,
	})
}

func (r *jsonRenderer) Cancel(info CancelInfo) error {
	return r.emit(map[string]any{
		"event":  "cancel",
		"cancel": info,
	})
}

func (r *jsonRenderer) emit(payload any) error {
	enc := json.NewEncoder(r.w)
	enc.SetIndent("", "  ")
	return enc.Encode(payload)
}
