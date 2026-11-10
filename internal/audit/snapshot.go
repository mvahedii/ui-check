package audit

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"regexp"
)

const (
	maxViewport = 4096
	designPath  = "evidence/design.json"
	domPath     = "evidence/dom.json"
)

var testID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,100}$`)

type designNode struct {
	ID       string   `json:"id"`
	ParentID string   `json:"parentId,omitempty"`
	TestID   string   `json:"testId"`
	Y        *float64 `json:"y"`
	Height   float64  `json:"height"`
	Border   *float64 `json:"topBorder,omitempty"`
}

type Snapshot struct {
	SchemaVersion int        `json:"schemaVersion"`
	Source        string     `json:"source"`
	Version       string     `json:"version"`
	FrameID       string     `json:"frameId"`
	Unit          string     `json:"unit"`
	Width         int        `json:"width"`
	Height        int        `json:"height"`
	ReferencePNG  string     `json:"referencePNG"`
	Parent        designNode `json:"parent"`
	Child         designNode `json:"child"`
	png           []byte
}

func LoadSnapshot(path string) (Snapshot, error) {
	data, err := readLocal(path, 256<<10)
	if err != nil {
		return Snapshot{}, fmt.Errorf("read design snapshot: %w", err)
	}
	var design Snapshot
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&design); err != nil {
		return Snapshot{}, fmt.Errorf("invalid design snapshot JSON: %w", err)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return Snapshot{}, errors.New("snapshot must contain exactly one JSON object")
	}
	if err := design.validate(); err != nil {
		return Snapshot{}, err
	}
	data, err = readLocal(filepath.Join(filepath.Dir(path), design.ReferencePNG), 20<<20)
	if err != nil {
		return Snapshot{}, fmt.Errorf("read design reference: %w", err)
	}
	cfg, err := png.DecodeConfig(bytes.NewReader(data))
	if err != nil || cfg.Width != design.Width || cfg.Height != design.Height {
		return Snapshot{}, errors.New("reference PNG dimensions must match the design at 1:1 scale")
	}
	if _, err := png.Decode(bytes.NewReader(data)); err != nil {
		return Snapshot{}, errors.New("invalid design reference PNG")
	}
	design.png = data
	return design, nil
}

func (s Snapshot) validate() error {
	if s.SchemaVersion != 1 || s.Source != "synthetic" || s.Unit != "figma-px" || s.Version == "" || s.FrameID == "" {
		return errors.New("expected a versioned M1 synthetic design snapshot in figma-px")
	}
	if s.Width < 1 || s.Height < 1 || s.Width > maxViewport || s.Height > maxViewport {
		return errors.New("invalid snapshot viewport")
	}
	if filepath.Base(s.ReferencePNG) != s.ReferencePNG || filepath.Ext(s.ReferencePNG) != ".png" || !testID.MatchString(s.ReferencePNG[:len(s.ReferencePNG)-4]) {
		return errors.New("referencePNG must name a sibling PNG")
	}
	p, c := s.Parent, s.Child
	if p.ID == "" || c.ID == "" || p.ID == c.ID || c.ParentID != p.ID || p.ParentID != "" ||
		!testID.MatchString(p.TestID) || !testID.MatchString(c.TestID) || p.TestID == c.TestID {
		return errors.New("snapshot must identify distinct parent/child nodes with a valid direct relationship and test IDs")
	}
	if p.Y == nil || c.Y == nil || p.Border == nil || c.Border != nil ||
		*p.Y < 0 || p.Height <= 0 || c.Height <= 0 || *p.Border < 0 ||
		*p.Y+p.Height > float64(s.Height) || *c.Y < *p.Y+*p.Border || *c.Y+c.Height > *p.Y+p.Height {
		return errors.New("snapshot does not describe a measurable top inset within the frame")
	}
	return nil
}

func readLocal(path string, maxBytes int64) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > maxBytes {
		return nil, errors.New("expected a regular file within the size limit")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxBytes+1))
	if err == nil && int64(len(data)) > maxBytes {
		return nil, errors.New("file exceeds size limit")
	}
	return data, err
}
