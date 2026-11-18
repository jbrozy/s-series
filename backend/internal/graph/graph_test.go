package graph_test

import (
	"fmt"
	"testing"

	"scheck/internal/analysis"
	"scheck/internal/graph"
	"scheck/internal/s1000d"
)

func dm(t *testing.T, id, ic, info, body string) *s1000d.Doc {
	x := fmt.Sprintf(`<dmodule><identAndStatusSection><dmAddress><dmIdent>
<dmCode modelIdentCode="T" systemDiffCode="A" systemCode="29" subSystemCode="1" subSubSystemCode="0" assyCode="00" disassyCode="00" disassyCodeVariant="A" infoCode="%s" infoCodeVariant="A" itemLocationCode="A"/>
<language languageIsoCode="en" countryIsoCode="US"/><issueInfo issueNumber="001" inWork="00"/></dmIdent>
<dmAddressItems><dmTitle><techName>Pump</techName><infoName>%s</infoName></dmTitle></dmAddressItems></dmAddress></identAndStatusSection>
<content>%s</content></dmodule>`, ic, info, body)
	d, err := s1000d.Parse(id, id+".xml", []byte(x))
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestBuild(t *testing.T) {
	ref := `<dmRef><dmRefIdent><dmCode modelIdentCode="T" systemDiffCode="A" systemCode="29" subSystemCode="1" subSubSystemCode="0" assyCode="00" disassyCode="00" disassyCodeVariant="A" infoCode="%s" infoCodeVariant="A" itemLocationCode="A"/></dmRefIdent></dmRef>`
	a := dm(t, "a", "040", "Description", `<description><levelledPara><title>X</title>
<para>See `+fmt.Sprintf(ref, "520")+` and `+fmt.Sprintf(ref, "999")+`. <acronym><acronymTerm>EDP</acronymTerm><acronymDefinition>Engine driven pump</acronymDefinition></acronym></para>
<figure><title>Pump</title><graphic infoEntityIdent="ICN-A"/></figure></levelledPara></description>`)
	b := dm(t, "b", "520", "Remove procedure", `<procedure><preliminaryRqmts><reqSupplies><supplyDescrGroup>
<supplyDescr><name>Fluid</name><identNumber><manufacturerCode>K1</manufacturerCode><partAndSerialNumber><partNumber>P-1</partNumber></partAndSerialNumber></identNumber></supplyDescr>
</supplyDescrGroup></reqSupplies></preliminaryRqmts><mainProcedure><proceduralStep><para>Remove.</para></proceduralStep></mainProcedure></procedure>`)
	pm, err := s1000d.Parse("p", "pm.xml", []byte(`<pm><identAndStatusSection><pmAddress><pmIdent><pmCode modelIdentCode="T" pmIssuer="K1" pmNumber="00001" pmVolume="00"/></pmIdent>
<pmAddressItems><pmTitle>Manual</pmTitle></pmAddressItems></pmAddress></identAndStatusSection>
<content><pmEntry>`+fmt.Sprintf(ref, "040")+`</pmEntry></content></pm>`))
	if err != nil {
		t.Fatal(err)
	}
	g := graph.Build(&analysis.Corpus{Docs: []*s1000d.Doc{a, b, pm}}, map[string]string{"ICN-A": "f-icn", "ICN-UNUSED": "f-u"})
	types := map[string]int{}
	for _, n := range g.Nodes() {
		types[n.Type]++
	}
	edges := map[string]int{}
	for _, e := range g.Edges() {
		edges[e.Type]++
	}
	want := map[string]int{"model": 1, "sns": 3, "dm": 3, "pm": 1, "icn": 2, "part": 1, "acronym": 1}
	for k, v := range want {
		if types[k] != v {
			t.Errorf("nodes %s: got %d want %d (%v)", k, types[k], v, types)
		}
	}
	wantE := map[string]int{"references": 2, "contains": 1, "uses_graphic": 1, "requires_part": 1, "defines": 1, "part_of": 5}
	for k, v := range wantE {
		if edges[k] != v {
			t.Errorf("edges %s: got %d want %d (%v)", k, edges[k], v, edges)
		}
	}
	for _, n := range g.Nodes() {
		if n.Type == "sns" && n.Props["level"] == "assembly" && n.Label != "29-10-00 Pump" {
			t.Errorf("assembly label %q", n.Label)
		}
		if n.Key == "dm:DMC-T-A-29-10-00-00A-999A-A" && n.Props["missing"] != true {
			t.Error("missing reference target not flagged")
		}
	}
}
