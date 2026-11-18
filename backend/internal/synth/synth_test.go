package synth_test

import (
	"bytes"
	"encoding/json"
	"regexp"
	"strconv"
	"testing"

	"scheck/internal/analysis"
	"scheck/internal/s1000d"
	"scheck/internal/synth"
)

const proc = `<?xml version="1.0" encoding="UTF-8"?>
<dmodule><identAndStatusSection><dmAddress><dmIdent>
<dmCode modelIdentCode="T" systemDiffCode="A" systemCode="29" subSystemCode="1" subSubSystemCode="0" assyCode="00" disassyCode="00" disassyCodeVariant="A" infoCode="520" infoCodeVariant="A" itemLocationCode="A"/>
<language languageIsoCode="en" countryIsoCode="US"/><issueInfo issueNumber="001" inWork="00"/></dmIdent>
<dmAddressItems><dmTitle><techName>Pump</techName><infoName>Remove procedure</infoName></dmTitle></dmAddressItems></dmAddress></identAndStatusSection>
<content><procedure><warningsAndCautions><warning id="w1"><warningAndCautionPara>Make sure that the system is depressurized before you remove the pump.</warningAndCautionPara></warning></warningsAndCautions>
<mainProcedure>
<proceduralStep warningRefs="w1"><para>Release the hydraulic system pressure.</para></proceduralStep>
<proceduralStep><para>Disconnect the pressure line from the pump.</para></proceduralStep>
<proceduralStep><para>Remove the four nuts. The nuts are torqued to 25 Nm.</para></proceduralStep>
<proceduralStep><para>Do not reuse the nuts. Nut torque was 25 Nm.</para></proceduralStep>
</mainProcedure></procedure></content></dmodule>`

func TestSemanticSamples(t *testing.T) {
	d, err := s1000d.Parse("f1", "dm.xml", []byte(proc))
	if err != nil {
		t.Fatal(err)
	}
	c := &analysis.Corpus{Docs: []*s1000d.Doc{d}, ICNs: map[string]string{}, Dictionary: map[string]bool{}}
	var buf bytes.Buffer
	stats, err := synth.Generate(c, nil, synth.Options{Count: 120, Seed: 1, CleanRatio: 0.1}, &buf)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range []string{"step_order", "missing_warning", "value_conflict", "unit_error", "meaning_inverted", "procedure_mismatch"} {
		if stats.Mutations[m] == 0 {
			t.Errorf("no samples for %s: %v", m, stats.Mutations)
		}
	}
	lineRe := regexp.MustCompile(`\[L(\d+)\]`)
	dec := json.NewDecoder(&buf)
	for dec.More() {
		var rec struct {
			Messages []struct{ Content string } `json:"messages"`
		}
		if err := dec.Decode(&rec); err != nil {
			t.Fatal(err)
		}
		visible := map[int]bool{}
		for _, m := range lineRe.FindAllStringSubmatch(rec.Messages[1].Content, -1) {
			n, _ := strconv.Atoi(m[1])
			visible[n] = true
		}
		var ans struct{ Findings []analysis.LLMFinding }
		if err := json.Unmarshal([]byte(rec.Messages[2].Content), &ans); err != nil {
			t.Fatal(err)
		}
		for _, f := range ans.Findings {
			if !visible[f.Line] {
				t.Errorf("label line %d not visible in input (%s)", f.Line, f.Code)
			}
		}
	}
	// the source document must be unchanged after generation
	d2, _ := s1000d.Parse("f1", "dm.xml", []byte(proc))
	if d.Root.OuterXML() != d2.Root.OuterXML() {
		t.Error("generator modified the source document")
	}
}
