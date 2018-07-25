package opennebula

import (
	"encoding/xml"
	"fmt"
	"log"
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
)

type UserVm struct {
	Id          string       `xml:"ID"`
	Name        string       `xml:"NAME"`
	Uid         int          `xml:"UID"`
	Gid         int          `xml:"GID"`
	Uname       string       `xml:"UNAME"`
	Gname       string       `xml:"GNAME"`
	Permissions *Permissions `xml:"PERMISSIONS"`
	State       int          `xml:"STATE"`
	LcmState    int          `xml:"LCM_STATE"`
	VmTemplate  *VmTemplate  `xml:"TEMPLATE"`
}

type UserVms struct {
	UserVm []*UserVm `xml:"VM"`
}

type VmTemplate struct {
	Context *Context `xml:"CONTEXT"`
}

type Context struct {
	IP string `xml:"ETH0_IP"`
}

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
		"",
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
	var vm *UserVm
	var attributes map[string]string

	name := d.Get("name").(string)
	if name == "" {
		name = d.Get("instance").(string)
	}

	// Try to find the vm by ID, if specified
	if d.Id() != "" {
		client := meta.(*Client)
		resp, err := client.Call("one.vm.info", intId(d.Id()))
		if err == nil {
			if err = xml.Unmarshal([]byte(resp), &vm); err != nil {
				return err
			}
			if attributes, err = parseResponse([]byte(resp), VmElementName); err != nil {
				return err
			}
		} else {
			log.Printf("Could not find VM by ID %s", d.Id())
			return err
		}
	} else {
		return fmt.Errorf("VM ID not set for VM: %s", name)
	}

	d.Set("instance", vm.Name)
	d.Set("uid", vm.Uid)
	d.Set("gid", vm.Gid)
	d.Set("uname", vm.Uname)
	d.Set("gname", vm.Gname)
	d.Set("state", vm.State)
	d.Set("lcmstate", vm.LcmState)
	ipAttribute := d.Get("ip_attribute").(string)
	if ipAttribute == "" {
		ipAttribute = DefaultIpAttribute
	}
	ip := attributes[ipAttribute]
	d.Set("ip", ip)
	d.Set("permissions", permissionString(vm.Permissions))

	return nil
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
	} else {
		log.Printf("[INFO] Sorry, only 'permissions' updates are supported at the moment.")
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
	var vm *UserVm
	client := meta.(*Client)

	log.Printf("Waiting for VM (%s) to be in state Done", d.Id())

	stateConf := &resource.StateChangeConf{
		Pending: []string{"anythingelse"},
		Target:  []string{state},
		Refresh: func() (interface{}, string, error) {
			log.Println("Refreshing VM state...")
			if d.Id() != "" {
				resp, err := client.Call("one.vm.info", intId(d.Id()))
				if err == nil {
					if err = xml.Unmarshal([]byte(resp), &vm); err != nil {
						return nil, "", fmt.Errorf("Couldn't fetch VM state: %s", err)
					}
				} else {
					return nil, "", fmt.Errorf("Could not find VM by ID %s", d.Id())
				}
			}
			log.Printf("VM is currently in state %v and in LCM state %v", vm.State, vm.LcmState)
			if vm.State == 3 && vm.LcmState == 3 {
				return vm, "running", nil
			} else if vm.State == 6 {
				return vm, "done", nil
			} else {
				return nil, "anythingelse", nil
			}
		},
		Timeout:    10 * time.Minute,
		Delay:      10 * time.Second,
		MinTimeout: 3 * time.Second,
	}

	return stateConf.WaitForState()
}

func waitForAttribute(d *schema.ResourceData, meta interface{}, attributeName string) error {
	var vm UserVm
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
						return &vm, attributeName, nil
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
		return nil, err
	}
}
