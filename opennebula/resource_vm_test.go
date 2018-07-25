package opennebula

import (
	"encoding/xml"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/hashicorp/terraform/helper/resource"
	"github.com/hashicorp/terraform/terraform"
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

var vmConfigBasicTemplate = `
resource "opennebula_vm" "test" {
  name = "test-vm"
  permissions = "642"
  %s
}
`

var vmConfigUpdateTemplate = `
resource "opennebula_vm" "test" {
  name = "test-vm"
  permissions = "666"
  %s
}
`

func TestAccVm(t *testing.T) {
	baseConfig := createConfig(vmConfigBasicTemplate)
	updateConfig := createConfig(vmConfigUpdateTemplate)
	resource.Test(t, resource.TestCase{
		PreCheck:     func() { testAccPreCheck(t) },
		Providers:    testAccProviders,
		CheckDestroy: testAccCheckVmDestroy,
		Steps: []resource.TestStep{
			{
				Config: baseConfig,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("opennebula_vm.test", "name", "test-vm"),
					resource.TestCheckResourceAttr("opennebula_vm.test", "template_id", getTemplateId()),
					resource.TestCheckResourceAttr("opennebula_vm.test", "wait_for_attribute", getWaitForAttribute()),
					resource.TestCheckResourceAttr("opennebula_vm.test", "ip_attribute", getIpAttribute()),
					resource.TestCheckResourceAttr("opennebula_vm.test", "permissions", "642"),
					resource.TestCheckResourceAttrSet("opennebula_vm.test", "ip"),
					resource.TestCheckResourceAttrSet("opennebula_vm.test", "uid"),
					resource.TestCheckResourceAttrSet("opennebula_vm.test", "gid"),
					resource.TestCheckResourceAttrSet("opennebula_vm.test", "uname"),
					resource.TestCheckResourceAttrSet("opennebula_vm.test", "gname"),
					testAccCheckVmPermissions(&Permissions{
						Owner_U: 1,
						Owner_M: 1,
						Group_U: 1,
						Other_M: 1,
					}),
				),
			},
			{
				Config: updateConfig,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("opennebula_vm.test", "permissions", "666"),
					testAccCheckVmPermissions(&Permissions{
						Owner_U: 1,
						Owner_M: 1,
						Group_U: 1,
						Group_M: 1,
						Other_U: 1,
						Other_M: 1,
					}),
				),
			},
		},
	})
}

func createConfig(template string) string {
	additionalConfigTemplate := "template_id = %s\n  wait_for_attribute=\"%s\"\n  ip_attribute=\"%s\""
	additionalConfig := fmt.Sprintf(additionalConfigTemplate, getTemplateId(), getWaitForAttribute(), getIpAttribute())

	return fmt.Sprintf(template, additionalConfig)
}

func getTemplateId() string {
	return readEnvironmentVariable("OPENNEBULA_VM_TEMPLATE_ID", "")
}

func getWaitForAttribute() string {
	return readEnvironmentVariable("OPENNEBULA_VM_WAIT_FOR_ATTRIBUTE", DefaultIpAttribute)
}

func getIpAttribute() string {
	return readEnvironmentVariable("OPENNEBULA_VM_IP_ATTRIBUTE", DefaultIpAttribute)
}

func readEnvironmentVariable(key string, defaultValue string) string {
	value := os.Getenv(key)
	if value != "" {
		return value
	} else {
		return defaultValue
	}
}

func testAccCheckVmDestroy(s *terraform.State) error {
	client := testAccProvider.Meta().(*Client)

	for _, rs := range s.RootModule().Resources {
		resp, err := client.Call("one.vm.info", intId(rs.Primary.ID), false)
		if err == nil && !strings.Contains(resp, "<STATE>6</STATE>") {
			return fmt.Errorf("Expected vm %s to have been destroyed", rs.Primary.ID)
		}
	}

	return nil
}

func testAccCheckVmPermissions(expected *Permissions) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		client := testAccProvider.Meta().(*Client)

		for _, rs := range s.RootModule().Resources {
			resp, err := client.Call("one.vm.info", intId(rs.Primary.ID), false)
			if err != nil {
				return fmt.Errorf("Expected vm %s to exist when checking permissions", rs.Primary.ID)
			}

			var vm UserVm
			if err = xml.Unmarshal([]byte(resp), &vm); err != nil {
				return err
			}

			if !reflect.DeepEqual(vm.Permissions, expected) {
				return fmt.Errorf(
					"Permissions for vnet %s were expected to be %s. Instead, they were %s",
					rs.Primary.ID,
					permissionString(expected),
					permissionString(vm.Permissions),
				)
			}
		}

		return nil
	}
}
