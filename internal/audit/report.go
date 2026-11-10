package audit

import (
	_ "embed"
	"encoding/json"
	"errors"
	"html/template"
	"io"
	"os"
	"path/filepath"
	"strings"

	"ui-check/internal/browser"
)

//go:embed report.html
var reportHTML string

var pageTemplate = template.Must(template.New("report").Parse(reportHTML))

func WriteReport(out string, result Report, design Snapshot, scene browser.Scene) (string, error) {
	if result.Measurement != nil {
		if err := validateEvidence(*result.Measurement, design, scene); err != nil {
			return "", err
		}
	}
	for _, finding := range result.Findings {
		if err := validateEvidence(finding.Measurement, design, scene); err != nil {
			return "", err
		}
	}
	if len(design.png) == 0 || len(scene.PNG) == 0 {
		return "", errors.New("report requires design reference and browser capture PNGs")
	}

	if err := os.MkdirAll(out, 0700); err != nil {
		return "", err
	}
	staging, err := os.MkdirTemp(out, ".staging-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(staging)
	result.RunID = "run-" + strings.TrimPrefix(filepath.Base(staging), ".staging-")

	for _, dir := range []string{"assets", "evidence"} {
		if err := os.Mkdir(filepath.Join(staging, dir), 0700); err != nil {
			return "", err
		}
	}
	for name, value := range map[string]any{
		"report.json": result, designPath: design, domPath: scene,
	} {
		if err := writeFile(staging, name, func(w io.Writer) error {
			enc := json.NewEncoder(w)
			enc.SetIndent("", "  ")
			return enc.Encode(value)
		}); err != nil {
			return "", err
		}
	}
	for name, data := range map[string][]byte{
		"assets/target.png": scene.PNG, filepath.Join("evidence", design.ReferencePNG): design.png,
	} {
		if err := os.WriteFile(filepath.Join(staging, name), data, 0600); err != nil {
			return "", err
		}
	}
	if err := writeFile(staging, "index.html", func(w io.Writer) error {
		return pageTemplate.Execute(w, struct {
			Report
			Reference string
		}{result, "evidence/" + design.ReferencePNG})
	}); err != nil {
		return "", err
	}
	dir := filepath.Join(out, result.RunID)
	return dir, os.Rename(staging, dir)
}

func validateEvidence(m Measurement, design Snapshot, scene browser.Scene) error {
	parent, child, reason := pair(design, scene)
	if reason != "" || m.Verification != "verified" || m.Unit != "css-px" || len(m.Evidence) != 4 {
		return errors.New("invalid evidence for verified measurement")
	}
	required := map[Evidence]bool{
		{Kind: "design-node", ID: design.Parent.ID, Artifact: designPath}:                      true,
		{Kind: "design-node", ID: design.Child.ID, Artifact: designPath}:                       true,
		{Kind: "dom-element", ID: parent.ID, Artifact: domPath, Selector: selector(parent.ID)}: true,
		{Kind: "dom-element", ID: child.ID, Artifact: domPath, Selector: selector(child.ID)}:   true,
	}
	for _, ref := range m.Evidence {
		if !required[ref] {
			return errors.New("duplicate or unresolvable evidence reference")
		}
		delete(required, ref)
	}
	expected, actual := measureInset(design, parent, child)
	if m.Expected != expected || m.Actual != actual || m.Delta != actual-expected {
		return errors.New("reported numbers differ from the evidence")
	}
	return nil
}

func writeFile(dir, name string, write func(io.Writer) error) error {
	f, err := os.OpenFile(filepath.Join(dir, name), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	return errors.Join(write(f), f.Close())
}
