package opennebula

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParsingValidResponse(t *testing.T) {
	xmlResponse := `<VM>
						<PARENT_ELEMENT> parent value
							<CHILD_ELEMENT>child value</CHILD_ELEMENT>
						</PARENT_ELEMENT>
					</VM>`
	attributes, err := parseResponse([]byte(xmlResponse), "VM")

	assert.NoError(t, err)
	assert.Len(t, attributes, 2)
	assert.Equal(t, "parent value", attributes["PARENT_ELEMENT"])
	assert.Equal(t, "child value", attributes["PARENT_ELEMENT/CHILD_ELEMENT"])
}

func TestParsingValidNestedVmElement(t *testing.T) {
	xmlResponse := `<ROOT>
						<EXTRA_LEVEL>
							<VM>
								<ELEMENT>value</ELEMENT>
							</VM>
						</EXTRA_LEVEL>
					</ROOT>`
	attributes, err := parseResponse([]byte(xmlResponse), "VM")

	assert.NoError(t, err)
	assert.Len(t, attributes, 1)
	assert.Equal(t, "value", attributes["ELEMENT"])
}

func TestParsingResponseWithoutVmElement(t *testing.T) {
	xmlResponse := "<SOME_ELEMENT>some value</SOME_ELEMENT>"
	attributes, err := parseResponse([]byte(xmlResponse), "VM")

	assert.Error(t, err)
	assert.Empty(t, attributes)
}

func TestParsingEmptyResponse(t *testing.T) {
	attributes, err := parseResponse([]byte(""), "VM")

	assert.Error(t, err)
	assert.Empty(t, attributes)
}

func TestParsingNilResponse(t *testing.T) {
	attributes, err := parseResponse(nil, "VM")

	assert.Error(t, err)
	assert.Empty(t, attributes)
}

func TestParsingInvalidResponse(t *testing.T) {
	xmlResponse := "<VM><SOME_ELEMENT>some value</VM>"
	attributes, err := parseResponse([]byte(xmlResponse), "VM")

	assert.Error(t, err)
	assert.Empty(t, attributes)
}

func TestParsingIncompleteResponse(t *testing.T) {
	xmlResponse := "<VM><SOME_ELEMENT>some value</SOME_EL"
	attributes, err := parseResponse([]byte(xmlResponse), "VM")

	assert.Error(t, err)
	assert.Empty(t, attributes)
}

func TestParsingResponseIgnoreVmCharInfo(t *testing.T) {
	xmlResponse := `<VM> 
						first
						<ELEMENT>value</ELEMENT>
						second
					</VM>`
	attributes, err := parseResponse([]byte(xmlResponse), "VM")

	assert.NoError(t, err)
	assert.Len(t, attributes, 1)
	assert.Equal(t, "value", attributes["ELEMENT"])
}

func TestParsingResponseConcatCharInfo(t *testing.T) {
	xmlResponse := `<VM> 
						<PARENT_ELEMENT> 
							parent
							<CHILD_ELEMENT>child value</CHILD_ELEMENT>
							value
						</PARENT_ELEMENT>
					</VM>`
	attributes, err := parseResponse([]byte(xmlResponse), "VM")

	assert.NoError(t, err)
	assert.Len(t, attributes, 2)
	assert.Equal(t, "child value", attributes["PARENT_ELEMENT/CHILD_ELEMENT"])
	assert.Equal(t, "parent value", attributes["PARENT_ELEMENT"])
}

func TestParsingResponseNoStartElement(t *testing.T) {
	xmlResponse := `<PARENT_ELEMENT>
						<CHILD_ELEMENT>child value</CHILD_ELEMENT>
					</PARENT_ELEMENT>`
	attributes, err := parseResponse([]byte(xmlResponse), "")
	assert.Error(t, err)
	assert.Empty(t, attributes)
}

func TestParsingResponseNotOccuringStartElement(t *testing.T) {
	xmlResponse := `<PARENT_ELEMENT>
						<CHILD_ELEMENT>child value</CHILD_ELEMENT>
					</PARENT_ELEMENT>`
	attributes, err := parseResponse([]byte(xmlResponse), "VM")
	assert.Error(t, err)
	assert.Empty(t, attributes)
}
