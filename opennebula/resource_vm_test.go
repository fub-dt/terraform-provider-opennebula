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

var vmConfigBasicTemplate = `
resource "opennebula_vm" "test" {
  name = "test-vm"
  permissions = "642"
  user_template_attributes = "attr1=avalue\nattr2=anothervalue"
  %s
}
`

var vmConfigUpdateTemplate = `
resource "opennebula_vm" "test" {
  name = "test-vm"
  permissions = "666"
  user_template_attributes = "attr1=changed\nattr2=anothervalue"
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
					resource.TestCheckResourceAttr("opennebula_vm.test", "user_template_attributes", "attr1=avalue\nattr2=anothervalue"),
					resource.TestCheckResourceAttrSet("opennebula_vm.test", "ip"),
					resource.TestCheckResourceAttrSet("opennebula_vm.test", "uid"),
					resource.TestCheckResourceAttrSet("opennebula_vm.test", "gid"),
					resource.TestCheckResourceAttrSet("opennebula_vm.test", "uname"),
					resource.TestCheckResourceAttrSet("opennebula_vm.test", "gname"),
					resource.TestCheckResourceAttrSet("opennebula_vm.test", "state"),
					resource.TestCheckResourceAttrSet("opennebula_vm.test", "lcmstate"),
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
					resource.TestCheckResourceAttr("opennebula_vm.test", "user_template_attributes", "attr1=changed\nattr2=anothervalue"),
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

			var vm struct {
				Permissions *Permissions `xml:"PERMISSIONS"`
			}
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
