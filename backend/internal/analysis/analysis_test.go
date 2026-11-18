package analysis_test

import (
	"fmt"
	"strings"
	"testing"

	"scheck/internal/analysis"
	"scheck/internal/rules"
	"scheck/internal/s1000d"
)

func dm(t *testing.T, id, sc, info, tech, infoName, content string) *s1000d.Doc {
	t.Helper()
	xml := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<dmodule><identAndStatusSection><dmAddress><dmIdent>
<dmCode modelIdentCode="T" systemDiffCode="A" systemCode="%s" subSystemCode="1" subSubSystemCode="0" assyCode="00" disassyCode="00" disassyCodeVariant="A" infoCode="%s" infoCodeVariant="A" itemLocationCode="A"/>
<language languageIsoCode="de" countryIsoCode="DE"/><issueInfo issueNumber="001" inWork="00"/></dmIdent>
<dmAddressItems><dmTitle><techName>%s</techName><infoName>%s</infoName></dmTitle></dmAddressItems></dmAddress>
<dmStatus><security securityClassification="01"/></dmStatus></identAndStatusSection>
<content>%s</content></dmodule>`, sc, info, tech, infoName, content)
	d, err := s1000d.Parse(id, id+".xml", []byte(xml))
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func codes(fs []analysis.Finding) map[string]int {
	m := map[string]int{}
	for _, f := range fs {
		m[f.Code]++
	}
	return m
}

func TestListDuplicates(t *testing.T) {
	d := dm(t, "a", "29", "040", "Hydraulikpumpe", "Beschreibung", `<description><levelledPara><title>Aufbau</title>
<para><randomList><listItem><para>Pumpengehäuse</para></listItem><listItem><para>Antriebswelle</para></listItem>
<listItem><para>Antriebswelle.</para></listItem></randomList></para></levelledPara></description>`)
	got := codes(analysis.CheckLists(&analysis.Corpus{Docs: []*s1000d.Doc{d}}))
	if got["LIST_DUPLICATE"] != 1 {
		t.Fatalf("expected one duplicate, got %v", got)
	}
}

func TestTitleTypoAndSiblings(t *testing.T) {
	var docs []*s1000d.Doc
	for i, sc := range []string{"21", "22", "23", "24"} {
		docs = append(docs, dm(t, fmt.Sprint(i), sc, "040", "Hydraulikpumpe", "Beschreibung",
			`<description><levelledPara><title>Allgemeines</title><para>Die Hydraulikpumpe erzeugt Druck.</para></levelledPara></description>`))
	}
	docs = append(docs, dm(t, "x", "25", "040", "Hydarulikpumpe", "Beschreibung",
		`<description><levelledPara><title>Allgemeines</title><para>Text.</para></levelledPara><levelledPara><title>Allgemeines</title><para>Mehr.</para></levelledPara></description>`))
	c := &analysis.Corpus{Docs: docs, Dictionary: map[string]bool{}}
	got := codes(append(analysis.CheckTitles(c), analysis.CheckSpelling(c)...))
	if got["TITLE_DUPLICATE_SIBLING"] != 1 {
		t.Errorf("sibling duplicate not found: %v", got)
	}
	if got["TITLE_TYPO"] == 0 && got["TITLE_VARIANT"] == 0 {
		t.Errorf("typo in techName not found: %v", got)
	}
}

func TestReferences(t *testing.T) {
	d := dm(t, "a", "29", "040", "Pumpe", "Beschreibung", `<description><levelledPara><title>X</title>
<figure id="fig-1"><title>Bild</title><graphic infoEntityIdent="ICN-MISSING"/></figure>
<para>Siehe <internalRef internalRefId="fig-1"/> und <internalRef internalRefId="fig-2"/>.</para></levelledPara></description>`)
	got := codes(analysis.CheckReferences(&analysis.Corpus{Docs: []*s1000d.Doc{d}, ICNs: map[string]string{"ICN-UNUSED": "ICN-UNUSED.png"}}))
	for _, want := range []string{"REF_INTERNAL_MISSING", "REF_ICN_MISSING", "ICN_UNUSED"} {
		if got[want] != 1 {
			t.Errorf("%s: got %v", want, got)
		}
	}
}

func TestBusinessRules(t *testing.T) {
	d := dm(t, "a", "29", "520", "Pumpe", "Ausbau", `<procedure><mainProcedure>
<proceduralStep><para>Leitungen usw. abbauen.</para></proceduralStep></mainProcedure></procedure>`)
	rs := []rules.Rule{
		{Name: "Begriffe", Type: rules.TypeForbiddenTerms, Severity: "warning", Enabled: true,
			Config: []byte(`{"terms":[{"term":"usw.","replacement":"vollständig aufzählen"}]}`)},
		{Name: "Klassifikation", Type: rules.TypeXPathValues, Severity: "error", Enabled: true,
			Config: []byte(`{"xpath":"//@securityClassification","allowed":["02"]}`)},
	}
	fs := rules.Evaluate(&analysis.Corpus{Docs: []*s1000d.Doc{d}}, rs)
	if len(fs) != 2 {
		t.Fatalf("expected 2 rule findings, got %d: %+v", len(fs), fs)
	}
	if !strings.Contains(fs[0].Message+fs[1].Message, "usw.") {
		t.Errorf("forbidden term not reported: %+v", fs)
	}
}
