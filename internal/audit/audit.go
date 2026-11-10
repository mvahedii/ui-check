package audit

import (
	"errors"
	"fmt"
	"math"
	"time"

	"ui-check/internal/browser"
)

type Status string

const (
	Pass         Status = "PASS"
	Fail         Status = "FAIL"
	Error        Status = "ERROR"
	Inconclusive Status = "INCONCLUSIVE"
)

func (s Status) ExitCode() int {
	switch s {
	case Pass:
		return 0
	case Fail:
		return 1
	case Inconclusive:
		return 3
	default:
		return 2
	}
}

type Policy struct {
	Tolerance float64 `json:"geometryToleranceCssPx"`
	FailOn    string  `json:"failOn"`
}

func (p Policy) Validate() error {
	if math.IsNaN(p.Tolerance) || math.IsInf(p.Tolerance, 0) || p.Tolerance < 0 {
		return errors.New("geometry tolerance must be finite and non-negative")
	}
	if p.FailOn != "minor" && p.FailOn != "major" && p.FailOn != "critical" {
		return errors.New("fail-on must be minor, major or critical")
	}
	return nil
}

type Evidence struct {
	Kind     string `json:"kind"`
	ID       string `json:"id"`
	Artifact string `json:"artifactPath"`
	Selector string `json:"selector,omitempty"`
}

type Measurement struct {
	Expected     float64    `json:"expected"`
	Actual       float64    `json:"actual"`
	Delta        float64    `json:"delta"`
	Unit         string     `json:"unit"`
	Verification string     `json:"verification"`
	Evidence     []Evidence `json:"evidence"`
}

type Finding struct {
	ID       string `json:"id"`
	Category string `json:"category"`
	Property string `json:"property"`
	Severity string `json:"severity"`
	Measurement
}

type Report struct {
	SchemaVersion int          `json:"schemaVersion"`
	RunID         string       `json:"runId"`
	Timestamp     time.Time    `json:"timestamp"`
	Status        Status       `json:"status"`
	DesignSource  string       `json:"designSource"`
	DesignVersion string       `json:"designVersion"`
	FrameID       string       `json:"frameId"`
	Target        string       `json:"target"`
	Section       string       `json:"section"`
	Browser       string       `json:"browserVersion"`
	OS            string       `json:"os"`
	Width         int          `json:"viewportWidth"`
	Height        int          `json:"viewportHeight"`
	DPR           float64      `json:"dpr"`
	Policy        Policy       `json:"policy"`
	Coverage      float64      `json:"coverage"`
	MatchMethod   string       `json:"matchMethod"`
	Confidence    *float64     `json:"matchConfidence,omitempty"`
	Measurement   *Measurement `json:"measurement,omitempty"`
	Findings      []Finding    `json:"findings"`
	Unverified    []string     `json:"unverified"`
}

func selector(id string) string { return fmt.Sprintf(`[data-testid="%s"]`, id) }

func Compare(design Snapshot, scene browser.Scene, policy Policy) Report {
	result := Report{
		SchemaVersion: 1, Timestamp: time.Now().UTC(), Status: Inconclusive,
		DesignSource: design.Source, DesignVersion: design.Version, FrameID: design.FrameID,
		Target: scene.URL, Section: selector(design.Parent.TestID), Browser: scene.Version, OS: scene.OS,
		Width: design.Width, Height: design.Height, DPR: scene.DPR, Policy: policy,
		MatchMethod: "unique-test-id-and-direct-parent",
		Findings:    []Finding{}, Unverified: []string{},
	}
	parent, child, reason := pair(design, scene)
	if reason != "" {
		result.Unverified = append(result.Unverified, reason)
		return result
	}

	expected, actual := measureInset(design, parent, child)
	measurement := Measurement{
		Expected: expected, Actual: actual, Delta: actual - expected, Unit: "css-px", Verification: "verified",
		Evidence: []Evidence{
			{Kind: "design-node", ID: design.Parent.ID, Artifact: designPath},
			{Kind: "design-node", ID: design.Child.ID, Artifact: designPath},
			{Kind: "dom-element", ID: parent.ID, Artifact: domPath, Selector: selector(parent.ID)},
			{Kind: "dom-element", ID: child.ID, Artifact: domPath, Selector: selector(child.ID)},
		},
	}
	confidence := 1.0 // Explicit fixture mapping passed every constraint, rather than a calibrated probability.
	result.Coverage, result.Confidence, result.Measurement, result.Status = 1, &confidence, &measurement, Pass
	if math.Abs(measurement.Delta) > policy.Tolerance {
		result.Findings = append(result.Findings, Finding{
			ID: "top-inset", Category: "spacing", Property: "visual-top-inset", Severity: "minor", Measurement: measurement,
		})
		if policy.FailOn == "minor" {
			result.Status = Fail
		}
	}
	return result
}

func pair(design Snapshot, scene browser.Scene) (browser.Element, browser.Element, string) {
	var parent, child browser.Element
	if scene.Truncated || scene.Width != design.Width || scene.Height != design.Height || scene.DPR != 1 {
		return parent, child, "capture cannot establish the required 1:1 viewport and complete DOM"
	}
	if scene.BlockedRequests {
		return parent, child, "capture blocked a page request outside the permitted method or origin"
	}
	var parents, children int
	for _, element := range scene.Elements {
		switch element.ID {
		case design.Parent.TestID:
			parent = element
			parents++
		case design.Child.TestID:
			child = element
			children++
		}
	}
	if parents != 1 || children != 1 || child.ParentID != parent.ID {
		return parent, child, "expected exactly one visible parent and direct child with the configured test IDs"
	}
	if !parent.Visible || !child.Visible || !parent.GeometrySupported || !child.GeometrySupported ||
		parent.Y < 0 || child.Y < parent.Y+parent.BorderTop ||
		child.Y+child.Height > math.Min(parent.Y+parent.Height, float64(scene.Height)) ||
		child.X < math.Max(parent.X, 0) ||
		child.X+child.Width > math.Min(parent.X+parent.Width, float64(scene.Width)) {
		return parent, child, "rendered pair is hidden or its geometry is outside the supported inset case"
	}
	return parent, child, ""
}

func measureInset(design Snapshot, parent, child browser.Element) (expected, actual float64) {
	return *design.Child.Y - *design.Parent.Y - *design.Parent.Border, child.Y - parent.Y - parent.BorderTop
}
