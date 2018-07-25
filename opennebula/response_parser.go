package opennebula

import (
	"bytes"
	"encoding/xml"
	"strings"
)

func parseResponse(data []byte, startElement string) (map[string]string, error) {
	decoder := xml.NewDecoder(bytes.NewReader(data))
	for {
		t, err := decoder.Token()
		if t == nil || err != nil {
			return nil, err
		}

		switch tt := t.(type) {
		case xml.StartElement:
			if tt.Name.Local == startElement {
				return parseSubTree(decoder, tt.Name.Local)
			}
		}
	}
}

func parseSubTree(decoder xml.TokenReader, endElement string) (map[string]string, error) {
	attributes := make(map[string]string)
	var path []string
	for {
		t, err := decoder.Token()
		if t == nil || err != nil {
			return nil, err
		}

		switch tt := t.(type) {
		case xml.StartElement:
			path = append(path, tt.Name.Local)
		case xml.CharData:
			value := strings.TrimSpace(string(tt))
			if len(value) > 0 && len(path) > 0 {
				key := strings.Join(path, PathSeparator)
				if presentValue, isPresent := attributes[key]; isPresent {
					value = presentValue + ValueSepartor + value
				}
				attributes[key] = value
			}
		case xml.EndElement:
			if tt.Name.Local == endElement {
				return attributes, nil
			}
			if path[len(path)-1] == tt.Name.Local {
				path = path[:len(path)-1]
			}
		}
	}
}
