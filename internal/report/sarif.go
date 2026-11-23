package report

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/0xbadshah/zephy/internal/match"
)

// SARIF 2.1.0 rendering for GitHub code scanning. EOL isn't a CVE, so the
// severity mapping is policy: past-EOL = error, expiring = warning.
type sarifLog struct {
	Schema  string     `json:"$schema"`
	Version string     `json:"version"`
	Runs    []sarifRun `json:"runs"`
}

type sarifRun struct {
	Tool    sarifTool  `json:"tool"`
	Results []sarifRes `json:"results,omitempty"`
}

type sarifTool struct {
	Driver struct {
		Name           string      `json:"name"`
		InformationURI string      `json:"informationUri"`
		Rules          []sarifRule `json:"rules"`
	} `json:"driver"`
}

type sarifRule struct {
	ID               string `json:"id"`
	ShortDescription struct {
		Text string `json:"text"`
	} `json:"shortDescription"`
}

type sarifRes struct {
	RuleID  string `json:"ruleId"`
	Level   string `json:"level"`
	Message struct {
		Text string `json:"text"`
	} `json:"message"`
	Locations []sarifLoc `json:"locations"`
}

type sarifLoc struct {
	PhysicalLocation sarifPhysical `json:"physicalLocation"`
}

type sarifPhysical struct {
	ArtifactLocation struct {
		URI string `json:"uri"`
	} `json:"artifactLocation"`
	Region *sarifRegion `json:"region,omitempty"`
}

type sarifRegion struct {
	StartLine int `json:"startLine"`
}

func (r Report) SARIF() (string, error) {
	log := sarifLog{
		Schema:  "https://json.schemastore.org/sarif-2.1.0.json",
		Version: "2.1.0",
	}
	log.Runs = append(log.Runs, sarifRun{})
	run := &log.Runs[0]
	run.Tool.Driver.Name = "zephy"
	run.Tool.Driver.InformationURI = "https://github.com/0xbadshah/zephy"
	run.Tool.Driver.Rules = []sarifRule{
		{ID: "EOL001"}, {ID: "EOL002"}, {ID: "EOL003"},
	}
	run.Tool.Driver.Rules[0].ShortDescription.Text = "Component is past its end-of-life date"
	run.Tool.Driver.Rules[1].ShortDescription.Text = "Component reaches end-of-life within the configured window"
	run.Tool.Driver.Rules[2].ShortDescription.Text = "Package is deprecated or abandoned upstream (curated list)"

	for _, f := range r.Findings {
		res := sarifRes{RuleID: "EOL001", Level: "error"}
		if f.Status == match.StatusExpiring {
			res.RuleID, res.Level = "EOL002", "warning"
		}
		if f.Status == match.StatusDeprecated || f.Status == match.StatusAbandoned || f.Status == match.StatusUnsupported {
			res.RuleID, res.Level = "EOL003", "note"
		}
		res.Message.Text = sarifMessage(f)
		res.Locations = []sarifLoc{{PhysicalLocation: sarifLocation(f)}}
		run.Results = append(run.Results, res)
	}
	b, err := json.MarshalIndent(log, "", "  ")
	return string(b), err
}

func sarifMessage(f match.Finding) string {
	msg := fmt.Sprintf("%s %s is %s", f.Product, f.Version, f.Status)
	var detail []string
	if f.Release != "" {
		detail = append(detail, "release "+f.Release)
	}
	if f.EOLOn != "" {
		detail = append(detail, "EOL "+f.EOLOn)
	}
	if len(detail) > 0 {
		msg += " (" + strings.Join(detail, ", ") + ")"
	}
	if f.Status == match.StatusEOL && f.DaysPast > 0 {
		msg += fmt.Sprintf(", %d days ago", f.DaysPast)
	}
	return msg
}

// sarifLocation turns a finding's location into a SARIF physical
// location. Repo files carry "path:line": the path becomes a relative
// URI and the line a region, so code scanning can point at it. Image and
// SBOM findings carry a reference (image:..., sbom:...), not a file, and
// pass through as is.
func sarifLocation(f match.Finding) sarifPhysical {
	var loc sarifPhysical
	if isReference(f.File) {
		loc.ArtifactLocation.URI = f.File
		return loc
	}
	file, line := splitLine(f.File)
	if line > 0 {
		loc.Region = &sarifRegion{StartLine: line}
	}
	loc.ArtifactLocation.URI = (&url.URL{Path: file}).String()
	return loc
}
