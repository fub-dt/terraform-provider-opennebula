package opennebula

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type MockClient struct {
	mock.Mock
}

func (m *MockClient) Call(command string, params ...interface{}) (string, error) {
	args := m.Called(command, params)
	return args.String(0), args.Error(1)
}

func (m *MockClient) IsSuccess(result []interface{}) (res string, err error) {
	args := m.Called(result)
	return args.String(0), args.Error(1)
}

func TestLoadVMInfo(t *testing.T) {
	mockClient := new(MockClient)
	mockClient.On("Call", "one.vm.info", []interface{}{1}).Return("<VM><SOME_ELEMENT>some value</SOME_ELEMENT></VM>", nil)
	attributes, err := loadVMInfo(mockClient, 1)
	assert.NoError(t, err)
	assert.NotEmpty(t, attributes)
}

func TestLoadVMInfoWithError(t *testing.T) {
	mockClient := new(MockClient)
	mockClient.On("Call", "one.vm.info", []interface{}{1}).Return("not relevant", fmt.Errorf("error"))
	attributes, err := loadVMInfo(mockClient, 1)
	assert.Error(t, err)
	assert.Empty(t, attributes)
}

func TestParsingValidVmInfo(t *testing.T) {
	xmlResponse := `<VM>
						<PARENT_ELEMENT> parent value
							<CHILD_ELEMENT>child value</CHILD_ELEMENT>
						</PARENT_ELEMENT>
					</VM>`
	attributes, err := parseVMInfo([]byte(xmlResponse))

	assert.NoError(t, err)
	assert.Len(t, attributes, 2)
	assert.Equal(t, "parent value", attributes["PARENT_ELEMENT"])
	assert.Equal(t, "child value", attributes["PARENT_ELEMENT/CHILD_ELEMENT"])
}

func TestParsingValidNestedVmInfo(t *testing.T) {
	xmlResponse := `<ROOT>
						<EXTRA_LEVEL>
							<VM>
								<ELEMENT>value</ELEMENT>
							</VM>
						</EXTRA_LEVEL>
					</ROOT>`
	attributes, err := parseVMInfo([]byte(xmlResponse))

	assert.NoError(t, err)
	assert.Len(t, attributes, 1)
	assert.Equal(t, "value", attributes["ELEMENT"])
}

func TestParsingVmInfoWithoutVmElement(t *testing.T) {
	xmlResponse := "<SOME_ELEMENT>some value</SOME_ELEMENT>"
	attributes, err := parseVMInfo([]byte(xmlResponse))

	assert.Error(t, err)
	assert.Empty(t, attributes)
}

func TestParsingEmptyVmInfo(t *testing.T) {
	attributes, err := parseVMInfo([]byte(""))

	assert.Error(t, err)
	assert.Empty(t, attributes)
}

func TestParsingNilVmInfo(t *testing.T) {
	attributes, err := parseVMInfo(nil)

	assert.Error(t, err)
	assert.Empty(t, attributes)
}

func TestParsingInvalidVmInfo(t *testing.T) {
	xmlResponse := "<VM><SOME_ELEMENT>some value</VM>"
	attributes, err := parseVMInfo([]byte(xmlResponse))

	assert.Error(t, err)
	assert.Empty(t, attributes)
}

func TestParsingIncompleteVmInfo(t *testing.T) {
	xmlResponse := "<VM><SOME_ELEMENT>some value</SOME_EL"
	attributes, err := parseVMInfo([]byte(xmlResponse))

	assert.Error(t, err)
	assert.Empty(t, attributes)
}

func TestParsingVmInfoIgnoreVmCharInfo(t *testing.T) {
	xmlResponse := `<VM> 
						first
						<ELEMENT>value</ELEMENT>
						second
					</VM>`
	attributes, err := parseVMInfo([]byte(xmlResponse))

	assert.NoError(t, err)
	assert.Len(t, attributes, 1)
	assert.Equal(t, "value", attributes["ELEMENT"])
}

func TestParsingVmInfoConcatCharInfo(t *testing.T) {
	xmlResponse := `<VM> 
						<PARENT_ELEMENT> 
							parent
							<CHILD_ELEMENT>child value</CHILD_ELEMENT>
							value
						</PARENT_ELEMENT>
					</VM>`
	attributes, err := parseVMInfo([]byte(xmlResponse))

	assert.NoError(t, err)
	assert.Len(t, attributes, 2)
	assert.Equal(t, "child value", attributes["PARENT_ELEMENT/CHILD_ELEMENT"])
	assert.Equal(t, "parent value", attributes["PARENT_ELEMENT"])
}
