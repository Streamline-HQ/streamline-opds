package main

import (
	"encoding/xml"
	"time"

	"github.com/datahearth/streamline/extensions/sdk"
)

const (
	navigationType  = "application/atom+xml;profile=opds-catalog;kind=navigation"
	acquisitionType = "application/atom+xml;profile=opds-catalog;kind=acquisition"
	openSearchType  = "application/opensearchdescription+xml"
	relAcquisition  = "http://opds-spec.org/acquisition"
	relImage        = "http://opds-spec.org/image"
)

type feedKind int

const (
	kindNavigation feedKind = iota
	kindAcquisition
)

func (k feedKind) mediaType() string {
	if k == kindAcquisition {
		return acquisitionType
	}
	return navigationType
}

type feed struct {
	XMLName xml.Name `xml:"http://www.w3.org/2005/Atom feed"`
	ID      string   `xml:"id"`
	Title   string   `xml:"title"`
	Updated atomTime `xml:"updated"`
	Links   []link   `xml:"link"`
	Entries []entry  `xml:"entry"`
}

type entry struct {
	ID      string       `xml:"id"`
	Title   string       `xml:"title"`
	Updated atomTime     `xml:"updated"`
	Authors []atomAuthor `xml:"author"`
	Content *content     `xml:"content,omitempty"`
	Links   []link       `xml:"link"`
}

type atomAuthor struct {
	Name string `xml:"name"`
}

type content struct {
	Type string `xml:"type,attr"`
	Text string `xml:",chardata"`
}

type link struct {
	Rel  string `xml:"rel,attr"`
	Href string `xml:"href,attr"`
	Type string `xml:"type,attr,omitempty"`
}

type atomTime time.Time

// A zero time (an empty feed has no newest entry) renders as now: Atom
// requires updated, and the epoch reads as a feed untouched since 1970.
func (t atomTime) MarshalXML(e *xml.Encoder, start xml.StartElement) error {
	v := time.Time(t)
	if v.IsZero() {
		v = time.Now()
	}
	return e.EncodeElement(v.UTC().Format(time.RFC3339), start)
}

func newest(cur atomTime, t time.Time) atomTime {
	if t.After(time.Time(cur)) {
		return atomTime(t)
	}
	return cur
}

func selfLink(href string, kind feedKind) link {
	return link{Rel: "self", Href: href, Type: kind.mediaType()}
}

func render(kind feedKind, f *feed) (sdk.Response, error) {
	body, err := xml.Marshal(f)
	if err != nil {
		return sdk.Response{}, err
	}
	return sdk.Response{
		Status:  200,
		Headers: map[string][]string{"Content-Type": {kind.mediaType()}},
		Body:    append([]byte(xml.Header), body...),
	}, nil
}
