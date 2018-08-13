package opennebula

import (
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/hashicorp/terraform/helper/resource"
	"github.com/hashicorp/terraform/helper/schema"
)

const (
	PathSeparator      = "/"
	ValueSepartor      = " "
	VmElementName      = "VM"
	DefaultIpAttribute = "TEMPLATE/CONTEXT/ETH0_IP"
	StateAttribute     = "STATE"
	LcmStateAttribute  = "LCM_STATE"
)

func resourceVm() *schema.Resource {
	return &schema.Resource{
		Create: resourceVmCreate,
		Read:   resourceVmRead,
		Exists: resourceVmExists,
		Update: resourceVmUpdate,
		Delete: resourceVmDelete,
		Importer: &schema.ResourceImporter{
			State: schema.ImportStatePassthrough,
		},

		Schema: map[string]*schema.Schema{
			"name": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "Name of the VM. If empty, defaults to 'templatename-<vmid>'",
			},
			"instance": {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "Final name of the VM instance",
			},
			"template_id": {
				Type:        schema.TypeInt,
				Required:    true,
				Description: "Id of the VM template to use. Either 'template_name' or 'template_id' is required",
			},
			"permissions": {
				Type:        schema.TypeString,
				Required:    true,
				Description: "Permissions for the template (in Unix format, owner-group-other, use-manage-admin)",
				ValidateFunc: func(v interface{}, k string) (ws []string, errors []error) {
					value := v.(string)

					if len(value) != 3 {
						errors = append(errors, fmt.Errorf("%q has specify 3 permission sets: owner-group-other", k))
					}

					all := true
					for _, c := range strings.Split(value, "") {
						if c < "0" || c > "7" {
							all = false
						}
					}
					if !all {
						errors = append(errors, fmt.Errorf("Each character in %q should specify a Unix-like permission set with a number from 0 to 7", k))
					}

					return
				},
			},

			"uid": {
				Type:        schema.TypeInt,
				Computed:    true,
				Description: "ID of the user that will own the VM",
			},
			"gid": {
				Type:        schema.TypeInt,
				Computed:    true,
				Description: "ID of the group that will own the VM",
			},
			"uname": {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "Name of the user that will own the VM",
			},
			"gname": {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "Name of the group that will own the VM",
			},
			"ip": {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "IP address that is assigned to the VM",
			},
			"state": {
				Type:        schema.TypeInt,
				Computed:    true,
				Description: "Current state of the VM",
			},
			"lcmstate": {
				Type:        schema.TypeInt,
				Computed:    true,
				Description: "Current LCM state of the VM",
			},
			"wait_for_attribute": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "Wait for specific attribute from VM Info to become available during vm creation",
			},
			"ip_attribute": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "Use different attribute from VM Info. TEMPLATE/CONTEXT/ETH0_IP is the default value",
			},
			"user_template_attributes": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "User template attributes. A new line (\\n) separated list of name=value pairs",
			},
		},
	}
}

func resourceVmCreate(d *schema.ResourceData, meta interface{}) error {
	client := meta.(*Client)

	resp, err := client.Call(
		"one.template.instantiate",
		d.Get("template_id"),
		d.Get("name"),
		false,
		d.Get("user_template_attributes"),
		false,
	)
	if err != nil {
		return err
	}

	d.SetId(resp)

	_, err = waitForVmState(d, meta, "running")
	if err != nil {
		return fmt.Errorf(
			"Error waiting for virtual machine (%s) to be in state RUNNING: %s", d.Id(), err)
	}

	attribute := d.Get("wait_for_attribute").(string)
	if attribute != "" {
		err = waitForAttribute(d, meta, attribute)
		if err != nil {
			return fmt.Errorf("Error waiting for attribute %s of virtual machine %s: %s", attribute, d.Id(), err)
		}
	}

	if _, err = changePermissions(intId(d.Id()), permission(d.Get("permissions").(string)), client, "one.vm.chmod"); err != nil {
		return err
	}

	return resourceVmRead(d, meta)
}

func resourceVmRead(d *schema.ResourceData, meta interface{}) error {
	var attributes map[string]string
	var err error

	if d.Id() != "" {
		client := meta.(*Client)
		if attributes, err = loadVMInfo(client, intId(d.Id())); err != nil {
			return err
		}
	} else {
		name := d.Get("name").(string)
		if name == "" {
			name = d.Get("instance").(string)
		}
		return fmt.Errorf("VM ID not set for VM: %s", name)
	}

	saveVmInfoToState(d, attributes)

	return nil
}

func saveVmInfoToState(state *schema.ResourceData, attributes map[string]string) {
	state.Set("instance", attributes["NAME"])
	state.Set("uid", convertToInt(attributes["UID"]))
	state.Set("gid", convertToInt(attributes["GID"]))
	state.Set("uname", attributes["UNAME"])
	state.Set("gname", attributes["GNAME"])
	state.Set("state", convertToInt(attributes[StateAttribute]))
	state.Set("lcmstate", convertToInt(attributes[LcmStateAttribute]))
	ipAttribute := state.Get("ip_attribute").(string)
	if ipAttribute == "" {
		ipAttribute = DefaultIpAttribute
	}
	ip := attributes[ipAttribute]
	state.Set("ip", ip)
	state.Set("permissions", permissionString(buildPermissions(attributes)))
}

func buildPermissions(attributes map[string]string) *Permissions {
	permissions := Permissions{
		Owner_U: convertToInt(attributes["PERMISSIONS/OWNER_U"]),
		Owner_M: convertToInt(attributes["PERMISSIONS/OWNER_M"]),
		Owner_A: convertToInt(attributes["PERMISSIONS/OWNER_A"]),
		Group_U: convertToInt(attributes["PERMISSIONS/GROUP_U"]),
		Group_M: convertToInt(attributes["PERMISSIONS/GROUP_M"]),
		Group_A: convertToInt(attributes["PERMISSIONS/GROUP_A"]),
		Other_U: convertToInt(attributes["PERMISSIONS/OTHER_U"]),
		Other_M: convertToInt(attributes["PERMISSIONS/OTHER_M"]),
		Other_A: convertToInt(attributes["PERMISSIONS/OTHER_A"]),
	}

	return &permissions
}

func convertToInt(value string) int {
	i, err := strconv.Atoi(value)
	if err != nil {
		log.Fatalf("Unexpected value '%s' received from OpenNebula. Expected an integer", value)
	}

	return i
}

func resourceVmExists(d *schema.ResourceData, meta interface{}) (bool, error) {
	err := resourceVmRead(d, meta)
	// a terminated VM is in state 6 (DONE)
	if err != nil || d.Id() == "" || d.Get("state").(int) == 6 {
		return false, err
	}

	return true, nil
}

func resourceVmUpdate(d *schema.ResourceData, meta interface{}) error {
	client := meta.(*Client)

	if d.HasChange("permissions") {
		resp, err := changePermissions(intId(d.Id()), permission(d.Get("permissions").(string)), client, "one.vm.chmod")
		if err != nil {
			return err
		}
		log.Printf("[INFO] Successfully updated VM %s\n", resp)
	}

	if d.HasChange("user_template_attributes") {
		if err := updateUserTemplate(client, intId(d.Id()), d.Get("user_template_attributes").(string)); err != nil {
			return err
		}
	}

	return nil
}

func resourceVmDelete(d *schema.ResourceData, meta interface{}) error {
	err := resourceVmRead(d, meta)
	if err != nil || d.Id() == "" {
		return err
	}

	client := meta.(*Client)
	resp, err := client.Call("one.vm.action", "terminate-hard", intId(d.Id()))
	if err != nil {
		return err
	}

	_, err = waitForVmState(d, meta, "done")
	if err != nil {
		return fmt.Errorf(
			"Error waiting for virtual machine (%s) to be in state DONE: %s", d.Id(), err)
	}

	log.Printf("[INFO] Successfully terminated VM %s\n", resp)
	return nil
}

func waitForVmState(d *schema.ResourceData, meta interface{}, state string) (interface{}, error) {
	client := meta.(*Client)

	log.Printf("Waiting for VM (%s) to be in state Done", d.Id())

	stateConf := &resource.StateChangeConf{
		Pending: []string{"anythingelse"},
		Target:  []string{state},
		Refresh: func() (interface{}, string, error) {
			log.Println("Refreshing VM state...")
			if d.Id() != "" {
				attributes, err := loadVMInfo(client, intId(d.Id()))
				if err == nil {
					state := attributes[StateAttribute]
					lcmState := attributes[LcmStateAttribute]
					log.Printf("VM is currently in state %s and in LCM state %s", state, lcmState)
					if state == "3" && lcmState == "3" {
						return &attributes, "running", nil
					} else if state == "6" {
						return &attributes, "done", nil
					}
				} else {
					return nil, "", fmt.Errorf("Could not find VM by ID %s", d.Id())
				}
			}
			return nil, "anythingelse", nil
		},
		Timeout:    10 * time.Minute,
		Delay:      10 * time.Second,
		MinTimeout: 3 * time.Second,
	}

	return stateConf.WaitForState()
}

func waitForAttribute(d *schema.ResourceData, meta interface{}, attributeName string) error {
	client := meta.(*Client)

	log.Printf("Waiting for VM (%s) to have attribute %s", d.Id(), attributeName)

	stateConf := &resource.StateChangeConf{
		Pending: []string{"attributeNotFound"},
		Target:  []string{attributeName},
		Refresh: func() (interface{}, string, error) {
			log.Println("Refreshing VM info...")
			if d.Id() != "" {
				attributes, err := loadVMInfo(client, intId(d.Id()))
				if err == nil {
					if _, present := attributes[attributeName]; present {
						return &attributes, attributeName, nil
					}
				} else {
					return nil, "", fmt.Errorf("Could not find VM by ID %s", d.Id())
				}
			}
			return nil, "attributeNotFound", nil
		},
		Timeout:    10 * time.Minute,
		Delay:      10 * time.Second,
		MinTimeout: 3 * time.Second,
	}

	_, err := stateConf.WaitForState()
	return err
}

func loadVMInfo(client OneClient, id int) (map[string]string, error) {
	resp, err := client.Call("one.vm.info", id)
	if err == nil {
		return parseResponse([]byte(resp), VmElementName)
	} else {
		log.Printf("Could not load VM Info with ID %d due to error: %s", id, err)
		return nil, err
	}
}

func updateUserTemplate(client OneClient, id int, attribute string) error {
	resp, err := client.Call("one.vm.update", id, attribute, 1)
	if err == nil {
		log.Printf("[INFO] Successfully updated user template for VM %s\n", resp)
		return nil
	} else {
		return err
	}
}
